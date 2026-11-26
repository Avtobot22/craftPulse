package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_ciJUnit      = `<testsuites><testsuite name="ci"><testcase name="ok"/></testsuite></testsuites>`
	_ciCoverage   = `<coverage line-rate="1"><packages/></coverage>`
	_ciParentTime = "2026-11-12T05:42:00.000Z"
	_ciRetryTime  = "2026-11-12T06:42:00.000Z"
)

type gitLabCIFixture struct {
	gitLabFixture
	retried atomic.Bool
	broken  atomic.Bool
	large   atomic.Bool
}

func (f *gitLabCIFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != _gitLabToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v4/projects/7")
	parentTime, parentStatus := _ciParentTime, "failed"
	if f.retried.Load() {
		parentTime, parentStatus = _ciRetryTime, "success"
	}
	parent := ciPipelineFixture("500", "push", parentStatus, parentTime)
	child := ciPipelineFixture("501", "parent_pipeline", "success", _ciParentTime)
	query := r.URL.Query()
	w.Header().Set("Content-Type", "application/json")
	switch path {
	case "/pipelines":
		requireCIQuery(w, query.Get("order_by") == "updated_at" && query.Get("sort") == "asc", func() {
			if query.Get("source") == "parent_pipeline" {
				_, _ = io.WriteString(w, "["+child+"]")
				return
			}
			_, _ = io.WriteString(w, "["+parent+"]")
		})
	case "/pipelines/500":
		_, _ = io.WriteString(w, parent)
	case "/pipelines/501":
		_, _ = io.WriteString(w, child)
	case "/api/graphql":
		_, _ = io.WriteString(w, `{"data":{"project":{"pipeline":{"upstream":{"id":"gid://gitlab/Ci::Pipeline/500","project":{"id":"gid://gitlab/Project/7"}},"sourceJob":null,"mergeRequest":null}}}}`)
	case "/pipelines/500/bridges":
		_, _ = io.WriteString(w, `[{"id":92,"downstream_pipeline":`+child+`}]`)
	case "/pipelines/501/jobs":
		requireCIQuery(w, query.Get("include_retried") == "true", func() {
			_, _ = io.WriteString(w, "["+ciJobFixture("77", "child job", "success", "5.4321", "", false, "")+"]")
		})
	case "/pipelines/500/jobs":
		requireCIQuery(w, query.Get("include_retried") == "true", func() { f.serveJobs(w, query.Get("page")) })
	case "/jobs/72/artifacts", "/jobs/79/artifacts":
		f.serveReport(w, query.Get("file_type"))
	case "/jobs/75/artifacts":
		w.WriteHeader(http.StatusNotFound)
	default:
		f.gitLabFixture.ServeHTTP(w, r)
	}
}

func requireCIQuery(w http.ResponseWriter, valid bool, respond func()) {
	if !valid {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	respond()
}

func ciPipelineFixture(id, source, status, stamp string) string {
	return `{"id":` + id + `,"iid":` + id + `,"project_id":7,"source":"` + source + `","status":"` + status + `","ref":"main","sha":"` + _glHead + `","started_at":"2026-11-12T05:40:00Z","finished_at":"` + stamp + `","updated_at":"` + stamp + `"}`
}

func ciJobFixture(id, name, status, duration, failure string, allowFailure bool, artifacts string) string {
	allow := "false"
	if allowFailure {
		allow = "true"
	}
	times := `"started_at":"2026-11-12T05:40:00Z","finished_at":"2026-11-12T05:41:00Z"`
	if status == "manual" {
		times = `"started_at":null,"finished_at":null`
	}
	return `{"id":` + id + `,"name":"` + name + `","status":"` + status + `","allow_failure":` + allow + `,"failure_reason":"` + failure + `","duration":` + duration + `,"created_at":"2026-11-12T05:39:00Z",` + times + `,"runner":{"runner_type":"project_type"}` + artifacts + `}`
}

func (f *gitLabCIFixture) serveJobs(w http.ResponseWriter, page string) {
	const reports = `,"artifacts":[{"file_type":"junit"},{"file_type":"cobertura"}]`
	const expired = `,"artifacts_expire_at":"2020-01-02T00:00:00Z","artifacts":[{"file_type":"junit"}]`
	if page == "2" {
		if f.broken.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, "["+ciJobFixture("71", "test: [linux]", "failed", "60", "script_failure", false, "")+"]")
		return
	}
	jobs := []string{
		ciJobFixture("78", "canceled", "canceled", "2", "", false, ""),
		ciJobFixture("76", "manual", "manual", "null", "", true, ""),
		ciJobFixture("75", "expired", "success", "1", "", false, expired),
		ciJobFixture("74", "timeout", "failed", "60", "job_execution_timeout", false, ""),
		ciJobFixture("73", "allowed", "failed", "1", "script_failure", true, ""),
		ciJobFixture("72", "test: [windows]", "success", "40", "", false, reports),
	}
	if f.retried.Load() {
		jobs = append([]string{ciJobFixture("79", "test: [linux]", "success", "30", "", false, reports)}, jobs...)
	}
	w.Header().Set("X-Next-Page", "2")
	_, _ = io.WriteString(w, "["+strings.Join(jobs, ",")+"]")
}

func (f *gitLabCIFixture) serveReport(w http.ResponseWriter, kind string) {
	w.Header().Set("Content-Type", "application/octet-stream")
	switch kind {
	case "junit":
		z := gzip.NewWriter(w)
		if f.large.Load() {
			for range 129 {
				_, _ = io.WriteString(z, strings.Repeat("x", 64<<10))
			}
		} else {
			_, _ = io.WriteString(z, _ciJUnit)
		}
		_ = z.Close()
	case "cobertura":
		_, _ = io.WriteString(w, _ciCoverage)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newGitLabCIEnv(t *testing.T, source http.Handler) gitLabTestEnv {
	t.Helper()
	e := newGitLabTestEnvWithSource(t, source)
	e.target.Fetch.SchemaId = adaptersdk.FetchInputSchemaIdCiRuns
	e.target.Manifest.SchemaId = adaptersdk.ManifestInputSchemaIdCiRuns
	return e
}

func TestContract_GitLabAdapterCI(t *testing.T) {
	t.Parallel()
	e := newGitLabCIEnv(t, &gitLabCIFixture{})
	contracttest.Run(t, e.target)
}

func TestGitLabAdapterCIRetryReportsAndManifest(t *testing.T) {
	t.Parallel()
	fixture := &gitLabCIFixture{}
	e := newGitLabCIEnv(t, fixture)
	e.probe(t)

	before, result := e.fetch(t, e.target.Fetch)
	require.NotNil(t, before.Manifest.Selection.NextCursor)
	original := ciRecords(t, before, result)
	require.Equal(t, 1, decodeGitLabCIAttempt(t, original["ci_attempt/71"]).AttemptNo)

	fixture.retried.Store(true)
	input := e.target.Fetch
	input.Cursor = &adaptersdk.Cursor{ModifiedAt: before.Manifest.Selection.NextCursor.ModifiedAt}
	child, childResult := e.fetch(t, input)
	childRecords := ciRecords(t, child, childResult)
	childRun := decodeGitLabCIRun(t, childRecords["ci_run/501"])
	require.Equal(t, "500", *childRun.ParentRunNativeId)
	require.Equal(t, "92", *childRun.TriggerJobNativeId)
	childJob := decodeGitLabCIAttempt(t, childRecords["ci_attempt/77"])
	require.Equal(t, "501", childJob.RunNativeId)
	require.NotNil(t, childJob.ParentRunNativeId)
	require.Equal(t, "500", *childJob.ParentRunNativeId)

	require.NotNil(t, child.Manifest.Selection.NextCursor)
	input.Cursor = &adaptersdk.Cursor{NativeId: child.Manifest.Selection.NextCursor.NativeId, ModifiedAt: child.Manifest.Selection.NextCursor.ModifiedAt}
	after, result := e.fetch(t, input)
	require.True(t, after.Manifest.Selection.End)
	records := ciRecords(t, after, result)
	first, second, windows := decodeGitLabCIAttempt(t, records["ci_attempt/71"]), decodeGitLabCIAttempt(t, records["ci_attempt/79"]), decodeGitLabCIAttempt(t, records["ci_attempt/72"])
	require.Equal(t, []int{1, 2, 1}, []int{first.AttemptNo, second.AttemptNo, windows.AttemptNo})
	require.Equal(t, []int{60, 30, 40}, []int{*first.DurationS, *second.DurationS, *windows.DurationS})
	require.Equal(t, "test", first.JobKey)
	require.Equal(t, first.ConfigFingerprint, second.ConfigFingerprint)
	require.NotEqual(t, first.ConfigFingerprint, windows.ConfigFingerprint)
	require.Equal(t, original["ci_attempt/71"].SourceRevision, records["ci_attempt/71"].SourceRevision)
	require.NotEqual(t, original["ci_run/500"].SourceRevision, records["ci_run/500"].SourceRevision)
	require.Equal(t, "success", decodeGitLabCIRun(t, records["ci_run/500"]).NativeStatus)
	require.Equal(t, "failed", first.NativeStatus)
	require.Equal(t, "success", second.NativeStatus)
	require.True(t, decodeGitLabCIAttempt(t, records["ci_attempt/73"]).AllowFailure)
	require.Equal(t, "job_execution_timeout", *decodeGitLabCIAttempt(t, records["ci_attempt/74"]).FailureReason)
	require.Equal(t, "project_type", *first.RunnerClass)
	require.Equal(t, "2026-11-12T05:39:00Z", first.CreatedAt.Format(time.RFC3339))
	require.Equal(t, "2026-11-12T05:40:00Z", first.StartedAt.Format(time.RFC3339))
	require.Equal(t, "2026-11-12T05:41:00Z", first.FinishedAt.Format(time.RFC3339))
	require.Equal(t, envelope.CiAttemptV2ArtifactStatusExpired, *decodeGitLabCIAttempt(t, records["ci_attempt/75"]).ArtifactStatus)
	manual := decodeGitLabCIAttempt(t, records["ci_attempt/76"])
	require.Equal(t, "manual", manual.NativeStatus)
	require.Equal(t, envelope.CiAttemptV2OutcomeSkipped, manual.Outcome)
	require.Nil(t, manual.StartedAt)
	require.Equal(t, envelope.CiAttemptV2OutcomeCanceled, decodeGitLabCIAttempt(t, records["ci_attempt/78"]).Outcome)

	for _, attempt := range []string{"72", "79"} {
		verifyCIFile(t, records, attempt, "junit", "junit", _ciJUnit)
		verifyCIFile(t, records, attempt, "coverage", "cobertura", _ciCoverage)
	}
	again, repeat := e.fetch(t, input)
	for id, rec := range ciRecords(t, again, repeat) {
		require.Equal(t, records[id].SourceRevision, rec.SourceRevision, id)
	}
	require.Equal(t, []string{"501", "500"}, ciManifestIDs(t, e))

	input.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindCiRun, NativeId: "501"}}
	input.Cursor = nil
	keyed, keyResult := e.fetch(t, input)
	require.Equal(t, childRecords["ci_run/501"].SourceRevision, ciRecords(t, keyed, keyResult)["ci_run/501"].SourceRevision)
}

func TestGitLabAdapterCIPartialHistoryAndOversizeReport(t *testing.T) {
	t.Parallel()
	fixture := &gitLabCIFixture{}
	fixture.broken.Store(true)
	fixture.large.Store(true)
	e := newGitLabCIEnv(t, fixture)
	e.probe(t)
	pkg, result := e.fetch(t, e.target.Fetch)
	require.Equal(t, []envelope.TruncatedHistory{{RecordKind: envelope.TruncatedHistoryRecordKindCiRun, NativeId: "500"}}, pkg.Manifest.Selection.Truncated)
	records := ciRecords(t, pkg, result)
	require.NotContains(t, records, "ci_attempt/71")
	require.Equal(t, envelope.CiAttemptV2ArtifactStatusSizeLimit, *decodeGitLabCIAttempt(t, records["ci_attempt/72"]).ArtifactStatus)
	verifyCIFile(t, records, "72", "coverage", "cobertura", _ciCoverage)
	require.NotContains(t, records, "report_file/junit:files/reports/72/junit.xml")
}

type gitLabRecordedSnapshot struct {
	Pipeline json.RawMessage `json:"pipeline"`
	Jobs     json.RawMessage `json:"jobs"`
}

type gitLabRecordedPipeline struct {
	Previous  *gitLabRecordedSnapshot           `json:"previous"`
	Pipeline  json.RawMessage                   `json:"pipeline"`
	Jobs      json.RawMessage                   `json:"jobs"`
	Bridges   json.RawMessage                   `json:"bridges"`
	GraphQL   json.RawMessage                   `json:"graphql"`
	Artifacts map[string]gitLabRecordedArtifact `json:"artifacts"`
}

type gitLabRecordedArtifact struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
	XMLSHA256   string `json:"xml_sha256"`
}

type gitLabRecordedSource struct {
	previous  atomic.Bool
	Version   string                            `json:"version"`
	Pipelines map[string]gitLabRecordedPipeline `json:"pipelines"`
}

func (f *gitLabRecordedSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != _gitLabToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/v4/projects/1")
	switch {
	case path == "":
		_, _ = io.WriteString(w, `{"id":1,"path_with_namespace":"fixture/project","visibility":"private"}`)
	case path == "/members/all/2":
		_, _ = io.WriteString(w, `{"id":2,"access_level":20,"state":"active","membership_state":"active"}`)
	case path == "/merge_requests":
		_, _ = io.WriteString(w, "[]")
	case path == "/api/graphql":
		var variables struct {
			IID string `json:"iid"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("variables")), &variables); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeGitLabFixture(w, f.Pipelines[variables.IID].GraphQL)
	case strings.HasPrefix(path, "/pipelines/"):
		id, collection, _ := strings.Cut(strings.TrimPrefix(path, "/pipelines/"), "/")
		run := f.Pipelines[id]
		if f.previous.Load() && run.Previous != nil {
			run.Pipeline, run.Jobs = run.Previous.Pipeline, run.Previous.Jobs
		}
		switch collection {
		case "":
			writeGitLabFixture(w, run.Pipeline)
		case "jobs":
			writeGitLabFixture(w, run.Jobs)
		case "bridges":
			writeGitLabFixture(w, run.Bridges)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	case strings.HasPrefix(path, "/jobs/"):
		id, _, _ := strings.Cut(strings.TrimPrefix(path, "/jobs/"), "/")
		artifact, found := f.Pipelines["7"].Artifacts[id+"/"+r.URL.Query().Get("file_type")]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", artifact.ContentType)
		w.WriteHeader(artifact.Status)
		_, _ = w.Write(artifact.Body)
	default:
		(&gitLabFixture{}).ServeHTTP(w, r)
	}
}

func writeGitLabFixture(w http.ResponseWriter, value json.RawMessage) {
	if len(value) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = w.Write(value)
}

func TestGitLabAdapterCIRecordedStand(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/gitlab-ci/stand.json")
	require.NoError(t, err)
	var recorded gitLabRecordedSource
	require.NoError(t, json.Unmarshal(body, &recorded))
	require.Equal(t, "19.5.5", recorded.Version)
	e := newGitLabCIEnv(t, &recorded)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "1"}
	e.target.Probe.Scopes = []adaptersdk.ScopeRef{scope}
	e.probe(t)
	in := e.target.Fetch
	in.Scope = scope
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindCiRun, NativeId: "7"}}
	recorded.previous.Store(true)
	oldPage, oldResult := e.fetch(t, in)
	old := ciRecords(t, oldPage, oldResult)
	require.NotContains(t, old, "ci_attempt/14")
	recorded.previous.Store(false)

	in.ObjectKeys = nil
	for _, id := range []string{"4", "6", "7", "8"} {
		in.ObjectKeys = append(in.ObjectKeys, adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindCiRun, NativeId: id})
	}
	records := make(map[string]*envelope.Record)
	for range in.ObjectKeys {
		pkg, result := e.fetch(t, in)
		for key, record := range ciRecords(t, pkg, result) {
			require.NotContains(t, records, key)
			records[key] = record
		}
		if pkg.Manifest.Selection.End {
			break
		}
		next := pkg.Manifest.Selection.NextCursor
		require.NotNil(t, next)
		in.Cursor = &adaptersdk.Cursor{NativeId: next.NativeId, ModifiedAt: next.ModifiedAt}
	}
	require.Equal(t, "1", *decodeGitLabCIRun(t, records["ci_run/4"]).MrNativeId)
	require.Equal(t, "push", *decodeGitLabCIRun(t, records["ci_run/6"]).Source)
	require.InDelta(t, 5.434828, *decodeGitLabCIAttempt(t, records["ci_attempt/4"]).DurationSeconds, 1e-9)
	oldRun := decodeGitLabCIRun(t, old["ci_run/7"])
	newRun := decodeGitLabCIRun(t, records["ci_run/7"])
	require.True(t, newRun.UpdatedAt.After(*oldRun.UpdatedAt))
	require.Equal(t, old["ci_attempt/9"].SourceRevision, records["ci_attempt/9"].SourceRevision)
	require.Equal(t, 2, decodeGitLabCIAttempt(t, records["ci_attempt/14"]).AttemptNo)
	first, windows := decodeGitLabCIAttempt(t, records["ci_attempt/5"]), decodeGitLabCIAttempt(t, records["ci_attempt/6"])
	retry := decodeGitLabCIAttempt(t, records["ci_attempt/13"])
	require.Equal(t, []int{1, 2, 1}, []int{first.AttemptNo, retry.AttemptNo, windows.AttemptNo})
	require.Equal(t, "failed", first.NativeStatus)
	require.Equal(t, "success", retry.NativeStatus)
	require.Equal(t, first.ConfigFingerprint, retry.ConfigFingerprint)
	require.NotEqual(t, windows.ConfigFingerprint, retry.ConfigFingerprint)
	require.True(t, decodeGitLabCIAttempt(t, records["ci_attempt/7"]).AllowFailure)
	expired := decodeGitLabCIAttempt(t, records["ci_attempt/8"]).ArtifactStatus
	require.NotNil(t, expired)
	require.Equal(t, envelope.CiAttemptV2ArtifactStatusExpired, *expired)
	require.Equal(t, "job_execution_timeout", *decodeGitLabCIAttempt(t, records["ci_attempt/9"]).FailureReason)
	require.Equal(t, "manual", decodeGitLabCIAttempt(t, records["ci_attempt/10"]).NativeStatus)
	child := decodeGitLabCIRun(t, records["ci_run/8"])
	require.NotNil(t, child.ParentRunNativeId)
	require.NotNil(t, child.TriggerJobNativeId)
	require.Equal(t, "7", *child.ParentRunNativeId)
	require.Equal(t, "11", *child.TriggerJobNativeId)
	require.Equal(t, "7", *decodeGitLabCIAttempt(t, records["ci_attempt/12"]).ParentRunNativeId)

	for key, artifact := range recorded.Pipelines["7"].Artifacts {
		job, fileType, _ := strings.Cut(key, "/")
		kind := "junit"
		if fileType != "junit" {
			kind = "coverage"
		}
		record := records["report_file/"+kind+":files/reports/"+job+"/"+fileType+".xml"]
		if artifact.Status != http.StatusOK {
			require.Nil(t, record, key)
			continue
		}
		require.NotNil(t, record, key)
		require.Equal(t, artifact.XMLSHA256, record.File.Sha256, key)
	}
}

func ciManifestIDs(t *testing.T, e gitLabTestEnv) []string {
	t.Helper()
	var ids []string
	input := e.target.Manifest
	for range 3 {
		status, body := e.call(t, http.MethodPost, "manifest", input)
		require.Equal(t, http.StatusOK, status, string(body))
		var page adaptersdk.ManifestPage
		require.NoError(t, json.NewDecoder(bytes.NewReader(body)).Decode(&page))
		for _, item := range page.Items {
			require.Equal(t, "ci_run", item.RecordKind)
			require.NotNil(t, item.UpdatedAt)
			ids = append(ids, item.NativeId)
		}
		if page.NextCursor == nil {
			return ids
		}
		input.Cursor = page.NextCursor
	}
	t.Fatal("GitLab manifest did not reach the end")
	return nil
}

func ciRecords(t *testing.T, pkg *envelope.Package, result *envelope.Result) map[string]*envelope.Record {
	t.Helper()
	out := map[string]*envelope.Record{}
	for _, row := range result.Records {
		require.Nil(t, row.Rejection, "%s", row.NativeID)
		record, ok, err := pkg.Record(row.No)
		require.NoError(t, err)
		require.True(t, ok)
		key := string(record.RecordKind) + "/" + record.NativeId
		require.NotContains(t, out, key)
		out[key] = record
	}
	return out
}

func decodeGitLabCIRun(t *testing.T, rec *envelope.Record) envelope.CiRunV2 {
	t.Helper()
	require.NotNil(t, rec)
	require.Equal(t, "ci_run", string(rec.RecordKind))
	var out envelope.CiRunV2
	require.NoError(t, json.Unmarshal(rec.Payload, &out))
	return out
}

func decodeGitLabCIAttempt(t *testing.T, rec *envelope.Record) envelope.CiAttemptV2 {
	t.Helper()
	require.NotNil(t, rec)
	require.Equal(t, "ci_attempt", string(rec.RecordKind))
	var out envelope.CiAttemptV2
	require.NoError(t, json.Unmarshal(rec.Payload, &out))
	return out
}

func verifyCIFile(t *testing.T, records map[string]*envelope.Record, job, kind, format, content string) {
	t.Helper()
	rec := records["report_file/"+kind+":files/reports/"+job+"/"+format+".xml"]
	require.NotNil(t, rec)
	require.NotNil(t, rec.File)
	sum := sha256.Sum256([]byte(content))
	assert.Equal(t, hex.EncodeToString(sum[:]), rec.File.Sha256)
	assert.Equal(t, len(content), rec.File.SizeBytes)
	var payload envelope.ReportFileV1
	require.NoError(t, json.Unmarshal(rec.Payload, &payload))
	require.NotNil(t, payload.RunNativeId)
	require.NotNil(t, payload.AttemptNativeId)
	assert.Equal(t, "500", *payload.RunNativeId)
	assert.Equal(t, job, *payload.AttemptNativeId)
	assert.Equal(t, _glHead, *payload.Sha)
}
