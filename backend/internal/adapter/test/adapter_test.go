package testadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _key = "test-service-key"

type server struct {
	t      *testing.T
	url    string
	client *http.Client
}

// serve runs the adapter behind the library handler; Fetch, Manifest and CheckAccess do not touch the
// credential store, so it has no database.
func serve(t *testing.T, a *Adapter) *server {
	t.Helper()
	h, err := adaptersdk.NewHandler(adaptersdk.Options{Source: a, Store: adaptersdk.NewStore(nil, nil), ServiceKeys: []string{_key}, SpoolDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &server{t: t, url: srv.URL, client: srv.Client()}
}

type reply struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func (s *server) post(op string, body any) reply {
	s.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(s.t.Context(), http.MethodPost,
		s.url+"/v1/connections/0193a5b0-0000-7000-8000-000000000001/"+op, bytes.NewReader(data))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+_key)
	req.Header.Set("X-Craftpulse-Organization", "0193a5b0-0000-7000-8000-000000000002")
	req.Header.Set("X-Craftpulse-Contract-Version", "1")
	req.Header.Set("X-Correlation-Id", "test-"+op)
	resp, err := s.client.Do(req)
	if err != nil {
		return reply{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	return reply{status: resp.StatusCode, header: resp.Header, body: got, err: err}
}

func (r reply) code() adaptersdk.AdapterErrorCode {
	var e adaptersdk.AdapterError
	_ = json.Unmarshal(r.body, &e)
	return e.Code
}

func scope() map[string]any {
	return map[string]any{"kind": "tracker_project", "native_scope_id": "10000"}
}

func fetchInput(schema string, cursor *envelope.Cursor, limit int) map[string]any {
	in := map[string]any{"schema_id": schema, "scope": scope(), "cursor": nil, "period": map[string]any{"from": nil, "to": nil}, "limit": limit, "object_keys": []any{}}
	if cursor != nil {
		in["cursor"] = map[string]any{"modified_at": cursor.ModifiedAt, "native_id": cursor.NativeId}
	}
	return in
}

func manifestInput(schema string, cursor *string, limit int) map[string]any {
	return map[string]any{"schema_id": schema, "scope": scope(), "period": map[string]any{"from": nil, "to": nil}, "cursor": cursor, "limit": limit}
}

func newAdapter(t *testing.T, cfg Config) *Adapter {
	t.Helper()
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type fetched struct {
	ids     [][]string
	updated map[string]time.Time
	results []*envelope.Result
}

// fetchAll follows next_cursor and checks every package.
func fetchAll(t *testing.T, s *server, schema string, limit int) fetched {
	t.Helper()
	out := fetched{updated: map[string]time.Time{}}
	var cursor *envelope.Cursor
	for range 20 {
		r := s.post("fetch", fetchInput(schema, cursor, limit))
		if r.err != nil || r.status != http.StatusOK {
			t.Fatalf("fetch: %v %d %s", r.err, r.status, r.body)
		}
		res := envelope.Check(bytes.NewReader(r.body), envelope.DefaultLimits)
		if res.Envelope != nil || res.Rejected() > 0 {
			t.Fatalf("package is rejected: %+v", res)
		}
		out.results = append(out.results, res)
		ids := make([]string, 0, len(res.Records))
		for _, rec := range res.Records {
			ids = append(ids, rec.NativeID)
		}
		out.ids = append(out.ids, ids)
		if res.Manifest.Selection.End {
			return out
		}
		cursor = res.Manifest.Selection.NextCursor
	}
	t.Fatal("selection did not end")
	return out
}

func TestFetch_FollowsRecordedFeedAcrossPages(t *testing.T) {
	t.Parallel()
	s := serve(t, newAdapter(t, Config{}))
	got := fetchAll(t, s, "tasks", 3)
	want := [][]string{{"20001", "20002", "20003"}, {"20004", "20005", "20006"}, {"20002", "20007", "20005"}}
	if !slices.EqualFunc(got.ids, want, slices.Equal) {
		t.Fatalf("pages %v, want %v", got.ids, want)
	}
	last := got.results[2].Manifest.Selection
	if last.NextCursor != nil || last.TotalCount == nil || *last.TotalCount != 9 {
		t.Fatalf("last page selection %+v", last)
	}
	again := fetchAll(t, serve(t, newAdapter(t, Config{})), "tasks", 3)
	if !slices.EqualFunc(again.ids, want, slices.Equal) {
		t.Fatalf("a second run gives %v", again.ids)
	}
}

func TestFetch_MarksTruncatedChangelog(t *testing.T) {
	t.Parallel()
	got := fetchAll(t, serve(t, newAdapter(t, Config{})), "task_events", 100)
	truncated := got.results[0].Manifest.Selection.Truncated
	if len(truncated) != 1 || truncated[0].NativeId != "20003" || truncated[0].HistoryFrom == nil {
		t.Fatalf("truncated %+v, want task 20003 with history_from", truncated)
	}
}

func TestManifest_MatchesRecordedObjects(t *testing.T) {
	t.Parallel()
	s := serve(t, newAdapter(t, Config{}))

	objects := map[string]bool{}
	for _, res := range fetchAll(t, s, "tasks", 3).results {
		for _, rec := range res.Records {
			objects[rec.NativeID] = true
		}
	}
	var pages [][]adaptersdk.ManifestItem
	var cursor *string
	for range 10 {
		r := s.post("manifest", manifestInput("tasks", cursor, 4))
		var page adaptersdk.ManifestPage
		if r.status != http.StatusOK || json.Unmarshal(r.body, &page) != nil {
			t.Fatalf("manifest: %d %s", r.status, r.body)
		}
		pages = append(pages, page.Items)
		if cursor = page.NextCursor; cursor == nil {
			break
		}
	}
	items := slices.Concat(pages...)
	if len(pages) != 2 || len(items) != len(objects) {
		t.Fatalf("manifest has %d items in %d pages, the feed has %d objects", len(items), len(pages), len(objects))
	}
	wantUpdated := map[string]string{"20002": "2026-03-03T08:00:00Z", "20005": "2026-03-03T10:00:00Z"}
	for _, it := range items {
		if !objects[it.NativeId] || it.UpdatedAt == nil {
			t.Fatalf("manifest item %+v is not an object of the feed", it)
		}
		if want, changed := wantUpdated[it.NativeId]; changed && it.UpdatedAt.Format(time.RFC3339) != want {
			t.Fatalf("object %s changed between pages has version %s, want %s", it.NativeId, it.UpdatedAt, want)
		}
		delete(objects, it.NativeId)
	}
	if len(objects) != 0 {
		t.Fatalf("objects missing from the manifest: %v", objects)
	}
}

// outcome is what the core sees: a transport failure, or a status with a broken package or an error code.
type outcome struct {
	status int
	code   adaptersdk.AdapterErrorCode
	retry  string
	failed bool
	broken bool
}

func (r reply) outcome() outcome {
	o := outcome{status: r.status, retry: r.header.Get("Retry-After")}
	switch {
	case r.err != nil:
		o.failed = true
	case r.status == http.StatusOK && strings.HasPrefix(r.header.Get("Content-Type"), "application/gzip"):
		res := envelope.Check(bytes.NewReader(r.body), envelope.DefaultLimits)
		o.broken = res.Envelope != nil || res.Rejected() > 0
	case r.status != http.StatusOK:
		o.code = r.code()
	}
	return o
}

// sequence reads the three pages of tasks, calling page 2 three times.
func sequence(t *testing.T, faults []Fault) []outcome {
	t.Helper()
	s := serve(t, newAdapter(t, Config{Faults: faults}))
	s.client.Timeout = 2 * time.Second
	page2 := &envelope.Cursor{ModifiedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), NativeId: "20003"}
	page3 := &envelope.Cursor{ModifiedAt: time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC), NativeId: "20006"}
	out := make([]outcome, 0, 5)
	for _, c := range []*envelope.Cursor{nil, page2, page2, page2, page3} {
		out = append(out, s.post("fetch", fetchInput("tasks", c, 3)).outcome())
	}
	return out
}

func TestFaults_EachKindIsReproducible(t *testing.T) {
	t.Parallel()
	ok := outcome{status: http.StatusOK}
	tests := []struct {
		fault Fault
		want  outcome
	}{
		{Fault{Kind: FaultAuth}, outcome{status: 403, code: adaptersdk.AdapterErrorCodeAuth}},
		{Fault{Kind: FaultPermissionMissing}, outcome{status: 403, code: adaptersdk.AdapterErrorCodePermissionMissing}},
		{Fault{Kind: FaultRateLimited, RetryAfterSeconds: 60}, outcome{status: 429, code: adaptersdk.AdapterErrorCodeRateLimited, retry: "60"}},
		{Fault{Kind: FaultUnavailable}, outcome{status: 503, code: adaptersdk.AdapterErrorCodeUnavailable}},
		{Fault{Kind: FaultPreparing, RetryAfterSeconds: 300}, outcome{status: 503, code: adaptersdk.AdapterErrorCodePreparing, retry: "300"}},
		{Fault{Kind: FaultHang, DelayMS: 5000}, outcome{failed: true}},
		{Fault{Kind: FaultDropConnection}, outcome{status: 200, failed: true}},
		{Fault{Kind: FaultTruncated}, outcome{status: 200, broken: true}},
		{Fault{Kind: FaultSchemaViolation}, outcome{status: 200, broken: true}},
	}
	for _, tt := range tests {
		t.Run(tt.fault.Kind, func(t *testing.T) {
			t.Parallel()
			f := tt.fault
			f.Operation, f.SchemaID, f.Page, f.Attempts = OpFetch, "tasks", 2, []int{1, 2}
			first := sequence(t, []Fault{f})
			want := []outcome{ok, tt.want, tt.want, ok, ok}
			for i := range want {
				if first[i].status != want[i].status || first[i].code != want[i].code || first[i].broken != want[i].broken || first[i].failed != want[i].failed ||
					(want[i].retry != "" && !closeSeconds(first[i].retry, want[i].retry)) {
					t.Fatalf("call %d: %+v, want %+v (all %+v)", i+1, first[i], want[i], first)
				}
			}
			second := sequence(t, []Fault{f})
			for i := range first {
				if first[i].status != second[i].status || first[i].code != second[i].code || first[i].broken != second[i].broken || first[i].failed != second[i].failed {
					t.Fatalf("the same scenario gives %+v, then %+v", first, second)
				}
			}
		})
	}
}

// closeSeconds compares Retry-After values that may differ by the second the call crossed.
func closeSeconds(got, want string) bool {
	g, errG := strconv.Atoi(got)
	w, errW := strconv.Atoi(want)
	return errG == nil && errW == nil && g >= w-1 && g <= w
}

func TestFaults_OtherOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAdapter(t, Config{Faults: []Fault{
		{Operation: OpProbe, Attempts: []int{1}, Kind: FaultAuth},
		{Operation: OpProbe, Attempts: []int{2}, Kind: FaultSchemaViolation},
		{Operation: OpManifest, SchemaID: "tasks", Page: 1, Attempts: []int{1}, Kind: FaultSchemaViolation},
		{Operation: OpCheckAccess, Attempts: []int{1}, Kind: FaultSchemaViolation},
	}})
	in := adaptersdk.ProbeInput{BaseUrl: "https://jira.example.test", Scopes: []adaptersdk.ScopeRef{{Kind: adaptersdk.ScopeRefKindTrackerProject, NativeScopeId: "10000"}}}
	_, err := a.Probe(ctx, adaptersdk.Call{}, in)
	if e, ok := errors.AsType[*adaptersdk.Error](err); !ok || e.Code != adaptersdk.AdapterErrorCodeAuth {
		t.Fatalf("probe attempt 1: %v", err)
	}
	res, err := a.Probe(ctx, adaptersdk.Call{}, in)
	if err != nil || res.InstanceKey != "" {
		t.Fatalf("probe attempt 2 must break the schema: %+v %v", res, err)
	}
	res, err = a.Probe(ctx, adaptersdk.Call{}, in)
	if err != nil || res.InstanceKey == "" || res.Scopes[0].State != adaptersdk.ScopeCheckStateVerified {
		t.Fatalf("probe attempt 3: %+v %v", res, err)
	}

	s := serve(t, a)
	var page adaptersdk.ManifestPage
	_ = json.Unmarshal(s.post("manifest", manifestInput("tasks", nil, 4)).body, &page)
	if len(page.Items) == 0 || page.Items[0].NativeId != "" {
		t.Fatalf("manifest must break the schema: %+v", page.Items)
	}
	var access adaptersdk.CheckAccessResult
	_ = json.Unmarshal(s.post("check-access", checkAccessInput()).body, &access)
	if len(access.Decisions) != 1 {
		t.Fatalf("check access must lose a decision: %+v", access.Decisions)
	}
}

func TestProbe_EditionAndPartialHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	in := adaptersdk.ProbeInput{BaseUrl: "https://jira.example.test", Scopes: []adaptersdk.ScopeRef{}}
	_, err := newAdapter(t, Config{Edition: EditionCloud}).Probe(ctx, adaptersdk.Call{}, in)
	if e, ok := errors.AsType[*adaptersdk.Error](err); !ok || e.Code != adaptersdk.AdapterErrorCodeUnsupportedEdition {
		t.Fatalf("Jira Cloud must be refused: %v", err)
	}

	a := newAdapter(t, Config{HistoryDenied: true})
	res, err := a.Probe(ctx, adaptersdk.Call{}, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Capabilities {
		history := strings.HasPrefix(string(c.Capability), "read_") && strings.HasSuffix(string(c.Capability), "_history")
		if history != (c.State == adaptersdk.CapabilityStateMissing) {
			t.Fatalf("capability %s is %s", c.Capability, c.State)
		}
	}
	if r := serve(t, a).post("fetch", fetchInput("task_events", nil, 10)); r.status != http.StatusForbidden || r.code() != adaptersdk.AdapterErrorCodePermissionMissing {
		t.Fatalf("history read must be refused: %d %s", r.status, r.body)
	}
}

const _user = "0193a5b0-1111-7000-8000-000000000001"

func checkAccessInput() map[string]any {
	return map[string]any{
		"user_id": _user, "account_native_id": "demo-person-01",
		"items": []map[string]any{
			{"item_kind": "area", "area_kind": "tracker_project", "object_kind": nil, "native_ns": "", "native_id": "10000"},
			{"item_kind": "object", "area_kind": nil, "object_kind": "task", "native_ns": "", "native_id": "20003"},
		},
	}
}

func decisions(t *testing.T, s *server) []string {
	t.Helper()
	r := s.post("check-access", checkAccessInput())
	var res adaptersdk.CheckAccessResult
	if r.err != nil || r.status != http.StatusOK || json.Unmarshal(r.body, &res) != nil {
		t.Fatalf("check access: %v %d %s", r.err, r.status, r.body)
	}
	out := make([]string, 0, len(res.Decisions))
	for _, d := range res.Decisions {
		out = append(out, string(d.Decision))
	}
	return out
}

func writeScenario(t *testing.T, path string, cfg Config, at time.Time) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAccess_SwitchesWithoutRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "scenario.json")
	user := _user
	allow := Config{Access: Access{Rules: []AccessRule{
		{UserID: user, ItemKind: "object", NativeID: "20003", Decision: "denied", ReasonCode: "security_level"},
		{UserID: user, Decision: "allowed"},
	}}}
	writeScenario(t, path, allow, time.Now().Add(-time.Hour))
	a, err := NewFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := serve(t, a)
	if got := decisions(t, s); !slices.Equal(got, []string{"allowed", "denied"}) {
		t.Fatalf("decisions %v", got)
	}

	revoked := Config{Access: Access{Default: "denied"}}
	writeScenario(t, path, revoked, time.Now())
	if got := decisions(t, s); !slices.Equal(got, []string{"denied", "denied"}) {
		t.Fatalf("after revocation %v", got)
	}
	if err := a.SetConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	if got := decisions(t, s); !slices.Equal(got, []string{"unknown", "unknown"}) {
		t.Fatalf("without rules %v", got)
	}

	if err := a.SetConfig(Config{Access: Access{Default: "allowed", DelayMS: 2100}}); err != nil {
		t.Fatal(err)
	}
	s.client.Timeout = 2 * time.Second
	if r := s.post("check-access", checkAccessInput()); r.err == nil {
		t.Fatalf("a check slower than 2 s must not answer in time: %d", r.status)
	}
}

func TestParseConfig_RejectsMistakes(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"unknown field":         `{"fault": []}`,
		"unknown kind":          `{"faults": [{"operation": "fetch", "kind": "explode"}]}`,
		"unknown operation":     `{"faults": [{"operation": "publish", "kind": "auth"}]}`,
		"stream fault on probe": `{"faults": [{"operation": "probe", "kind": "truncated"}]}`,
		"unknown decision":      `{"access": {"default": "maybe"}}`,
		"unknown edition":       `{"edition": "server"}`,
	} {
		if _, err := ParseConfig([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseConfig([]byte(`{"faults": [{"operation": "fetch", "page": 2, "attempts": [1], "kind": "drop_connection"}]}`)); err != nil {
		t.Fatal(err)
	}
}
