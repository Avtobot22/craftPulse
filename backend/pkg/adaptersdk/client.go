package adaptersdk

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

const (
	// SourceTimeout is how long an adapter waits for the source; the core waits 40 s for the adapter.
	SourceTimeout       = 30 * time.Second
	DefaultMaxBodyBytes = 25 << 20
	_maxSourceRedirects = 3
	_drainBytes         = 64 << 10
)

type ClientConfig struct {
	Policy  netguard.Policy
	RootCAs *x509.CertPool
	// Timeout bounds the whole exchange including the body; zero is SourceTimeout.
	Timeout time.Duration
	// MaxResponseBytes bounds a response body; zero is DefaultMaxBodyBytes.
	MaxResponseBytes int64
	// Resolver and Connect replace name resolution and the socket, for tests; nil uses the system ones.
	Resolver netguard.Resolver
	Connect  func(ctx context.Context, network, address string) (net.Conn, error)
}

// SourceClient is the only way an adapter reaches its source: HTTPS only, every address checked when
// the connection is made, at most three redirects, bounded time and size (TZ 13.7).
type SourceClient struct {
	http    *http.Client
	maxBody int64
	now     func() time.Time
}

func NewSourceClient(cfg ClientConfig) *SourceClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = SourceTimeout
	}
	maxBody := cfg.MaxResponseBytes
	if maxBody == 0 {
		maxBody = DefaultMaxBodyBytes
	}
	dialer := &netguard.Dialer{Policy: cfg.Policy, Resolver: cfg.Resolver, Connect: cfg.Connect}
	return &SourceClient{
		http:    netguard.NewClient(dialer, netguard.ClientOptions{Timeout: timeout, MaxRedirects: _maxSourceRedirects, RootCAs: cfg.RootCAs}),
		maxBody: maxBody,
		now:     time.Now,
	}
}

// Do returns a 2xx response with a size-bounded body; every other outcome is a classified *Error.
func (c *SourceClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req) //nolint:gosec // the address is checked by netguard when the connection is made
	if err != nil {
		return nil, classifyError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.CopyN(io.Discard, resp.Body, _drainBytes)
		_ = resp.Body.Close()
		return nil, classifyStatus(resp, c.now())
	}
	resp.Body = &limitedBody{rc: resp.Body, left: c.maxBody}
	return resp, nil
}

// DecodeJSON reads a JSON body of a response from Do and closes it; a body that does not decode is
// invalid_response, a body cut by the deadline is timeout.
func DecodeJSON(resp *http.Response, v any) error {
	defer func() { _ = resp.Body.Close() }()
	err := json.NewDecoder(resp.Body).Decode(v)
	if err == nil {
		return nil
	}
	_, syntax := errors.AsType[*json.SyntaxError](err)
	_, mismatch := errors.AsType[*json.UnmarshalTypeError](err)
	if syntax || mismatch || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &Error{Code: AdapterErrorCodeInvalidResponse, Detail: "source response is not valid JSON", cause: err}
	}
	return classifyError(err)
}

type limitedBody struct {
	rc   io.ReadCloser
	left int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		var one [1]byte
		n, err := b.rc.Read(one[:])
		if n > 0 {
			return 0, errResponseTooLarge
		}
		if err == nil {
			err = io.EOF
		}
		return 0, err
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.rc.Close() }
