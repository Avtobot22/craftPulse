package adapterapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

const (
	// CallTimeout is how long the core waits for an adapter; the adapter waits 30 s for its source (TZ 10.3).
	CallTimeout   = 40 * time.Second
	_maxJSONBytes = 32 << 20
	_drainBytes   = 64 << 10
	_resultOK     = "ok"
)

// Builtin is an adapter of the installation: its root address without /v1 and its service key.
type Builtin struct {
	URL string
	Key entity.Secret
}

type Config struct {
	Builtin map[entity.Provider]Builtin
	// InternalCA is the only root a built-in adapter may chain to (TZ 13.6).
	InternalCA *x509.CertPool
	// ClientRoots verify adapters of clients; nil is the system roots.
	ClientRoots *x509.CertPool
	// Policy is the network scope of adapters of clients (TZ 13.7).
	Policy netguard.Policy
	// SourcePolicy is the network scope of sources; a source address is checked against it when saved.
	SourcePolicy netguard.Policy
	// Timeout bounds a call including its body; zero is CallTimeout.
	Timeout    time.Duration
	Registerer prometheus.Registerer
	// Resolver and Connect replace name resolution and sockets for adapters of clients, in tests.
	Resolver netguard.Resolver
	Connect  func(ctx context.Context, network, address string) (net.Conn, error)
}

// Caller calls adapters: built-in ones by addresses of the installation with trust only to its CA,
// adapters of clients through netguard with the system roots or the CA given at installation.
type Caller struct {
	builtin  map[entity.Provider]builtinAdapter
	clients  *http.Client
	policy   netguard.Policy
	sources  netguard.Policy
	resolver netguard.Resolver
	timeout  time.Duration
	calls    *prometheus.CounterVec
	notAfter *prometheus.GaugeVec
}

type builtinAdapter struct {
	url  string
	key  entity.Secret
	http *http.Client
}

func NewCaller(cfg Config) (*Caller, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = CallTimeout
	}
	c := &Caller{
		builtin: make(map[entity.Provider]builtinAdapter, len(cfg.Builtin)),
		timeout: timeout,
		calls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_adapter_calls_total",
			Help: "Calls of adapters by connection, operation and result class with its code.",
		}, []string{"connection_id", "operation", "class", "code"}),
		notAfter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "craftpulse_adapter_certificate_not_after_timestamp_seconds",
			Help: "Expiry of the certificate a built-in adapter presented on the last connection.",
		}, []string{"provider"}),
	}
	if cfg.Registerer != nil {
		if err := cfg.Registerer.Register(c.calls); err != nil {
			return nil, fmt.Errorf("adapterapi: register calls metric: %w", err)
		}
		if err := cfg.Registerer.Register(c.notAfter); err != nil {
			return nil, fmt.Errorf("adapterapi: register certificate metric: %w", err)
		}
	}
	for provider, b := range cfg.Builtin {
		if !strings.HasPrefix(b.URL, "https://") {
			return nil, fmt.Errorf("adapterapi: built-in adapter %s must have an https address", provider)
		}
		if cfg.InternalCA == nil {
			return nil, errors.New("adapterapi: built-in adapters need the internal CA")
		}
		c.builtin[provider] = builtinAdapter{url: strings.TrimRight(b.URL, "/"), key: b.Key, http: c.builtinHTTP(provider, cfg.InternalCA, timeout)}
	}
	c.policy, c.sources, c.resolver = cfg.Policy, cfg.SourcePolicy, cfg.Resolver
	if c.resolver == nil {
		c.resolver = net.DefaultResolver
	}
	dialer := &netguard.Dialer{Policy: cfg.Policy, Resolver: cfg.Resolver, Connect: cfg.Connect}
	c.clients = netguard.NewClient(dialer, netguard.ClientOptions{Timeout: timeout, RootCAs: cfg.ClientRoots})
	return c, nil
}

func (c *Caller) builtinHTTP(provider entity.Provider, roots *x509.CertPool, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) > 0 {
					c.notAfter.WithLabelValues(string(provider)).Set(float64(cs.PeerCertificates[0].NotAfter.Unix()))
				}
				return nil
			},
		},
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        16,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// call is one request of an operation: the generated client is built for the address of the target.
type call struct {
	gen    *Client
	key    entity.Secret
	org    uuid.UUID
	conn   uuid.UUID
	ver    string
	corrID string
	op     entity.AdapterOp
	target entity.AdapterTarget
}

func (c *Caller) prepare(ctx context.Context, t entity.AdapterTarget, op entity.AdapterOp) (call, error) {
	var (
		root string
		key  entity.Secret
		doer *http.Client
	)
	switch t.Kind {
	case entity.AdapterBuiltin:
		b, ok := c.builtin[t.Provider]
		if !ok {
			return call{}, c.record(t, op, &entity.AdapterError{
				Class: entity.CallAdapterUnavailable, Code: entity.AdapterNotConfigured,
				Detail: "no built-in adapter for provider " + string(t.Provider),
			})
		}
		root, key, doer = b.url, b.key, b.http
	case entity.AdapterClient:
		root, key, doer = strings.TrimRight(t.URL, "/"), t.Key, c.clients
	default:
		return call{}, fmt.Errorf("adapterapi: unknown adapter kind %q", t.Kind)
	}
	gen, err := NewClient(root+"/v1", WithHTTPClient(doer))
	if err != nil {
		return call{}, c.record(t, op, &entity.AdapterError{Class: entity.CallAdapterUnavailable, Code: entity.AdapterNotConfigured, Detail: "invalid adapter address"})
	}
	corr := logger.CorrelationID(ctx)
	if corr == uuid.Nil {
		corr = uuid.Must(uuid.NewV7())
	}
	return call{
		gen: gen, key: key, org: uuid.UUID(t.OrganizationID), conn: uuid.UUID(t.ConnectionID),
		ver: t.ContractVersion, corrID: corr.String(), op: op, target: t,
	}, nil
}

func (cl call) authorize(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+cl.key.Value())
	return nil
}

// send runs one request and classifies everything but a success with the wanted status.
// A call the caller has cancelled or timed out itself returns the error of its context unclassified.
func (c *Caller) send(parent context.Context, cl call, want int, do func(ctx context.Context) (*http.Response, error)) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	resp, err := do(ctx)
	if err != nil {
		cancel()
		if parent.Err() != nil {
			return nil, nil, parent.Err()
		}
		return nil, nil, c.record(cl.target, cl.op, transportError(err))
	}
	if resp.StatusCode == want {
		return resp, cancel, nil
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	return nil, nil, c.record(cl.target, cl.op, answerError(resp))
}

func (c *Caller) record(t entity.AdapterTarget, op entity.AdapterOp, err error) error {
	class, code := _resultOK, ""
	if ae, ok := errors.AsType[*entity.AdapterError](err); ok {
		class, code = string(ae.Class), ae.Code
	}
	c.calls.WithLabelValues(t.ConnectionID.String(), string(op), class, code).Inc()
	return err
}

func transportError(err error) *entity.AdapterError {
	out := &entity.AdapterError{Class: entity.CallAdapterUnavailable, Code: entity.AdapterUnreachable, Detail: err.Error()}
	var (
		netErr      net.Error
		unknownCA   x509.UnknownAuthorityError
		hostname    x509.HostnameError
		invalidCert x509.CertificateInvalidError
		verifyErr   *tls.CertificateVerificationError
	)
	switch {
	case errors.Is(err, netguard.ErrRefused):
		out.Code, out.Detail = entity.AdapterOutsideNetworkScope, "address refused"
		if refused, ok := errors.AsType[*netguard.RefusedError](err); ok {
			out.Detail += ": " + string(refused.Reason)
			if refused.Reason == netguard.ReasonTooManyRedirects {
				out.Code = entity.AdapterContractViolation
			}
		}
	case errors.As(err, &verifyErr), errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalidCert):
		out.Code = entity.AdapterCertificateUntrusted
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		out.Code = entity.AdapterTimeout
	}
	return out
}

// answerError reads an AdapterError body; an answer that is not one is a violation of the contract.
func answerError(resp *http.Response) *entity.AdapterError {
	violation := &entity.AdapterError{
		Class: entity.CallAdapterUnavailable, Code: entity.AdapterContractViolation,
		Detail: "unexpected answer status " + strconv.Itoa(resp.StatusCode),
	}
	if resp.StatusCode < 400 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		_, _ = io.CopyN(io.Discard, resp.Body, _drainBytes)
		return violation
	}
	var body AdapterError
	if err := json.NewDecoder(io.LimitReader(resp.Body, _maxJSONBytes)).Decode(&body); err != nil || !body.Code.Valid() {
		return violation
	}
	out := &entity.AdapterError{Class: entity.CallSource, Code: string(body.Code), RetryAt: body.RetryAt, Detail: body.Detail}
	switch body.Code {
	case AdapterErrorCodeUnsupportedContractVersion:
		out.Class = entity.CallContractUnsupported
	case AdapterErrorCodeUnauthorized:
		out.Class, out.Code = entity.CallAdapterUnavailable, entity.AdapterKeyRejected
	default:
	}
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
		out.RetryAfter = time.Duration(seconds) * time.Second
	}
	return out
}

func violation(detail string) *entity.AdapterError {
	return &entity.AdapterError{Class: entity.CallAdapterUnavailable, Code: entity.AdapterContractViolation, Detail: detail}
}

// decodeJSON reads the body of a success; a body that does not decode breaks the contract.
func decodeJSON(resp *http.Response, v any) error {
	if err := json.NewDecoder(io.LimitReader(resp.Body, _maxJSONBytes)).Decode(v); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &entity.AdapterError{Class: entity.CallAdapterUnavailable, Code: entity.AdapterTimeout, Detail: err.Error()}
		}
		return violation("answer body does not decode: " + err.Error())
	}
	return nil
}
