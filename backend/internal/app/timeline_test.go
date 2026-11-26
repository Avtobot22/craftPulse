package app

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestProjectTimeline_RevisionActorsAndHistory(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	task, mr, _, _, _ := seedTaskCard(t, e)
	e.grantManager(t)
	detail := decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, _tasksPath+"/"+task.String(), "TaskDetail"))
	require.Len(t, detail.Links, 1)
	res := e.json(t, e.admin, http.MethodPost, "/api/v1/links/"+detail.Links[0].Id.String()+"/decisions",
		http.Header{"Idempotency-Key": {uuid.NewString()}}, map[string]string{"decision": "confirm"})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	old := e.publish(t)
	path := "/api/v1/projects/" + e.project.String() + "/timeline?"
	from := _linkMerged.Add(-2 * time.Hour)
	to := _linkMerged.Add(2 * time.Hour)
	q := url.Values{"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
	read := func() openapi.TimelinePage {
		return decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, path+q.Encode(), "TimelinePage"))
	}
	q.Set("revision_id", old.Revision.ID.String())
	require.NotEmpty(t, read().Data)

	e.clock.Advance(time.Hour)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	submit := e.codeImporter(t, admin)
	start := _linkMerged.Add(-30 * time.Minute).In(time.FixedZone("source", 3*60*60))
	observed := e.clock.Now()
	submit(ciManifest("42"), timelineRun(t, "start", start, nil))
	e.clock.Advance(time.Minute)
	finish := _linkMerged.Add(time.Minute)
	submit(ciManifest("42"), timelineRun(t, "finish", start, &finish),
		timelineAttempt(t, "7701", "failed", 1, start, start.Add(time.Minute)),
		timelineAttempt(t, "7702", "success", 2, start.Add(time.Minute), finish),
	)
	run := uuid.UUID(e.objectID(t, "ci_run", "5701"))
	q.Set("object_kind", "pipeline")
	assert.False(t, slices.ContainsFunc(read().Data, func(event openapi.TimelineEvent) bool { return event.Object.Id == run }))

	timelineActors(t, e, admin)
	backfillAt, webhookAt := _linkMerged.Add(-time.Minute), _linkMerged.Add(time.Second)
	backfill := statusChange(t, "20004", "777001", backfillAt, "JIRAUSER1")
	webhook := statusChange(t, "20004", "777002", webhookAt, "JIRAUSER1")
	require.Equal(t, entity.ReceiptAccepted, e.collectVia(t, entity.ViaBackfill, jiraEvents("10000"), backfill))
	require.Equal(t, entity.ReceiptAccepted, e.collectVia(t, entity.ViaWebhook, jiraEvents("10000"), webhook))
	for range 3 {
		require.Equal(t, entity.ReceiptDuplicate, e.collectVia(t, entity.ViaWebhook, jiraEvents("10000"), webhook))
	}
	// A flow change forces a fresh published cut after the late CI facts arrived.
	e.collectLate(t)
	e.recount(t)
	current := e.readReport(t, e.admin, nil)
	require.NotEqual(t, old.Revision.ID, current.Revision.ID)
	q.Set("revision_id", current.Revision.ID.String())
	pipelines := read().Data
	late := timelineEvent(t, pipelines, run, "pipeline_started")
	assert.True(t, late.Late)
	assert.True(t, start.Equal(late.EventTime))
	assert.True(t, observed.Equal(late.ObservedAt))
	assert.Equal(t, 10800, late.EventOffsetSeconds)
	assert.EqualValues(t, "import", late.ObservedVia)
	assert.EqualValues(t, "system", late.Actor.Kind)
	require.NotNil(t, late.Object.SourceUrl)
	assert.Contains(t, *late.Object.SourceUrl, "/-/pipelines/5701")
	done := timelineEvent(t, pipelines, run, "pipeline_finished")
	require.NotNil(t, done.AfterValue)
	assert.Equal(t, "success", *done.AfterValue)
	for _, id := range []string{"7701", "7702"} {
		attempt := uuid.UUID(e.objectID(t, "ci_attempt", id))
		end := timelineEvent(t, pipelines, attempt, "attempt_finished")
		require.NotNil(t, end.Object.SourceUrl)
		assert.Contains(t, *end.Object.SourceUrl, "/-/jobs/"+id)
	}
	assert.True(t, slices.IsSortedFunc(pipelines, func(a, b openapi.TimelineEvent) int { return b.EventTime.Compare(a.EventTime) }))

	q.Set("object_kind", "merge_request")
	var replies []openapi.TimelineEvent
	events := read().Data
	for i := range events {
		if events[i].Object.Id == mr && events[i].EventKind == "comment" {
			replies = append(replies, events[i])
		}
	}
	require.Len(t, replies, 3)
	assert.ElementsMatch(t, []openapi.ActorRefKind{"human", "bot", "unknown"}, []openapi.ActorRefKind{replies[0].Actor.Kind, replies[1].Actor.Kind, replies[2].Actor.Kind})
	require.NotNil(t, replies[0].Actor.AccountId)
	require.NotNil(t, replies[1].Actor.AccountId)
	assert.NotEqual(t, *replies[0].Actor.AccountId, *replies[1].Actor.AccountId)
	q.Set("actor_kind", "human")
	humans := read().Data
	require.Len(t, humans, 1)
	assert.EqualValues(t, "human", humans[0].Actor.Kind)

	q.Del("actor_kind")
	q.Set("object_kind", "task")
	_, err := e.owner.Exec(t.Context(), "UPDATE sync.state SET history_from = NULL, completeness = 'unknown' WHERE connection_id = $1 AND data_kind = 'tasks'", e.conn)
	require.NoError(t, err)
	unknown := read()
	require.NotNil(t, unknown.Sources)
	require.Len(t, *unknown.Sources, 1)
	assert.Nil(t, unknown.HistoryFrom)
	assert.EqualValues(t, "unknown", (*unknown.Sources)[0].Completeness)
	_, err = e.owner.Exec(t.Context(), "UPDATE sync.state SET history_from = $2, completeness = 'partial' WHERE connection_id = $1 AND data_kind = 'tasks'", e.conn, from)
	require.NoError(t, err)
	page := read()
	require.NotNil(t, page.HistoryFrom)
	assert.True(t, from.Equal(*page.HistoryFrom))
	for _, when := range []time.Time{backfillAt, webhookAt} {
		matching := 0
		for i := range page.Data {
			event := &page.Data[i]
			if event.Object.Id == task && when.Equal(event.EventTime) {
				matching++
				want := "backfill"
				if when.Equal(webhookAt) {
					want = "webhook"
				}
				assert.EqualValues(t, want, event.ObservedVia)
			}
		}
		assert.Equal(t, 1, matching)
	}
	require.NotNil(t, page.Sources)
	require.Len(t, *page.Sources, 1)
	assert.Equal(t, e.conn, (*page.Sources)[0].ConnectionId)
	assert.EqualValues(t, "jira_cloud", (*page.Sources)[0].Provider)
	assert.EqualValues(t, "partial", (*page.Sources)[0].Completeness)
	require.NotNil(t, (*page.Sources)[0].HistoryFrom)
	assert.True(t, from.Equal(*(*page.Sources)[0].HistoryFrom))

	// Only the part of the pipeline within the task→MR link's validity belongs to the task history.
	_, err = e.owner.Exec(t.Context(), "UPDATE link.edge SET valid = tstzrange($3, NULL, '[)') WHERE from_object_id = $1 AND to_object_id = $2 AND status = 'confirmed'",
		task, mr, _linkMerged)
	require.NoError(t, err)
	history := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, _tasksPath+"/"+task.String()+"/events", "TimelinePage"))
	assert.False(t, slices.ContainsFunc(history.Data, func(event openapi.TimelineEvent) bool {
		return event.Object.Id == run && event.EventKind == "pipeline_started"
	}))
	timelineEvent(t, history.Data, run, "pipeline_finished")

	q.Set("limit", "1")
	first := read()
	require.NotNil(t, first.NextCursor)
	q.Set("cursor", *first.NextCursor)
	second := read()
	require.Len(t, second.Data, 1)
	assert.Equal(t, page.Data[:2], append(first.Data, second.Data...))
	q.Set("object_kind", "pipeline")
	assert.Equal(t, http.StatusBadRequest, e.do(t, e.admin, http.MethodGet, path+q.Encode(), nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(t, e.admin, http.MethodGet,
		"/api/v1/projects/"+uuid.NewString()+"/timeline?"+q.Encode(), nil).Code)
}

func TestProjectTimeline_GitLabCIBeforeRepositoryMetadata(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	e.grantManager(t)
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	manifest := ciManifest("42")
	manifest.Area.Key = new("team/subgroup/ci-only")
	start, finish := _linkMerged, _linkMerged.Add(time.Minute)
	rc := e.codeImporter(t, admin)(manifest, timelineRun(t, "finish", start, &finish),
		timelineAttempt(t, "7701", "success", 1, start, finish))
	area, err := e.runner.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, "42")
	require.NoError(t, err)
	var repositories int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM normalize.repository_version WHERE repository_id = $1", uuid.UUID(area)).Scan(&repositories))
	require.Zero(t, repositories)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		uuid.UUID(e.orgID), e.project, uuid.UUID(area), e.clock.Now())
	require.NoError(t, err)

	q := url.Values{"from": {start.Format(time.RFC3339)}, "to": {finish.Add(time.Minute).Format(time.RFC3339)}, "object_kind": {"pipeline"}}
	path := "/api/v1/projects/" + e.project.String() + "/timeline?" + q.Encode()
	page := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, path, "TimelinePage"))
	require.Len(t, page.Data, 4)
	attempt := uuid.UUID(e.objectID(t, "ci_attempt", "7701"))
	for _, event := range page.Data {
		route := "/-/pipelines/5701"
		if event.Object.Id == attempt {
			route = "/-/jobs/7701"
		}
		assert.Equal(t, new(_gitlabInstance+"/team/subgroup/ci-only"+route), event.Object.SourceUrl, event.EventKind)
	}
}

func timelineRun(t *testing.T, revision string, start time.Time, finish *time.Time) *envelope.Record {
	t.Helper()
	status := "running"
	if finish != nil {
		status = "success"
	}
	return mrRecord(t, "ci_run", "5701", "", revision, envelope.CiRunV1{
		ProjectNativeId: "42", RefName: "main", Sha: _headB, MrNativeId: new("9001"),
		NativeStatus: status, Outcome: "unknown", StartedAt: &start, FinishedAt: finish, UpdatedAt: finish,
	})
}

func timelineAttempt(t *testing.T, id, status string, no int, start, finish time.Time) *envelope.Record {
	t.Helper()
	return mrRecord(t, "ci_attempt", id, "", "r1", map[string]any{
		"run_native_id": "5701", "job_key": "test", "job_name": "test", "config_fingerprint": _linux,
		"attempt_no": no, "native_status": status, "outcome": "unknown", "allow_failure": false,
		"started_at": start, "finished_at": finish, "duration_s": int64(finish.Sub(start).Seconds()),
	})
}

func timelineActors(t *testing.T, e *reportEnv, admin entity.Subject) {
	t.Helper()
	submit := e.codeImporter(t, admin)
	records := make([]*envelope.Record, 0, 6)
	for i, native := range []string{"801", "802", "803"} {
		records = append(records, accountRecord(t, native, "Same Name"))
		at := _linkMerged.Add(time.Duration(i+5) * time.Minute)
		event := mrRecord(t, "mr_event", "note/"+native, "9001", "r1", envelope.MrEventV2{
			MrNativeId: "9001", EventKind: "comment", ActorAccountNativeId: &native, HeadSha: new(_headB),
		})
		event.RecordClass, event.PayloadSchema, event.EventTime = "event", "mr_event/2", &at
		records = append(records, event)
	}
	rc := submit(mrManifest("42"), records...)
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	assignments := make([]entity.AssignmentRequest, 0, 2)
	for i, kind := range []entity.PersonKind{entity.PersonHuman, entity.PersonBot} {
		var account uuid.UUID
		require.NoError(t, e.owner.QueryRow(t.Context(),
			"SELECT id FROM org.source_account WHERE instance_id = $1 AND native_account_id = $2",
			uuid.UUID(*rc.SourceInstanceID), []string{"801", "802"}[i]).Scan(&account))
		assignments = append(assignments, entity.AssignmentRequest{SourceAccountID: entity.ID(account), ActorKind: kind, CreatePerson: kind == entity.PersonHuman})
	}
	out, err := orgs.AssignAccounts(t.Context(), admin, entity.ID(uuid.New()), assignments)
	require.NoError(t, err)
	require.Len(t, out, 2)
	for _, item := range out {
		require.Equal(t, entity.AssignmentSaved, item.Outcome)
	}
}

func timelineEvent(t *testing.T, events []openapi.TimelineEvent, object uuid.UUID, kind string) openapi.TimelineEvent {
	t.Helper()
	for i := range events {
		if events[i].Object.Id == object && events[i].EventKind == kind {
			return events[i]
		}
	}
	t.Fatalf("missing %s for %s", kind, object)
	return openapi.TimelineEvent{}
}
