package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/adapter/github"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const _githubToken = "fixture-github-secret"

//go:embed testdata/github/responses.json
var githubResponses []byte

type githubFixture struct {
	responses map[string]json.RawMessage
	failure   atomic.Int32
	moved     atomic.Bool
}

func (f *githubFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+_githubToken || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	if path == "/repos/demo/project-7/pulls" && r.URL.Query().Get("per_page") != "1" {
		switch failure := f.failure.Load(); {
		case failure == -1:
			<-r.Context().Done()
			return
		case failure == http.StatusNotFound && page == 2:
			w.WriteHeader(http.StatusNotFound)
			return
		case failure == http.StatusTooManyRequests:
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"secondary rate limit"}`)
			return
		case failure == 499:
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
			w.WriteHeader(http.StatusForbidden)
			return
		case failure != 0 && failure != http.StatusNotFound:
			w.WriteHeader(int(failure))
			return
		}
	}
	key := path
	if _, ok := f.responses[key]; !ok {
		key = path + "?page=" + strconv.Itoa(page)
	}
	body, ok := f.responses[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	next := strconv.Itoa(page + 1)
	if _, ok := f.responses[path+"?page="+next]; ok {
		// GitHub uses a numeric repository alias in some pagination responses.
		alias := strings.Replace(path, "/repos/demo/project-7", "/repositories/7", 1)
		w.Header().Set("Link", "<https://api.github.com"+alias+"?page="+next+">; rel=\"next\"")
	}
	if f.moved.Load() && (path == "/repos/demo/project-7/pulls/2" || path == "/repos/demo/project-7/pulls" && page == 2) {
		body = json.RawMessage(strings.ReplaceAll(string(body), "2026-11-21T10:05:00+03:00", "2026-11-21T10:07:00+03:00"))
	}
	_, _ = w.Write(body)
}

func newGitHubFixture(t *testing.T) (gitLabTestEnv, *githubFixture) {
	t.Helper()
	f := &githubFixture{}
	require.NoError(t, json.Unmarshal(githubResponses, &f.responses))
	ca := newTestCA(t, "GitHub fixtures")
	upstream := httptest.NewUnstartedServer(f)
	upstream.TLS, _ = ca.serverTLS(t, time.Now().Add(time.Hour), "api.github.com")
	upstream.StartTLS()
	t.Cleanup(upstream.Close)
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		RootCAs: ca.pool, Timeout: time.Second, Resolver: mapResolver{"api.github.com": _gitPublic},
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	db := newAdapterDB(t, true)
	logs := &lockedBuffer{}
	handler, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: github.New(client), Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)),
		ServiceKeys: []string{_adapterKey}, Log: zerolog.New(logs), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "7"}
	area := adaptersdk.AccessItemAreaKindRepository
	return gitLabTestEnv{logs: logs, target: contracttest.Target{
		BaseURL: srv.URL, ServiceKey: _adapterKey, Organization: uuid.NewString(), Connection: uuid.NewString(),
		Secret:   `{"token":"` + _githubToken + `"}`,
		Probe:    adaptersdk.ProbeInput{BaseUrl: "https://github.com", Scopes: []adaptersdk.ScopeRef{scope}},
		Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdMergeRequests, Scope: scope, Limit: 1, ObjectKeys: []adaptersdk.ObjectKey{}},
		Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdMergeRequests, Scope: scope, Limit: 1},
		CheckAccess: &adaptersdk.CheckAccessInput{UserId: uuid.New(), AccountNativeId: "201", Items: []adaptersdk.AccessItem{
			{ItemKind: adaptersdk.AccessItemItemKindArea, AreaKind: &area, NativeId: "7"},
		}},
	}}, f
}

func TestContract_GitHubAdapter(t *testing.T) {
	t.Parallel()
	e, _ := newGitHubFixture(t)
	contracttest.Run(t, e.target)
	assert.Contains(t, e.logs.String(), "GitHub request")
	assert.NotContains(t, e.logs.String(), _githubToken)
}

func TestGitHubAdapterReadsHistoryAndManifest(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	probe := e.probe(t)
	require.NotNil(t, probe.Edition)
	assert.Equal(t, "GitHub.com", *probe.Edition)
	assert.Nil(t, probe.AccessMode)
	for _, c := range probe.Capabilities {
		assert.Equal(t, adaptersdk.CapabilityStateVerified, c.State)
	}
	in := e.target.Fetch
	p, result := e.fetch(t, in)
	require.NotNil(t, p.Manifest.Selection.NextCursor)
	assert.Equal(t, "501", p.Manifest.Selection.NextCursor.NativeId)
	_, repeated := e.fetch(t, in)
	kinds := map[string]int{}
	for i, row := range result.Records {
		kinds[row.Kind]++
		assert.Equal(t, row.SourceRevision, repeated.Records[i].SourceRevision)
		assert.Equal(t, row.Hash, repeated.Records[i].Hash)
		record, ok, err := p.Record(row.No)
		require.NoError(t, err)
		require.True(t, ok)
		if record.NativeId == "timeline/302" {
			require.NotNil(t, record.EventTime)
			assert.Equal(t, "2026-11-21T10:02:00+03:00", record.EventTime.Format(time.RFC3339))
		}
		if record.NativeId == "207" {
			assert.Contains(t, string(record.Payload), `"account_kind":"bot"`)
		}
	}
	assert.Equal(t, 2, kinds["mr_revision"])
	assert.Equal(t, 113, kinds["mr_event"])
	m := e.target.Manifest
	status, body := e.call(t, http.MethodPost, "manifest", m)
	require.Equal(t, http.StatusOK, status)
	var mp adaptersdk.ManifestPage
	require.NoError(t, json.Unmarshal(body, &mp))
	require.Len(t, mp.Items, 1)
	assert.Equal(t, "501", mp.Items[0].NativeId)
	m.Cursor = mp.NextCursor
	require.NotNil(t, m.Cursor)
	status, body = e.call(t, http.MethodPost, "manifest", m)
	require.Equal(t, http.StatusOK, status)
	require.NoError(t, json.Unmarshal(body, &mp))
	require.Len(t, mp.Items, 1)
	assert.Equal(t, "502", mp.Items[0].NativeId)

	in.Cursor = &adaptersdk.Cursor{ModifiedAt: p.Manifest.Selection.NextCursor.ModifiedAt, NativeId: "501"}
	end := in.Cursor.ModifiedAt.Add(time.Minute)
	in.Period.To = &end
	f.moved.Store(true)
	_, moved := e.fetch(t, in)
	for _, r := range moved.Records {
		assert.NotEqual(t, "merge_request", r.Kind)
	}
	in.Period.To = nil
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: in.Cursor.ModifiedAt.Add(-10 * time.Minute)}
	p, _ = e.fetch(t, in)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: p.Manifest.Selection.NextCursor.ModifiedAt, NativeId: "501"}
	p, second := e.fetch(t, in)
	require.Len(t, p.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "502", p.Manifest.Selection.Truncated[0].NativeId)
	for _, r := range second.Records {
		if r.Kind != "mr_event" {
			continue
		}
		rec, ok, err := p.Record(r.No)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Nil(t, rec.EventTime)
		assert.Contains(t, string(rec.Payload), `"event_time_unknown":true`)
	}

	in.Cursor = nil
	in.Scope.NativeScopeId = "8"
	_, other := e.fetch(t, in)
	for _, r := range other.Records {
		if r.Kind == "merge_request" {
			assert.Equal(t, "601", r.NativeID)
		}
	}
	in.Scope.NativeScopeId = "7"
	in.SchemaId = adaptersdk.FetchInputSchemaIdAccounts
	p, accounts := e.fetch(t, in)
	assert.Len(t, accounts.Records, 2)
	rec, ok, err := p.Record(accounts.Records[0].No)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, string(rec.Payload), "reporter@example.test")
}

func TestGitHubAdapterSourceFailures(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.target.Probe.Scopes = append(e.target.Probe.Scopes, adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "99"})
	probe := e.probe(t)
	require.Len(t, probe.Scopes, 2)
	assert.Equal(t, adaptersdk.ScopeCheckStateMissing, probe.Scopes[1].State)
	assert.NotNil(t, probe.Scopes[1].ReasonCode)
	for _, c := range probe.Capabilities {
		assert.Equal(t, adaptersdk.CapabilityStatePartial, c.State)
	}
	in := e.target.Probe
	in.BaseUrl = "https://ghe.example.com"
	status, body := e.call(t, http.MethodPost, "probe", in)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, string(body), "unsupported_edition")
	for _, tc := range []struct {
		status int32
		code   adaptersdk.AdapterErrorCode
	}{
		{401, adaptersdk.AdapterErrorCodeAuth}, {403, adaptersdk.AdapterErrorCodeAuth},
		{429, adaptersdk.AdapterErrorCodeRateLimited}, {499, adaptersdk.AdapterErrorCodeRateLimited},
		{500, adaptersdk.AdapterErrorCodeUnavailable}, {404, adaptersdk.AdapterErrorCodeUnavailable},
		{-1, adaptersdk.AdapterErrorCodeTimeout},
	} {
		f.failure.Store(tc.status)
		status, body := e.call(t, http.MethodPost, "fetch", e.target.Fetch)
		require.GreaterOrEqual(t, status, 400)
		var got adaptersdk.AdapterError
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, tc.code, got.Code)
		if tc.code == adaptersdk.AdapterErrorCodeRateLimited {
			assert.NotNil(t, got.RetryAt)
		}
	}
}
