package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/probe"
)

type roleInput struct {
	Role      string     `json:"role"`
	TeamID    *uuid.UUID `json:"team_id"`
	ProjectID *uuid.UUID `json:"project_id"`
	ServiceID *uuid.UUID `json:"service_id"`
}

type auditEvent struct {
	ID          uuid.UUID  `json:"id"`
	OccurredAt  time.Time  `json:"occurred_at"`
	ActorID     *uuid.UUID `json:"actor_id"`
	Operation   string     `json:"operation"`
	ObjectID    *uuid.UUID `json:"object_id"`
	Outcome     string     `json:"outcome"`
	ReasonCode  *string    `json:"reason_code"`
	BaseVersion *string    `json:"base_version"`
	NewVersion  *string    `json:"new_version"`
}

var _changing = http.Header{"Origin": {_site}, "Content-Type": {"application/json"}}

func (e *loginEnv) userID(t *testing.T, orgID entity.ID, subject string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM org.app_user WHERE organization_id = $1 AND subject = $2",
		uuid.UUID(orgID), subject).Scan(&id))
	return id
}

func (e *loginEnv) roles(t *testing.T, b *browser, user uuid.UUID, ifMatch string, roles []roleInput) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"data": roles})
	require.NoError(t, err)
	h := _changing.Clone()
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return e.send(t, b, http.MethodPut, "/api/v1/users/"+user.String()+"/roles", h, bytes.NewReader(raw))
}

func (e *loginEnv) events(t *testing.T, b *browser, query string) []auditEvent {
	t.Helper()
	var all []auditEvent
	target := "/api/v1/audit-events?limit=2" + query
	for next := target; ; {
		rec := e.do(t, b, http.MethodGet, next, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		page := decode[struct {
			Data       []auditEvent `json:"data"`
			NextCursor *string      `json:"next_cursor"`
		}](t, rec)
		all = append(all, page.Data...)
		if page.NextCursor == nil {
			return all
		}
		require.Less(t, len(all), 500)
		next = target + "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
}

func byOperation(events []auditEvent, operation string) []auditEvent {
	var out []auditEvent
	for _, ev := range events {
		if ev.Operation == operation {
			out = append(out, ev)
		}
	}
	return out
}

func versions(ev auditEvent) [2]string {
	var v [2]string
	if ev.BaseVersion != nil {
		v[0] = *ev.BaseVersion
	}
	if ev.NewVersion != nil {
		v[1] = *ev.NewVersion
	}
	return v
}

func TestUsers_RolesBlockingAndSessions(t *testing.T) {
	e := newLoginEnv(t)
	admin := e.admin(t, "demo")
	adminID := e.userID(t, e.orgID, _adminSub)
	setup := emptyStructure()
	setup.Teams = []named{{Name: "Platform", Status: "active"}}
	require.Equal(t, http.StatusOK, e.replace(t, admin, `"0"`, setup).Code)
	st, _ := e.structure(t, admin)
	team := st.team("Platform")
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	user := e.userID(t, e.orgID, _otherSub)
	usersPath := "/api/v1/users/" + user.String()

	listed := e.do(t, admin, http.MethodGet, usersPath+"/roles", nil)
	require.Equal(t, http.StatusOK, listed.Code)
	assert.JSONEq(t, `{"data":[]}`, listed.Body.String())
	assert.Equal(t, `"0"`, listed.Header().Get("ETag"))

	granted := e.roles(t, admin, user, `"0"`, []roleInput{{Role: "developer", TeamID: &team}})
	require.Equal(t, http.StatusOK, granted.Code, granted.Body.String())
	assert.Equal(t, `"1"`, granted.Header().Get("ETag"))
	assert.Equal(t, http.StatusOK, e.do(t, member, http.MethodGet, "/api/v1/teams", nil).Code, "the role applies without a new login")

	stale := e.roles(t, admin, user, `"0"`, []roleInput{})
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code)
	assert.Equal(t, http.StatusPreconditionRequired, e.roles(t, admin, user, "", []roleInput{}).Code)
	ghost := uuid.New()
	for _, tt := range []struct {
		give      roleInput
		wantField string
		wantCode  string
	}{
		{give: roleInput{Role: "administrator", TeamID: &team}, wantField: "/data/0", wantCode: "not_allowed_value"},
		{give: roleInput{Role: "process_owner"}, wantField: "/data/0/team_id", wantCode: "required"},
		{give: roleInput{Role: "service_owner", TeamID: &team}, wantField: "/data/0/service_id", wantCode: "required"},
		{give: roleInput{Role: "developer", TeamID: &team, ProjectID: &ghost}, wantField: "/data/0", wantCode: "not_allowed_value"},
		{give: roleInput{Role: "developer", TeamID: &ghost}, wantField: "/data/0/team_id", wantCode: "unknown_reference"},
	} {
		rec := e.roles(t, admin, user, `"1"`, []roleInput{tt.give})
		t.Logf("invalid role %+v: %d %s", tt.give, rec.Code, rec.Body.String())
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		p := decode[problemBody](t, rec)
		require.NotEmpty(t, p.Errors)
		assert.Equal(t, tt.wantField, p.Errors[0].Field)
		assert.Equal(t, tt.wantCode, p.Errors[0].Code)
	}

	revoked := e.roles(t, admin, user, `"1"`, []roleInput{})
	require.Equal(t, http.StatusOK, revoked.Code)
	assert.Equal(t, `"2"`, revoked.Header().Get("ETag"))
	assert.Equal(t, http.StatusForbidden, e.do(t, member, http.MethodGet, "/api/v1/teams", nil).Code, "the organization is visible, the action is gone")
	assert.Equal(t, http.StatusNotFound, e.do(t, member, http.MethodGet, "/api/v1/teams/"+team.String()+"/report", nil).Code,
		"the team is no longer visible")

	keyID := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO auth.api_key (organization_id, id, token_hash, owner_user_id, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), now() + interval '30 days')`, uuid.UUID(e.orgID), keyID, bytes.Repeat([]byte{1}, 32), user, adminID)
	require.NoError(t, err)
	stolen := member.cookies["__Host-cp_session"]
	for range 2 {
		assert.Equal(t, http.StatusNoContent, e.do(t, admin, http.MethodPost, usersPath+"/block", _changing).Code)
	}
	res, _ := e.me(t, member)
	assert.Equal(t, http.StatusUnauthorized, res.Code, "a blocked user loses access on the next request")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.api_key WHERE revoked_at IS NOT NULL"))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.session WHERE end_reason = 'blocked'"))
	blockedUsers := e.do(t, admin, http.MethodGet, "/api/v1/users?status=blocked", nil)
	assert.Contains(t, blockedUsers.Body.String(), user.String())
	assert.NotContains(t, blockedUsers.Body.String(), adminID.String())

	assert.Equal(t, http.StatusNoContent, e.do(t, admin, http.MethodPost, usersPath+"/unblock", _changing).Code)
	res, _ = e.me(t, &browser{cookies: map[string]string{"__Host-cp_session": stolen}})
	assert.Equal(t, http.StatusUnauthorized, res.Code, "the old cookie stays ended after unblocking")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.api_key WHERE revoked_at IS NOT NULL"), "keys stay disabled")

	again := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, again, _otherSub).Code)
	for range 2 {
		assert.Equal(t, http.StatusNoContent, e.do(t, admin, http.MethodDelete, usersPath+"/sessions", _changing).Code)
	}
	res, _ = e.me(t, again)
	assert.Equal(t, http.StatusUnauthorized, res.Code, "ended sessions are refused on the next request")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.session WHERE end_reason = 'admin'"))

	events := e.events(t, admin, "")
	for i := 1; i < len(events); i++ {
		prev, cur := events[i-1], events[i]
		assert.False(t, prev.OccurredAt.Before(cur.OccurredAt), "newest first")
		assert.NotEqual(t, prev.ID, cur.ID, "no repeats between pages")
	}
	roleChanges := byOperation(events, "user.roles.replace")
	require.Len(t, roleChanges, 2)
	assert.Equal(t, [][2]string{{"1", "2"}, {"0", "1"}}, [][2]string{versions(roleChanges[0]), versions(roleChanges[1])})
	for _, op := range []string{"user.block", "user.unblock", "user.sessions.end"} {
		got := byOperation(events, op)
		require.Len(t, got, 1, "%s is recorded once; the repeat changes nothing", op)
		assert.Equal(t, adminID, *got[0].ActorID, "who changed it")
		assert.Equal(t, user, *got[0].ObjectID, "whose account")
	}
	assert.NotEmpty(t, byOperation(e.events(t, admin, "&outcome=denied"), "listTeams"), "refusals of the user are in the journal")
	assert.Empty(t, e.events(t, admin, "&actor_id="+uuid.NewString()))
	assert.Empty(t, e.events(t, admin, "&occurred_after="+url.QueryEscape(time.Now().Add(time.Hour).Format(time.RFC3339))))

	journal := e.do(t, admin, http.MethodGet, "/api/v1/audit-events?limit=200", nil).Body.String()
	rows, err := e.owner.Query(t.Context(), "SELECT id::text FROM auth.session")
	require.NoError(t, err)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		assert.NotContains(t, journal, id, "session IDs never reach the journal")
		assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM audit.event WHERE details::text LIKE '%"+id+"%'"))
	}
	rows.Close()
	t.Logf("journal: %s", journal)
}

func TestUsers_OtherOrganizationAndRoles(t *testing.T) {
	e := newLoginEnv(t)
	demo, other := e.admin(t, "demo"), e.admin(t, "other")
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	demoUser := e.userID(t, e.orgID, _otherSub)
	demoEvents := e.events(t, demo, "")
	require.NotEmpty(t, demoEvents)

	users := e.do(t, other, http.MethodGet, "/api/v1/users?sort=-created_at", nil)
	require.Equal(t, http.StatusOK, users.Code)
	assert.NotContains(t, users.Body.String(), demoUser.String())
	assert.Len(t, decode[struct {
		Data []any `json:"data"`
	}](t, users).Data, 1, "only the administrator of the other organization")
	for _, rec := range []*httptest.ResponseRecorder{
		e.do(t, other, http.MethodGet, "/api/v1/users/"+demoUser.String()+"/roles", nil),
		e.do(t, other, http.MethodPost, "/api/v1/users/"+demoUser.String()+"/block", _changing),
		e.do(t, other, http.MethodDelete, "/api/v1/users/"+demoUser.String()+"/sessions", _changing),
	} {
		assert.Equal(t, http.StatusNotFound, rec.Code)
	}
	res, _ := e.me(t, member)
	assert.Equal(t, http.StatusOK, res.Code, "the user of the first organization keeps working")
	journal := e.do(t, other, http.MethodGet, "/api/v1/audit-events?limit=200", nil).Body.String()
	for _, ev := range demoEvents {
		assert.NotContains(t, journal, ev.ID.String())
	}

	for _, target := range []string{
		"/api/v1/users", "/api/v1/audit-events", "/api/v1/installation/protocols/latest",
		"/api/v1/users/" + demoUser.String() + "/roles",
	} {
		rec := e.do(t, member, http.MethodGet, target, nil)
		assert.Equal(t, http.StatusForbidden, rec.Code, target)
	}
	assert.Equal(t, http.StatusForbidden, e.do(t, member, http.MethodPost, "/api/v1/users/"+demoUser.String()+"/block", _changing).Code)

	sorted := e.do(t, demo, http.MethodGet, "/api/v1/users?limit=1&sort=display_name", nil)
	require.Equal(t, http.StatusOK, sorted.Code)
	first := decode[struct {
		NextCursor *string `json:"next_cursor"`
	}](t, sorted)
	require.NotNil(t, first.NextCursor)
	wrongOrder := e.do(t, demo, http.MethodGet, "/api/v1/users?sort=created_at&cursor="+url.QueryEscape(*first.NextCursor), nil)
	assert.Equal(t, http.StatusBadRequest, wrongOrder.Code, "a cursor belongs to its order")
}

func TestInstallation_LatestProtocol(t *testing.T) {
	e := newLoginEnv(t)
	admin := e.admin(t, "demo")
	assert.Equal(t, http.StatusNotFound, e.do(t, admin, http.MethodGet, "/api/v1/installation/protocols/latest", nil).Code)
	site := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(site.Close)
	uc := newInstallation(e.app, e.keyring, storeProbe{}, probe.New(e.idp.srv.Client()), e.clock)
	params := entity.InstallParams{
		ReleaseSHA: _testSHA,
		SiteURL:    site.URL,
		Organizations: []entity.NewOrganization{{
			Slug: "demo", Provider: entity.OIDCProvider{Issuer: e.idp.issuer()},
		}},
	}
	_, err := uc.Check(t.Context(), params)
	require.NoError(t, err)
	e.clock.Advance(time.Minute)
	latest, err := uc.Check(t.Context(), params)
	require.NoError(t, err)

	rec := e.do(t, admin, http.MethodGet, "/api/v1/installation/protocols/latest", nil)
	t.Logf("protocol: %s", rec.Body.String())

	require.Equal(t, http.StatusOK, rec.Code)
	p := decode[struct {
		ID         uuid.UUID `json:"id"`
		ReleaseSHA string    `json:"release_sha"`
		Outcome    string    `json:"outcome"`
		Checks     []struct {
			CheckKind string `json:"check_kind"`
			Outcome   string `json:"outcome"`
		} `json:"checks"`
	}](t, rec)
	assert.Equal(t, uuid.UUID(latest.ID), p.ID, "the last check")
	assert.Equal(t, _testSHA, p.ReleaseSHA)
	assert.Equal(t, string(latest.Outcome), p.Outcome)
	assert.Len(t, p.Checks, len(latest.Checks))
	assert.NotContains(t, rec.Body.String(), "detail", "details of the checks stay in the protocol")
}
