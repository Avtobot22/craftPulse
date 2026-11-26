package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

func codeEnv(t *testing.T) (*normalizeEnv, *browser) {
	t.Helper()
	e := newNormalizeEnv(t)
	m := e.modules(t)
	calculator := calc.New(calc.Deps{TestCoverage: e.norm, Facts: e.norm})
	views := newView(e.app, viewModules{code: e.norm, projectAreas: m.orgs, reviewCalculator: calculator, runCalculator: calculator, instances: e.connect, rights: m.access}, e.clock)
	m.code = views
	reg := prometheus.NewRegistry()
	deps := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, m)
	e.router = httpctrl.NewRouter(zerolog.New(e.logs), reg, health.Probes{Ready: &health.Readiness{}}, deps)
	b := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)
	return e, b
}

func codeRead(t *testing.T, e *loginEnv, b *browser, path, schema string) *httptest.ResponseRecorder {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	validateSourceResponse(t, rec, schema)
	return rec
}

func TestCodeAPI_V28PaginationAndTruncatedHistory(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	manifest := ciManifest("42")
	manifest.Area.Key = new("team/subgroup/core")
	data := pack(t, manifest, v28(t)...)
	e.imported(t, e.admin, data)
	e.drain(t)
	id := runID(t, e, "5001")
	path := "/api/v1/pipelines/" + id.String()
	pipeline := decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, path, "Pipeline"))
	assert.Equal(t, new(_gitlabInstance+"/team/subgroup/core/-/pipelines/5001"), pipeline.SourceUrl)
	assert.Equal(t, 3, pipeline.AttemptsTotal)
	assert.Equal(t, 1, pipeline.RetriedJobs)
	assert.Equal(t, openapi.PipelineOutcome("success"), pipeline.Outcome)
	assert.Equal(t, openapi.PipelineAttemptHistory("complete"), pipeline.AttemptHistory)
	assert.Equal(t, openapi.DataState(entity.StateNotEvaluated), pipeline.Tests.State)
	assert.Nil(t, pipeline.Tests.Passed)
	assert.Empty(t, pipeline.Reports)
	first := decode[openapi.AttemptPage](t, codeRead(t, e.loginEnv, b, path+"/attempts?limit=1", "AttemptPage"))
	require.NotNil(t, first.UsageSeconds.Value)
	assert.Equal(t, float32(130), *first.UsageSeconds.Value)
	seen := map[uuid.UUID]bool{}
	configs := map[string]bool{}
	urls := []string{}
	failed := false
	page := first
	for {
		require.Len(t, page.Data, 1)
		a := page.Data[0]
		require.False(t, seen[a.Id])
		seen[a.Id] = true
		configs[a.ConfigFingerprint] = true
		if assert.NotNil(t, a.SourceUrl) {
			urls = append(urls, *a.SourceUrl)
		}
		failed = failed || a.Outcome == openapi.AttemptOutcome("failed")
		if a.Outcome == openapi.AttemptOutcome("success") {
			assert.Nil(t, a.FailureKind)
		}
		if page.NextCursor == nil {
			break
		}
		cursor := *page.NextCursor
		assert.Equal(t, http.StatusBadRequest, e.do(t, b, http.MethodGet, path+"/attempts?outcome=failed&cursor="+cursor, nil).Code)
		page = decode[openapi.AttemptPage](t, codeRead(t, e.loginEnv, b, path+"/attempts?limit=1&cursor="+cursor, "AttemptPage"))
	}
	assert.Len(t, seen, 3)
	assert.Len(t, configs, 2)
	assert.ElementsMatch(t, []string{
		_gitlabInstance + "/team/subgroup/core/-/jobs/7001",
		_gitlabInstance + "/team/subgroup/core/-/jobs/7002",
		_gitlabInstance + "/team/subgroup/core/-/jobs/7003",
	}, urls)
	assert.True(t, failed)
	filtered := decode[openapi.AttemptPage](t, codeRead(t, e.loginEnv, b, path+"/attempts?outcome=failed", "AttemptPage"))
	require.Len(t, filtered.Data, 1)
	assert.Equal(t, float32(130), *filtered.UsageSeconds.Value, "filter and pagination do not change run usage")
	// A later source observation explicitly marks the retry history as truncated.
	m := ciManifest("42", "5001")
	rec := ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p2", updated: _t2})
	e.imported(t, e.admin, pack(t, m, rec))
	e.drain(t)
	truncated := decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, path, "Pipeline"))
	assert.Equal(t, openapi.PipelineAttemptHistory("truncated"), truncated.AttemptHistory)
	assert.Nil(t, truncated.SourceUrl, "without a repository URL or a project path in this version")
	bounded := decode[openapi.AttemptPage](t, codeRead(t, e.loginEnv, b, path+"/attempts", "AttemptPage"))
	assert.Equal(t, openapi.DataState(entity.StateBounded), bounded.UsageSeconds.State)
	assert.Nil(t, bounded.UsageSeconds.Value)
	require.NotNil(t, bounded.UsageSeconds.Bound)
	assert.Equal(t, float32(130), bounded.UsageSeconds.Bound.Value)
	require.NotNil(t, first.NextCursor)
	e.clock.Advance(24*time.Hour + time.Second)
	require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(t, b, http.MethodGet, path+"/attempts?cursor="+*first.NextCursor, nil).Code)
}

func TestCodeAPI_NormalizedPipelineReports(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	e.imported(t, e.admin, pack(t, ciManifest("42"), v28(t)...))
	e.drain(t)
	specs := v40()[:2]
	require.Equal(t, entity.ReceiptAccepted, e.imported(t, e.admin, reportPackage(t, _headA, specs...)).Status)
	// Same run coordinates with a different SHA do not make an analysis belong to this pipeline.
	other := e.imported(t, e.admin, reportPackage(t, _reportSHA, sarifSpecs()[0]))
	require.Equal(t, entity.ReceiptAccepted, other.Status)
	e.drain(t)
	wrongSHA := e.report(t, other.ID, "code")
	require.NotNil(t, wrongSHA.RunID)
	assert.Equal(t, uuid.UUID(runID(t, e, "5001")), *wrongSHA.RunID)

	pipeline := decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, "/api/v1/pipelines/"+runID(t, e, "5001").String(), "Pipeline"))
	require.Len(t, pipeline.Reports, 1)
	assert.Equal(t, openapi.AnalysisKindJunit, pipeline.Reports[0].AnalysisKind)
	assert.Equal(t, openapi.PipelineReportsStatePartial, pipeline.Reports[0].State)
	assert.Equal(t, openapi.DataState(entity.StateProvisional), pipeline.Tests.State)
	assert.Equal(t, ptr(10), pipeline.Tests.Passed)
	assert.Equal(t, ptr(2), pipeline.Tests.Failed)
	assert.Equal(t, ptr(0), pipeline.Tests.Error)
	assert.Equal(t, ptr(2), pipeline.Tests.Skipped)
}

func TestCodeAPI_MRLinksAndHiddenTask(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	area := linkedTask(t, e, "jira-one.test", "1", "PA-7")
	bindLinkArea(t, e, area)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "team/repo", "r1")))
	e.drain(t)
	bindLinkArea(t, e, mrArea(t, e, rc, "42"))
	record := mrState(t, "9001", "42", "PA-7 implement cache", "m1", _t1, nil)
	var state envelope.MergeRequestV1
	require.NoError(t, json.Unmarshal(record.Payload, &state))
	state.ReadyAt = &_t1
	record.Payload = mustMarshal(t, state)
	e.imported(t, e.admin, pack(t, mrManifest("42"), record))
	e.drain(t)
	id := mrID(t, e, "9001")
	path := "/api/v1/merge-requests/" + id.String()
	mr := decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, b, path, "MergeRequest"))
	assert.Equal(t, _headA, mr.HeadSha)
	require.NotNil(t, mr.BaseSha)
	assert.Equal(t, strings.Repeat("b", 40), *mr.BaseSha)
	require.NotNil(t, mr.ReviewWait)
	require.NotNil(t, mr.ReadyAt)
	assert.True(t, mr.ReadyAt.Equal(_t1))
	assert.Equal(t, openapi.ActorRefKind("unknown"), mr.Author.Kind)
	require.NotNil(t, mr.Author.AccountId)
	person := e.person(t, e.orgID, "Confirmed author")
	assigned := e.send(t, b, http.MethodPost, "/api/v1/account-assignments", http.Header{"Idempotency-Key": {uuid.NewString()}, "Origin": {_site}, "Content-Type": {"application/json"}}, bytes.NewReader(mustMarshal(t, map[string]any{
		"items": []map[string]any{decision(*mr.Author.AccountId, "human", &person, nil, false)},
	})))
	require.Equal(t, http.StatusOK, assigned.Code, assigned.Body.String())
	assert.Equal(t, openapi.ActorRefKind("human"), decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, b, path, "MergeRequest")).Author.Kind)
	require.NotNil(t, mr.SourceUrl)
	assert.Equal(t, _gitlabInstance+"/team/repo/-/merge_requests/7", *mr.SourceUrl)
	require.NotNil(t, mr.Links)
	require.NotEmpty(t, *mr.Links)
	for _, link := range *mr.Links {
		assert.Equal(t, openapi.LinkStatus("candidate"), link.Status)
	}
	// Add a confirmed relation using the public link module, independently of candidates.
	tx := persistent.NewTransactor(e.app)
	links := linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Clock: e.clock, NewID: newID})
	taskID := entity.ID((*mr.Links)[0].To.Id)
	require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		_, err := links.Record(ctx, e.orgID, entity.LinkProposal{Type: entity.LinkTaskMR, From: taskID, To: id, Method: entity.LinkSourceExplicit})
		return err
	}))
	mr = decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, b, path, "MergeRequest"))
	confirmed := false
	for _, link := range *mr.Links {
		confirmed = confirmed || link.Status == openapi.LinkStatus("confirmed")
	}
	assert.True(t, confirmed)
	// A reader of the repository project does not inherit access to a task outside that project.
	project := uuid.UUID(e.project)
	require.Equal(t, http.StatusOK, e.roles(t, b, uuid.UUID(e.member.UserID), `"0"`, []roleInput{{Role: "developer", ProjectID: &project}}).Code)
	_, err := e.owner.Exec(t.Context(), "DELETE FROM org.project_area WHERE organization_id=$1 AND project_id=$2 AND area_id=$3", uuid.UUID(e.orgID), project, uuid.UUID(area))
	require.NoError(t, err)
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	mr = decode[openapi.MergeRequest](t, codeRead(t, e.loginEnv, member, path, "MergeRequest"))
	require.NotNil(t, mr.AccessLimited)
	assert.True(t, *mr.AccessLimited)
	assert.Empty(t, *mr.Links)
	assert.NotContains(t, codeRead(t, e.loginEnv, member, path, "MergeRequest").Body.String(), taskID.String())
}

func TestCodeAPI_SourceAccessRevokedWithinSixtySeconds(t *testing.T) {
	t.Parallel()
	e := newLinkEnv(t)
	mr := e.object(t, "merge_request", "mr-visible", false)
	run := e.object(t, "ci_run", "run-visible", false)
	// Canonical facts belong to the test adapter's collected area, so these reads exercise real CheckAccess.
	_, err := e.owner.Exec(t.Context(), `INSERT INTO normalize.mr_version
 (organization_id,known_seq,receipt_id,record_no,source_revision,content_hash,observed_at,mr_id,area_id,iid,title,state,draft,source_branch,target_branch,head_sha,reviewer_field,reviewer_account_ids,opened_at)
 VALUES ($1,1,$2,1,'v1',decode(repeat('00',32),'hex'),$3,$4,$5,7,'Visible MR','open',false,'feature','main',$6,false,'{}',$3)`, uuid.UUID(e.orgID), uuid.New(), e.clock.Now(), mr, e.area, _headA)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.ci_run_version
 (organization_id,known_seq,receipt_id,record_no,source_revision,content_hash,observed_at,run_id,area_id,mr_id,ref_name,sha,native_status,outcome,started_at,finished_at,attempt_history,allow_failure_semantics)
 VALUES ($1,1,$2,1,'v1',decode(repeat('00',32),'hex'),$3,$4,$5,$6,'main',$7,'success','success',$3,$3,'complete',true)`, uuid.UUID(e.orgID), uuid.New(), e.clock.Now(), run, e.area, mr, _headA)
	require.NoError(t, err)
	paths := []struct{ path, schema string }{
		{"/api/v1/merge-requests/" + mr.String(), "MergeRequest"},
		{"/api/v1/pipelines/" + run.String(), "Pipeline"},
		{"/api/v1/pipelines/" + run.String() + "/attempts", "AttemptPage"},
		{"/api/v1/pipelines/" + run.String() + "/test-executions", "TestExecutionPage"},
	}
	for _, p := range paths {
		codeRead(t, e.loginEnv, e.member, p.path, p.schema)
		codeRead(t, e.loginEnv, e.admin, p.path, p.schema)
	}
	// The other user's decision is unchanged. A cached allowance must expire at most sixty seconds later.
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{{UserID: e.adminID.String(), Decision: "allowed"}}})
	e.clock.Advance(61 * time.Second)
	for _, p := range paths {
		denied := e.do(t, e.member, http.MethodGet, p.path, nil)
		require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
		assert.NotContains(t, denied.Body.String(), "Visible MR")
		codeRead(t, e.loginEnv, e.admin, p.path, p.schema)
	}
}
