package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	responses           map[string]json.RawMessage
	publication         *githubPublicationFixture
	failure             atomic.Int32
	moved               atomic.Bool
	reorder             atomic.Bool
	firstPage           atomic.Int32
	commentCount        atomic.Int32
	firstComments       atomic.Int32
	commentPageFailure  atomic.Int32
	activityUnavailable atomic.Bool
	activityRefChanged  atomic.Bool
	firstActivity       atomic.Int32
	forkUnavailable     atomic.Bool
	historyScenario     atomic.Int32
	historyFailure      atomic.Pointer[githubHistoryFailure]
	manyAuthors         atomic.Bool
	manyReviewers       atomic.Bool
}

type githubHistoryFailure struct {
	path   string
	status int
}

func (f *githubFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.publication != nil && f.publication.serve(w, r) {
		return
	}
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
	if failure := f.historyFailure.Load(); failure != nil && path == failure.path && page == 1 {
		w.WriteHeader(failure.status)
		return
	}
	switch path {
	case "/repositories/9":
		if f.forkUnavailable.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	case "/repos/demo/project-8/issues/1/comments":
		if count := f.commentCount.Load(); count > 0 {
			f.serveLongComments(w, page, int(count))
			return
		}
	case "/repos/demo/project-8/issues/1/timeline":
		if scenario := f.historyScenario.Load(); scenario != 0 {
			serveGitHubTimeline(w, page, scenario)
			return
		}
	case "/repos/demo/fork/activity":
		f.serveActivity(w, r)
		return
	}
	if (f.manyAuthors.Load() || f.manyReviewers.Load()) && strings.HasPrefix(path, "/user/") {
		if n, err := strconv.Atoi(strings.TrimPrefix(path, "/user/")); err == nil && n >= 1001 && n <= 1100 {
			_, _ = fmt.Fprintf(w, `{"id":%d,"login":"author-%d","name":"Fixture author","type":"User"}`, n, n)
			return
		}
	}
	if path == "/repos/demo/project-7/pulls" && r.URL.Query().Get("per_page") != "1" {
		if f.reorder.Load() {
			f.serveReorderedPulls(w, page)
			return
		}
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
	if f.moved.Load() && (path == "/repos/demo/project-7/pulls/2" || path == "/repos/demo/project-7/pulls" && page == 2 ||
		path == "/repos/demo/project-8/pulls/1") {
		body = json.RawMessage(strings.ReplaceAll(string(body), "2026-11-21T10:05:00+03:00", "2026-11-21T10:07:00+03:00"))
	}
	if path == "/repos/demo/project-8/pulls/1" && f.activityRefChanged.Load() {
		body = json.RawMessage(strings.ReplaceAll(string(body), `"ref": "topic"`, `"ref": "new-topic"`))
	}
	if path == "/repos/demo/project-8/pulls/1" && f.manyReviewers.Load() {
		var reviewers strings.Builder
		reviewers.WriteString(`"requested_reviewers": [`)
		for n := 1001; n <= 1040; n++ {
			_, _ = fmt.Fprintf(&reviewers, `{"id":%d},`, n)
		}
		body = json.RawMessage(strings.Replace(string(body), `"requested_reviewers": [`, reviewers.String(), 1))
	}
	_, _ = w.Write(body)
}

const (
	_longCommentCount        = 4001
	_commentContinuationPage = 10
	_githubReadyTimeline     = 1
	_githubUnchangedHead     = 2
	_githubForcePushes       = 3
)

func serveGitHubTimeline(w http.ResponseWriter, page int, scenario int32) {
	body := `[]`
	switch scenario {
	case _githubReadyTimeline:
		switch page {
		case 1:
			body = `[{"id":301,"event":"converted_to_draft","created_at":"2026-11-21T10:01:00+03:00"}]`
		case 13:
			body = `[{"id":302,"event":"ready_for_review","created_at":"2026-11-21T10:02:00+03:00"}]`
		}
		if page < 25 {
			w.Header().Set("Link", fmt.Sprintf(`<https://api.github.com/repositories/8/issues/1/timeline?page=%d>; rel="next"`, page+1))
		}
	case _githubForcePushes:
		body = `[
			{"id":305,"event":"head_ref_force_pushed","created_at":"2026-11-21T10:01:00+03:00",
			 "commit_id":"3333333333333333333333333333333333333333","base_sha":"2222222222222222222222222222222222222222"},
			{"id":306,"event":"head_ref_force_pushed","created_at":"2026-11-21T10:03:00+03:00"}
		]`
	}
	_, _ = io.WriteString(w, body)
}

func (f *githubFixture) serveLongComments(w http.ResponseWriter, page, count int) {
	if page == 1 {
		f.firstComments.Add(1)
	}
	const perPage = 30
	start, end := (page-1)*perPage+1, min(page*perPage, count)
	var failure int32
	if page == _commentContinuationPage {
		failure = f.commentPageFailure.Load()
	}
	switch {
	case failure >= http.StatusBadRequest:
		w.WriteHeader(int(failure))
		return
	case failure == http.StatusOK:
		w.Header().Set("Link", fmt.Sprintf(`<https://api.github.com/repositories/8/issues/1/comments?page=%d; rel="next"`, page+1))
	case end < count:
		w.Header().Set("Link", fmt.Sprintf(`<https://api.github.com/repositories/8/issues/1/comments?page=%d>; rel="next"`, page+1))
	}
	rows := make([]json.RawMessage, 0, end-start+1)
	for n := start; n <= end; n++ {
		author := 201
		if n == _longCommentCount-1 {
			author = 207
		}
		if f.manyAuthors.Load() {
			author = 1000 + n
		}
		rows = append(rows, json.RawMessage(fmt.Sprintf(`{"id":%d,"user":{"id":%d},"created_at":"2026-11-21T10:04:00+03:00"}`, n, author)))
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func (f *githubFixture) serveActivity(w http.ResponseWriter, r *http.Request) {
	if f.activityUnavailable.Load() {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	query := r.URL.Query()
	ref, token := "refs/heads/topic", "page/2="
	if f.activityRefChanged.Load() {
		ref, token = "refs/heads/new-topic", "moved/page/2="
	}
	if query.Get("ref") != ref || query.Has("activity_type") || query.Get("direction") != "desc" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	scenario := f.historyScenario.Load()
	if scenario == _githubUnchangedHead || scenario == _githubReadyTimeline {
		_, _ = io.WriteString(w, `[{"id":88,"activity_type":"push","ref":"refs/heads/topic","timestamp":"2026-11-21T09:59:00+03:00",
			"before":"5555555555555555555555555555555555555555","after":"1111111111111111111111111111111111111111"}]`)
		return
	}
	key := r.URL.Path
	if after := query.Get("after"); after != "" {
		if after != token {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		key += "?after=page/2="
	} else {
		f.firstActivity.Add(1)
		w.Header().Set("Link", `<https://api.github.com/repositories/9/activity?after=`+url.QueryEscape(token)+`>; rel="next"`)
	}
	body, ok := f.responses[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if ref != "refs/heads/topic" {
		body = json.RawMessage(strings.ReplaceAll(string(body), "refs/heads/topic", ref))
	}
	if scenario == _githubForcePushes {
		body = json.RawMessage(strings.ReplaceAll(string(body), `"activity_type": "push"`, `"activity_type": "force_push"`))
	}
	_, _ = w.Write(body)
}

// After page one is returned, pull 31 jumps to the top and displaces pull 30
// onto page two. The first scan sees pull 30 twice and misses pull 31.
func (f *githubFixture) serveReorderedPulls(w http.ResponseWriter, page int) {
	numbers := make([]int, 0, 60)
	if page != 1 || f.firstPage.Add(1) > 1 {
		numbers = append(numbers, 31)
	}
	for n := 1; n <= 60; n++ {
		if len(numbers) != 0 && numbers[0] == 31 && n == 31 {
			continue
		}
		numbers = append(numbers, n)
	}
	if page == 1 {
		w.Header().Set("Link", `<https://api.github.com/repositories/7/pulls?page=2>; rel="next"`)
	}
	rows := make([]json.RawMessage, 0, 30)
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	for _, n := range numbers[(page-1)*30 : page*30] {
		at := now.Add(-time.Duration(n) * time.Minute)
		if n == 31 {
			at = now
		}
		rows = append(rows, json.RawMessage(fmt.Sprintf(`{"id":%d,"number":%d,"updated_at":%q,"base":{"repo":{"id":7}}}`, 500+n, n, at.Format(time.RFC3339))))
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func newGitHubFixture(t *testing.T) (gitLabTestEnv, *githubFixture) {
	t.Helper()
	return newGitHubFixtureWithPublication(t, nil)
}

func newGitHubFixtureWithPublication(t *testing.T, publication *githubPublicationFixture) (gitLabTestEnv, *githubFixture) {
	t.Helper()
	f := &githubFixture{publication: publication}
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

func assertGitHubProbeCapabilities(t *testing.T, probe adaptersdk.ProbeResult, readState adaptersdk.CapabilityState) {
	t.Helper()
	require.Len(t, probe.Capabilities, 5)
	for _, c := range probe.Capabilities {
		switch c.Capability {
		case adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityCapabilityWebhook:
			assert.Equal(t, adaptersdk.CapabilityStateMissing, c.State)
			assert.NotNil(t, c.ReasonCode)
		default:
			assert.Equal(t, readState, c.State)
		}
	}
}

func TestGitHubAdapterReadsHistoryAndManifest(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	probe := e.probe(t)
	require.NotNil(t, probe.Edition)
	assert.Equal(t, "GitHub.com", *probe.Edition)
	assert.Nil(t, probe.AccessMode)
	assertGitHubProbeCapabilities(t, probe, adaptersdk.CapabilityStateVerified)
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
	assert.Equal(t, 1, kinds["mr_revision"])
	assert.Equal(t, 112, kinds["mr_event"])
	require.Len(t, p.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "501", p.Manifest.Selection.Truncated[0].NativeId, "historical base SHA is missing")
	for _, row := range result.Records {
		if row.Kind != "mr_revision" {
			continue
		}
		rec, ok, err := p.Record(row.No)
		require.NoError(t, err)
		require.True(t, ok)
		var revision envelope.MrRevisionV1
		require.NoError(t, json.Unmarshal(rec.Payload, &revision))
		assert.Equal(t, "timeline/305", rec.NativeId)
		assert.Equal(t, strings.Repeat("3", 40), revision.HeadSha)
		assert.Nil(t, revision.BaseSha)
	}
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
	assertGitHubProbeCapabilities(t, probe, adaptersdk.CapabilityStatePartial)
	in := e.target.Probe
	in.BaseUrl = "https://ghe.example.com"
	status, body := e.call(t, http.MethodPost, "probe", in)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, string(body), "unsupported_edition")
	for _, tc := range []struct {
		status int32
		code   adaptersdk.AdapterErrorCode
	}{
		{401, adaptersdk.AdapterErrorCodeAuth},
		{403, adaptersdk.AdapterErrorCodeAuth},
		{429, adaptersdk.AdapterErrorCodeRateLimited},
		{499, adaptersdk.AdapterErrorCodeRateLimited},
		{500, adaptersdk.AdapterErrorCodeUnavailable},
		{404, adaptersdk.AdapterErrorCodeUnavailable},
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

func TestGitHubAdapterRejectsFetchWhenPullHistoryAccessIsLost(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	for _, tc := range []struct {
		name string
		path string
	}{
		{"timeline", "/repos/demo/project-7/issues/1/timeline"},
		{"reviews", "/repos/demo/project-7/pulls/1/reviews"},
	} {
		for _, upstreamStatus := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, upstreamStatus), func(t *testing.T) {
				f.historyFailure.Store(&githubHistoryFailure{path: tc.path, status: upstreamStatus})
				status, body := e.call(t, http.MethodPost, "fetch", e.target.Fetch)
				require.Equal(t, http.StatusForbidden, status)
				var failure adaptersdk.AdapterError
				require.NoError(t, json.Unmarshal(body, &failure))
				assert.Equal(t, adaptersdk.AdapterErrorCodeAuth, failure.Code)
			})
		}
	}
}

func TestGitHubAdapterReconcilesMovedManifestPR(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.reorder.Store(true)
	in := e.target.Manifest
	in.Limit = 100
	status, body := e.call(t, http.MethodPost, "manifest", in)
	require.Equal(t, http.StatusOK, status)
	var got adaptersdk.ManifestPage
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Nil(t, got.NextCursor)
	assert.GreaterOrEqual(t, f.firstPage.Load(), int32(3))
	want := make([]string, 0, 60)
	for n := 1; n <= 60; n++ {
		want = append(want, strconv.Itoa(500+n))
	}
	ids := make([]string, 0, len(got.Items))
	for _, p := range got.Items {
		ids = append(ids, p.NativeId)
	}
	assert.ElementsMatch(t, want, ids)
	require.NotEmpty(t, got.Items)
	assert.Equal(t, "531", got.Items[len(got.Items)-1].NativeId)
}

func TestGitHubAdapterReadsOrdinaryPushesAndDismissedReview(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	revisions := make(map[string]string)
	for i := range 2 {
		if i > 0 {
			in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
			f.moved.Store(true)
		}
		p, records := githubRecords(t, e, in)
		require.Len(t, p.Manifest.Selection.Truncated, 1)
		assert.Equal(t, "601", p.Manifest.Selection.Truncated[0].NativeId)
		assert.Equal(t, envelope.ManifestCompletenessPartial, p.Manifest.Completeness)
		var heads, pushes []string
		var dismissed bool
		for _, rec := range records {
			if rec.NativeId == "reviews/408" {
				dismissed = assert.Contains(t, string(rec.Payload), `"review_state":"dismissed"`)
			}
			if !strings.HasPrefix(rec.NativeId, "activity/9/") {
				continue
			}
			key := string(rec.RecordKind) + "/" + rec.NativeId
			hash, err := envelope.ContentHash(&rec)
			require.NoError(t, err)
			require.NotNil(t, rec.SourceRevision)
			assert.Equal(t, hash, *rec.SourceRevision)
			if i == 0 {
				revisions[key] = hash
			} else {
				assert.Equal(t, revisions[key], hash)
			}
			if rec.RecordKind == "mr_event" {
				var event envelope.MrEventV2
				require.NoError(t, json.Unmarshal(rec.Payload, &event))
				require.NotNil(t, rec.EventTime)
				require.NotNil(t, event.HeadSha)
				assert.Equal(t, envelope.MrEventV2EventKindHeadPushed, event.EventKind)
				pushes = append(pushes, *event.HeadSha+" "+rec.EventTime.Format(time.RFC3339))
			} else {
				var revision envelope.MrRevisionV1
				require.NoError(t, json.Unmarshal(rec.Payload, &revision))
				assert.Nil(t, revision.BaseSha)
				heads = append(heads, revision.HeadSha+" "+revision.CreatedAt.Format(time.RFC3339))
			}
		}
		want := []string{strings.Repeat("1", 40) + " 2026-11-21T10:03:00+03:00", strings.Repeat("3", 40) + " 2026-11-21T10:01:00+03:00"}
		assert.ElementsMatch(t, want, heads)
		assert.ElementsMatch(t, want, pushes)
		assert.True(t, dismissed)
	}

	f.activityUnavailable.Store(true)
	p, records := githubRecords(t, e, in)
	require.Len(t, p.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "601", p.Manifest.Selection.Truncated[0].NativeId)
	pr, ok := records["merge_request/601"]
	require.True(t, ok)
	assert.Contains(t, string(pr.Payload), strings.Repeat("1", 40))
	assert.Contains(t, string(pr.Payload), strings.Repeat("2", 40))
	for _, rec := range records {
		assert.NotEqual(t, "mr_revision", string(rec.RecordKind))
	}
}

func TestGitHubAdapterContinuesLongHistory(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.commentCount.Store(_longCommentCount)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	p, first := githubRecords(t, e, in)
	require.Len(t, p.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "601", p.Manifest.Selection.Truncated[0].NativeId)
	assert.True(t, p.Manifest.Selection.End)
	assert.Nil(t, p.Manifest.Selection.NextCursor)
	assert.NotContains(t, first, "mr_event/comments/4001")

	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
	seen := make(map[string]struct{}, _longCommentCount)
	for page := 1; ; page++ {
		require.LessOrEqual(t, page, 25, "history cursor did not reach the end")
		p, records := githubRecords(t, e, in)
		for key := range records {
			if strings.HasPrefix(key, "mr_event/comments/") {
				require.NotContains(t, seen, key)
				seen[key] = struct{}{}
			}
		}
		if p.Manifest.Selection.End {
			assert.Nil(t, p.Manifest.Selection.NextCursor)
			break
		}
		next := p.Manifest.Selection.NextCursor
		require.NotNil(t, next)
		assert.Contains(t, next.NativeId, "history/601/")
		in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
		if page == 1 {
			for _, tc := range []struct {
				status int32
				code   adaptersdk.AdapterErrorCode
			}{
				{http.StatusForbidden, adaptersdk.AdapterErrorCodeAuth},
				{http.StatusNotFound, adaptersdk.AdapterErrorCodeUnavailable},
				{http.StatusOK, adaptersdk.AdapterErrorCodeUnavailable}, // Malformed Link.
			} {
				f.commentPageFailure.Store(tc.status)
				status, body := e.call(t, http.MethodPost, "fetch", in)
				require.GreaterOrEqual(t, status, 400)
				var failure adaptersdk.AdapterError
				require.NoError(t, json.Unmarshal(body, &failure))
				require.Equal(t, tc.code, failure.Code)
			}
			f.commentPageFailure.Store(0)
		}
	}
	require.Len(t, seen, _longCommentCount)
	assert.Contains(t, seen, "mr_event/comments/4001")
	assert.Equal(t, int32(2), f.firstComments.Load(), "only the initial read and the start of addressed reading visit comments page 1")

	in.SchemaId = adaptersdk.FetchInputSchemaIdAccounts
	in.Cursor, in.ObjectKeys = nil, []adaptersdk.ObjectKey{}
	_, accounts := githubRecords(t, e, in)
	assert.Contains(t, accounts, "account/207", "account collection still reaches the 4,000th comment")
}

func TestGitHubAdapterUpdatesReadyAtAfterTimelineContinuation(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.historyScenario.Store(_githubReadyTimeline)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	partial, first := githubRecords(t, e, in)
	assert.Len(t, partial.Manifest.Selection.Truncated, 1)
	assert.NotContains(t, string(first["merge_request/601"].Payload), "ready_at")

	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
	p, _ := githubRecords(t, e, in)
	next := p.Manifest.Selection.NextCursor
	require.NotNil(t, next)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
	p, records := githubRecords(t, e, in)
	assert.False(t, p.Manifest.Selection.End)
	assert.Empty(t, p.Manifest.Selection.Truncated)
	ready := records["mr_event/timeline/302"]
	require.NotNil(t, ready.EventTime)
	assert.Equal(t, "2026-11-21T10:02:00+03:00", ready.EventTime.Format(time.RFC3339))
	assert.Contains(t, string(records["merge_request/601"].Payload), `"ready_at":"2026-11-21T10:02:00+03:00"`)
	next = p.Manifest.Selection.NextCursor
	require.NotNil(t, next)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
	p, records = githubRecords(t, e, in)
	assert.True(t, p.Manifest.Selection.End)
	assert.Empty(t, p.Manifest.Selection.Truncated)
	assert.Contains(t, string(records["merge_request/601"].Payload), `"ready_at":"2026-11-21T10:02:00+03:00"`)
}

func TestGitHubAdapterCompletesHistoryWithoutHeadChanges(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.historyScenario.Store(_githubUnchangedHead)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	for _, keys := range [][]adaptersdk.ObjectKey{nil, {{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}} {
		in.ObjectKeys = keys
		p, records := githubRecords(t, e, in)
		assert.True(t, p.Manifest.Selection.End)
		assert.Empty(t, p.Manifest.Selection.Truncated)
		assert.Equal(t, envelope.ManifestCompletenessComplete, p.Manifest.Completeness)
		for _, rec := range records {
			assert.NotEqual(t, "mr_revision", string(rec.RecordKind))
		}
	}
}

func TestGitHubAdapterUsesForcePushActivityWithoutDuplicatingTimeline(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.historyScenario.Store(_githubForcePushes)
	f.commentCount.Store(240)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
	p, first := githubRecords(t, e, in)
	assert.Contains(t, first, "mr_event/activity/9/91")
	assert.Contains(t, first, "mr_revision/activity/9/91")
	assert.Contains(t, string(first["mr_revision/activity/9/91"].Payload), strings.Repeat("1", 40))
	assert.Contains(t, first, "mr_revision/timeline/305")
	assert.NotContains(t, first, "mr_event/timeline/306")
	next := p.Manifest.Selection.NextCursor
	require.NotNil(t, next)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
	p, rest := githubRecords(t, e, in)
	assert.True(t, p.Manifest.Selection.End)
	assert.NotContains(t, rest, "mr_event/activity/9/89")
	assert.NotContains(t, rest, "mr_revision/activity/9/89")
}

func TestGitHubAdapterResumesAccountEnrichment(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.commentCount.Store(100)
	f.manyAuthors.Store(true)
	for _, schema := range []adaptersdk.FetchInputSchemaId{
		adaptersdk.FetchInputSchemaIdMergeRequests, adaptersdk.FetchInputSchemaIdAccounts,
	} {
		in := e.target.Fetch
		in.Scope.NativeScopeId = "8"
		in.SchemaId = schema
		seen := make(map[string]struct{})
		for step := 0; ; step++ {
			require.Less(t, step, 8, "profile cursor did not reach the end")
			p, records := githubRecords(t, e, in)
			if step == 0 {
				assert.NotContains(t, records, "account/1100")
			}
			for key, rec := range records {
				if strings.HasPrefix(key, "account/1") {
					assert.Contains(t, string(rec.Payload), "Fixture author")
					assert.NotContains(t, seen, key)
					seen[key] = struct{}{}
				}
				if step > 0 {
					assert.NotContains(t, key, "mr_event/")
				}
			}
			if p.Manifest.Selection.End {
				break
			}
			next := p.Manifest.Selection.NextCursor
			require.NotNil(t, next)
			in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
		}
		assert.Len(t, seen, 100)
	}
}

func TestGitHubAdapterResumesReviewerEnrichmentFromUpdatedSnapshot(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	f.historyScenario.Store(_githubReadyTimeline)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
	p, _ := githubRecords(t, e, in)
	next := p.Manifest.Selection.NextCursor
	require.NotNil(t, next)

	f.manyReviewers.Store(true)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
	p, first := githubRecords(t, e, in)
	assert.Contains(t, string(first["merge_request/601"].Payload), `"ready_at":"2026-11-21T10:02:00+03:00"`)
	assert.Contains(t, first, "account/1001")
	assert.NotContains(t, first, "account/1040")
	next = p.Manifest.Selection.NextCursor
	require.NotNil(t, next)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}
	_, rest := githubRecords(t, e, in)
	assert.Contains(t, rest, "account/1040")
	assert.NotContains(t, rest, "account/1001")
	assert.NotContains(t, rest, "mr_event/timeline/302")
}

func TestGitHubAdapterContinuesActivityForSameHeadRef(t *testing.T) {
	t.Parallel()
	e, f := newGitHubFixture(t)
	e.probe(t)
	// Eight pages of comments leave room for exactly one page of repository activity.
	f.commentCount.Store(240)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"}}
	p, first := githubRecords(t, e, in)
	assert.Contains(t, first, "mr_event/activity/9/91")
	assert.NotContains(t, first, "mr_event/activity/9/89")
	next := p.Manifest.Selection.NextCursor
	require.NotNil(t, next)
	in.Cursor = &adaptersdk.Cursor{ModifiedAt: next.ModifiedAt, NativeId: next.NativeId}

	f.forkUnavailable.Store(true)
	status, body := e.call(t, http.MethodPost, "fetch", in)
	require.GreaterOrEqual(t, status, 400)
	var failure adaptersdk.AdapterError
	require.NoError(t, json.Unmarshal(body, &failure))
	assert.Equal(t, adaptersdk.AdapterErrorCodeUnavailable, failure.Code)
	f.forkUnavailable.Store(false)

	f.moved.Store(true)
	p, resumed := githubRecords(t, e, in)
	assert.True(t, p.Manifest.Selection.End)
	assert.Equal(t, envelope.ManifestCompletenessPartial, p.Manifest.Completeness)
	assert.Contains(t, resumed, "mr_event/activity/9/89")
	assert.NotContains(t, resumed, "mr_event/activity/9/91")
	assert.NotContains(t, resumed, "mr_event/comments/1")
	assert.Equal(t, int32(1), f.firstActivity.Load())

	f.moved.Store(false)
	f.activityRefChanged.Store(true)
	p, restarted := githubRecords(t, e, in)
	assert.True(t, p.Manifest.Selection.End)
	assert.Equal(t, envelope.ManifestCompletenessPartial, p.Manifest.Completeness)
	assert.Contains(t, restarted, "mr_event/activity/9/91")
	assert.Contains(t, restarted, "mr_event/activity/9/89")
	assert.NotContains(t, restarted, "mr_event/comments/1")
	assert.Equal(t, int32(2), f.firstActivity.Load())
}

func TestGitHubAdapterKeepsReviewTeamsDistinct(t *testing.T) {
	t.Parallel()
	e, _ := newGitHubFixture(t)
	e.probe(t)
	in := e.target.Fetch
	in.Scope.NativeScopeId = "8"
	_, records := githubRecords(t, e, in)
	pr, ok := records["merge_request/601"]
	require.True(t, ok)
	var snapshot envelope.MergeRequestV1
	require.NoError(t, json.Unmarshal(pr.Payload, &snapshot))
	require.NotNil(t, snapshot.ReviewerField)
	assert.True(t, *snapshot.ReviewerField)
	require.NotNil(t, snapshot.ReviewerAccountNativeIds)
	assert.ElementsMatch(t, []string{"201", "team/202"}, *snapshot.ReviewerAccountNativeIds)
	assert.NotContains(t, records, "mr_event/timeline/413", "an unknown team cannot be replayed as a reviewer")
	for _, tc := range []struct {
		id, target string
		kind       envelope.MrEventV2EventKind
	}{
		{"410", "team/201", envelope.MrEventV2EventKindReviewerAssigned},
		{"411", "team/202", envelope.MrEventV2EventKindReviewerAssigned},
		{"412", "team/201", envelope.MrEventV2EventKindReviewerRemoved},
	} {
		rec, ok := records["mr_event/timeline/"+tc.id]
		require.True(t, ok)
		var event envelope.MrEventV2
		require.NoError(t, json.Unmarshal(rec.Payload, &event))
		require.NotNil(t, event.TargetAccountNativeId)
		assert.Equal(t, tc.target, *event.TargetAccountNativeId)
		assert.Equal(t, tc.kind, event.EventKind)
	}
}

func githubRecords(t *testing.T, e gitLabTestEnv, in adaptersdk.FetchInput) (pkg *envelope.Package, records map[string]envelope.Record) {
	t.Helper()
	p, result := e.fetch(t, in)
	records = make(map[string]envelope.Record, len(result.Records))
	for _, row := range result.Records {
		rec, ok, err := p.Record(row.No)
		require.NoError(t, err)
		require.True(t, ok)
		key := row.Kind + "/" + rec.NativeId
		require.NotContains(t, records, key)
		records[key] = *rec
	}
	return p, records
}
