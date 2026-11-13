package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/probe"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_site        = "https://craftpulse.test"
	_adminSub    = "17"
	_otherSub    = "42"
	_demoClient  = "demo-client"
	_demoSecret  = _secretMarker + "-demo"
	_otherClient = "other-client"
	_otherSecret = _secretMarker + "-other"
)

type loginEnv struct {
	app     *pgxpool.Pool
	owner   *pgxpool.Pool
	idp     *testIDP
	clock   *clock.Manual
	router  http.Handler
	metrics http.Handler
	logs    *bytes.Buffer
	orgID   entity.ID
	otherID entity.ID
	keyring *crypto.Keyring
	// secrets collects every token, state, nonce and code seen in the test; none may reach logs or audit.
	secrets []string
}

func newLoginEnv(t *testing.T) *loginEnv {
	t.Helper()
	db := newDB(t)
	db.migrate(t)
	keyring, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: bytes.Repeat([]byte{9}, 32)})
	require.NoError(t, err)
	e := &loginEnv{
		app:   db.pool(t, "craftpulse_app"),
		owner: db.pool(t, "craftpulse_owner"),
		idp:   newTestIDP(t, map[string]string{_demoClient: _demoSecret, _otherClient: _otherSecret}),
		clock: clock.NewManual(_now),
		logs:  &bytes.Buffer{},
	}
	applied, err := newInstallation(e.app, keyring, storeProbe{}, probe.New(e.idp.srv.Client()), e.clock).Apply(t.Context(), entity.InstallParams{
		Organizations: []entity.NewOrganization{
			{
				Slug: "demo", Name: "Demo",
				Provider:          entity.OIDCProvider{Issuer: e.idp.issuer(), ClientID: _demoClient, ClientSecret: _demoSecret},
				FirstAdminSubject: _adminSub,
			},
			{
				Slug: "other", Name: "Other",
				Provider:          entity.OIDCProvider{Issuer: e.idp.issuer(), ClientID: _otherClient, ClientSecret: _otherSecret},
				FirstAdminSubject: _adminSub,
			},
		},
	})
	require.NoError(t, err)
	e.orgID, e.otherID, e.keyring = applied[0].ID, applied[1].ID, keyring

	reg := prometheus.NewRegistry()
	api := newAPI(e.app, keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, nil, nil)
	e.router = httpctrl.NewRouter(zerolog.New(e.logs), reg, health.Probes{Ready: &health.Readiness{}}, api)
	e.metrics = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	t.Cleanup(func() { e.assertNoSecrets(t) })
	return e
}

// browser keeps cookies the way a browser does for one site.
type browser struct {
	cookies map[string]string
}

func newBrowser() *browser { return &browser{cookies: map[string]string{}} }

func (e *loginEnv) do(t *testing.T, b *browser, method, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, b, method, target, header, http.NoBody)
}

func (e *loginEnv) send(t *testing.T, b *browser, method, target string, header http.Header, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, body)
	for k, v := range header {
		req.Header[k] = v
	}
	for name, value := range b.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
			continue
		}
		assert.True(t, c.Secure && c.HttpOnly && c.SameSite == http.SameSiteLaxMode && c.Path == "/", c.Name)
		b.cookies[c.Name] = c.Value
		e.secrets = append(e.secrets, c.Value)
	}
	return rec
}

// startLogin opens the login address and returns what the provider received.
func (e *loginEnv) startLogin(t *testing.T, b *browser, returnTo string) string {
	t.Helper()
	return e.startLoginTo(t, b, "demo", returnTo)
}

func (e *loginEnv) startLoginTo(t *testing.T, b *browser, slug, returnTo string) string {
	t.Helper()
	target := "/o/" + slug + "/login"
	if returnTo != "" {
		target += "?return_to=" + url.QueryEscape(returnTo)
	}
	rec := e.do(t, b, http.MethodGet, target, nil)
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	return rec.Header().Get("Location")
}

func (e *loginEnv) callback(t *testing.T, b *browser, a approval, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return e.callbackTo(t, b, "demo", a, extra)
}

func (e *loginEnv) callbackTo(t *testing.T, b *browser, slug string, a approval, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{"code": {a.code}, "state": {a.state}, "iss": {e.idp.issuer()}}
	for k, v := range extra {
		q[k] = v
	}
	e.secrets = append(e.secrets, a.code, a.state, a.nonce)
	return e.do(t, b, http.MethodGet, "/o/"+slug+"/callback?"+q.Encode(), nil)
}

func (e *loginEnv) login(t *testing.T, b *browser, subject string, edit ...func(*grant)) *httptest.ResponseRecorder {
	t.Helper()
	return e.loginTo(t, b, "demo", subject, edit...)
}

func (e *loginEnv) loginTo(t *testing.T, b *browser, slug, subject string, edit ...func(*grant)) *httptest.ResponseRecorder {
	t.Helper()
	return e.callbackTo(t, b, slug, e.idp.approve(t, e.startLoginTo(t, b, slug, "/teams?tab=1"), subject, edit...), nil)
}

func (e *loginEnv) me(t *testing.T, b *browser) (*httptest.ResponseRecorder, currentUser) {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/me", nil)
	var cu currentUser
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cu))
	}
	return rec, cu
}

type currentUser struct {
	User struct {
		ID          uuid.UUID `json:"id"`
		DisplayName string    `json:"display_name"`
	} `json:"user"`
	Organization struct {
		ID     uuid.UUID `json:"id"`
		Slug   string    `json:"slug"`
		Status string    `json:"status"`
	} `json:"organization"`
	Roles []struct {
		Role string `json:"role"`
	} `json:"roles"`
	Areas struct {
		Teams []struct {
			ID uuid.UUID `json:"id"`
		} `json:"teams"`
	} `json:"areas"`
	Actions []string `json:"actions"`
	Session struct {
		ExpiresAt     time.Time `json:"expires_at"`
		IdleExpiresAt time.Time `json:"idle_expires_at"`
	} `json:"session"`
}

type sessionRow struct {
	UserID    uuid.UUID
	Ended     bool
	EndReason string
	TokenHash []byte
}

func (e *loginEnv) sessions(t *testing.T) []sessionRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT user_id, ended_at IS NOT NULL, coalesce(end_reason, ''), token_hash
		FROM auth.session ORDER BY created_at, id`)
	require.NoError(t, err)
	defer rows.Close()
	var out []sessionRow
	for rows.Next() {
		var s sessionRow
		require.NoError(t, rows.Scan(&s.UserID, &s.Ended, &s.EndReason, &s.TokenHash))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

type auditRow struct {
	Operation string
	Outcome   string
	Reason    string
	Details   string
}

func (e *loginEnv) audit(t *testing.T, operation string) []auditRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT operation, outcome, coalesce(reason_code, ''), coalesce(details::text, '')
		FROM audit.event WHERE operation = $1 ORDER BY id`, operation)
	require.NoError(t, err)
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var a auditRow
		require.NoError(t, rows.Scan(&a.Operation, &a.Outcome, &a.Reason, &a.Details))
		out = append(out, a)
	}
	require.NoError(t, rows.Err())
	return out
}

// evidence writes the response, auth.session and the login audit of a case into the test log.
func (e *loginEnv) evidence(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	t.Logf("response: %d %s", rec.Code, rec.Header().Get("Location"))
	for _, s := range e.sessions(t) {
		t.Logf("auth.session: user=%s ended=%t reason=%q token_hash=%d bytes", s.UserID, s.Ended, s.EndReason, len(s.TokenHash))
	}
	for _, a := range append(e.audit(t, "session.login"), e.audit(t, "session.logout")...) {
		t.Logf("audit.event: %s %s %q %s", a.Operation, a.Outcome, a.Reason, a.Details)
	}
}

func (e *loginEnv) assertNoSecrets(t *testing.T) {
	var audit bytes.Buffer
	rows, err := e.owner.Query(context.Background(), "SELECT coalesce(details::text, '') FROM audit.event")
	require.NoError(t, err)
	for rows.Next() {
		var d string
		require.NoError(t, rows.Scan(&d))
		audit.WriteString(d)
	}
	rows.Close()
	for _, s := range append(e.secrets, "MARKER-access-token", "MARKER-refresh-token", _demoSecret, _otherSecret) {
		if s == "" {
			continue
		}
		assert.NotContains(t, e.logs.String(), s, "log")
		assert.NotContains(t, audit.String(), s, "audit")
	}
}

func (e *loginEnv) metric(t *testing.T, line string) bool {
	t.Helper()
	rec := httptest.NewRecorder()
	e.metrics.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	return strings.Contains(rec.Body.String(), line)
}

func TestLogin_FirstAdministrator(t *testing.T) {
	e := newLoginEnv(t)
	b := newBrowser()

	rec := e.login(t, b, _adminSub)
	e.evidence(t, rec)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/teams?tab=1", rec.Header().Get("Location"))
	assert.NotContains(t, b.cookies, "__Host-cp_login")
	token := b.cookies["__Host-cp_session"]
	require.NotEmpty(t, token)

	res, cu := e.me(t, b)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Equal(t, uuid.UUID(e.orgID), cu.Organization.ID)
	assert.Equal(t, "demo", cu.Organization.Slug)
	assert.Equal(t, "active", cu.Organization.Status)
	assert.Equal(t, "Same Name", cu.User.DisplayName)
	require.Len(t, cu.Roles, 1)
	assert.Equal(t, "administrator", cu.Roles[0].Role)
	assert.Equal(t, []string{"audit.read", "connections.manage", "installation.read", "org.manage", "org.read", "receipts.read", "session.manage", "session.read", "sync.read", "users.manage"}, cu.Actions)
	assert.Equal(t, _now.Add(12*time.Hour), cu.Session.ExpiresAt)
	assert.Equal(t, _now.Add(time.Hour), cu.Session.IdleExpiresAt)

	sessions := e.sessions(t)
	require.Len(t, sessions, 1)
	sum := sha256.Sum256([]byte(token))
	assert.Equal(t, sum[:], sessions[0].TokenHash, "only the SHA-256 of the cookie is stored")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.login_attempt"), "the login attempt is used once")
	login := e.audit(t, "session.login")
	require.Len(t, login, 1)
	assert.Equal(t, "succeeded", login[0].Outcome)
	assert.Contains(t, login[0].Details, `"assigned_role": "administrator"`)
	assert.Contains(t, login[0].Details, `"subject": "17"`)

	again := e.login(t, b, _adminSub)
	e.evidence(t, again)
	require.Equal(t, http.StatusFound, again.Code)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.role_assignment"), "a repeated login does not assign the role again")
	assert.Equal(t, 2, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.organization"))
	sessions = e.sessions(t)
	require.Len(t, sessions, 2)
	assert.Equal(t, "replaced", sessions[0].EndReason, "the browser keeps one session")
	assert.False(t, sessions[1].Ended)
	assert.NotEqual(t, token, b.cookies["__Host-cp_session"], "every login issues a new session ID")
}

func TestLogin_OnlyDeclaredAccountBecomesAdministrator(t *testing.T) {
	tests := []struct {
		name        string
		giveSubject string
		giveSetup   string
	}{
		{name: "same name, other subject", giveSubject: _otherSub},
		{
			name:        "same subject of another issuer",
			giveSubject: _adminSub,
			giveSetup:   "UPDATE org.organization SET first_admin_issuer = 'https://other-gitlab.example'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLoginEnv(t)
			if tt.giveSetup != "" {
				_, err := e.owner.Exec(t.Context(), tt.giveSetup+" WHERE organization_id = $1", uuid.UUID(e.orgID))
				require.NoError(t, err)
			}
			b := newBrowser()

			rec := e.login(t, b, tt.giveSubject)
			e.evidence(t, rec)

			require.Equal(t, http.StatusFound, rec.Code)
			assert.Equal(t, "/o/demo?login_error=organization_not_configured", rec.Header().Get("Location"))
			assert.NotContains(t, b.cookies, "__Host-cp_session")
			assert.Empty(t, e.sessions(t))
			assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.role_assignment"))
			assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.app_user"))
			assert.Equal(t, 2, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.organization WHERE status = 'setup'"))
			login := e.audit(t, "session.login")
			require.Len(t, login, 1)
			assert.Equal(t, auditRow{Operation: "session.login", Outcome: "denied", Reason: "organization_not_configured"},
				auditRow{Operation: login[0].Operation, Outcome: login[0].Outcome, Reason: login[0].Reason})
			assert.NotContains(t, login[0].Details, "first_admin")
		})
	}
}

func TestLogin_RejectsForgedResponse(t *testing.T) {
	other := newTestIDP(t, nil)
	tests := []struct {
		name      string
		giveGrant func(*grant)
		giveQuery url.Values
		wantError string
	}{
		{name: "wrong state", giveQuery: url.Values{"state": {"forged"}}, wantError: "invalid_response"},
		{name: "wrong iss parameter", giveQuery: url.Values{"iss": {"https://evil.test"}}, wantError: "invalid_response"},
		{name: "provider error", giveQuery: url.Values{"error": {"access_denied"}}, wantError: "provider_rejected"},
		{name: "no code", giveQuery: url.Values{"code": {""}}, wantError: "invalid_response"},
		{name: "token iss", giveGrant: func(g *grant) { g.claims["iss"] = "https://evil.test" }, wantError: "invalid_response"},
		{name: "token aud", giveGrant: func(g *grant) { g.claims["aud"] = "other-client" }, wantError: "invalid_response"},
		{name: "token nonce", giveGrant: func(g *grant) { g.claims["nonce"] = "forged" }, wantError: "invalid_response"},
		{name: "token expired", giveGrant: func(g *grant) { g.claims["exp"] = time.Now().Add(-time.Minute).Unix() }, wantError: "invalid_response"},
		{name: "token signature", giveGrant: func(g *grant) { g.key = other.key }, wantError: "invalid_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLoginEnv(t)
			b := newBrowser()
			var edit []func(*grant)
			if tt.giveGrant != nil {
				edit = append(edit, tt.giveGrant)
			}

			rec := e.callback(t, b, e.idp.approve(t, e.startLogin(t, b, "/"), _adminSub, edit...), tt.giveQuery)
			e.evidence(t, rec)

			require.Equal(t, http.StatusFound, rec.Code)
			assert.Equal(t, "/o/demo?login_error="+tt.wantError, rec.Header().Get("Location"))
			assert.NotContains(t, b.cookies, "__Host-cp_session")
			assert.NotContains(t, b.cookies, "__Host-cp_login")
			assert.Empty(t, e.sessions(t))
			assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.role_assignment"))
			login := e.audit(t, "session.login")
			require.Len(t, login, 1)
			assert.Equal(t, "failed", login[0].Outcome)
			assert.Equal(t, tt.wantError, login[0].Reason)
			assert.True(t, e.metric(t, fmt.Sprintf(`craftpulse_auth_login_failures_total{provider=%q,reason=%q} 1`, e.idp.issuer(), tt.wantError)))
		})
	}
}

func TestLogin_ReplayedResponse(t *testing.T) {
	e := newLoginEnv(t)
	b := newBrowser()
	location := e.startLogin(t, b, "/")
	loginCookie := b.cookies["__Host-cp_login"]
	a := e.idp.approve(t, location, _adminSub)
	require.Equal(t, http.StatusFound, e.callback(t, b, a, nil).Code)
	before := e.sessions(t)

	attacker := newBrowser()
	attacker.cookies["__Host-cp_login"] = loginCookie
	rec := e.callback(t, attacker, a, nil)
	e.evidence(t, rec)

	assert.Equal(t, "/o/demo?login_error=invalid_response", rec.Header().Get("Location"))
	assert.NotContains(t, attacker.cookies, "__Host-cp_session")
	assert.Equal(t, before, e.sessions(t), "the replayed response creates no session")
	login := e.audit(t, "session.login")
	require.Len(t, login, 2)
	assert.Equal(t, "invalid_response", login[1].Reason)
}

func TestLogin_UnknownOrganizationLooksLikeUnavailableProvider(t *testing.T) {
	e := newLoginEnv(t)

	rec := e.do(t, newBrowser(), http.MethodGet, "/o/nobody/login", nil)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/o/nobody?login_error=provider_unavailable", rec.Header().Get("Location"))
}

func TestLogin_ReturnPathStaysOnSite(t *testing.T) {
	for _, give := range []string{"https://evil.test/x", "//evil.test", "/\\evil.test", "/%2F/evil.test", "teams"} {
		t.Run(give, func(t *testing.T) {
			e := newLoginEnv(t)
			b := newBrowser()

			rec := e.callback(t, b, e.idp.approve(t, e.startLogin(t, b, give), _adminSub), nil)

			assert.Equal(t, "/", rec.Header().Get("Location"))
		})
	}
}

func TestSession_Expires(t *testing.T) {
	t.Run("one hour without activity", func(t *testing.T) {
		e := newLoginEnv(t)
		b := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)

		e.clock.Advance(59 * time.Minute)
		active, cu := e.me(t, b)
		require.Equal(t, http.StatusOK, active.Code)
		assert.Equal(t, _now.Add(119*time.Minute), cu.Session.IdleExpiresAt, "a request extends the idle limit")
		e.clock.Advance(time.Hour)
		rec, _ := e.me(t, b)
		e.evidence(t, rec)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "auth_required", problemCode(t, rec))
		assert.Contains(t, rec.Header().Get("Set-Cookie"), "__Host-cp_session=; Path=/; Max-Age=0")
	})
	t.Run("twelve hours after login", func(t *testing.T) {
		e := newLoginEnv(t)
		b := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)
		token := b.cookies["__Host-cp_session"]

		for range 14 {
			e.clock.Advance(50 * time.Minute)
			rec, _ := e.me(t, b)
			require.Equal(t, http.StatusOK, rec.Code, "active after %s", e.clock.Now().Sub(_now))
		}
		e.clock.Advance(20 * time.Minute)
		rec, _ := e.me(t, &browser{cookies: map[string]string{"__Host-cp_session": token}})
		e.evidence(t, rec)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "auth_required", problemCode(t, rec))
	})
}

func TestSession_EleventhClosesOldest(t *testing.T) {
	e := newLoginEnv(t)
	first := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, first, _adminSub).Code)
	for range entity.SessionsPerUser {
		e.clock.Advance(time.Second)
		require.Equal(t, http.StatusFound, e.login(t, newBrowser(), _adminSub).Code)
	}

	rec, _ := e.me(t, first)
	e.evidence(t, rec)

	sessions := e.sessions(t)
	require.Len(t, sessions, entity.SessionsPerUser+1)
	assert.Equal(t, "limit", sessions[0].EndReason)
	assert.Equal(t, entity.SessionsPerUser, count(t, t.Context(), e.owner, "SELECT count(*) FROM auth.session WHERE ended_at IS NULL"))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSession_Logout(t *testing.T) {
	e := newLoginEnv(t)
	b, other := newBrowser(), newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)
	require.Equal(t, http.StatusFound, e.login(t, other, _adminSub).Code)
	stale := b.cookies["__Host-cp_session"]
	origin := http.Header{"Origin": {_site}}

	rec := e.do(t, b, http.MethodPost, "/api/v1/session/logout", origin)
	e.evidence(t, rec)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.NotContains(t, b.cookies, "__Host-cp_session")
	staleBrowser := &browser{cookies: map[string]string{"__Host-cp_session": stale}}
	res, _ := e.me(t, staleBrowser)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	again := e.do(t, &browser{cookies: map[string]string{"__Host-cp_session": stale}}, http.MethodPost, "/api/v1/session/logout", origin)
	assert.Equal(t, http.StatusUnauthorized, again.Code)
	stillActive, _ := e.me(t, other)
	assert.Equal(t, http.StatusOK, stillActive.Code, "other sessions keep working")
	assert.Equal(t, "logout", e.sessions(t)[0].EndReason)
	logout := e.audit(t, "session.logout")
	require.Len(t, logout, 1)
	assert.Equal(t, "succeeded", logout[0].Outcome)
}

func TestSession_ForeignOriginChangesNothing(t *testing.T) {
	e := newLoginEnv(t)
	b := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)

	for _, origin := range []http.Header{{"Origin": {"https://evil.test"}}, nil} {
		rec := e.do(t, b, http.MethodPost, "/api/v1/session/logout", origin)
		e.evidence(t, rec)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, "origin_rejected", problemCode(t, rec))
	}
	assert.False(t, e.sessions(t)[0].Ended)
	assert.Empty(t, e.audit(t, "session.logout"))
	res, _ := e.me(t, b)
	assert.Equal(t, http.StatusOK, res.Code)
}

func TestAccess_RolesApplyToNextRequest(t *testing.T) {
	e := newLoginEnv(t)
	require.Equal(t, http.StatusFound, e.login(t, newBrowser(), _adminSub).Code)
	b := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, b, _otherSub).Code)
	teams := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range teams {
		_, err := e.owner.Exec(t.Context(), `INSERT INTO org.team (organization_id, id, name, status, created_at) VALUES ($1, $2, $3, 'active', now())`,
			uuid.UUID(e.orgID), id, "team "+id.String()[:4])
		require.NoError(t, err)
	}

	_, cu := e.me(t, b)
	assert.Empty(t, cu.Roles)
	assert.Empty(t, cu.Areas.Teams)
	assert.Equal(t, []string{"session.manage", "session.read"}, cu.Actions, "a new user has no area")
	assert.Equal(t, http.StatusForbidden, e.do(t, b, http.MethodGet, "/api/v1/teams", nil).Code, "organization is visible, action is not granted")
	assert.Equal(t, http.StatusForbidden, e.do(t, b, http.MethodGet, "/api/v1/audit-events", nil).Code)
	hidden := e.do(t, b, http.MethodGet, "/api/v1/teams/"+teams[0].String()+"/report", nil)
	assert.Equal(t, http.StatusNotFound, hidden.Code, "a team outside the area is not visible")
	assert.Equal(t, "not_found", problemCode(t, hidden))

	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment (organization_id, user_id, role, team_id, assigned_by, assigned_at)
		VALUES ($1, $2, 'developer', $3, $2, now())`, uuid.UUID(e.orgID), cu.User.ID, teams[0])
	require.NoError(t, err)

	_, cu = e.me(t, b)
	require.Len(t, cu.Areas.Teams, 1)
	assert.Equal(t, teams[0], cu.Areas.Teams[0].ID)
	assert.Contains(t, cu.Actions, "org.read")
	listed := e.do(t, b, http.MethodGet, "/api/v1/teams", nil)
	assert.Equal(t, http.StatusOK, listed.Code, "the new role applies to the next request")
	assert.Contains(t, listed.Body.String(), teams[0].String())
	assert.NotContains(t, listed.Body.String(), teams[1].String())
	visible := e.do(t, b, http.MethodGet, "/api/v1/teams/"+teams[0].String()+"/report", nil)
	assert.Equal(t, http.StatusForbidden, visible.Code, "the team is visible, report.read is not granted yet")
	assert.Equal(t, "forbidden", problemCode(t, visible))
	assert.Equal(t, http.StatusNotFound, e.do(t, b, http.MethodGet, "/api/v1/teams/"+teams[1].String()+"/report", nil).Code)

	_, err = e.owner.Exec(t.Context(), "UPDATE org.role_assignment SET revoked_at = now(), revoked_by = user_id WHERE role = 'developer'")
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, e.do(t, b, http.MethodGet, "/api/v1/teams", nil).Code, "a revoked role stops on the next request")

	denials := count(t, t.Context(), e.owner, "SELECT count(*) FROM audit.event WHERE outcome = 'denied' AND operation <> 'session.login'")
	assert.Equal(t, 6, denials)
	assert.True(t, e.metric(t, fmt.Sprintf(`craftpulse_api_access_denied_total{reason="outside_scope",subject=%q} 2`, cu.User.ID)))
}

func TestLogin_BlockedUser(t *testing.T) {
	e := newLoginEnv(t)
	require.Equal(t, http.StatusFound, e.login(t, newBrowser(), _adminSub).Code)
	_, err := e.owner.Exec(t.Context(), "UPDATE org.app_user SET status = 'blocked', blocked_at = now()")
	require.NoError(t, err)
	b := newBrowser()

	rec := e.login(t, b, _adminSub)
	e.evidence(t, rec)

	assert.Equal(t, "/o/demo?login_error=user_blocked", rec.Header().Get("Location"))
	assert.NotContains(t, b.cookies, "__Host-cp_session")
	assert.Len(t, e.sessions(t), 1)
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p.Code
}
