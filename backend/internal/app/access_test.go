package app

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _checkAccess = "check_access"

type accessNoteBody struct {
	Code         string     `json:"code"`
	ConnectionID *uuid.UUID `json:"connection_id"`
}

type projectPageBody struct {
	Data          []named          `json:"data"`
	NextCursor    *string          `json:"next_cursor"`
	AccessLimited bool             `json:"access_limited"`
	AccessNotes   []accessNoteBody `json:"access_notes"`
}

type areaPageBody struct {
	Data []struct {
		ID           uuid.UUID  `json:"id"`
		Kind         string     `json:"kind"`
		DisplayName  string     `json:"display_name"`
		ConnectionID *uuid.UUID `json:"connection_id"`
	} `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

// accessEnv is the collected Jira project 10000 linked to the project Core. The administrator has a role over
// the whole organization and the account demo-person-01; the member is a developer of Core and gets the
// account demo-person-02 only when a test links it.
type accessEnv struct {
	*connectionsEnv
	conn, instance, area, project uuid.UUID
	adminID, memberID             uuid.UUID
	member                        *browser
}

func newAccessEnv(t *testing.T) *accessEnv {
	t.Helper()
	e := &accessEnv{connectionsEnv: newConnectionsEnv(t, testadapter.Config{})}
	e.conn = e.activeConnection(t)
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT instance_id, area_id FROM connect.connection_scope WHERE connection_id = $1",
		e.conn).Scan(&e.instance, &e.area))
	e.member = newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, e.member, _otherSub).Code)
	e.adminID, e.memberID = e.userID(t, e.orgID, _adminSub), e.userID(t, e.orgID, _otherSub)

	setup := emptyStructure()
	setup.Projects = []named{{Name: "Core", Status: "active"}}
	_, tag := e.structure(t, e.admin)
	rec := e.replace(t, e.admin, tag, setup)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	v1 := decode[structureBody](t, rec)
	e.project = *v1.Projects[0].ID
	linked := v1.input()
	linked.ProjectAreas = []any{map[string]any{"project_id": e.project, "area_id": e.area, "valid_from": nil, "valid_to": nil}}
	rec = e.replace(t, e.admin, rec.Header().Get("ETag"), linked)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	read, _ := e.structure(t, e.admin)
	require.Len(t, read.ProjectAreas, 1, "the structure returns the source areas of projects")

	require.Equal(t, http.StatusOK, e.roles(t, e.admin, e.memberID, `"0"`, []roleInput{{Role: "developer", ProjectID: &e.project}}).Code)
	e.link(t, _adminSub, "demo-person-01")
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.adminID.String(), Decision: "allowed"}, {UserID: e.memberID.String(), Decision: "allowed"},
	}})
	return e
}

// link confirms the account of the instance as the person of the user (UC-08).
func (e *accessEnv) link(t *testing.T, subject, native string) {
	t.Helper()
	person := e.person(t, e.orgID, subject)
	var account uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM org.source_account WHERE instance_id = $1 AND native_account_id = $2",
		e.instance, native).Scan(&account))
	code, res := e.assign(t, uuid.New(), decision(account, "human", &person, nil, false))
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, entity.AssignmentSaved, res.Items[0].Outcome)
	_, err := e.owner.Exec(t.Context(), "UPDATE org.app_user SET person_id = $1 WHERE organization_id = $2 AND subject = $3",
		person, uuid.UUID(e.orgID), subject)
	require.NoError(t, err)
}

// rights switches CheckAccess of the test adapter while the core runs.
func (e *accessEnv) rights(t *testing.T, a testadapter.Access) {
	t.Helper()
	require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Access: a}))
}

func (e *accessEnv) projects(t *testing.T, b *browser) projectPageBody {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/projects", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[projectPageBody](t, rec)
}

func (e *accessEnv) sees(t *testing.T, b *browser) bool {
	t.Helper()
	page := e.projects(t, b)
	return slices.ContainsFunc(page.Data, func(p named) bool { return *p.ID == e.project })
}

// tasks reads the tasks and the events of the area for the user the way a report does: the area, then the
// objects with their own restriction in it, then the read with the resulting scope.
func (e *accessEnv) tasks(t *testing.T, user uuid.UUID) (tasks, events []string) {
	t.Helper()
	s := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(user)}
	scope, err := e.connectionsEnv.rights.Authorize(t.Context(), s, entity.AccessRequest{Action: "org.read"})
	require.NoError(t, err)
	area := entity.ID(e.area)
	q := entity.SourceQuery{Operation: "readTasks", Areas: []entity.ID{area}}
	scope, err = e.connectionsEnv.rights.Decide(t.Context(), s, scope, q)
	require.NoError(t, err)
	objects, err := e.runner.norm.RestrictedObjects(t.Context(), scope, e.orgID, q.Areas)
	require.NoError(t, err)
	scope, err = e.connectionsEnv.rights.Decide(t.Context(), s, scope, entity.SourceQuery{Operation: q.Operation, Objects: objects})
	require.NoError(t, err)
	var seq int64
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT last_seq FROM platform.org_clock WHERE organization_id = $1", uuid.UUID(e.orgID)).Scan(&seq))
	versions, err := e.runner.norm.TasksAt(t.Context(), scope, e.orgID, area, seq)
	require.NoError(t, err)
	for i := range versions {
		tasks = append(tasks, e.native(t, versions[i].TaskID))
	}
	evs, err := e.runner.norm.TaskEventsAt(t.Context(), scope, e.orgID, area, seq)
	require.NoError(t, err)
	for i := range evs {
		if native := e.native(t, evs[i].TaskID); !slices.Contains(events, native) {
			events = append(events, native)
		}
	}
	return tasks, events
}

func (e *accessEnv) native(t *testing.T, id entity.ID) string {
	t.Helper()
	var native string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT native_id FROM normalize.source_object WHERE id = $1", uuid.UUID(id)).Scan(&native))
	return native
}

func (e *accessEnv) checks() int { return e.adapter.journal.calls(_checkAccess) }

func TestAccess_ProjectFollowsTheRightInTheSource(t *testing.T) {
	e := newAccessEnv(t)
	e.link(t, _otherSub, "demo-person-02")
	before := e.checks()

	member := e.projects(t, e.member)
	require.Len(t, member.Data, 1, "the developer of Core with the right in the source gets the project")
	assert.Equal(t, e.project, *member.Data[0].ID)
	assert.False(t, member.AccessLimited)
	assert.Empty(t, member.AccessNotes)
	assert.True(t, e.sees(t, e.admin))
	assert.Equal(t, 2, e.checks()-before, "one check per user and area")

	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{{UserID: e.adminID.String(), Decision: "allowed"}}})
	e.clock.Advance(30 * time.Second)
	assert.True(t, e.sees(t, e.member), "the decision of the source is kept up to 60 s")
	e.clock.Advance(29 * time.Second)
	assert.True(t, e.sees(t, e.member))
	assert.True(t, e.sees(t, e.admin))
	assert.Equal(t, 2, e.checks()-before, "repeated reads within 60 s do not call CheckAccess")

	e.clock.Advance(2 * time.Second)
	revoked := e.projects(t, e.member)
	assert.Empty(t, revoked.Data, "61 s after the check the project is gone")
	assert.True(t, revoked.AccessLimited)
	assert.Equal(t, []accessNoteBody{{Code: "restricted_by_access"}}, revoked.AccessNotes, "the note tells neither the number nor the name")
	assert.Nil(t, revoked.NextCursor)
	assert.True(t, e.sees(t, e.admin), "the other user keeps the project")
	assert.Equal(t, 4, e.checks()-before, "after 60 s both users are checked again")
	tasks, _ := e.tasks(t, e.memberID)
	assert.Empty(t, tasks, "the tasks of the closed area are not given")
	denials := e.audit(t, "readTasks")
	require.Len(t, denials, 1, "the refusal is a separate audit record")
	assert.Equal(t, "denied", denials[0].Outcome)
	assert.Equal(t, "source_denied", denials[0].Reason)
}

func TestAccess_AreaThatCannotBeCheckedIsClosed(t *testing.T) {
	e := newAccessEnv(t)
	before := e.checks()
	noAccount := e.projects(t, e.member)
	assert.Empty(t, noAccount.Data, "a user without an account in the instance does not get its data")
	assert.Equal(t, []accessNoteBody{{Code: "restricted_by_access"}}, noAccount.AccessNotes)
	tasks, events := e.tasks(t, e.memberID)
	assert.Empty(t, tasks)
	assert.Empty(t, events)
	assert.Equal(t, before, e.checks(), "without an account the source is not asked")

	e.link(t, _otherSub, "demo-person-02")
	unavailable := []accessNoteBody{{Code: "access_check_unavailable", ConnectionID: &e.conn}}
	cases := []struct {
		name  string
		setup func(t *testing.T)
		want  []accessNoteBody
	}{
		{name: "unknown", want: unavailable, setup: func(t *testing.T) {
			e.rights(t, testadapter.Access{Rules: []testadapter.AccessRule{{UserID: e.memberID.String(), Decision: "unknown"}}})
		}},
		{name: "answer longer than 2 s", want: unavailable, setup: func(t *testing.T) {
			e.rights(t, testadapter.Access{Default: "allowed", DelayMS: 2300})
		}},
		{name: "rate limit pause", want: unavailable, setup: func(t *testing.T) {
			e.rights(t, testadapter.Access{Default: "allowed"})
			_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection SET paused_until = $1 WHERE id = $2", e.clock.Now().Add(10*time.Minute), e.conn)
			require.NoError(t, err)
		}},
		{name: "access lost", want: []accessNoteBody{{Code: "source_access_lost", ConnectionID: &e.conn}}, setup: func(t *testing.T) {
			_, err := e.owner.Exec(t.Context(), `UPDATE connect.connection SET paused_until = NULL, status = 'access_lost', access_lost_at = $1
				WHERE id = $2`, e.clock.Now(), e.conn)
			require.NoError(t, err)
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e.clock.Advance(61 * time.Second)
			tt.setup(t)
			started := time.Now()

			page := e.projects(t, e.member)

			assert.Less(t, time.Since(started), 4*time.Second, "a read fits NFR-001 when the source is slow")
			assert.Empty(t, page.Data)
			assert.True(t, page.AccessLimited)
			assert.Equal(t, tt.want, page.AccessNotes)
		})
	}
	assert.Empty(t, decode[areaPageBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/areas", nil)).Data,
		"listAreas gives only areas of active connections")
	assert.True(t, e.metric(t, `craftpulse_access_check_failures_total{connection_id="`+e.conn.String()+`"}`),
		"failed checks are counted by connection")
}

func TestAccess_TaskWithItsOwnRestriction(t *testing.T) {
	e := newAccessEnv(t)
	e.link(t, _otherSub, "demo-person-02")
	_, err := e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET restricted = true WHERE object_kind = 'task' AND native_id = '20002'")
	require.NoError(t, err)
	e.rights(t, testadapter.Access{Default: "denied", Rules: []testadapter.AccessRule{
		{UserID: e.memberID.String(), ItemKind: "object", NativeID: "20002", Decision: "denied", ReasonCode: "security_level"},
		{UserID: e.memberID.String(), Decision: "allowed"},
		{UserID: e.adminID.String(), Decision: "allowed"},
	}})

	adminTasks, adminEvents := e.tasks(t, e.adminID)
	memberTasks, memberEvents := e.tasks(t, e.memberID)

	assert.Contains(t, adminTasks, "20002")
	assert.Contains(t, adminEvents, "20002")
	assert.Len(t, memberTasks, len(adminTasks)-1)
	assert.NotContains(t, memberTasks, "20002", "a task the source closes is not shown")
	assert.NotContains(t, memberEvents, "20002", "its events do not count")
	assert.True(t, e.sees(t, e.member), "the area itself stays open")
	denials := e.audit(t, "readTasks")
	require.Len(t, denials, 1)
	assert.Equal(t, "source_denied", denials[0].Reason)
	assert.Contains(t, denials[0].Details, "refused")

	none, err := e.runner.norm.TasksAt(t.Context(), entity.Scope{}, e.orgID, entity.ID(e.area), 1<<40)
	require.NoError(t, err)
	assert.Empty(t, none, "a read outside the scope gives nothing")
}

func TestAccess_ListAreas(t *testing.T) {
	e := newAccessEnv(t)

	page := decode[areaPageBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/areas", nil))
	require.Len(t, page.Data, 1)
	a := page.Data[0]
	assert.Equal(t, e.area, a.ID)
	assert.Equal(t, "tracker_project", a.Kind)
	assert.NotEmpty(t, a.DisplayName)
	require.NotNil(t, a.ConnectionID)
	assert.Equal(t, e.conn, *a.ConnectionID)
	assert.Nil(t, page.NextCursor)

	byConnection := decode[areaPageBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/areas?connection_id="+e.conn.String(), nil))
	assert.Len(t, byConnection.Data, 1)
	assert.Empty(t, decode[areaPageBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/areas?kind=repository", nil)).Data)
	assert.Empty(t, decode[areaPageBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/areas?connection_id="+uuid.NewString(), nil)).Data)
	assert.Equal(t, http.StatusBadRequest, e.do(t, e.admin, http.MethodGet, "/api/v1/areas?cursor=bad", nil).Code)
	assert.Equal(t, http.StatusForbidden, e.do(t, e.member, http.MethodGet, "/api/v1/areas", nil).Code)
}
