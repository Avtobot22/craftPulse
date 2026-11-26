package app

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _tasksPath = "/api/v1/tasks"

func TestTaskViews_CardAndCandidateDecision(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	task, mr, pipeline, oldPipeline, repo := seedTaskCard(t, e)
	detailPath := _tasksPath + "/" + task.String()
	read := func() openapi.TaskDetail {
		return decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, detailPath, "TaskDetail"))
	}
	initial := read()
	require.Len(t, initial.Links, 1)
	assert.Equal(t, openapi.LinkStatusCandidate, initial.Links[0].Status)
	assert.Equal(t, mr, initial.Links[0].To.Id)
	require.NotNil(t, initial.AgeSeconds)
	assert.Positive(t, *initial.AgeSeconds)
	require.Len(t, initial.BlockedIntervals, 1)
	assert.Nil(t, initial.BlockedIntervals[0].To)
	assert.Nil(t, initial.Target)
	assert.Nil(t, initial.OverTarget)
	historyPath := detailPath + "/events"
	history := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, historyPath, "TimelinePage"))
	require.NotEmpty(t, history.Data)
	for _, event := range history.Data {
		assert.Equal(t, task, event.Object.Id)
	}

	e.grantManager(t)
	res := e.json(t, e.admin, http.MethodPost, "/api/v1/links/"+initial.Links[0].Id.String()+"/decisions",
		http.Header{"Idempotency-Key": {uuid.NewString()}}, map[string]string{"decision": "confirm"})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	detail := read()
	require.Len(t, detail.Links, 2)
	assert.Equal(t, mr, detail.Links[0].To.Id)
	assert.Equal(t, openapi.LinkStatusConfirmed, detail.Links[0].Status)
	assert.Equal(t, pipeline, detail.Links[1].To.Id)
	assert.Equal(t, mr, detail.Links[1].FromObjectId)
	assert.Equal(t, openapi.MrRun, detail.Links[1].LinkType)
	assert.False(t, slices.ContainsFunc(detail.Links, func(link openapi.Link) bool { return link.To.Id == oldPipeline }))
	assert.True(t, slices.ContainsFunc(detail.Observations, func(o taskObservationBody) bool {
		return o.Kind == openapi.TaskDetailObservationsKindAwaitingReviewer &&
			o.Action == openapi.AssignReviewer && len(o.Basis) == 1 && o.Basis[0].Id == mr
	}))
	assert.Equal(t, http.StatusOK, e.do(t, e.admin, http.MethodGet, "/api/v1/pipelines/"+pipeline.String(), nil).Code)
	history = decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, historyPath, "TimelinePage"))
	assert.True(t, slices.ContainsFunc(history.Data, func(event openapi.TimelineEvent) bool {
		return event.Object.Id == task && event.EventKind == "field_change" && event.Field != nil &&
			*event.Field == "customfield_10020" && event.Object.SourceUrl != nil
	}))
	assert.True(t, slices.ContainsFunc(history.Data, func(event openapi.TimelineEvent) bool {
		return event.Object.Id == mr && event.EventKind == "ready" && event.Object.SourceUrl != nil &&
			strings.HasSuffix(*event.Object.SourceUrl, "/-/merge_requests/7#note_51")
	}))
	assert.True(t, slices.ContainsFunc(history.Data, func(event openapi.TimelineEvent) bool {
		return event.Object.Id == pipeline && event.EventKind == "pipeline_started" && event.Object.SourceUrl != nil &&
			strings.HasSuffix(*event.Object.SourceUrl, "/-/pipelines/5001")
	}))
	first := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, historyPath+"?limit=1", "TimelinePage"))
	require.Len(t, first.Data, 1)
	require.NotNil(t, first.NextCursor)
	query := url.Values{"limit": {"1"}, "cursor": {*first.NextCursor}}
	second := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.admin, historyPath+"?"+query.Encode(), "TimelinePage"))
	require.Len(t, second.Data, 1)
	require.GreaterOrEqual(t, len(history.Data), 2)
	assert.Equal(t, history.Data[:2], append(first.Data, second.Data...))

	// Move the repository under a different project. The developer can still read the tracker task.
	e.link(t, _otherSub, "demo-person-02")
	_, err := e.owner.Exec(t.Context(), "DELETE FROM org.project_area WHERE area_id = $1", repo)
	require.NoError(t, err)
	external := uuid.New()
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.project (organization_id,id,name,status,created_at) VALUES ($1,$2,'Private repository project','active',$3)", uuid.UUID(e.orgID), external, e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		uuid.UUID(e.orgID), external, repo, e.clock.Now())
	require.NoError(t, err)
	limited := codeRead(t, e.loginEnv, e.member, detailPath, "TaskDetail")
	masked := decode[openapi.TaskDetail](t, limited)
	assert.Empty(t, masked.Links)
	assert.ElementsMatch(t, []string{"links", "observations"}, masked.Redactions)
	for _, marker := range []string{mr.String(), pipeline.String(), "Private ready MR", "!7", "#5001", "awaiting_reviewer"} {
		assert.NotContains(t, limited.Body.String(), marker)
	}
	maskedHistory := decode[openapi.TimelinePage](t, codeRead(t, e.loginEnv, e.member, historyPath, "TimelinePage"))
	assert.True(t, maskedHistory.AccessLimited)
	require.NotEmpty(t, maskedHistory.Data)
	for _, event := range maskedHistory.Data {
		assert.Equal(t, task, event.Object.Id)
	}
	page := taskPage(t, e.loginEnv, e.member, url.Values{"project_id": {e.project.String()}})
	assert.True(t, page.AccessLimited)
	assert.Contains(t, taskPageIDs(page), task)

	roles := e.do(t, e.admin, http.MethodGet, "/api/v1/users/"+e.memberID.String()+"/roles", nil)
	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, roles.Header().Get("ETag"), []roleInput{{Role: "developer", ProjectID: &external}}).Code)
	denied := e.do(t, e.member, http.MethodGet, detailPath, nil)
	assert.Equal(t, http.StatusNotFound, denied.Code)
	assert.NotContains(t, denied.Body.String(), "Payment retry")
	deniedHistory := e.do(t, e.member, http.MethodGet, historyPath, nil)
	assert.Equal(t, http.StatusNotFound, deniedHistory.Code)
	assert.NotContains(t, deniedHistory.Body.String(), "Payment retry")
	assert.NotContains(t, taskPageIDs(taskPage(t, e.loginEnv, e.member, nil)), task)
}

type taskObservationBody = struct {
	Action   openapi.TaskDetailObservationsAction    `json:"action"`
	Basis    []openapi.ObjectRef                     `json:"basis"`
	Kind     openapi.TaskDetailObservationsKind      `json:"kind"`
	Question *openapi.TaskDetailObservationsQuestion `json:"question,omitempty"`
	Since    *time.Time                              `json:"since"`
}

func seedTaskCard(t *testing.T, e *reportEnv) (task, mr, run, previousRun, area uuid.UUID) {
	t.Helper()
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	submit := e.mrImporter(t, admin)
	rc := submit(mrRepository(t, "42", "team/task-card", "r1"))
	repo, err := e.runner.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, "42")
	require.NoError(t, err)
	area = uuid.UUID(repo)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		uuid.UUID(e.orgID), e.project, area, e.clock.Now())
	require.NoError(t, err)
	ready := _linkMerged
	change := mrRecord(t, "task_event", "card-blocked", "20004", "card-blocked", envelope.TaskEventV1{
		TaskNativeId: "20004", EventKind: envelope.TaskEventV1EventKindFieldChange,
		FieldNativeId: ptr("customfield_10020"), AfterValue: ptr("Impediment"),
	})
	change.RecordClass, change.EventTime = "event", &ready
	e.collect(t, jiraEvents("10000"), change)
	submit(mrRecord(t, "merge_request", "9001", "", "v1", envelope.MergeRequestV1{
		ProjectNativeId: "42", Iid: ptr(7), Title: ptr("Private ready MR"), State: "open",
		SourceBranch: "feature/PA-4-retry", TargetBranch: "main", HeadSha: _headB,
		OpenedAt: ready.Add(-time.Hour), ReadyAt: &ready, UpdatedAt: &ready,
		ReviewerField: ptr(true), ReviewerAccountNativeIds: ptr([]string{}),
	}), mrRevision(t, "9001", "previous-head", "previous-head", _headA),
		mrEvent(t, "9001", "note/51", "ready-note", "ready", ready, true))
	e.codeImporter(t, admin)(ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headB, status: "success", revision: "p1", updated: ready, mr: "9001"}),
		ciRun(t, runSpec{id: "5002", project: "42", sha: _headA, status: "failed", revision: "p1", updated: ready, mr: "9001"}))
	return uuid.UUID(e.objectID(t, "task", "20004")), uuid.UUID(e.objectID(t, "merge_request", "9001")),
		uuid.UUID(e.objectID(t, "ci_run", "5001")), uuid.UUID(e.objectID(t, "ci_run", "5002")), area
}

func (e *reportEnv) grantManager(t *testing.T) {
	t.Helper()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment
 (organization_id,user_id,role,project_id,assigned_by,assigned_at) VALUES ($1,$2,'project_manager',$3,$2,$4)`,
		uuid.UUID(e.orgID), e.adminID, e.project, e.clock.Now())
	require.NoError(t, err)
}

func TestTaskViews_PagesTargetsAndSourceRights(t *testing.T) {
	t.Parallel()
	e := newReportEnv(t, true)
	e.link(t, _otherSub, "demo-person-02")
	task := uuid.UUID(e.objectID(t, "task", "20004"))
	secure := uuid.UUID(e.objectID(t, "task", "20002"))
	_, err := e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET restricted = true WHERE id = $1", secure)
	require.NoError(t, err)
	e.rights(t, testadapter.Access{Default: "allowed", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "object", NativeID: "20002", Decision: "denied", ReasonCode: "security_level"},
	}})
	assert.Equal(t, http.StatusNotFound, e.do(t, e.member, http.MethodGet, _tasksPath+"/"+secure.String(), nil).Code)
	visible := taskPage(t, e.loginEnv, e.member, url.Values{"project_id": {e.project.String()}})
	assert.True(t, visible.AccessLimited)
	assert.NotContains(t, taskPageIDs(visible), secure)
	assert.Contains(t, taskPageIDs(visible), task)

	base := url.Values{"team_id": {e.team.String()}, "sort": {"-age"}}
	want := taskPage(t, e.loginEnv, e.admin, base)
	require.Greater(t, len(want.Data), 2)
	base.Set("limit", "1")
	var got []openapi.TaskSummary
	for range len(want.Data) {
		page := taskPage(t, e.loginEnv, e.admin, base)
		require.Len(t, page.Data, 1)
		got = append(got, page.Data...)
		if page.NextCursor == nil {
			break
		}
		base.Set("cursor", *page.NextCursor)
		e.clock.Advance(20 * time.Second)
	}
	assert.Equal(t, want.Data, got)
	base.Set("sort", "key")
	assert.Equal(t, http.StatusBadRequest, e.do(t, e.admin, http.MethodGet, _tasksPath+"?"+base.Encode(), nil).Code)

	e.seedTargetTasks(t)
	withTarget := uuid.UUID(e.objectID(t, "task", "36001"))
	unknown := e.objectID(t, "task", "36002")
	unknownAge := decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, _tasksPath+"/"+unknown.String(), "TaskDetail"))
	assert.Nil(t, unknownAge.AgeSeconds)
	res := e.confirm(t, e.admin, _targetVersions, `"0"`, uuid.New(), manualTarget(_day))
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	filtered := taskPage(t, e.loginEnv, e.admin, url.Values{
		"project_id": {e.project.String()}, "status_meaning": {"active"}, "over_target": {"true"}, "sort": {"age"},
	})
	require.Contains(t, taskPageIDs(filtered), withTarget)
	for _, row := range filtered.Data {
		card := decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, _tasksPath+"/"+row.Id.String(), "TaskDetail"))
		assert.Equal(t, row.AgeSeconds, card.AgeSeconds)
		assert.Equal(t, row.OverTarget, card.OverTarget)
		assert.Equal(t, row.Status, card.Status)
		require.NotNil(t, card.Target)
		assert.Equal(t, _day, card.Target.DurationSeconds)
		assert.Equal(t, 1, card.Target.VersionNo)
	}
}

func (e *reportEnv) seedTargetTasks(t *testing.T) {
	t.Helper()
	e.collect(t, jiraTasks("10000"), mrRecord(t, "task", "36001", "", "target-v1", envelope.TaskV1{
		ProjectNativeId: "10000", TaskKey: ptr("PA-36001"), Title: ptr("Target task"), TypeNativeId: "10001",
		StatusNativeId: "3", CreatedAt: e.day(0), Size: ptr("S"),
	}), mrRecord(t, "task", "36002", "", "unknown-start-v1", envelope.TaskV1{
		ProjectNativeId: "10000", TaskKey: ptr("PA-36002"), Title: ptr("Started before available history"), TypeNativeId: "10001",
		StatusNativeId: "3", CreatedAt: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), Size: ptr("S"),
	}))
}

func TestTaskViews_APIKeyProjects(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	secret := "cpk_" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	hash := sha256.Sum256([]byte(secret))
	id := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO auth.api_key
 (organization_id, id, token_hash, owner_user_id, created_by, created_at, expires_at) VALUES ($1,$2,$3,$4,$4,$5,$6)`,
		uuid.UUID(e.orgID), id, hash[:], e.adminID, e.clock.Now(), e.clock.Now().Add(time.Hour))
	require.NoError(t, err)
	b := newBrowser()
	h := http.Header{"Authorization": {"Bearer " + secret}}
	unbound := e.do(t, b, http.MethodGet, _tasksPath, h)
	require.Equal(t, http.StatusOK, unbound.Code, unbound.Body.String())
	assert.Empty(t, decode[openapi.TaskPage](t, unbound).Data)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO auth.api_key_grant (organization_id,api_key_id,team_id) VALUES ($1,$2,$3)",
		uuid.UUID(e.orgID), id, e.team)
	require.NoError(t, err)
	list := e.do(t, b, http.MethodGet, _tasksPath+"?status_meaning=unknown", h)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	validateSourceResponse(t, list, "TaskPage")
	page := decode[openapi.TaskPage](t, list)
	require.NotEmpty(t, page.Data)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, b, http.MethodGet, _tasksPath+"/"+page.Data[0].Id.String(), h).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, b, http.MethodGet, _tasksPath+"?token="+secret, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, b, http.MethodGet, _tasksPath, h).Code)
}

func taskPage(t *testing.T, e *loginEnv, b *browser, q url.Values) openapi.TaskPage {
	t.Helper()
	return decode[openapi.TaskPage](t, codeRead(t, e, b, _tasksPath+"?"+q.Encode(), "TaskPage"))
}

func taskPageIDs(page openapi.TaskPage) []uuid.UUID {
	out := make([]uuid.UUID, len(page.Data))
	for i := range page.Data {
		out[i] = page.Data[i].Id
	}
	return out
}
