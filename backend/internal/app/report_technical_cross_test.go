package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestReport_TechnicalCrossScopesAndCohort(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	ctx := t.Context()
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	decisions, err := newDecision(e.app, nil, e.clock)
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: e.process, decisions: decisions}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	tenants := platform.NewRepo(e.app)
	tx := persistent.NewTransactor(e.app)
	var seq int64
	require.NoError(t, tx.InOrgTx(ctx, e.orgID, func(ctx context.Context) error {
		var err error
		seq, _, err = tenants.Clock(ctx, e.orgID)
		return err
	}))
	mapping, ok, err := e.process.MappingAt(ctx, e.orgID, entity.ID(e.team), seq)
	require.NoError(t, err)
	require.True(t, ok)
	mapping.Rules = append(mapping.Rules, entity.MappingRule{
		InstanceID: entity.ID(e.instance), Kind: entity.RuleCodeRequiredType, NativeRef: "10001", Meaning: entity.MeaningCodeRequired,
	})
	_, _, err = e.process.CreateMappingVersion(ctx, admin, entity.ID(e.team), mapping.VersionNo, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: mapping.Timezone, Rules: mapping.Rules,
	})
	require.NoError(t, err)
	submit := e.codeImporter(t, admin)
	rc := submit(mrManifest("42"), mrRepository(t, "42", "team/repo-42", "v1"))
	area, err := e.runner.norm.AreaOf(ctx, e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, "42")
	require.NoError(t, err)
	_, err = e.owner.Exec(ctx, "INSERT INTO org.project_area (organization_id, project_id, area_id, valid, known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		uuid.UUID(e.orgID), e.project, uuid.UUID(area), e.day(0))
	require.NoError(t, err)
	mrRecordOf := func(id string, iid int, head string) *envelope.Record {
		at := e.day(2)
		return mrRecord(t, "merge_request", id, "", "v1", envelope.MergeRequestV1{
			ProjectNativeId: "42", Iid: &iid, State: "open", HeadSha: head, OpenedAt: at, UpdatedAt: &at,
			SourceBranch: "feature", TargetBranch: "main",
		})
	}
	submit(mrManifest("42"), mrRecordOf("9001", 1, _headA), mrRecordOf("9002", 2, _headB))
	task := e.objectID(t, "task", "31003")
	mr1, mr2 := e.objectID(t, "merge_request", "9001"), e.objectID(t, "merge_request", "9002")
	run := ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "v1", updated: e.day(4), mr: "9001"})
	var pipeline map[string]any
	require.NoError(t, json.Unmarshal(run.Payload, &pipeline))
	pipeline["started_at"], pipeline["finished_at"] = e.day(3), e.day(4)
	run.Payload = mustMarshal(t, pipeline)
	submit(ciManifest("42"), run,
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "tests", fingerprint: _linux, no: 1, status: "failed", seconds: 60}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "tests", fingerprint: _linux, no: 2, status: "success", seconds: 45}))
	runID := e.objectID(t, "ci_run", "5001")
	links := linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(e.app), Tenants: tenants, Tx: tx, Clock: e.clock, NewID: newID})
	store := changerepo.NewRepo(e.app)
	require.NoError(t, tx.InOrgTx(ctx, e.orgID, func(ctx context.Context) error {
		for _, mr := range []entity.ID{mr1, mr2} {
			if _, err := links.Record(ctx, e.orgID, entity.LinkProposal{Type: entity.LinkTaskMR, From: task, To: mr, Method: entity.LinkSourceExplicit}); err != nil {
				return err
			}
		}
		seq, _, err := tenants.Clock(ctx, e.orgID)
		if err != nil {
			return err
		}
		profile := entity.CheckProfileVersion{
			ID: entity.ID(uuid.New()), ProjectID: entity.ID(e.project), VersionNo: 1, KnownSeq: seq,
			ConfirmedBy: admin.UserID, ConfirmedAt: e.day(0), ConfirmationID: entity.ID(uuid.New()), ContentHash: make([]byte, 32),
		}
		if err := store.InsertProfileVersion(ctx, e.orgID, profile); err != nil {
			return err
		}
		finding := entity.SummaryFinding{Lifecycle: entity.FindingNew, FindingCoordinates: entity.FindingCoordinates{
			FindingAccess: entity.FindingAccess{AreaID: area, Kind: entity.FindingSecurity}, FindingID: entity.ID(uuid.New()),
		}}
		code := entity.TechnicalStream{ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisCode}, Complete: true, Comparable: true}
		security := entity.TechnicalStream{ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisSAST}, Complete: true, Comparable: true, Findings: []entity.SummaryFinding{finding}}
		for i, rev := range []*entity.SummaryRevision{
			{MRID: mr1, HeadSHA: _headA, PipelineState: "complete", Technical: entity.TechnicalSummary{Streams: []entity.TechnicalStream{code, security}}},
			{MRID: mr2, HeadSHA: _headA, PipelineState: "complete", Technical: entity.TechnicalSummary{Streams: []entity.TechnicalStream{code}}},
			{MRID: mr2, HeadSHA: _headB, PipelineState: "reports_missing", Technical: entity.TechnicalSummary{Streams: []entity.TechnicalStream{
				{ExpectedReport: entity.ExpectedReport{Kind: entity.AnalysisJUnit}},
			}}},
		} {
			rev.BuiltAt, rev.KnowledgeSeq, rev.ProfileVersionID, rev.InputHash = e.clock.Now(), seq, profile.ID, make([]byte, 32)
			// The summary was built after the historical events. Its data still belongs at their date.
			rev.Technical.EffectiveAt = e.day(4)
			rev.InputHash[0] = byte(i)
			if _, err := store.SaveSummary(ctx, e.orgID, rev); err != nil {
				return err
			}
		}
		return nil
	}))

	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.adminID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.roles(t, e.admin, e.adminID, rec.Header().Get("ETag"), []roleInput{
		{Role: "administrator"}, {Role: "process_owner", TeamID: &e.team}, {Role: "security_access", ProjectID: &e.project},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, rec.Header().Get("ETag"),
		[]roleInput{{Role: "dev_lead", TeamID: &e.team}}).Code)
	e.link(t, _otherSub, "demo-person-02")
	published := e.publish(t)
	assert.Equal(t, ptr(1.0), published.value(t, "X03", "warning", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X03", "new_security", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X02", "missing_report", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X04", "fail_to_pass", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X05", "current_head_ci", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X05", "complete_reports", e.week).Numerator)
	assert.Equal(t, ptr(1.0), published.value(t, "X06", "sast:new", e.week).Numerator)
	aged := e.readReport(t, e.admin, url.Values{"min_task_age_seconds": {"999999999"}})
	assert.Nil(t, aged.value(t, "X03", "warning", e.week).Numerator)
	report := decode[openapi.TeamReport](t, e.report(t, e.admin, nil))
	require.True(t, slices.ContainsFunc(report.Metrics, func(s openapi.MetricSeries) bool {
		return s.MetricId == "X03" && s.Group != nil && s.Group.Signal != nil && *s.Group.Signal == "new_security"
	}))
	cohort := func(b *browser) openapi.CohortPage {
		t.Helper()
		q := url.Values{"metric_id": {"X03"}, "estimator": {"warning"}, "point_start": {e.week.Format(time.RFC3339)}, "role": {"numerator"}}
		page := e.checked(t, b, "/api/v1/revisions/"+published.Revision.ID.String()+"/cohort?"+q.Encode())
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		return decode[openapi.CohortPage](t, page)
	}
	full := cohort(e.admin)
	require.Len(t, full.Data, 1)
	assert.Equal(t, uuid.UUID(task), full.Data[0].Object.Id)
	assert.ElementsMatch(t, []entity.ID{mr1, mr2, runID}, crossBasisIDs(&full.Data[0]))
	require.NotNil(t, full.Data[0].Signals)
	assert.Contains(t, *full.Data[0].Signals, "new_security")

	pipelineQuery := url.Values{"metric_id": {"X04"}, "estimator": {"fail_to_pass"}, "point_start": {e.week.Format(time.RFC3339)}, "role": {"numerator"}}
	pipelineResponse := e.checked(t, e.admin, "/api/v1/revisions/"+published.Revision.ID.String()+"/cohort?"+pipelineQuery.Encode())
	require.Equal(t, http.StatusOK, pipelineResponse.Code, pipelineResponse.Body.String())
	pipelinePage := decode[openapi.CohortPage](t, pipelineResponse)
	require.Len(t, pipelinePage.Data, 1)
	assert.Equal(t, uuid.UUID(runID), pipelinePage.Data[0].Object.Id)
	assert.Equal(t, "pipeline", string(pipelinePage.Data[0].Object.Kind))
	assert.ElementsMatch(t, []entity.ID{task, mr1}, crossBasisIDs(&pipelinePage.Data[0]))

	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	narrow := e.readReport(t, e.member, nil)
	assert.Equal(t, published.Revision.ID, narrow.Revision.ID)
	assert.Nil(t, narrow.value(t, "X03", "new_security", e.week).Numerator)
	assert.Nil(t, narrow.value(t, "X06", "sast:new", e.week).Numerator)
	assert.Empty(t, cohort(e.member).Data)
	assert.Equal(t, ptr(1.0), narrow.value(t, "X02", "missing_report", e.week).Numerator)
}
