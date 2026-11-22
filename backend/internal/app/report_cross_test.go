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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestReport_ReviewAndCIContextOnSameRevision(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	submit := e.codeImporter(t, admin)
	ctx := t.Context()
	addRepo := func(native string) entity.ID {
		t.Helper()
		rc := submit(mrManifest(native), mrRepository(t, native, "team/repo-"+native, "v1"))
		area, err := e.runner.norm.AreaOf(ctx, e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, native)
		require.NoError(t, err)
		_, err = e.owner.Exec(ctx, "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
			uuid.UUID(e.orgID), e.project, uuid.UUID(area), e.clock.Now())
		require.NoError(t, err)
		return area
	}
	visibleRepo, hiddenRepo := addRepo("42"), addRepo("43")
	ready, finish := e.day(2), e.day(4)
	mrRecordOf := func(id, repo, head, revision string, reviewerField bool, updated time.Time) *envelope.Record {
		t.Helper()
		return mrRecord(t, "merge_request", id, "", revision, envelope.MergeRequestV1{
			ProjectNativeId: repo, Iid: ptr(7), Title: ptr("Payment retry"), State: "open",
			SourceBranch: "feature/payment-retry", TargetBranch: "main", HeadSha: head,
			OpenedAt: ready.Add(-time.Hour), ReadyAt: &ready, UpdatedAt: &updated,
			ReviewerField: &reviewerField, ReviewerAccountNativeIds: &[]string{},
		})
	}
	submit(mrManifest("42"), mrRecordOf("9001", "42", _headA, "v1", true, ready),
		mrRecordOf("9003", "42", _headA, "v1", false, ready), reviewEvent(t, "ready-cross", "ready", "1", ready))
	submit(mrManifest("43"), mrRecordOf("9002", "43", _headA, "v1", true, ready),
		mrEvent(t, "9002", "hidden-review", "r1", "review", e.day(3), false))
	visibleMR, hiddenMR, unknownMR := e.objectID(t, "merge_request", "9001"), e.objectID(t, "merge_request", "9002"), e.objectID(t, "merge_request", "9003")
	task, unknownTask := e.objectID(t, "task", "20004"), e.objectID(t, "task", "31003")
	tx := persistent.NewTransactor(e.app)
	links := linkuc.New(linkuc.Deps{
		Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Clock: e.clock, NewID: newID,
		Auditor: audit.New(auditrepo.NewRepo(e.app), tx, e.clock),
	})
	require.NoError(t, tx.InOrgTx(ctx, e.orgID, func(ctx context.Context) error {
		for _, endpoints := range [][2]entity.ID{{task, visibleMR}, {task, hiddenMR}, {unknownTask, unknownMR}} {
			if _, err := links.Record(ctx, e.orgID, entity.LinkProposal{
				Type: entity.LinkTaskMR, From: endpoints[0], To: endpoints[1], Method: entity.LinkSourceExplicit,
			}); err != nil {
				return err
			}
		}
		return nil
	}))

	run := func(id, repo, mr, status string) *envelope.Record {
		t.Helper()
		record := ciRun(t, runSpec{id: id, project: repo, sha: _headA, status: status, revision: "v1", updated: finish, mr: mr})
		var payload map[string]any
		require.NoError(t, json.Unmarshal(record.Payload, &payload))
		payload["started_at"], payload["finished_at"] = e.day(3), finish
		record.Payload = mustMarshal(t, payload)
		return record
	}
	submit(ciManifest("42"), run("5001", "42", "9001", "success"),
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "tests", fingerprint: _linux, no: 1, status: "failed", seconds: 60}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "tests", fingerprint: _linux, no: 2, status: "success", seconds: 45}))
	submit(ciManifest("43"), run("5002", "43", "9002", "failed"))
	visibleRun, hiddenRun := e.objectID(t, "ci_run", "5001"), e.objectID(t, "ci_run", "5002")

	r := e.publish(t)
	review := r.value(t, "X01", "no_reviewer", e.week)
	require.NotNil(t, review.Numerator)
	assert.InDelta(t, 1, *review.Numerator, 0)
	assert.InDelta(t, 1, *review.Denominator, 0)
	assert.InDelta(t, 1, *r.value(t, "X02", "failed", e.week).Numerator, 0)
	assert.InDelta(t, 1, *r.value(t, "X02", "passed_after_retry", e.week).Numerator, 0)
	missing := r.value(t, "X02", "missing_report", e.week)
	assert.Equal(t, "NOT_EVALUATED", missing.State)
	assert.Equal(t, ptr("expected_report_profile_unavailable"), missing.ReasonCode)
	assert.Nil(t, missing.Value)
	raw := e.report(t, e.admin, nil)
	require.Equal(t, http.StatusOK, raw.Code, raw.Body.String())
	report := decode[openapi.TeamReport](t, raw)
	assert.Subset(t, crossReportMetrics(&report), []string{"A05", "A06", "A07", "A10", "A22", "X01", "X02"})

	cohort := func(b *browser, revision uuid.UUID, metric, estimator string, role string) openapi.CohortPage {
		t.Helper()
		q := url.Values{"metric_id": {metric}, "estimator": {estimator}, "point_start": {e.week.Format(time.RFC3339)}, "role": {role}}
		rec := e.checked(t, b, "/api/v1/revisions/"+revision.String()+"/cohort?"+q.Encode())
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return decode[openapi.CohortPage](t, rec)
	}
	visible := cohort(e.admin, r.Revision.ID, "X01", "no_reviewer", "numerator")
	require.Len(t, visible.Data, 1)
	it := &visible.Data[0]
	assert.Equal(t, uuid.UUID(task), it.Object.Id)
	assert.ElementsMatch(t, []entity.ID{visibleMR, visibleRun, hiddenMR, hiddenRun}, crossBasisIDs(it))
	assert.NotNil(t, it.TaskAgeSeconds)
	assert.Equal(t, ptr(float32(e.next.Sub(ready).Seconds())), it.Value)
	unknown := cohort(e.admin, r.Revision.ID, "X01", "no_reviewer", "missing")
	require.Len(t, unknown.Data, 1)
	assert.Equal(t, uuid.UUID(unknownTask), unknown.Data[0].Object.Id)
	assert.Equal(t, ptr(entity.ReasonReviewerUnknown), unknown.Data[0].ReasonCode)

	crossCardParity(t, e, links, r.Revision.ID, task, visibleRepo, hiddenRepo, it.Signals)

	// Restrict the imported area through the same access adapter that collects the tasks.
	_, err := e.owner.Exec(ctx, `INSERT INTO connect.connection_scope
 (organization_id,connection_id,instance_id,scope_kind,native_scope_id,display_name,area_id,status,added_at)
 VALUES ($1,$2,$3,'repository','43','Hidden repository',$4,'active',$5)`, uuid.UUID(e.orgID), e.conn, e.instance, uuid.UUID(hiddenRepo), e.clock.Now())
	require.NoError(t, err)
	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, rec.Header().Get("ETag"),
		[]roleInput{{Role: "dev_lead", TeamID: &e.team}}).Code)
	e.link(t, _otherSub, "demo-person-02")
	e.rights(t, testadapter.Access{Default: "allowed", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "area", NativeID: "43", Decision: "denied"},
	}})
	e.clock.Advance(61 * time.Second)
	narrow := e.readReport(t, e.member, nil)
	assert.Equal(t, r.Revision.ID, narrow.Revision.ID)
	assert.True(t, narrow.AccessLimited)
	assert.InDelta(t, 0, *narrow.value(t, "X02", "failed", e.week).Numerator, 0)
	narrowCohort := cohort(e.member, r.Revision.ID, "X01", "no_reviewer", "numerator")
	require.Len(t, narrowCohort.Data, 1)
	assert.True(t, narrowCohort.AccessLimited)
	assert.ElementsMatch(t, []entity.ID{visibleMR, visibleRun}, crossBasisIDs(&narrowCohort.Data[0]))
	assert.NotContains(t, *narrowCohort.Data[0].Signals, "ci_failed")
	hiddenGroup := url.Values{"metric_id": {"A22"}, "estimator": {"unknown:mr:review"}, "point_start": {e.week.Format(time.RFC3339)}}
	path := "/api/v1/revisions/" + r.Revision.ID.String() + "/cohort?" + hiddenGroup.Encode()
	assert.Equal(t, http.StatusOK, e.checked(t, e.admin, path).Code)
	assert.Equal(t, http.StatusNotFound, e.checked(t, e.member, path).Code)

	submit(mrManifest("42"), mrRecordOf("9001", "42", _headB, "v2", true, e.day(5)))
	e.recount(t)
	newReport := e.readReport(t, e.admin, nil)
	require.NotEqual(t, r.Revision.ID, newReport.Revision.ID)
	newHead := newReport.value(t, "X02", "passed_after_retry", e.week)
	assert.Equal(t, "UNKNOWN", newHead.State)
	assert.Nil(t, newHead.Numerator)
	assert.InDelta(t, 1, *e.readReport(t, e.admin, url.Values{"revision_id": {r.Revision.ID.String()}}).
		value(t, "X02", "passed_after_retry", e.week).Numerator, 0)

	dailyQuery := url.Values{"step": {"day"}}
	assert.Equal(t, http.StatusNotFound, e.report(t, e.admin, dailyQuery).Code)
	e.recount(t)
	daily := decode[openapi.TeamReport](t, e.report(t, e.admin, dailyQuery))
	assert.Equal(t, "day", string(daily.Step))
	for i := range daily.Metrics {
		m := &daily.Metrics[i]
		if m.MetricId != "A22" || m.Group == nil || m.Group.EventKind == nil || *m.Group.EventKind != "mr:ready" {
			continue
		}
		require.NotNil(t, m.Group.ActorKind)
		assert.Equal(t, "unknown", string(*m.Group.ActorKind))
		return
	}
	t.Fatal("daily revision did not contain the ready event group")
}

func crossCardParity(t *testing.T, e *reportEnv, links *linkuc.UseCase, revision uuid.UUID, task, visibleRepo, hiddenRepo entity.ID, signals *[]string) {
	t.Helper()
	ctx := t.Context()
	rev, err := e.calc.RevisionOf(ctx, e.orgID, entity.ID(revision))
	require.NoError(t, err)
	facts, err := e.runner.norm.AreaFacts(ctx, e.orgID, entity.ID(e.area), rev.KnowledgeSeq)
	require.NoError(t, err)
	i := slices.IndexFunc(facts.Tasks, func(v entity.TaskVersion) bool { return v.TaskID == task })
	require.NotEqual(t, -1, i)
	mapping, found, err := e.process.MappingAt(ctx, e.orgID, entity.ID(e.team), rev.KnowledgeSeq)
	require.NoError(t, err)
	require.True(t, found)
	in := entity.TaskObservationFacts{
		Task: facts.Tasks[i], TaskEvents: facts.Events, InstanceID: facts.InstanceID,
		Mapping: &mapping, KnowledgeSeq: rev.KnowledgeSeq, AsOf: e.next,
	}
	in.Links, err = links.EdgesAt(ctx, e.orgID, task, entity.LinksFrom, rev.KnowledgeSeq)
	require.NoError(t, err)
	for _, area := range []entity.ID{visibleRepo, hiddenRepo} {
		r, err := e.runner.norm.ReviewFacts(ctx, e.orgID, area, rev.KnowledgeSeq)
		require.NoError(t, err)
		in.Reviews = append(in.Reviews, r)
		for j := range r.MRs {
			edges, err := links.EdgesAt(ctx, e.orgID, r.MRs[j].MRID, entity.LinksFrom, rev.KnowledgeSeq)
			require.NoError(t, err)
			in.Links = append(in.Links, edges...)
		}
		ci, err := e.runner.norm.CIFacts(ctx, e.orgID, area, rev.KnowledgeSeq)
		require.NoError(t, err)
		in.CI.Runs = append(in.CI.Runs, ci.Runs...)
		in.CI.Attempts = append(in.CI.Attempts, ci.Attempts...)
	}
	got := []string{}
	for _, o := range calc.TaskObservations(&in) {
		kind := string(o.Kind)
		switch o.Kind {
		case entity.ObservationAwaitingReviewer, entity.ObservationAwaitingFirst, entity.ObservationCIFailed, entity.ObservationCIRetried:
			if !slices.Contains(got, kind) {
				got = append(got, kind)
			}
		default:
		}
	}
	require.NotNil(t, signals)
	assert.ElementsMatch(t, got, *signals)
}

func crossReportMetrics(r *openapi.TeamReport) []string {
	out := make([]string, 0, len(r.Metrics))
	for i := range r.Metrics {
		out = append(out, r.Metrics[i].MetricId)
	}
	return out
}

func crossBasisIDs(item *openapi.CohortItem) []entity.ID {
	if item.Basis == nil {
		return nil
	}
	ids := make([]entity.ID, 0, len(*item.Basis))
	for _, o := range *item.Basis {
		ids = append(ids, entity.ID(o.Id))
	}
	return ids
}
