package app

import (
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const _sonarUserToken = "fixture-sonar-user-secret"

type sonarAccessFixture struct {
	*sonarFixture
	identityStatus atomic.Int32
	projectStatus  atomic.Int32
	projectReads   atomic.Int32
}

func (f *sonarAccessFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Header.Get("Authorization") {
	case "Bearer " + _sonarToken:
		f.sonarFixture.ServeHTTP(w, r)
	case "Bearer " + _sonarUserToken:
		f.serveUser(w, r)
	default:
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (f *sonarAccessFixture) serveUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/users/current":
		switch status := f.identityStatus.Load(); status {
		case 0:
			_, _ = io.WriteString(w, `{"isLoggedIn":true,"login":"viewer"}`)
		case http.StatusOK:
			// Some credentials can reach the endpoint without authenticating a user.
			_, _ = io.WriteString(w, `{"isLoggedIn":false,"login":"viewer"}`)
		default:
			w.WriteHeader(int(status))
		}
	case "/api/components/show":
		f.projectReads.Add(1)
		switch r.URL.Query().Get("component") {
		case _sonarProject:
			switch status := f.projectStatus.Load(); status {
			case 0:
				_, _ = io.WriteString(w, `{"component":{"key":"orders:main","qualifier":"TRK"}}`)
			case http.StatusOK:
				_, _ = io.WriteString(w, `{"component":{"key":"orders:main","qualifier":"FIL"}}`)
			default:
				w.WriteHeader(int(status))
			}
		case "slow":
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newSonarAccessFixture(t *testing.T) (gitLabTestEnv, *sonarAccessFixture) {
	t.Helper()
	f := &sonarAccessFixture{sonarFixture: &sonarFixture{}}
	require.NoError(t, json.Unmarshal(sonarResponses, &f.responses))
	e := newSonarTestEnv(t, f, _sonarProject)
	require.Equal(t, new(adaptersdk.ProbeResultAccessModeUserToken), e.probe(t).AccessMode)
	in := e.target.CheckAccess
	status, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{
		Purpose: adaptersdk.CredentialsInputPurposeUserToken, UserId: &in.UserId, Secret: new(_sonarUserToken),
	})
	require.Equal(t, http.StatusNoContent, status, string(body))
	t.Cleanup(func() {
		assert.NotContains(t, e.logs.String(), _sonarUserToken)
		assert.NotContains(t, e.logs.String(), _sonarToken)
	})
	return e, f
}

func checkSonarAccess(t *testing.T, e gitLabTestEnv, in adaptersdk.CheckAccessInput) []adaptersdk.AccessDecision {
	t.Helper()
	start := time.Now()
	status, body := e.call(t, http.MethodPost, "check-access", in)
	require.Equal(t, http.StatusOK, status, string(body))
	var out adaptersdk.CheckAccessResult
	require.NoError(t, json.Unmarshal(body, &out))
	require.Len(t, out.Decisions, len(in.Items))
	assert.Equal(t, adaptersdk.CheckAccessResultModeUserToken, out.Mode)
	assert.Equal(t, new(60), out.MaxAgeSeconds)
	assert.False(t, out.CheckedAt.Before(start))
	assert.False(t, out.CheckedAt.After(time.Now()))
	for _, d := range out.Decisions {
		assert.False(t, d.CheckedAt.Before(start))
		assert.False(t, d.CheckedAt.After(out.CheckedAt))
	}
	return out.Decisions
}

func TestSonarQubeCheckAccess_BrowseAndRevocation(t *testing.T) {
	t.Parallel()
	e, f := newSonarAccessFixture(t)
	in := *e.target.CheckAccess
	in.AccountNativeId = "viewer"
	// A successful source check reaches the revocation gate.
	got := checkSonarAccess(t, e, in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, got[0].Decision)
	assert.Equal(t, new("sonarqube_access_revocation_unverified"), got[0].ReasonCode)

	f.projectStatus.Store(http.StatusForbidden)
	got = checkSonarAccess(t, e, in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionDenied, got[0].Decision)
	assert.Equal(t, new("sonarqube_browse_denied"), got[0].ReasonCode)

	f.projectStatus.Store(0)
	got = checkSonarAccess(t, e, in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, got[0].Decision)
	assert.Equal(t, new("sonarqube_access_revocation_unverified"), got[0].ReasonCode)
	assert.EqualValues(t, 3, f.projectReads.Load())
}

func TestSonarQubeCheckAccess_Unknown(t *testing.T) {
	t.Parallel()
	e, f := newSonarAccessFixture(t)
	tests := []struct {
		name           string
		account        string
		identityStatus int32
		projectStatus  int32
		withoutToken   bool
		namespace      string
		projectReads   int32
		reason         string
	}{
		{name: "missing user token", withoutToken: true, reason: "sonarqube_user_token_missing"},
		{name: "mismatched account", account: "other-viewer", reason: "sonarqube_account_unverified"},
		{name: "anonymous identity", identityStatus: http.StatusOK, reason: "sonarqube_account_unverified"},
		{name: "rejected token", identityStatus: http.StatusUnauthorized, reason: "token_expired"},
		{name: "unexpected namespace", namespace: "other", reason: "sonarqube_access_item_unsupported"},
		{name: "unavailable project", projectStatus: http.StatusServiceUnavailable, projectReads: 1, reason: "sonarqube_project_unverified"},
		{name: "missing project", projectStatus: http.StatusNotFound, projectReads: 1, reason: "sonarqube_project_unverified"},
		{name: "wrong component type", projectStatus: http.StatusOK, projectReads: 1, reason: "sonarqube_project_unverified"},
		{name: "expired during browse", projectStatus: http.StatusUnauthorized, projectReads: 1, reason: "token_expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.identityStatus.Store(tt.identityStatus)
			f.projectStatus.Store(tt.projectStatus)
			f.projectReads.Store(0)
			in := *e.target.CheckAccess
			in.AccountNativeId = tt.account
			if tt.withoutToken {
				in.UserId = uuid.New()
			}
			item := in.Items[0]
			item.NativeNs = tt.namespace
			in.Items = []adaptersdk.AccessItem{item}
			got := checkSonarAccess(t, e, in)
			assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, got[0].Decision)
			assert.Equal(t, new(tt.reason), got[0].ReasonCode)
			assert.Equal(t, tt.projectReads, f.projectReads.Load())
		})
	}
}

func TestSonarQubeCheckAccess_Timeout(t *testing.T) {
	t.Parallel()
	e, f := newSonarAccessFixture(t)
	f.projectStatus.Store(http.StatusForbidden)
	in := *e.target.CheckAccess
	slow := in.Items[0]
	slow.NativeId = "slow"
	in.Items = append(append([]adaptersdk.AccessItem{}, in.Items...), slow)
	start := time.Now()
	got := checkSonarAccess(t, e, in)
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 2*time.Second)
	assert.Less(t, elapsed, 2800*time.Millisecond)
	assert.EqualValues(t, 2, f.projectReads.Load())
	for _, decision := range got {
		assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, decision.Decision)
		assert.Equal(t, new("sonarqube_access_timeout"), decision.ReasonCode)
	}
}
