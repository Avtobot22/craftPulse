package app

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/adapter/gitlab"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_gitLabToken  = "glpat-fixture-74326"
	_gitLabSecret = `{"token":"` + _gitLabToken + `"}`
	_glTime       = "2026-11-21T10:00:00+07:00"
	_glNew        = "2026-11-21T10:01:00+07:00"
	_glReady      = "2026-11-21T10:02:00+07:00"
	_glHead       = "1111111111111111111111111111111111111111"
	_glBase       = "2222222222222222222222222222222222222222"
	_glActor      = `{"id":2,"username":"reporter","name":"Reporter","state":"active"}`
)

type gitLabFixture struct {
	brokenHistory          atomic.Bool
	listStatus             atomic.Int32
	moveMR                 atomic.Bool
	releaseStalledResponse <-chan struct{}
}

func (f *gitLabFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != _gitLabToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	const p = "/api/v4/projects/7"
	const other = "/api/v4/projects/8"
	const mr = p + "/merge_requests/1"
	const nextMR = p + "/merge_requests/2"
	const otherMR = other + "/merge_requests/1"
	body := "[]"
	switch r.URL.Path {
	case "/api/v4/version":
		body = `{"version":"19.5.5","enterprise":false}`
	case "/api/v4/user", "/api/v4/users/2":
		body = _glActor
	case "/api/v4/users/9":
		body = `{"id":9,"username":"project_7_bot_fixture","name":"Integration bot","state":"active","public_email":"bot@example.test","bot":true}`
	case "/api/v4/users":
		body = "[" + _glActor + "]"
	case p, other:
		projectID := strings.TrimPrefix(r.URL.Path, "/api/v4/projects/")
		body = `{"id":` + projectID + `,"name":"Project","path_with_namespace":"demo/project-` + projectID + `","default_branch":"main","visibility":"private","web_url":"https://example.com/demo/project-` + projectID + `"}`
	case p + "/members/all/2", other + "/members/all/2":
		body = `{"id":2,"username":"reporter","name":"Reporter","state":"active","access_level":20,"membership_state":"active"}`
	case p + "/members/all/4":
		body = `{"id":4,"state":"blocked","access_level":40}`
	case p + "/members/all":
		body = `[` + _glActor + `,{"id":9,"username":"project_7_bot_fixture","name":"Integration bot","state":"active"}]`
	case p + "/merge_requests":
		if status := f.listStatus.Load(); status != 0 {
			if status < 0 {
				// Keep the response pending until the client has observed the timeout.
				<-f.releaseStalledResponse
				return
			}
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(int(status))
			return
		}
		secondTime := _glTime
		if f.moveMR.Load() {
			secondTime = _glNew
		}
		if r.URL.Query().Get("page") == "2" {
			body = `[` + glMR("7", "2", "20", secondTime) + `]`
		} else {
			w.Header().Set("X-Next-Page", "2")
			body = `[` + glMR("7", "1", "10", _glTime) + `]`
		}
	case other + "/merge_requests":
		body = `[` + glMR("8", "1", "30", _glTime) + `]`
	case mr:
		body = glMR("7", "1", "10", _glTime)
	case nextMR:
		at := _glTime
		if f.moveMR.Load() {
			at = _glNew
		}
		body = glMR("7", "2", "20", at)
	case otherMR:
		body = glMR("8", "1", "30", _glTime)
	case mr + "/discussions":
		if r.URL.Query().Get("page") == "2" {
			if f.brokenHistory.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			body = `[{"notes":[{"id":102,"body":"pipeline result","created_at":"` + _glReady + `","system":false,"author":{"id":9,"username":"project_7_bot_fixture","name":"Integration bot","state":"active"}}]}]`
		} else {
			w.Header().Set("X-Next-Page", "2")
			body = `[{"notes":[{"id":101,"body":"marked this merge request as **ready**","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `},{"id":103,"body":"changed the description","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `},{"id":105,"body":"added 1 commit\n\n[Compare with previous version](/demo/-/merge_requests/1/diffs?diff_id=302)","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `},{"id":106,"body":"requested review from @reporter","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `},{"id":107,"body":"removed review request for @reporter","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `},{"id":108,"body":"approved this merge request","created_at":"` + _glReady + `","system":true,"author":` + _glActor + `}]}]`
		}
	case mr + "/resource_state_events":
		if r.URL.Query().Get("page") == "2" {
			body = `[{"id":202,"state":"reopened","created_at":"` + _glNew + `","user":` + _glActor + `}]`
		} else {
			w.Header().Set("Link", "<https://example.com"+mr+"/resource_state_events?page=2>; rel=\"next\"")
			body = `[{"id":201,"state":"closed","created_at":"` + _glTime + `","user":` + _glActor + `}]`
		}
	case mr + "/versions":
		if r.URL.Query().Get("page") == "2" {
			body = `[{"id":301,"merge_request_id":10,"head_commit_sha":"` + _glBase + `","base_commit_sha":"` + _glBase + `","start_commit_sha":"` + _glBase + `","created_at":"` + _glTime + `"}]`
		} else {
			w.Header().Set("X-Next-Page", "2")
			body = `[{"id":302,"merge_request_id":10,"head_commit_sha":"` + _glHead + `","base_commit_sha":"` + _glBase + `","start_commit_sha":"` + _glBase + `","created_at":"` + _glNew + `"}]`
		}
	case nextMR + "/discussions":
		body = `[{"notes":[{"id":104,"body":"marked this merge request as **ready**","created_at":null,"system":true,"author":` + _glActor + `}]}]`
	case mr + "/closes_issues", nextMR + "/resource_state_events", nextMR + "/versions",
		otherMR + "/discussions", otherMR + "/resource_state_events", otherMR + "/versions":
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = io.WriteString(w, body)
}

func glMR(project, iid, globalID, updated string) string {
	return `{"id":` + globalID + `,"iid":` + iid + `,"project_id":` + project + `,"title":"Work","state":"opened","draft":false,
	"sha":"` + _glHead + `","diff_refs":{"head_sha":"` + _glHead + `","base_sha":"` + _glBase + `"},"source_branch":"topic","target_branch":"main",
	"author":` + _glActor + `,"reviewers":[` + _glActor + `],"created_at":"` + _glTime + `","updated_at":"` + updated + `"}`
}

type gitLabTestEnv struct {
	target  contracttest.Target
	logs    *lockedBuffer
	fixture *gitLabFixture
}

func newGitLabTestEnv(t *testing.T) gitLabTestEnv {
	t.Helper()
	fixture := &gitLabFixture{}
	env := newGitLabTestEnvWithSource(t, fixture)
	env.fixture = fixture
	return env
}

func newGitLabTestEnvWithSource(t *testing.T, handler http.Handler) gitLabTestEnv {
	t.Helper()
	upstream := httptest.NewTLSServer(handler)
	t.Cleanup(upstream.Close)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		RootCAs: roots, Resolver: mapResolver{"example.com": _gitPublic}, Timeout: 3 * time.Second,
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	db := newAdapterDB(t, true)
	logs := &lockedBuffer{}
	h, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: gitlab.New(client), Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)),
		ServiceKeys: []string{_adapterKey}, Log: zerolog.New(logs), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "7"}
	areaKind := adaptersdk.AccessItemAreaKindRepository
	return gitLabTestEnv{logs: logs, target: contracttest.Target{
		BaseURL: srv.URL, ServiceKey: _adapterKey, Organization: uuid.NewString(), Connection: uuid.NewString(), Secret: _gitLabSecret,
		Probe:    adaptersdk.ProbeInput{BaseUrl: "https://example.com/", Scopes: []adaptersdk.ScopeRef{scope}},
		Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdMergeRequests, Scope: scope, Limit: 1, ObjectKeys: []adaptersdk.ObjectKey{}},
		Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdMergeRequests, Scope: scope, Limit: 1},
		CheckAccess: &adaptersdk.CheckAccessInput{UserId: uuid.New(), AccountNativeId: "2", Items: []adaptersdk.AccessItem{
			{ItemKind: adaptersdk.AccessItemItemKindArea, AreaKind: &areaKind, NativeId: "7"},
		}},
	}}
}

func (e gitLabTestEnv) call(t *testing.T, method, operation string, input any) (status int, response []byte) {
	t.Helper()
	body, err := json.Marshal(input)
	require.NoError(t, err)
	target := e.target
	req, err := http.NewRequestWithContext(t.Context(), method, target.BaseURL+"/v1/connections/"+target.Connection+"/"+operation, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+target.ServiceKey)
	req.Header.Set("X-Craftpulse-Organization", target.Organization)
	req.Header.Set("X-Craftpulse-Contract-Version", adaptersdk.CurrentContractVersion)
	req.Header.Set("X-Correlation-Id", "gitlab-fixture-"+operation)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, out
}

func (e gitLabTestEnv) probe(t *testing.T) adaptersdk.ProbeResult {
	t.Helper()
	status, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{Purpose: adaptersdk.CredentialsInputPurposeCollect, Secret: &e.target.Secret})
	require.Equal(t, http.StatusNoContent, status, string(body))
	status, body = e.call(t, http.MethodPost, "probe", e.target.Probe)
	require.Equal(t, http.StatusOK, status, string(body))
	var out adaptersdk.ProbeResult
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

func (e gitLabTestEnv) fetch(t *testing.T, in adaptersdk.FetchInput) (*envelope.Package, *envelope.Result) {
	t.Helper()
	status, body := e.call(t, http.MethodPost, "fetch", in)
	require.Equal(t, http.StatusOK, status, string(body))
	result := envelope.CheckPage(bytes.NewReader(body), envelope.DefaultLimits)
	require.Nil(t, result.Envelope)
	for _, record := range result.Records {
		require.Nil(t, record.Rejection, "record %d, %s/%s", record.No, record.Kind, record.NativeID)
	}
	p, err := envelope.ReadCleaned(bytes.NewReader(body), envelope.DefaultLimits)
	require.NoError(t, err)
	return p, result
}

func TestContract_GitLabAdapter(t *testing.T) {
	t.Parallel()
	e := newGitLabTestEnv(t)
	contracttest.Run(t, e.target)
	assert.Contains(t, e.logs.String(), "GitLab request")
	assert.NotContains(t, e.logs.String(), _gitLabToken)
}

func TestGitLabAdapterReadsHistoryAndManifest(t *testing.T) {
	t.Parallel()
	e := newGitLabTestEnv(t)
	e.target.Probe.Scopes = append(e.target.Probe.Scopes, adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "99"})
	probe := e.probe(t)
	require.NotNil(t, probe.Edition)
	assert.Equal(t, "CE", *probe.Edition)
	assert.Equal(t, "https://example.com", probe.InstanceKey)
	assert.Equal(t, adaptersdk.ScopeCheckStateMissing, probe.Scopes[1].State)
	assert.NotNil(t, probe.Scopes[1].ReasonCode)
	for _, capability := range probe.Capabilities[:3] {
		assert.Equal(t, adaptersdk.CapabilityStatePartial, capability.State)
	}
	in := e.target.Fetch
	p, result := e.fetch(t, in)
	assert.Equal(t, "10", p.Manifest.Selection.NextCursor.NativeId)
	kinds := map[string][]string{}
	for _, r := range result.Records {
		kinds[r.Kind] = append(kinds[r.Kind], r.NativeID)
		rec, ok, err := p.Record(r.No)
		require.NoError(t, err)
		require.True(t, ok)
		switch r.NativeID {
		case "note/101":
			require.NotNil(t, rec.EventTime)
			assert.Equal(t, _glReady, rec.EventTime.Format(time.RFC3339))
			assert.JSONEq(t, `{"mr_native_id":"10","event_kind":"ready","actor_account_native_id":"2","system_note":true,"event_time_unknown":false}`, string(rec.Payload))
		case "version/302":
			require.NotNil(t, rec.EventTime)
			assert.Equal(t, _glReady, rec.EventTime.Format(time.RFC3339))
			assert.Contains(t, string(rec.Payload), `"actor_account_native_id":"2"`)
		case "note/102":
			assert.Contains(t, string(rec.Payload), `"actor_account_native_id":"9"`)
		}
	}
	assert.Equal(t, []string{"10"}, kinds["merge_request"])
	assert.ElementsMatch(t, []string{"301", "302"}, kinds["mr_revision"])
	assert.ElementsMatch(t, []string{"note/101", "note/102", "note/103", "note/106/reviewer_assigned/2", "note/107/reviewer_removed/2", "note/108", "state/201", "state/202", "version/302"}, kinds["mr_event"])
	_, repeated := e.fetch(t, in)
	require.Len(t, repeated.Records, len(result.Records))
	for i, r := range result.Records {
		assert.Equal(t, r.Hash, repeated.Records[i].Hash)
		assert.Equal(t, r.SourceRevision, repeated.Records[i].SourceRevision)
	}
	var manifest adaptersdk.ManifestPage
	status, body := e.call(t, http.MethodPost, "manifest", e.target.Manifest)
	require.Equal(t, http.StatusOK, status, string(body))
	require.NoError(t, json.Unmarshal(body, &manifest))
	require.Len(t, manifest.Items, 1)
	assert.Equal(t, "10", manifest.Items[0].NativeId)
	require.NotNil(t, manifest.NextCursor)
	nextManifest := e.target.Manifest
	nextManifest.Cursor = manifest.NextCursor
	status, body = e.call(t, http.MethodPost, "manifest", nextManifest)
	require.Equal(t, http.StatusOK, status, string(body))
	manifest = adaptersdk.ManifestPage{}
	require.NoError(t, json.Unmarshal(body, &manifest))
	require.Len(t, manifest.Items, 1)
	assert.Equal(t, "20", manifest.Items[0].NativeId)
	assert.Nil(t, manifest.NextCursor)

	// The next incremental round initially overlaps the old modification time.
	e.fixture.moveMR.Store(true)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: p.Manifest.Selection.NextCursor.ModifiedAt.Add(-time.Minute)}
	_, overlap := e.fetch(t, in)
	assert.True(t, slices.ContainsFunc(overlap.Records, func(r envelope.RecordResult) bool {
		return r.Kind == "merge_request" && r.NativeID == "10"
	}))
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: p.Manifest.Selection.NextCursor.ModifiedAt, NativeId: "10"}
	last, second := e.fetch(t, in)
	assert.True(t, last.Manifest.Selection.End)
	require.Len(t, last.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "20", last.Manifest.Selection.Truncated[0].NativeId)
	for _, r := range second.Records {
		if r.NativeID == "20" {
			rec, _, err := last.Record(r.No)
			require.NoError(t, err)
			assert.Contains(t, string(rec.Payload), `"updated_at":"`+_glNew+`"`)
		}
		if r.NativeID == "note/104" {
			rec, _, err := last.Record(r.No)
			require.NoError(t, err)
			assert.Nil(t, rec.EventTime)
			assert.Contains(t, string(rec.Payload), `"event_time_unknown":true`)
		}
	}
	in.Cursor = nil
	in.Scope.NativeScopeId = "8"
	_, other := e.fetch(t, in)
	assert.True(t, slices.ContainsFunc(other.Records, func(r envelope.RecordResult) bool {
		return r.Kind == "merge_request" && r.NativeID == "30"
	}))
	in.Scope.NativeScopeId = "7"
	in.SchemaId = adaptersdk.FetchInputSchemaIdAccounts
	accounts, accountResult := e.fetch(t, in)
	require.Len(t, accountResult.Records, 2)
	bot, _, err := accounts.Record(2)
	require.NoError(t, err)
	assert.Equal(t, "9", bot.NativeId)
	assert.Contains(t, string(bot.Payload), `"account_kind":"bot"`)
	assert.Contains(t, string(bot.Payload), `"email":"bot@example.test"`)
}

func TestGitLabAdapterSourceFailures(t *testing.T) {
	t.Parallel()
	releaseStalledResponse := make(chan struct{})
	fixture := &gitLabFixture{releaseStalledResponse: releaseStalledResponse}
	e := newGitLabTestEnvWithSource(t, fixture)
	t.Cleanup(func() { close(releaseStalledResponse) })
	e.probe(t)
	for _, tc := range []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "auth"},
		{http.StatusForbidden, "auth"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusBadGateway, "unavailable"},
	} {
		fixture.listStatus.Store(int32(tc.status))
		status, body := e.call(t, http.MethodPost, "fetch", e.target.Fetch)
		assert.NotEqual(t, http.StatusOK, status)
		assert.Contains(t, string(body), `"code":"`+tc.code+`"`)
		if tc.status == http.StatusTooManyRequests {
			assert.NotContains(t, string(body), `"retry_at":null`)
		}
	}
	fixture.listStatus.Store(-1)
	status, body := e.call(t, http.MethodPost, "fetch", e.target.Fetch)
	assert.Equal(t, http.StatusGatewayTimeout, status)
	assert.Contains(t, string(body), `"code":"timeout"`)

	fixture.listStatus.Store(0)
	fixture.brokenHistory.Store(true)
	status, body = e.call(t, http.MethodPost, "fetch", e.target.Fetch)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, string(body), `"code":"unavailable"`)

	access := *e.target.CheckAccess
	access.AccountNativeId = "4"
	status, body = e.call(t, http.MethodPost, "check-access", access)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"decision":"denied"`)
}
