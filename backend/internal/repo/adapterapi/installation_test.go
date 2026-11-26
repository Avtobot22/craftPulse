package adapterapi

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type installationTransport func(*http.Request) (*http.Response, error)

func (f installationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckBuiltins(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		version     string
		readyStatus int
		wantError   string
	}{
		{"ready", `{"commit_sha":"release","contract_version":"1"}`, http.StatusOK, ""},
		{"wrong SHA", `{"commit_sha":"older","contract_version":"1"}`, http.StatusOK, "version does not identify"},
		{"no contract", `{"commit_sha":"release"}`, http.StatusOK, "version does not identify"},
		{"not ready", `{"commit_sha":"release","contract_version":"1"}`, http.StatusServiceUnavailable, "/readyz returned HTTP 503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var paths []string
			client := &http.Client{Transport: installationTransport(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.URL.Path)
				switch req.URL.Path {
				case "/version":
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.version))}, nil
				case "/readyz":
					return &http.Response{StatusCode: tc.readyStatus, Body: http.NoBody}, nil
				default:
					return nil, errors.New("unexpected probe path")
				}
			})}
			caller := &Caller{builtin: map[entity.Provider]builtinAdapter{
				"jira_cloud": {url: "https://adapter-jiracloud:8443", http: client},
			}}
			detail, err := caller.CheckBuiltins(t.Context(), "release")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Empty(t, detail)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "jira_cloud contract 1", detail)
			assert.Equal(t, []string{"/version", "/readyz"}, paths)
		})
	}
}

func TestCheckBuiltinsReportsUntrustedCertificate(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: installationTransport(func(*http.Request) (*http.Response, error) {
		return nil, &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	})}
	caller := &Caller{builtin: map[entity.Provider]builtinAdapter{
		"jira_cloud": {url: "https://adapter-jiracloud:8443", http: client},
	}}
	_, err := caller.CheckBuiltins(t.Context(), "release")
	var failure *entity.CheckFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, entity.ReasonCertificate, failure.Reason)
}
