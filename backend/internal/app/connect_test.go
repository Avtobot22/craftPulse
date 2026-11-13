package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_jiraInstance     = "jira.test.craftpulse.internal"
	_clientAdapterKey = "client-adapter-key-5f1c0b7e"
)

var _probeIn = entity.ProbeInput{BaseURL: "https://example.atlassian.net", Scopes: []entity.ScopeRef{{Kind: entity.ScopeTrackerProject, NativeScopeID: "10000"}}}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTestCA(t *testing.T, name string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{cert: cert, key: key, pool: pool}
}

// serverTLS issues a server certificate for 127.0.0.1 and the names, valid until notAfter.
func (ca *testCA) serverTLS(t *testing.T, notAfter time.Time, names ...string) (*tls.Config, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "adapter"},
		DNSNames: names, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}}}, leaf
}

// adapterServer serves h over TLS and counts the calls of the contract that reach it.
type adapterServer struct {
	*httptest.Server
	src  *testadapter.Adapter
	db   adapterDB
	hits atomic.Int64
	mu   sync.Mutex
	auth []string
}

func serveTLSAdapter(t *testing.T, cfg *tls.Config, h http.Handler) *adapterServer {
	t.Helper()
	s := &adapterServer{}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			s.hits.Add(1)
			s.mu.Lock()
			s.auth = append(s.auth, r.Header.Get("Authorization"))
			s.mu.Unlock()
		}
		h.ServeHTTP(w, r)
	}))
	s.TLS = cfg
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

// testAdapter is the test adapter with its own database behind TLS, accepting the given service key.
func testAdapter(t *testing.T, tlsCfg *tls.Config, scenario testadapter.Config, key string) *adapterServer {
	t.Helper()
	db := newAdapterDB(t, true)
	src, err := testadapter.New(scenario)
	require.NoError(t, err)
	h, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: src, Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)), ServiceKeys: []string{key},
		Log: zerolog.Nop(), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := serveTLSAdapter(t, tlsCfg, h)
	srv.db, srv.src = db, src
	return srv
}

type connectEnv struct {
	uc    *connect.UseCase
	app   *pgxpool.Pool
	owner *pgxpool.Pool
	clock *clock.Manual
	reg   *prometheus.Registry
}

func newConnectEnv(t *testing.T, cfg adapterapi.Config) *connectEnv {
	t.Helper()
	db := newDB(t)
	db.migrate(t)
	e := &connectEnv{
		app: db.pool(t, "craftpulse_app"), owner: db.pool(t, "craftpulse_owner"),
		clock: clock.NewManual(time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)), reg: prometheus.NewRegistry(),
	}
	cfg.Registerer = e.reg
	if cfg.Resolver == nil {
		cfg.Resolver = mapResolver{}
	}
	caller, err := adapterapi.NewCaller(cfg)
	require.NoError(t, err)
	e.uc, err = newConnect(e.app, adapterKeyring(t), caller, e.clock)
	require.NoError(t, err)
	return e
}

// connection writes a draft on the Jira instance through the repository: these tests are about the calls,
// the form is checked by the API tests. url and key are set for an adapter of a client.
func (e *connectEnv) connection(t *testing.T, org entity.ID, url, key string, scopes ...entity.NewScope) entity.ID {
	t.Helper()
	id, err := e.draft(t, org, url, key, scopes...)
	require.NoError(t, err)
	return id
}

func (e *connectEnv) draft(t *testing.T, org entity.ID, url, key string, scopes ...entity.NewScope) (entity.ID, error) {
	t.Helper()
	inst, err := e.uc.EnsureInstance(t.Context(), org, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	c := entity.Connection{
		ID: entity.ID(uuid.Must(uuid.NewV7())), OrganizationID: org, InstanceID: inst.ID, Name: "Jira", AdapterKind: entity.AdapterBuiltin,
		ContractVersion: connect.ContractVersions[0], AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionDraft,
		RowVersion: 1, CreatedBy: entity.ID(uuid.New()), CreatedAt: e.clock.Now(),
	}
	if url != "" {
		blob, version, err := adapterKeyring(t).SealEnvelope([]byte(key), connect.AdapterKeyAAD(org, c.ID))
		require.NoError(t, err)
		c.AdapterKind, c.AdapterURL, c.AdapterKeyEnc, c.KeyVersion = entity.AdapterClient, url, blob, version
	}
	r := connectrepo.NewRepo(e.app)
	err = pg.InOrgTx(t.Context(), e.app, uuid.UUID(org), func(ctx context.Context) error {
		if err := r.CreateConnection(ctx, c); err != nil {
			return err
		}
		for _, sc := range scopes {
			if _, err := r.AddScope(ctx, org, c.ID, c.InstanceID, sc, e.clock.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	return c.ID, err
}

func (e *connectEnv) activate(t *testing.T, id entity.ID) {
	t.Helper()
	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection SET status = 'active' WHERE id = $1", uuid.UUID(id))
	require.NoError(t, err)
}

func (e *connectEnv) gauge(t *testing.T, name, label string) (float64, bool) {
	t.Helper()
	families, err := e.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == label {
					return m.GetGauge().GetValue(), true
				}
			}
		}
	}
	return 0, false
}

func adapterErr(t *testing.T, err error) *entity.AdapterError {
	t.Helper()
	ae, ok := errors.AsType[*entity.AdapterError](err)
	require.True(t, ok, "want *entity.AdapterError, got %v", err)
	return ae
}

func collect() entity.CredentialsInput {
	return entity.CredentialsInput{Purpose: "collect", Secret: "jira-token-CONNECT-31c2"}
}

func TestConnect_BuiltinAdapterTrustsOnlyTheInternalCA(t *testing.T) {
	t.Parallel()
	internal, foreign := newTestCA(t, "craftpulse internal CA"), newTestCA(t, "foreign CA")
	notAfter := time.Now().Add(20 * 24 * time.Hour).Truncate(time.Second)
	trusted, leaf := internal.serverTLS(t, notAfter)
	stranger, _ := foreign.serverTLS(t, notAfter)
	good := testAdapter(t, trusted, testadapter.Config{}, _adapterKey)
	bad := testAdapter(t, stranger, testadapter.Config{}, _adapterKey)

	org := entity.ID(uuid.New())
	untrusted := newConnectEnv(t, adapterapi.Config{
		InternalCA: internal.pool,
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: bad.URL, Key: _adapterKey}},
	})
	err := untrusted.uc.PutCredentials(t.Context(), org, untrusted.connection(t, org, "", ""), collect())
	ae := adapterErr(t, err)
	assert.Equal(t, entity.CallAdapterUnavailable, ae.Class)
	assert.Equal(t, entity.AdapterCertificateUntrusted, ae.Code)
	assert.Zero(t, bad.hits.Load(), "the call stops at the handshake")
	_, published := untrusted.gauge(t, "craftpulse_adapter_certificate_not_after_timestamp_seconds", "jira_cloud")
	assert.False(t, published, "an untrusted certificate is not reported as the adapter's")

	env := newConnectEnv(t, adapterapi.Config{
		InternalCA: internal.pool,
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: good.URL, Key: _adapterKey}},
	})
	id := env.connection(t, org, "", "")
	require.NoError(t, env.uc.PutCredentials(t.Context(), org, id, collect()))
	res, err := env.uc.Probe(t.Context(), org, id, _probeIn)
	require.NoError(t, err)
	assert.Equal(t, _jiraInstance, res.InstanceKey)
	assert.Equal(t, "1", res.ContractVersion)
	assert.Equal(t, []string{"Bearer " + _adapterKey, "Bearer " + _adapterKey}, good.auth, "the built-in key comes from the installation")

	expiry, published := env.gauge(t, "craftpulse_adapter_certificate_not_after_timestamp_seconds", "jira_cloud")
	require.True(t, published)
	assert.InDelta(t, float64(leaf.NotAfter.Unix()), expiry, 0, "the metric is the NotAfter of the certificate presented")
}

type mapResolver map[string]string

func (r mapResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	a, ok := r[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []netip.Addr{netip.MustParseAddr(a)}, nil
}

func TestConnect_ClientAdapterIsCheckedBeforeConnecting(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "client CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour), "adapter.client.test", "adapter.loop.test", "adapter.inside.test")
	srv := testAdapter(t, cfg, testadapter.Config{}, _clientAdapterKey)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	var dialed atomic.Int64
	env := newConnectEnv(t, adapterapi.Config{
		ClientRoots: ca.pool,
		Policy:      netguard.Policy{Installation: []netip.Prefix{netip.MustParsePrefix("10.1.0.0/16")}},
		Resolver:    mapResolver{"adapter.client.test": "203.0.113.10", "adapter.loop.test": "127.0.0.1", "adapter.inside.test": "10.1.2.3"},
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialed.Add(1)
			var d net.Dialer
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	})
	org := entity.ID(uuid.New())

	for _, host := range []string{"127.0.0.1", "adapter.loop.test", "adapter.inside.test"} {
		id := env.connection(t, org, "https://"+net.JoinHostPort(host, port), _clientAdapterKey)
		ae := adapterErr(t, env.uc.PutCredentials(t.Context(), org, id, collect()))
		assert.Equal(t, entity.CallAdapterUnavailable, ae.Class, host)
		assert.Equal(t, entity.AdapterOutsideNetworkScope, ae.Code, host)
	}
	assert.Zero(t, dialed.Load(), "a refused address is not dialed")
	assert.Zero(t, srv.hits.Load())

	id := env.connection(t, org, "https://"+net.JoinHostPort("adapter.client.test", port), _clientAdapterKey)
	require.NoError(t, env.uc.PutCredentials(t.Context(), org, id, collect()))
	_, err = env.uc.Probe(t.Context(), org, id, _probeIn)
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer " + _clientAdapterKey, "Bearer " + _clientAdapterKey}, srv.auth, "the key is decrypted from adapter_key_enc")

	var stored []byte
	require.NoError(t, env.owner.QueryRow(t.Context(), "SELECT adapter_key_enc FROM connect.connection WHERE id = $1", uuid.UUID(id)).Scan(&stored))
	assert.NotContains(t, string(stored), _clientAdapterKey, "the database holds only the ciphertext")
}

func TestConnect_RateLimitPausesEveryCallOfTheConnection(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	srv := testAdapter(t, cfg, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpProbe, Attempts: []int{1}, Kind: testadapter.FaultRateLimited, RetryAfterSeconds: 30},
	}}, _adapterKey)
	env := newConnectEnv(t, adapterapi.Config{
		InternalCA: ca.pool,
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
	})
	org := entity.ID(uuid.New())
	id := env.connection(t, org, "", "")
	ctx := t.Context()
	require.NoError(t, env.uc.PutCredentials(ctx, org, id, collect()))

	start := env.clock.Now()
	ae := adapterErr(t, func() error { _, err := env.uc.Probe(ctx, org, id, _probeIn); return err }())
	assert.Equal(t, entity.CallSource, ae.Class)
	assert.Equal(t, entity.SourceRateLimited, ae.Code)
	require.NotNil(t, ae.RetryAt)
	assert.Equal(t, start.Add(30*time.Second), *ae.RetryAt)
	state, err := env.uc.State(ctx, org, id)
	require.NoError(t, err)
	require.NotNil(t, state.Connection.PausedUntil)
	assert.True(t, start.Add(30*time.Second).Equal(*state.Connection.PausedUntil))

	reached := srv.hits.Load()
	fetch := entity.FetchInput{SchemaID: "tasks", Scope: _probeIn.Scopes[0], Limit: 3}
	for _, at := range []time.Duration{0, 10 * time.Second, 29*time.Second + 999*time.Millisecond} {
		env.clock.Advance(at - env.clock.Now().Sub(start))
		calls := map[string]error{
			"probe": func() error { _, err := env.uc.Probe(ctx, org, id, _probeIn); return err }(),
			"fetch": func() error { _, err := env.uc.Fetch(ctx, org, id, fetch); return err }(),
			"manifest": func() error {
				_, err := env.uc.Manifest(ctx, org, id, entity.ManifestInput{SchemaID: "tasks", Scope: fetch.Scope, Limit: 4})
				return err
			}(),
		}
		for name, err := range calls {
			assert.ErrorIs(t, err, entity.ErrConnectionPaused, "%s at %s", name, at)
		}
	}
	assert.Equal(t, reached, srv.hits.Load(), "no call that reaches the source reaches the adapter during the pause")
	require.NoError(t, env.uc.PutCredentials(ctx, org, id, collect()), "credentials stay in the adapter and are replaced during a pause")
	reached++
	assert.Equal(t, reached, srv.hits.Load())

	env.clock.Advance(time.Millisecond)
	_, err = env.uc.Probe(ctx, org, id, _probeIn)
	require.NoError(t, err, "calls resume after 30 s")
	body, err := env.uc.Fetch(ctx, org, id, fetch)
	require.NoError(t, err)
	n, err := io.Copy(io.Discard, body)
	require.NoError(t, err)
	require.NoError(t, body.Close())
	assert.Positive(t, n)
	assert.Equal(t, reached+2, srv.hits.Load())
}

func TestConnect_AdapterSilenceIsNotASourceTimeout(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	srv := testAdapter(t, cfg, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpProbe, Attempts: []int{1}, Kind: testadapter.FaultHang},
		{Operation: testadapter.OpProbe, Attempts: []int{2}, Kind: testadapter.FaultHang, DelayMS: 50},
	}}, _adapterKey)
	env := newConnectEnv(t, adapterapi.Config{
		InternalCA: ca.pool,
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
	})
	org := entity.ID(uuid.New())
	id := env.connection(t, org, "", "")
	require.NoError(t, env.uc.PutCredentials(t.Context(), org, id, collect()))

	started := time.Now()
	_, err := env.uc.Probe(t.Context(), org, id, _probeIn)
	waited := time.Since(started)
	ae := adapterErr(t, err)
	assert.Equal(t, entity.CallAdapterUnavailable, ae.Class, "an adapter silent for 40 s is unavailable")
	assert.Equal(t, entity.AdapterTimeout, ae.Code)
	assert.ErrorIs(t, err, entity.ErrAdapterUnavailable)
	assert.GreaterOrEqual(t, waited, adapterapi.CallTimeout)
	assert.Less(t, waited, 45*time.Second, "the core does not wait for the adapter's own 45 s")

	_, err = env.uc.Probe(t.Context(), org, id, _probeIn)
	ae = adapterErr(t, err)
	assert.Equal(t, entity.CallSource, ae.Class, "timeout answered by the adapter stays an error of the source")
	assert.Equal(t, entity.SourceTimeout, ae.Code)
	assert.NotErrorIs(t, err, entity.ErrAdapterUnavailable)
}

// versionedAdapter accepts only one contract version and declares the versions it supports in Probe.
func versionedAdapter(t *testing.T, cfg *tls.Config, accepts string, declares ...string) *adapterServer {
	t.Helper()
	return serveTLSAdapter(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if v := r.Header.Get("X-Craftpulse-Contract-Version"); v != accepts {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"code":"unsupported_contract_version","detail":"contract version `+v+` is not supported","retry_at":null,"correlation_id":"c"}`)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/probe") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"contract_versions": declares, "provider": "jira_cloud", "instance_key": _jiraInstance, "edition": "cloud",
			"version": nil, "access_mode": "service_account", "capabilities": []any{}, "scopes": []any{},
		})
	}))
}

func TestConnect_ContractVersions(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	org := entity.ID(uuid.New())
	probe := func(t *testing.T, srv *adapterServer) (entity.ProbeResult, error) {
		env := newConnectEnv(t, adapterapi.Config{
			InternalCA: ca.pool,
			Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
		})
		return env.uc.Probe(t.Context(), org, env.connection(t, org, "", ""), _probeIn)
	}

	t.Run("previous version is supported", func(t *testing.T) {
		t.Parallel()
		srv := versionedAdapter(t, cfg, "0", "0")
		res, err := probe(t, srv)
		require.NoError(t, err)
		assert.Equal(t, "0", res.ContractVersion)
		assert.Equal(t, int64(2), srv.hits.Load(), "the current version is tried first, then the previous one")
	})
	t.Run("newer version is refused", func(t *testing.T) {
		t.Parallel()
		_, err := probe(t, versionedAdapter(t, cfg, "2", "2"))
		ae := adapterErr(t, err)
		assert.Equal(t, entity.CallContractUnsupported, ae.Class)
		assert.ErrorIs(t, err, entity.ErrContractUnsupported)
	})
	t.Run("adapter that declares only a newer version", func(t *testing.T) {
		t.Parallel()
		_, err := probe(t, versionedAdapter(t, cfg, "1", "2"))
		assert.ErrorIs(t, err, entity.ErrContractUnsupported)
	})
	t.Run("connection on a version the core dropped", func(t *testing.T) {
		t.Parallel()
		srv := versionedAdapter(t, cfg, "1", "1")
		env := newConnectEnv(t, adapterapi.Config{
			InternalCA: ca.pool,
			Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
		})
		id := env.connection(t, org, "", "")
		_, err := env.owner.Exec(t.Context(), "UPDATE connect.connection SET contract_version = '-1' WHERE id = $1", uuid.UUID(id))
		require.NoError(t, err)
		_, err = env.uc.Probe(t.Context(), org, id, _probeIn)
		assert.ErrorIs(t, err, entity.ErrContractUnsupported)
		assert.Zero(t, srv.hits.Load(), "the core does not call an adapter on a version it does not support")
	})
}

func TestConnect_AnswerOutsideTheContractIsAnAdapterFailure(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	org := entity.ID(uuid.New())
	for name, answer := range map[string]func(w http.ResponseWriter){
		"proxy page": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "<html>bad gateway</html>")
		},
		"unknown error code": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"code":"overloaded","detail":"","retry_at":null,"correlation_id":"c"}`)
		},
		"probe without instance key": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"contract_versions":["1"],"provider":"jira_cloud","edition":null,"version":null,"access_mode":null,"capabilities":[],"scopes":[]}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := serveTLSAdapter(t, cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { answer(w) }))
			env := newConnectEnv(t, adapterapi.Config{
				InternalCA: ca.pool,
				Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
			})
			_, err := env.uc.Probe(t.Context(), org, env.connection(t, org, "", ""), _probeIn)
			ae := adapterErr(t, err)
			assert.Equal(t, entity.CallAdapterUnavailable, ae.Class)
			assert.Equal(t, entity.AdapterContractViolation, ae.Code)
		})
	}
}

func TestConnect_AuthAnswerMovesConnectionToAccessLost(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	srv := testAdapter(t, cfg, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, Kind: testadapter.FaultAuth},
	}}, _adapterKey)
	env := newConnectEnv(t, adapterapi.Config{
		InternalCA: ca.pool,
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
	})
	org := entity.ID(uuid.New())
	ctx := t.Context()
	draft, active := env.connection(t, org, "", ""), env.connection(t, org, "", "")
	env.activate(t, active)
	fetch := entity.FetchInput{SchemaID: "tasks", Scope: _probeIn.Scopes[0], Limit: 3}

	for _, id := range []entity.ID{draft, active} {
		require.NoError(t, env.uc.PutCredentials(ctx, org, id, collect()))
		_, err := env.uc.Fetch(ctx, org, id, fetch)
		ae := adapterErr(t, err)
		assert.Equal(t, entity.CallSource, ae.Class)
		assert.Equal(t, entity.SourceAuth, ae.Code)
	}
	failedAt := env.clock.Now()
	state, err := env.uc.State(ctx, org, active)
	require.NoError(t, err)
	assert.Equal(t, entity.ConnectionAccessLost, state.Connection.Status)
	require.NotNil(t, state.Connection.AccessLostAt)
	assert.True(t, failedAt.Equal(*state.Connection.AccessLostAt), "the date of the refusal is kept")

	env.clock.Advance(time.Hour)
	_, err = env.uc.Fetch(ctx, org, active, fetch)
	require.Error(t, err)
	state, err = env.uc.State(ctx, org, active)
	require.NoError(t, err)
	assert.True(t, failedAt.Equal(*state.Connection.AccessLostAt), "a repeated refusal does not move the date")

	state, err = env.uc.State(ctx, org, draft)
	require.NoError(t, err)
	assert.Equal(t, entity.ConnectionDraft, state.Connection.Status, "a draft is not a working connection to lose")
	assert.Equal(t, 1, count(t, ctx, env.owner, "SELECT count(*) FROM audit.event WHERE operation = 'connectionAccessLost' AND outcome = 'succeeded'"))
}

func TestConnect_OneActiveConnectionCollectsAScope(t *testing.T) {
	t.Parallel()
	env := newConnectEnv(t, adapterapi.Config{})
	org := entity.ID(uuid.New())
	ctx := t.Context()
	scope := entity.NewScope{Kind: entity.ScopeTrackerProject, NativeScopeID: "10000", DisplayName: "CORE"}

	first := env.connection(t, org, "", "", scope)
	inst, err := env.uc.EnsureInstance(ctx, org, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	assert.Equal(t, _jiraInstance, inst.InstanceKey)
	_, err = env.draft(t, org, "", "", scope)
	require.ErrorIs(t, err, entity.ErrScopeTaken, "the database refuses the same instance, kind and source ID")

	other := scope
	other.Kind = entity.ScopeRepository
	_, err = env.draft(t, org, "", "", other)
	require.NoError(t, err, "another kind with the same source ID is another scope")

	_, err = env.owner.Exec(ctx, "UPDATE connect.connection_scope SET status = 'removed', removed_at = now() WHERE connection_id = $1", uuid.UUID(first))
	require.NoError(t, err)
	_, err = env.draft(t, org, "", "", scope)
	require.NoError(t, err, "a removed scope is free for another connection")

	scopes, err := env.uc.Scopes(ctx, org, first)
	require.NoError(t, err)
	require.Len(t, scopes, 1)
	assert.Nil(t, scopes[0].AreaID, "the area appears after the first collection")
	area := entity.ID(uuid.New())
	require.NoError(t, env.uc.SetScopeArea(ctx, org, scopes[0].ID, area))
	require.NoError(t, env.uc.SetScopeArea(ctx, org, scopes[0].ID, area), "the same area again is a no-op")
	require.ErrorIs(t, env.uc.SetScopeArea(ctx, org, scopes[0].ID, entity.ID(uuid.New())), connect.ErrScopeArea)
	scopes, err = env.uc.Scopes(ctx, org, first)
	require.NoError(t, err)
	assert.Equal(t, &area, scopes[0].AreaID)
}

func TestConnect_ImportAndConnectionShareTheInstance(t *testing.T) {
	t.Parallel()
	env := newConnectEnv(t, adapterapi.Config{})
	org := entity.ID(uuid.New())
	ctx := t.Context()
	id := env.connection(t, org, "", "")
	state, err := env.uc.State(ctx, org, id)
	require.NoError(t, err)

	ids := make([]entity.ID, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			inst, err := env.uc.EnsureInstance(ctx, org, entity.ProviderJiraCloud, _jiraInstance)
			assert.NoError(t, err)
			ids[i] = inst.ID
		})
	}
	wg.Wait()
	for _, got := range ids {
		assert.Equal(t, state.Instance.ID, got, "a package of import declaring the instance gets the instance of the connection")
	}

	imported, err := env.uc.EnsureInstance(ctx, org, entity.ProviderUniversal, _jiraInstance)
	require.NoError(t, err)
	assert.NotEqual(t, state.Instance.ID, imported.ID, "another provider is another instance")
	assert.Equal(t, 2, count(t, ctx, env.owner, "SELECT count(*) FROM connect.source_instance"))
}

func TestConnect_TablesAreIsolatedByOrganization(t *testing.T) {
	t.Parallel()
	env := newConnectEnv(t, adapterapi.Config{})
	ctx := t.Context()
	orgA, orgB := entity.ID(uuid.New()), entity.ID(uuid.New())
	scope := entity.NewScope{Kind: entity.ScopeTrackerProject, NativeScopeID: "10000", DisplayName: "CORE"}
	env.connection(t, orgA, "", "", scope)
	connB := env.connection(t, orgB, "", "", scope)
	_, err := env.owner.Exec(ctx, `INSERT INTO connect.capability (organization_id, connection_id, capability, state, checked_at)
		VALUES ($1, $2, 'read_current', 'verified', now())`, uuid.UUID(orgB), uuid.UUID(connB))
	require.NoError(t, err)

	_, err = env.uc.State(ctx, orgA, connB)
	require.ErrorIs(t, err, entity.ErrNotFound, "A does not see the connection of B")

	err = pg.InOrgTx(ctx, env.app, uuid.UUID(orgA), func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		for _, table := range []string{"source_instance", "connection", "connection_scope"} {
			assert.Equal(t, 1, count(t, ctx, tx, "SELECT count(*) FROM connect."+table), table)
		}
		assert.Zero(t, count(t, ctx, tx, "SELECT count(*) FROM connect.capability"))
		tag, err := tx.Exec(ctx, "UPDATE connect.connection SET name = 'taken' WHERE id = $1", uuid.UUID(connB))
		require.NoError(t, err)
		assert.Zero(t, tag.RowsAffected(), "rows of B are not visible to A")
		_, err = tx.Exec(ctx, "INSERT INTO connect.source_instance (organization_id, provider, instance_key) VALUES ($1, 'git', 'x')", uuid.UUID(orgB))
		return err
	})
	require.Error(t, err, "A cannot write a row of B")
	assert.Zero(t, count(t, ctx, env.app, "SELECT count(*) FROM connect.connection"), "no organization set, no rows")
	assert.Equal(t, 2, count(t, ctx, env.owner, "SELECT count(*) FROM connect.connection"))
}
