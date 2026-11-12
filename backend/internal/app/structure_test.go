package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	orgrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/org"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
)

type named struct {
	ID     *uuid.UUID `json:"id"`
	Name   string     `json:"name"`
	Status string     `json:"status"`
}

type link struct {
	TeamID    uuid.UUID  `json:"team_id"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
	PersonID  *uuid.UUID `json:"person_id,omitempty"`
	ValidFrom *time.Time `json:"valid_from"`
	ValidTo   *time.Time `json:"valid_to"`
}

type structureBody struct {
	Version      int     `json:"version,omitempty"`
	Teams        []named `json:"teams"`
	Projects     []named `json:"projects"`
	Services     []named `json:"services"`
	Components   []any   `json:"components"`
	TeamProjects []link  `json:"team_projects"`
	ProjectAreas []any   `json:"project_areas"`
	TeamMembers  []link  `json:"team_members"`
}

func emptyStructure() structureBody {
	return structureBody{
		Teams: []named{}, Projects: []named{}, Services: []named{}, Components: []any{},
		TeamProjects: []link{}, ProjectAreas: []any{}, TeamMembers: []link{},
	}
}

// input turns a read structure into the body of the next confirmation.
func (s structureBody) input() structureBody {
	in := s
	in.Version = 0
	for _, l := range [][]link{in.TeamProjects, in.TeamMembers} {
		for i := range l {
			l[i].ValidFrom, l[i].ValidTo = nil, nil
		}
	}
	return in
}

func (s structureBody) team(name string) uuid.UUID {
	for _, t := range s.Teams {
		if t.Name == name {
			return *t.ID
		}
	}
	return uuid.Nil
}

type problemBody struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Errors []struct {
		Field string `json:"field"`
		Code  string `json:"code"`
	} `json:"errors"`
}

func (e *loginEnv) admin(t *testing.T, slug string) *browser {
	t.Helper()
	b := newBrowser()
	require.Equal(t, http.StatusFound, e.loginTo(t, b, slug, _adminSub).Code)
	return b
}

func (e *loginEnv) structure(t *testing.T, b *browser) (s structureBody, etag string) {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/organization/structure", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	assert.Equal(t, `"`+strconv.Itoa(s.Version)+`"`, rec.Header().Get("ETag"))
	return s, rec.Header().Get("ETag")
}

func (e *loginEnv) replace(t *testing.T, b *browser, ifMatch string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return e.replaceRaw(t, b, ifMatch, raw, "")
}

func (e *loginEnv) replaceRaw(t *testing.T, b *browser, ifMatch string, raw []byte, query string) *httptest.ResponseRecorder {
	t.Helper()
	h := http.Header{"Origin": {_site}, "Content-Type": {"application/json"}}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return e.send(t, b, http.MethodPut, "/api/v1/organization/structure"+query, h, bytes.NewReader(raw))
}

func (e *loginEnv) version(t *testing.T, orgID entity.ID) int {
	t.Helper()
	var v int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT structure_version FROM org.organization WHERE organization_id = $1",
		uuid.UUID(orgID)).Scan(&v))
	return v
}

func (e *loginEnv) structureAudit(t *testing.T) [][2]string {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT base_version, new_version FROM audit.event
		WHERE operation = 'organization.structure.replace' AND organization_id = $1 ORDER BY occurred_at, id`, uuid.UUID(e.orgID))
	require.NoError(t, err)
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var v [2]string
		require.NoError(t, rows.Scan(&v[0], &v[1]))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func (e *loginEnv) person(t *testing.T, orgID entity.ID, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.person (organization_id, id, display_name, kind, created_at)
		VALUES ($1, $2, $3, 'human', now())`, uuid.UUID(orgID), id, name)
	require.NoError(t, err)
	return id
}

func TestStructure_ConcurrentAdministrators(t *testing.T) {
	e := newLoginEnv(t)
	first := e.admin(t, "demo")
	second := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, second, _otherSub).Code)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment (organization_id, user_id, role, assigned_by, assigned_at)
		SELECT organization_id, id, 'administrator', id, now() FROM org.app_user WHERE subject = $1`, _otherSub)
	require.NoError(t, err)

	initial := emptyStructure()
	initial.Teams = []named{{Name: "Platform", Status: "active"}}
	require.Equal(t, http.StatusOK, e.replace(t, first, `"0"`, initial).Code)

	v1, tag1 := e.structure(t, first)
	formOfSecond, tagOfSecond := e.structure(t, second)
	require.Equal(t, 1, v1.Version)
	assert.Equal(t, tag1, tagOfSecond)

	byFirst := v1.input()
	byFirst.Teams[0].Name = "Platform A"
	saved := e.replace(t, first, tag1, byFirst)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	assert.Equal(t, `"2"`, saved.Header().Get("ETag"))

	bySecond := formOfSecond.input()
	bySecond.Teams[0].Name = "Platform B"
	stale := e.replace(t, second, tagOfSecond, bySecond)
	t.Logf("stale save: %d %s", stale.Code, stale.Body.String())
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code)
	assert.Equal(t, "version_conflict", decode[problemBody](t, stale).Code)
	v2, tag2 := e.structure(t, second)
	assert.Equal(t, 2, v2.Version)
	assert.Equal(t, "Platform A", v2.Teams[0].Name, "the newer version stays")

	again := v2.input()
	again.Teams[0].Name = "Platform B"
	rec := e.replace(t, second, tag2, again)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	v3 := decode[structureBody](t, rec)
	assert.Equal(t, 3, v3.Version)
	assert.Equal(t, "Platform B", v3.Teams[0].Name)
	assert.Equal(t, *v1.Teams[0].ID, *v3.Teams[0].ID, "a rename keeps the identity")
	assert.Equal(t, [][2]string{{"0", "1"}, {"1", "2"}, {"2", "3"}}, e.structureAudit(t))
	t.Logf("structure_version=%d audit=%v", e.version(t, e.orgID), e.structureAudit(t))
}

func TestStructure_IfMatchAndRepeat(t *testing.T) {
	e := newLoginEnv(t)
	b := e.admin(t, "demo")
	body := emptyStructure()
	body.Teams = []named{{Name: "Platform", Status: "active"}}
	require.Equal(t, http.StatusOK, e.replace(t, b, `"0"`, body).Code)
	current, tag := e.structure(t, b)

	missing := e.replace(t, b, "", current.input())
	assert.Equal(t, http.StatusPreconditionRequired, missing.Code)
	assert.Equal(t, "precondition_required", decode[problemBody](t, missing).Code)
	assert.Equal(t, http.StatusPreconditionFailed, e.replace(t, b, `"forged"`, current.input()).Code)

	repeat := e.replace(t, b, tag, current.input())
	require.Equal(t, http.StatusOK, repeat.Code, repeat.Body.String())
	assert.Equal(t, tag, repeat.Header().Get("ETag"), "the same body creates no version")
	assert.Equal(t, 1, e.version(t, e.orgID))
	assert.Len(t, e.structureAudit(t), 1)
}

func TestStructure_AuditFailureKeepsStructure(t *testing.T) {
	e := newLoginEnv(t)
	uc := org.New(org.Deps{
		Orgs: orgrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: persistent.NewTransactor(e.app),
		Auditor: failingAuditor{}, Clock: e.clock, NewID: newID,
	})

	_, err := uc.ReplaceStructure(t.Context(), entity.Subject{OrganizationID: e.orgID}, 0, entity.StructureInput{
		Teams: []entity.AreaInput{{Name: "Platform", Status: "active"}},
	})

	require.ErrorContains(t, err, "audit unavailable")
	assert.Zero(t, e.version(t, e.orgID))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.team"))
}

func TestStructure_RejectsInvalidBody(t *testing.T) {
	e := newLoginEnv(t)
	b := e.admin(t, "demo")
	person := e.person(t, e.orgID, "Anna")
	foreignTeam := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.team (organization_id, id, name, status, created_at) VALUES ($1, $2, 'Platform', 'active', now())`,
		uuid.UUID(e.otherID), foreignTeam)
	require.NoError(t, err)
	setup := emptyStructure()
	setup.Teams = []named{{Name: "Platform", Status: "active"}}
	setup.Projects = []named{{Name: "Core", Status: "active"}}
	require.Equal(t, http.StatusOK, e.replace(t, b, `"0"`, setup).Code)
	base, tag := e.structure(t, b)
	team, project := *base.Teams[0].ID, *base.Projects[0].ID
	pair := link{TeamID: team, ProjectID: &project}
	ghost := uuid.New()
	now := time.Now()

	tests := []struct {
		name      string
		giveEdit  func(*structureBody)
		giveRaw   string
		giveQuery string
		wantCode  int
		wantField string
		wantError string
	}{
		{
			name: "overlapping interval of one pair", giveEdit: func(s *structureBody) { s.TeamProjects = []link{pair, pair} },
			wantCode: http.StatusUnprocessableEntity, wantField: "/team_projects/1", wantError: "duplicate",
		},
		{
			name: "unknown team in a link", giveEdit: func(s *structureBody) { s.TeamProjects = []link{{TeamID: ghost, ProjectID: &project}} },
			wantCode: http.StatusUnprocessableEntity, wantField: "/team_projects/0/team_id", wantError: "unknown_reference",
		},
		{name: "team of another organization", giveEdit: func(s *structureBody) {
			s.Teams = append(s.Teams, named{ID: &foreignTeam, Name: "Platform", Status: "active"})
		}, wantCode: http.StatusUnprocessableEntity, wantField: "/teams/1/id", wantError: "unknown_reference"},
		{
			name: "unknown person", giveEdit: func(s *structureBody) { s.TeamMembers = []link{{TeamID: team, PersonID: &ghost}} },
			wantCode: http.StatusUnprocessableEntity, wantField: "/team_members/0/person_id", wantError: "unknown_reference",
		},
		{name: "unknown source area", giveEdit: func(s *structureBody) {
			s.ProjectAreas = []any{map[string]any{"project_id": project, "area_id": ghost, "valid_from": nil, "valid_to": nil}}
		}, wantCode: http.StatusUnprocessableEntity, wantField: "/project_areas/0/area_id", wantError: "unknown_reference"},
		{name: "unknown repository of a component", giveEdit: func(s *structureBody) {
			s.Components = []any{map[string]any{"id": nil, "service_id": ghost, "repository_area_id": ghost, "path_prefix": "", "status": "confirmed"}}
		}, wantCode: http.StatusUnprocessableEntity, wantField: "/components/0/repository_area_id", wantError: "unknown_reference"},
		{
			name: "existing team left out", giveEdit: func(s *structureBody) { s.Teams = []named{} },
			wantCode: http.StatusUnprocessableEntity, wantField: "/teams", wantError: "required",
		},
		{name: "date of a link on M0", giveEdit: func(s *structureBody) {
			s.TeamMembers = []link{{TeamID: team, PersonID: &person, ValidFrom: &now}}
		}, wantCode: http.StatusUnprocessableEntity, wantField: "/team_members/0/valid_from", wantError: "not_allowed_value"},
		{
			name: "empty name", giveEdit: func(s *structureBody) { s.Teams[0].Name = "  " },
			wantCode: http.StatusUnprocessableEntity, wantField: "/teams/0/name", wantError: "required",
		},
		{
			name:     "unknown body field",
			giveRaw:  `{"teams":[],"projects":[],"services":[],"components":[],"team_projects":[],"project_areas":[],"team_members":[],"colour":"red"}`,
			wantCode: http.StatusUnprocessableEntity, wantField: "colour", wantError: "unknown_field",
		},
		{
			name: "missing collection", giveRaw: `{"teams":[],"projects":[],"services":[],"components":[],"team_projects":[],"project_areas":[]}`,
			wantCode: http.StatusUnprocessableEntity, wantField: "/team_members", wantError: "required",
		},
		{name: "unknown query parameter", giveQuery: "?force=1", wantCode: http.StatusBadRequest, wantError: "unknown_parameter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := base.input()
			body.Teams = append([]named(nil), body.Teams...)
			if tt.giveEdit != nil {
				tt.giveEdit(&body)
			}
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			if tt.giveRaw != "" {
				raw = []byte(tt.giveRaw)
			}

			rec := e.replaceRaw(t, b, tag, raw, tt.giveQuery)
			t.Logf("response: %d %s", rec.Code, rec.Body.String())

			require.Equal(t, tt.wantCode, rec.Code)
			p := decode[problemBody](t, rec)
			if tt.wantField == "" {
				assert.Equal(t, tt.wantError, p.Code)
			} else {
				assert.Equal(t, "validation_failed", p.Code)
				require.NotEmpty(t, p.Errors)
				assert.Equal(t, tt.wantField, p.Errors[0].Field)
				assert.Equal(t, tt.wantError, p.Errors[0].Code)
			}
			assert.Equal(t, 1, e.version(t, e.orgID), "the structure does not change")
			assert.Len(t, e.structureAudit(t), 1)
			assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.team_project"))
		})
	}
}

func TestStructure_LinksAndTeamArea(t *testing.T) {
	e := newLoginEnv(t)
	b := e.admin(t, "demo")
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	person := e.person(t, e.orgID, "Anna")
	_, err := e.owner.Exec(t.Context(), "UPDATE org.app_user SET person_id = $1 WHERE subject = $2", person, _otherSub)
	require.NoError(t, err)
	setup := emptyStructure()
	setup.Teams = []named{{Name: "Platform", Status: "active"}, {Name: "Mobile", Status: "active"}}
	setup.Projects = []named{{Name: "Core", Status: "active"}}
	require.Equal(t, http.StatusOK, e.replace(t, b, `"0"`, setup).Code)
	v1, tag := e.structure(t, b)
	team, project := v1.team("Platform"), *v1.Projects[0].ID

	linked := v1.input()
	linked.TeamProjects = []link{{TeamID: team, ProjectID: &project}}
	linked.TeamMembers = []link{{TeamID: team, PersonID: &person}}
	rec := e.replace(t, b, tag, linked)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	v2 := decode[structureBody](t, rec)
	require.Len(t, v2.TeamProjects, 1)
	require.Len(t, v2.TeamMembers, 1)
	assert.Equal(t, _now, v2.TeamMembers[0].ValidFrom.UTC(), "on M0 the interval starts on saving")
	assert.Nil(t, v2.TeamMembers[0].ValidTo)

	teams := decode[struct {
		Data []named `json:"data"`
	}](t, e.do(t, member, http.MethodGet, "/api/v1/teams", nil))
	require.Len(t, teams.Data, 1, "a member sees the team by its composition")
	assert.Equal(t, team, *teams.Data[0].ID)
	projects := e.do(t, member, http.MethodGet, "/api/v1/projects?team_id="+team.String(), nil)
	assert.Contains(t, projects.Body.String(), project.String())
	empty := e.do(t, member, http.MethodGet, "/api/v1/projects?team_id="+v1.team("Mobile").String(), nil)
	assert.JSONEq(t, `{"data":[],"next_cursor":null}`, empty.Body.String())

	e.clock.Advance(50 * time.Minute)
	unlinked := v2.input()
	unlinked.TeamMembers = []link{}
	rec = e.replace(t, b, `"2"`, unlinked)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, decode[structureBody](t, rec).TeamMembers)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.team_member WHERE superseded_seq IS NOT NULL"))
	assert.Equal(t, 1, count(t, t.Context(), e.owner,
		"SELECT count(*) FROM org.team_member WHERE superseded_seq IS NULL AND NOT upper_inf(valid)"), "the closed interval is kept")
	assert.Equal(t, http.StatusForbidden, e.do(t, member, http.MethodGet, "/api/v1/teams", nil).Code, "the team area is gone on the next request")
}

func TestLists_PaginationAndIsolation(t *testing.T) {
	e := newLoginEnv(t)
	demo, other := e.admin(t, "demo"), e.admin(t, "other")
	names := []named{
		{Name: "Gamma", Status: "active"},
		{Name: "Alpha", Status: "active"},
		{Name: "Beta", Status: "active"},
		{Name: "Beta", Status: "active"},
		{Name: "Delta", Status: "archived"},
	}
	for _, b := range []*browser{demo, other} {
		body := emptyStructure()
		body.Teams = names
		body.Services = []named{{Name: "billing", Status: "active"}}
		require.Equal(t, http.StatusOK, e.replace(t, b, `"0"`, body).Code)
	}
	own, _ := e.structure(t, demo)
	foreign, _ := e.structure(t, other)

	asc := e.walk(t, demo, "/api/v1/teams?limit=1")
	require.Len(t, asc, len(names), "every team once, archived ones too for an administrator")
	ownIDs := map[uuid.UUID]bool{}
	for _, tm := range own.Teams {
		ownIDs[*tm.ID] = true
	}
	for i, tm := range asc {
		assert.True(t, ownIDs[*tm.ID], "team %s belongs to the organization", tm.Name)
		if i > 0 {
			prev := asc[i-1]
			assert.True(t, prev.Name < tm.Name || prev.Name == tm.Name && prev.ID.String() < tm.ID.String(), "sorted by name, then id")
		}
	}
	desc := e.walk(t, demo, "/api/v1/teams?limit=2&sort=-name")
	require.Len(t, desc, len(asc))
	for i := range desc {
		assert.Equal(t, *asc[len(asc)-1-i].ID, *desc[i].ID)
	}
	assert.Len(t, e.walk(t, demo, "/api/v1/services?limit=1"), 1)

	for _, target := range []string{
		"/api/v1/teams?limit=0", "/api/v1/teams?limit=201", "/api/v1/teams?cursor=MARKER",
		"/api/v1/services?cursor=" + url.QueryEscape(e.firstCursor(t, demo, "/api/v1/teams?limit=1")),
	} {
		rec := e.do(t, demo, http.MethodGet, target, nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code, target)
	}

	foreignTeam := e.do(t, demo, http.MethodGet, "/api/v1/teams/"+foreign.team("Alpha").String()+"/report", nil)
	missingTeam := e.do(t, demo, http.MethodGet, "/api/v1/teams/"+uuid.NewString()+"/report", nil)
	assert.Equal(t, http.StatusNotFound, foreignTeam.Code, "a team of another organization is not found")
	assert.Equal(t, decode[problemBody](t, missingTeam).Code, decode[problemBody](t, foreignTeam).Code)
	assert.Equal(t, http.StatusNotFound, missingTeam.Code)

	developer := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, developer, _otherSub).Code)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment (organization_id, user_id, role, team_id, assigned_by, assigned_at)
		SELECT organization_id, id, 'developer', $2, id, now() FROM org.app_user WHERE organization_id = $1 AND subject = $3`,
		uuid.UUID(e.orgID), own.team("Alpha"), _otherSub)
	require.NoError(t, err)
	scoped := e.walk(t, developer, "/api/v1/teams?limit=1")
	require.Len(t, scoped, 1)
	assert.Equal(t, own.team("Alpha"), *scoped[0].ID)
	assert.JSONEq(t, `{"data":[],"next_cursor":null}`, e.do(t, developer, http.MethodGet, "/api/v1/services", nil).Body.String())

	areas := e.do(t, demo, http.MethodGet, "/api/v1/areas?kind=repository", nil)
	assert.Equal(t, http.StatusOK, areas.Code)
	assert.JSONEq(t, `{"data":[],"next_cursor":null}`, areas.Body.String())
	assert.Equal(t, http.StatusForbidden, e.do(t, developer, http.MethodGet, "/api/v1/areas", nil).Code)
	assert.Equal(t, http.StatusForbidden, e.do(t, developer, http.MethodGet, "/api/v1/organization/structure", nil).Code)
}

// walk follows next_cursor to the end of a list.
func (e *loginEnv) walk(t *testing.T, b *browser, target string) []named {
	t.Helper()
	var all []named
	for next := target; ; {
		rec := e.do(t, b, http.MethodGet, next, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		page := decode[struct {
			Data       []named `json:"data"`
			NextCursor *string `json:"next_cursor"`
		}](t, rec)
		all = append(all, page.Data...)
		if page.NextCursor == nil {
			return all
		}
		require.Less(t, len(all), 100, "the list ends")
		next = target + "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
}

func (e *loginEnv) firstCursor(t *testing.T, b *browser, target string) string {
	t.Helper()
	page := decode[struct {
		NextCursor *string `json:"next_cursor"`
	}](t, e.do(t, b, http.MethodGet, target, nil))
	require.NotNil(t, page.NextCursor)
	return *page.NextCursor
}
