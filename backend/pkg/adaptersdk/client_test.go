package adaptersdk_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

// _sourceIP is where every name of the test source resolves; Connect sends it to the TLS test server.
const _sourceIP = "203.0.113.10"

type hosts struct {
	fixed   map[string]string
	rebinds atomic.Int32
}

func (h *hosts) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if host == "rebind.example.test" {
		if h.rebinds.Add(1) == 1 {
			return []netip.Addr{netip.MustParseAddr(_sourceIP)}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	if a, ok := h.fixed[host]; ok {
		return []netip.Addr{netip.MustParseAddr(a)}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

type testSource struct {
	url      string
	addr     string
	roots    *x509.CertPool
	client   *adaptersdk.SourceClient
	connects atomic.Int32
}

// newTestSource serves h over TLS as *.example.test: names resolve through a fake resolver and only
// the public test address is connected, so the guard runs exactly as in production.
func newTestSource(t *testing.T, h http.Handler, cfg adaptersdk.ClientConfig) *testSource {
	t.Helper()
	cert, roots := testCertificate(t)
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	ts := &testSource{url: "https://jira.example.test", addr: srv.Listener.Addr().String(), roots: roots}
	cfg.RootCAs = roots
	ts.client = ts.newClient(cfg)
	return ts
}

func (ts *testSource) newClient(cfg adaptersdk.ClientConfig) *adaptersdk.SourceClient {
	cfg.Resolver = &hosts{fixed: map[string]string{
		"jira.example.test": _sourceIP, "internal.example.test": "127.0.0.1",
		"metadata.example.test": "169.254.169.254", "db.example.test": "198.51.100.7",
	}}
	cfg.Connect = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != _sourceIP+":443" {
			return nil, errors.New("test connects only to the source address, got " + address)
		}
		ts.connects.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, network, ts.addr)
	}
	if cfg.Policy.Installation == nil {
		cfg.Policy.Installation = []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	}
	return adaptersdk.NewSourceClient(cfg)
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.test"},
		DNSNames: []string{"*.example.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

// getJSON decodes the body of a successful response into v.
func (ts *testSource) getJSON(t *testing.T, url string, v any) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	req.Close = true
	resp, err := ts.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return adaptersdk.DecodeJSON(resp, v)
}

func (ts *testSource) get(t *testing.T, url string) error {
	t.Helper()
	var discard any
	return ts.getJSON(t, url, &discard)
}

func contractError(t *testing.T, err error) *adaptersdk.Error {
	t.Helper()
	e, ok := errors.AsType[*adaptersdk.Error](err)
	require.True(t, ok, "want a contract error, got %v", err)
	return e
}

func sourceMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) })
	mux.HandleFunc("GET /status/{code}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("retry") {
			w.Header().Set("Retry-After", r.URL.Query().Get("retry"))
		}
		code := map[string]int{"401": 401, "403": 403, "404": 404, "429": 429, "503": 503, "400": 400}[r.PathValue("code")]
		w.WriteHeader(code)
		_, _ = w.Write([]byte("token=" + _secret))
	})
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /garbage", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"items": [1, 2`)) })
	mux.HandleFunc("GET /big", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"text":"` + strings.Repeat("x", 4096) + `"}`))
	})
	mux.HandleFunc("GET /redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusFound)
	})
	mux.HandleFunc("GET /hops/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		if n == 0 {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Redirect(w, r, "/hops/"+strconv.Itoa(n-1), http.StatusFound)
	})
	return mux
}

func TestSourceClient_ClassifiesResponses(t *testing.T) {
	t.Parallel()
	ts := newTestSource(t, sourceMux(), adaptersdk.ClientConfig{Timeout: 300 * time.Millisecond, MaxResponseBytes: 1024})
	retryDate := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)

	tests := []struct {
		path      string
		want      adaptersdk.AdapterErrorCode
		wantRetry time.Duration
	}{
		{"/status/401", adaptersdk.AdapterErrorCodeAuth, 0},
		{"/status/403", adaptersdk.AdapterErrorCodeAuth, 0},
		{"/status/404", adaptersdk.AdapterErrorCodeNotFound, 0},
		{"/status/429?retry=120", adaptersdk.AdapterErrorCodeRateLimited, 120 * time.Second},
		{"/status/429?retry=" + strings.ReplaceAll(retryDate.Format(http.TimeFormat), " ", "%20"), adaptersdk.AdapterErrorCodeRateLimited, 10 * time.Minute},
		{"/status/429", adaptersdk.AdapterErrorCodeRateLimited, 0},
		{"/status/503", adaptersdk.AdapterErrorCodeUnavailable, 0},
		{"/status/400", adaptersdk.AdapterErrorCodeInvalidResponse, 0},
		{"/slow", adaptersdk.AdapterErrorCodeTimeout, 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			err := ts.get(t, ts.url+tt.path)
			e := contractError(t, err)
			assert.Equal(t, tt.want, e.Code)
			assert.NotContains(t, e.Detail, _secret, "the source body never reaches the detail")
			if tt.wantRetry == 0 {
				assert.Nil(t, e.RetryAt)
				return
			}
			require.NotNil(t, e.RetryAt)
			assert.WithinDuration(t, time.Now().Add(tt.wantRetry), *e.RetryAt, 3*time.Second)
		})
	}

	for path, detail := range map[string]string{"/garbage": "not valid JSON", "/big": "size limit"} {
		var v map[string]any
		e := contractError(t, ts.getJSON(t, ts.url+path, &v))
		assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidResponse, e.Code, path)
		assert.Contains(t, e.Detail, detail, path)
	}
	var ok map[string]bool
	require.NoError(t, ts.getJSON(t, ts.url+"/ok", &ok))
	assert.True(t, ok["ok"])
	assert.Equal(t, 30*time.Second, adaptersdk.SourceTimeout)
}

func TestSourceClient_RefusesInternalDestinations(t *testing.T) {
	t.Parallel()
	ts := newTestSource(t, sourceMux(), adaptersdk.ClientConfig{})

	tests := []struct {
		name, url string
		want      netguard.Reason
		connects  int32
	}{
		{"ipv4 loopback", "https://127.0.0.1/ok", netguard.ReasonLoopback, 0},
		{"ipv6 loopback", "https://[::1]/ok", netguard.ReasonLoopback, 0},
		{"cloud metadata", "https://169.254.169.254/latest/meta-data", netguard.ReasonLinkLocal, 0},
		{"name of metadata", "https://metadata.example.test/", netguard.ReasonLinkLocal, 0},
		{"installation network", "https://db.example.test/", netguard.ReasonInstallation, 0},
		{"plain http", "http://jira.example.test/ok", netguard.ReasonInsecureScheme, 0},
		{"redirect to internal", "https://jira.example.test/redirect?to=https://internal.example.test/ok", netguard.ReasonLoopback, 1},
		{"redirect to http", "https://jira.example.test/redirect?to=http://jira.example.test/ok", netguard.ReasonInsecureScheme, 1},
		{"fourth redirect", "https://jira.example.test/hops/4", netguard.ReasonTooManyRedirects, -1},
	}
	for _, tt := range tests {
		before := ts.connects.Load()
		err := ts.get(t, tt.url)
		e := contractError(t, err)
		assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidRequest, e.Code, tt.name)
		assert.Contains(t, e.Detail, string(tt.want), tt.name)
		assert.ErrorIs(t, err, netguard.ErrRefused, tt.name)
		if tt.connects >= 0 {
			assert.Equal(t, tt.connects, ts.connects.Load()-before, tt.name+": connections to the source")
		}
	}

	err := ts.get(t, "https://jira.example.test/hops/3")
	require.NoError(t, err, "three redirects inside the source are followed")
}

func TestSourceClient_ChangedDNSAnswerDoesNotBypass(t *testing.T) {
	t.Parallel()
	ts := newTestSource(t, sourceMux(), adaptersdk.ClientConfig{})

	err := ts.get(t, "https://rebind.example.test/ok")
	require.NoError(t, err, "the first answer is public")
	err = ts.get(t, "https://rebind.example.test/ok")
	assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidRequest, contractError(t, err).Code)
	assert.ErrorIs(t, err, netguard.ErrRefused)
	assert.Equal(t, int32(1), ts.connects.Load(), "the rebound address is never connected")
}

func TestSourceClient_UntrustedCertificate(t *testing.T) {
	t.Parallel()
	ts := newTestSource(t, sourceMux(), adaptersdk.ClientConfig{})
	ts.client = ts.newClient(adaptersdk.ClientConfig{RootCAs: x509.NewCertPool()})
	e := contractError(t, ts.get(t, ts.url+"/ok"))
	assert.Equal(t, adaptersdk.AdapterErrorCodeUnavailable, e.Code)
	assert.Equal(t, "source certificate is not trusted", e.Detail)
}

func TestHandler_PassesSourceErrorsToCore(t *testing.T) {
	t.Parallel()
	ts := newTestSource(t, sourceMux(), adaptersdk.ClientConfig{})
	src := &fakeSource{probe: func(ctx context.Context, _ adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.BaseUrl, http.NoBody)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		resp, err := ts.client.Do(req)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		_ = resp.Body.Close()
		return adaptersdk.ProbeResult{Provider: adaptersdk.ProbeResultProviderJiraDc, InstanceKey: "jira.example.test"}, nil
	}}
	h := newHarness(t, adaptersdk.NewStore(nil, nil), src)
	probe := func(path string) (*reply, adaptersdk.AdapterError) {
		resp, body := h.call(http.MethodPost, "/probe", map[string]any{"base_url": ts.url + path, "scopes": []any{}})
		return resp, adapterError(t, body)
	}

	resp, e := probe("/status/429?retry=120")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, adaptersdk.AdapterErrorCodeRateLimited, e.Code)
	require.NotNil(t, e.RetryAt)
	assert.WithinDuration(t, time.Now().Add(2*time.Minute), *e.RetryAt, 3*time.Second)
	retry, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	require.NoError(t, err)
	assert.InDelta(t, 120, retry, 3)

	for path, want := range map[string]struct {
		status int
		code   adaptersdk.AdapterErrorCode
	}{
		"/status/401": {http.StatusForbidden, adaptersdk.AdapterErrorCodeAuth},
		"/status/503": {http.StatusServiceUnavailable, adaptersdk.AdapterErrorCodeUnavailable},
		"/status/404": {http.StatusNotFound, adaptersdk.AdapterErrorCodeNotFound},
	} {
		resp, e := probe(path)
		assert.Equal(t, want.status, resp.StatusCode, path)
		assert.Equal(t, want.code, e.Code, path)
		assert.Equal(t, "corr-test-1", e.CorrelationId, path)
	}
	resp, e = probe("/redirect?to=https://internal.example.test/")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "source address refused: loopback", e.Detail)
	assert.NotContains(t, h.logs.String(), _secret, "the source body is not logged")
}
