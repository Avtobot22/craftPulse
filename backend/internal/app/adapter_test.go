package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _adapterKey = "adapter-service-key-0123456789"

type adapterDB struct {
	name string
}

func newAdapterDB(t *testing.T, migrated bool) adapterDB {
	t.Helper()
	name := fmt.Sprintf("app%d", _dbSeq.Add(1))
	code, out := exec(t, []string{"sh", "/adapter-db.sh", name})
	require.Zero(t, code, out)
	d := adapterDB{name: name}
	if migrated {
		owner := d.pool(t, "owner")
		_, _, err := migrateAdapter(t.Context(), owner, adaptersdk.NewStore(owner, adapterKeyring(t)))
		require.NoError(t, err)
	}
	return d
}

func (d adapterDB) url(role string) string {
	user := "adapter_" + d.name
	if role == "owner" {
		user += "_owner"
	}
	return fmt.Sprintf("postgres://%s@%s/adapter_%s?sslmode=disable", user, _hostPort, d.name)
}

func (d adapterDB) pool(t *testing.T, role string) *pgxpool.Pool {
	t.Helper()
	pool, err := pg.New(t.Context(), d.url(role), _adapterPasswords[role])
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func adapterMasterKeys() config.MasterKeys {
	return config.MasterKeys{CurrentVersion: 1, Current: config.Secret(base64.StdEncoding.EncodeToString(make([]byte, 32)))}
}

func adapterKeyring(t *testing.T) *crypto.Keyring {
	t.Helper()
	keys, err := adapterMasterKeys().Decode()
	require.NoError(t, err)
	k, err := newKeyring(keys)
	require.NoError(t, err)
	return k
}

func readyz(t *testing.T, probes health.Probes) int {
	t.Helper()
	rec := httptest.NewRecorder()
	health.Handler(probes).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))
	return rec.Code
}

func TestAdapterProbes_ReadyNeedsDatabaseSchemaAndMasterKey(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t, true)
	pool := db.pool(t, "app")
	assert.Equal(t, http.StatusOK, readyz(t, adapterProbes(pool, adaptersdk.NewStore(pool, adapterKeyring(t)))))
	assert.Equal(t, http.StatusServiceUnavailable, readyz(t, adapterProbes(pool, adaptersdk.NewStore(pool, nil))), "master key is not loaded")

	down, err := pg.New(t.Context(), "postgres://adapter@127.0.0.1:1/adapter?sslmode=disable&connect_timeout=1", "x")
	require.NoError(t, err)
	t.Cleanup(down.Close)
	assert.Equal(t, http.StatusServiceUnavailable, readyz(t, adapterProbes(down, adaptersdk.NewStore(down, adapterKeyring(t)))), "database is unavailable")

	bare := newAdapterDB(t, false).pool(t, "app")
	assert.Equal(t, http.StatusServiceUnavailable, readyz(t, adapterProbes(bare, adaptersdk.NewStore(bare, adapterKeyring(t)))), "schema is not migrated")
}

type slowSource struct {
	adaptersdk.Unimplemented
	started chan struct{}
	release chan struct{}
}

func (s *slowSource) Probe(context.Context, adaptersdk.Call, adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	close(s.started)
	<-s.release
	return adaptersdk.ProbeResult{Provider: adaptersdk.ProbeResultProviderJiraDc, InstanceKey: "jira.example.test"}, nil
}

func writeCertificate(t *testing.T) (certFile, keyFile string, roots *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "adapter.internal"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots = x509.NewCertPool()
	roots.AddCert(parsed)
	return certFile, keyFile, roots
}

func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestServeAdapter_FinishesCallsInFlightOnShutdown(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t, true)
	certFile, keyFile, roots := writeCertificate(t)
	addr := freeAddr(t)
	cfg := config.Adapter{
		DB:         config.DB{URL: db.url("app"), Password: config.Secret(_adapterPasswords["app"]), MaxConns: 2},
		MasterKeys: adapterMasterKeys(),
	}
	srv := config.AdapterServer{HTTPAddr: addr, TLSCertFile: certFile, TLSKeyFile: keyFile, ServiceKey: _adapterKey}
	src := &slowSource{started: make(chan struct{}), release: make(chan struct{})}

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() {
		served <- serveAdapter(ctx, cfg, srv, func(*adaptersdk.SourceClient) adaptersdk.Source { return src }, zerolog.Nop(), 5*time.Second)
	}()

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	require.Eventually(t, func() bool {
		resp, err := client.Get("https://" + addr + "/readyz") //nolint:noctx // a probe of a test server
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 20*time.Millisecond, "adapter becomes ready")

	old := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MaxVersion: tls.VersionTLS11}}} //nolint:gosec // checks that the server refuses it
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+addr+"/livez", http.NoBody)
	require.NoError(t, err)
	resp, err := old.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	assert.Error(t, err, "TLS below 1.2 is refused")

	response := make(chan int, 1)
	go func() {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+addr+"/v1/connections/"+uuid.NewString()+"/probe",
			strings.NewReader(`{"base_url":"https://jira.example.test","scopes":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+_adapterKey)
		req.Header.Set("X-Craftpulse-Organization", uuid.NewString())
		req.Header.Set("X-Craftpulse-Contract-Version", "1")
		req.Header.Set("X-Correlation-Id", "corr-shutdown")
		resp, err := client.Do(req)
		if err != nil {
			response <- 0
			return
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		response <- resp.StatusCode
	}()
	<-src.started

	cancel()
	require.Eventually(t, func() bool {
		d := net.Dialer{Timeout: 100 * time.Millisecond}
		conn, err := d.DialContext(t.Context(), "tcp", addr)
		if err != nil {
			return true
		}
		_ = conn.Close()
		return false
	}, 5*time.Second, 10*time.Millisecond, "new calls are not accepted after SIGTERM")

	close(src.release)
	assert.Equal(t, http.StatusOK, <-response, "the call in flight is finished")
	require.NoError(t, <-served)
}
