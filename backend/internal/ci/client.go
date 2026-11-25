package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
)

const (
	_maxAttempts   = 5
	_baseDelay     = time.Second
	_maxDelay      = 30 * time.Second
	_maxRetryAfter = 2 * time.Minute
	_pollDelay     = 3 * time.Second
	_requestLimit  = 2 * time.Minute
)

// ErrOutcomeUnknown is returned when the intake result was not received in time: the package may still be
// accepted, so the step must not report success.
var ErrOutcomeUnknown = errors.New("intake outcome is unknown")

// RefusedError is a refusal of the package: an error answer of the intake, retries that ran out, a package
// over the profile limits or a package rejected as a whole.
type RefusedError struct {
	Status        int
	Code          string
	Detail        string
	CorrelationID string
}

func (e *RefusedError) Error() string {
	var b strings.Builder
	b.WriteString("package refused")
	if e.Status != 0 {
		b.WriteString(": HTTP " + strconv.Itoa(e.Status))
	}
	if e.Code != "" {
		b.WriteString(": " + e.Code)
	}
	if e.Detail != "" {
		b.WriteString(" (" + e.Detail + ")")
	}
	if e.CorrelationID != "" {
		b.WriteString(", correlation_id " + e.CorrelationID)
	}
	return b.String()
}

// ClientConfig replaces the network, the clock and the randomness of the client; zero values use real ones.
type ClientConfig struct {
	HTTP   *http.Client
	Out    io.Writer
	Sleep  func(context.Context, time.Duration) error
	Jitter func() float64
}

// Client calls the CI operations of the craftPulse API with the upload token of the project.
type Client struct {
	api    *ciapi.Client
	site   *url.URL
	out    io.Writer
	sleep  func(context.Context, time.Duration) error
	jitter func() float64
}

func NewClient(j *Job, cfg ClientConfig) (*Client, error) {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: _requestLimit}
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleep
	}
	if cfg.Jitter == nil {
		cfg.Jitter = rand.Float64 //nolint:gosec // the pause spread needs no cryptographic randomness
	}
	token := j.Token
	api, err := ciapi.NewClient(j.Site.String()+"/api/v1", ciapi.WithHTTPClient(cfg.HTTP),
		ciapi.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
			r.Header.Set("Authorization", "Bearer "+token)
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}
	return &Client{api: api, site: j.Site, out: cfg.Out, sleep: cfg.Sleep, jitter: cfg.Jitter}, nil
}

// Profile reads the check profile of the project of the token.
func (c *Client) Profile(ctx context.Context) (*ciapi.CiCheckProfile, error) {
	resp, err := c.send(ctx, "profile", func() (*http.Response, error) { return c.api.GetCiCheckProfile(ctx) })
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	parsed, err := ciapi.ParseGetCiCheckProfileResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("read check profile: %w", err)
	}
	if parsed.StatusCode() != http.StatusOK || parsed.JSON200 == nil {
		return nil, fmt.Errorf("read check profile: %w", refusal(parsed.StatusCode(), parsed.Body))
	}
	return parsed.JSON200, nil
}

// Upload sends the package and repeats the same bytes after 503, 429 and connection errors.
func (c *Client) Upload(ctx context.Context, pkg []byte) (*ciapi.ReceiptAccepted, error) {
	resp, err := c.send(ctx, "upload", func() (*http.Response, error) {
		return c.api.UploadPackageWithBody(ctx, "application/gzip", bytes.NewReader(pkg))
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	parsed, err := ciapi.ParseUploadPackageResponse(resp)
	switch {
	case err != nil && resp.StatusCode == http.StatusAccepted:
		return nil, fmt.Errorf("%w: the answer to the upload is unreadable", ErrOutcomeUnknown)
	case err != nil:
		return nil, &RefusedError{Status: resp.StatusCode}
	case parsed.StatusCode() != http.StatusAccepted:
		return nil, refusal(parsed.StatusCode(), parsed.Body)
	case parsed.JSON202 == nil:
		return nil, fmt.Errorf("%w: the answer to the upload has no receipt", ErrOutcomeUnknown)
	case parsed.JSON202.Status == ciapi.ReceiptAcceptedStatusRejected:
		return nil, &RefusedError{Status: http.StatusAccepted, Code: "rejected", Detail: "receipt " + parsed.JSON202.ReceiptId.String()}
	}
	return parsed.JSON202, nil
}

// Await polls the receipt until the intake gives its result or wait runs out; a repeated package is read
// from the first receipt of the same content.
func (c *Client) Await(ctx context.Context, accepted *ciapi.ReceiptAccepted, wait time.Duration) (*ciapi.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	id := accepted.ReceiptId
	if accepted.Status == ciapi.ReceiptAcceptedStatusDuplicate && accepted.DuplicateOf != nil {
		id = *accepted.DuplicateOf
	}
	delay := seconds(accepted.RetryAfterSeconds, 0)
	for {
		if err := c.sleep(ctx, delay); err != nil {
			return nil, fmt.Errorf("%w: no result in %s", ErrOutcomeUnknown, wait)
		}
		delay = _pollDelay
		r, retryAfter, err := c.receipt(ctx, id)
		switch {
		case err != nil:
			return nil, err
		case r == nil:
			delay = max(delay, retryAfter)
		case r.Status == ciapi.ReceiptStatusQuarantined ||
			(r.Channel == ciapi.ReceiptChannelCi && r.RetryAfterSeconds != nil && r.ResultUrl == nil):
			delay = seconds(r.RetryAfterSeconds, _pollDelay)
		case r.Status == ciapi.ReceiptStatusDuplicate && r.DuplicateOf != nil && *r.DuplicateOf != id:
			id, delay = *r.DuplicateOf, 0
		default:
			return r, nil
		}
	}
}

// receipt reads one receipt; a nil receipt without an error means the answer is worth asking again.
func (c *Client) receipt(ctx context.Context, id uuid.UUID) (*ciapi.Receipt, time.Duration, error) {
	resp, err := c.api.GetReceipt(ctx, id)
	if err != nil {
		return nil, 0, nil //nolint:nilerr // a lost answer is asked again until the wait runs out
	}
	defer resp.Body.Close()
	parsed, err := ciapi.ParseGetReceiptResponse(resp)
	switch {
	case err != nil || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		return nil, retryAfter(resp.Header), nil //nolint:nilerr // an unreadable or busy answer is asked again
	case parsed.StatusCode() != http.StatusOK || parsed.JSON200 == nil:
		return nil, 0, fmt.Errorf("read receipt: %w", refusal(parsed.StatusCode(), parsed.Body))
	}
	return parsed.JSON200, 0, nil
}

// ResultURL resolves the result link of a receipt against the site address.
func (c *Client) ResultURL(r *ciapi.Receipt) string {
	if r.ResultUrl == nil || *r.ResultUrl == "" {
		return ""
	}
	ref, err := url.Parse(*r.ResultUrl)
	if err != nil {
		return ""
	}
	return c.site.ResolveReference(ref).String()
}

func (c *Client) send(ctx context.Context, what string, call func() (*http.Response, error)) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := call()
		var (
			reason string
			wait   time.Duration
			last   *RefusedError
		)
		switch {
		case err != nil && ctx.Err() != nil:
			return nil, ctx.Err()
		case err != nil:
			reason, wait = "connection error: "+err.Error(), c.backoff(attempt)
			last = &RefusedError{Code: "unreachable", Detail: "craftPulse did not answer"}
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // an unread rest only loses the problem detail
			resp.Body.Close()
			last = refusal(resp.StatusCode, body)
			reason, wait = "HTTP "+strconv.Itoa(resp.StatusCode), c.backoff(attempt)
			if after := retryAfter(resp.Header); resp.StatusCode == http.StatusTooManyRequests && after > 0 {
				wait = after
			} else {
				wait = max(wait, after)
			}
		default:
			return resp, nil
		}
		if attempt == _maxAttempts {
			return nil, last
		}
		fmt.Fprintf(c.out, "%s: attempt %d of %d failed, %s; retry in %s\n", what, attempt, _maxAttempts, reason, wait.Round(100*time.Millisecond))
		if err := c.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// backoff doubles the pause from one second up to 30 seconds and spreads it by half either way.
func (c *Client) backoff(attempt int) time.Duration {
	d := min(_maxDelay, _baseDelay<<(attempt-1))
	return time.Duration(float64(d) * (0.5 + c.jitter()))
}

func refusal(status int, body []byte) *RefusedError {
	e := &RefusedError{Status: status}
	var p ciapi.Problem
	if json.Unmarshal(body, &p) == nil {
		e.Code = p.Code
		if p.Detail != nil {
			e.Detail = *p.Detail
		}
		if p.CorrelationId != uuid.Nil {
			e.CorrelationID = p.CorrelationId.String()
		}
	}
	return e
}

func retryAfter(h http.Header) time.Duration {
	n, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || n < 0 {
		return 0
	}
	return min(time.Duration(n)*time.Second, _maxRetryAfter)
}

func seconds(n *int, fallback time.Duration) time.Duration {
	if n == nil {
		return fallback
	}
	return min(time.Duration(*n)*time.Second, _maxRetryAfter)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
