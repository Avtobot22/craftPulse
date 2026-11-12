package probe_test

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/probe"
)

func discovery(t *testing.T, issuerOverride string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		issuer := srv.URL
		if issuerOverride != "" {
			issuer = issuerOverride
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": issuer, "authorization_endpoint": issuer + "/oauth/authorize",
			"token_endpoint": issuer + "/oauth/token", "jwks_uri": issuer + "/oauth/discovery/keys",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reason(t *testing.T, err error) string {
	t.Helper()
	f, ok := errors.AsType[*entity.CheckFailure](err)
	require.True(t, ok, "want *entity.CheckFailure, got %v", err)
	return f.Reason
}

func TestProbeIssuer_Valid(t *testing.T) {
	t.Parallel()
	srv := discovery(t, "")

	assert.NoError(t, probe.New(srv.Client()).ProbeIssuer(t.Context(), srv.URL))
}

func TestProbeIssuer_Classifies(t *testing.T) {
	t.Parallel()
	valid := discovery(t, "")
	wrongIssuer := discovery(t, "https://gitlab.example")
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := "https://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	cases := map[string]struct {
		client *http.Client
		issuer string
		want   string
	}{
		"untrusted certificate": {&http.Client{Timeout: 2 * time.Second}, valid.URL, entity.ReasonCertificate},
		"nothing listens":       {valid.Client(), closed, entity.ReasonAddress},
		"unknown host":          {valid.Client(), "https://idp.invalid", entity.ReasonAddress},
		"no answer":             {&http.Client{Timeout: 200 * time.Millisecond}, slow.URL, entity.ReasonNetworkScope},
		"other issuer":          {wrongIssuer.Client(), wrongIssuer.URL, entity.ReasonRegistration},
		"no discovery":          {valid.Client(), valid.URL + "/missing", entity.ReasonRegistration},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, reason(t, probe.New(tc.client).ProbeIssuer(t.Context(), tc.issuer)))
		})
	}
}

func TestProbeSite(t *testing.T) {
	t.Parallel()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(ok.Close)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(broken.Close)
	p := probe.New(&http.Client{Timeout: 2 * time.Second})

	assert.NoError(t, p.ProbeSite(t.Context(), ok.URL))
	assert.Equal(t, entity.ReasonUnavailable, reason(t, p.ProbeSite(t.Context(), broken.URL)))
}
