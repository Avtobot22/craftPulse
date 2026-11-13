package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

type uploadTokenBody struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID uuid.UUID  `json:"project_id"`
	Slot      int        `json:"slot"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	Value     string     `json:"value"`
}

type uploadTokenPageBody struct {
	Data       []uploadTokenBody `json:"data"`
	NextCursor *string           `json:"next_cursor"`
}

func (e *loginEnv) tokenProject(t *testing.T, orgID entity.ID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.project (organization_id,id,name,status,created_at) VALUES ($1,$2,'CI project','active',now())`, uuid.UUID(orgID), id)
	require.NoError(t, err)
	return id
}

func (e *loginEnv) createUpload(t *testing.T, b *browser, project uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, b, http.MethodPost, "/api/v1/upload-tokens", _changing, strings.NewReader(`{"project_id":"`+project.String()+`"}`))
}

func (e *loginEnv) issueUpload(t *testing.T, b *browser, project uuid.UUID) uploadTokenBody {
	t.Helper()
	rec := e.createUpload(t, b, project)
	require.Equal(t, http.StatusCreated, rec.Code, "create upload token status")
	token := decode[uploadTokenBody](t, rec)
	require.Regexp(t, `^cpt_[A-Za-z0-9_-]{43}$`, token.Value)
	assert.Equal(t, project, token.ProjectID)
	assert.Equal(t, "/api/v1/upload-tokens/"+token.ID.String(), rec.Header().Get("Location"))
	e.secrets = append(e.secrets, token.Value)
	return token
}

func (e *loginEnv) bearer(t *testing.T, value, target string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(t, newBrowser(), http.MethodGet, target, http.Header{"Authorization": {"Bearer " + value}})
}

func TestUploadTokens_LifecycleAndPagination(t *testing.T) {
	e := newLoginEnv(t)
	admin := e.admin(t, "demo")
	project := e.tokenProject(t, e.orgID)
	first := e.issueUpload(t, admin, project)
	second := e.issueUpload(t, admin, project)
	assert.ElementsMatch(t, []int{1, 2}, []int{first.Slot, second.Slot})
	assert.NotEqual(t, first.Value, second.Value)
	conflict := e.createUpload(t, admin, project)
	require.Equal(t, http.StatusConflict, conflict.Code)
	assert.Equal(t, "state_conflict", decode[problemBody](t, conflict).Code)
	assert.NotContains(t, conflict.Body.String(), first.Value)
	assert.Equal(t, http.StatusNotImplemented, e.bearer(t, first.Value, "/api/v1/ci/check-profile").Code)
	var stored []byte
	var createdBy uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT token_hash,created_by FROM auth.upload_token WHERE organization_id=$1 AND id=$2`, uuid.UUID(e.orgID), first.ID).Scan(&stored, &createdBy))
	digest := sha256.Sum256([]byte(first.Value))
	assert.Equal(t, digest[:], stored)
	assert.Equal(t, e.userID(t, e.orgID, _adminSub), createdBy)
	for range 2 {
		rec := e.do(t, admin, http.MethodDelete, "/api/v1/upload-tokens/"+first.ID.String(), _changing)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	}
	assert.Equal(t, http.StatusUnauthorized, e.bearer(t, first.Value, "/api/v1/ci/check-profile").Code)
	replacement := e.issueUpload(t, admin, project)
	assert.Equal(t, first.Slot, replacement.Slot)
	assert.NotEqual(t, first.ID, replacement.ID)
	target := "/api/v1/upload-tokens?project_id=" + project.String() + "&limit=1"
	var all []uploadTokenBody
	var firstCursor string
	for next := target; ; {
		rec := e.do(t, admin, http.MethodGet, next, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), `"value"`)
		assert.NotContains(t, rec.Body.String(), `token_hash`)
		assert.NotContains(t, rec.Body.String(), hex.EncodeToString(digest[:]))
		page := decode[uploadTokenPageBody](t, rec)
		all = append(all, page.Data...)
		if page.NextCursor == nil {
			break
		}
		require.Less(t, len(all), 4, "pagination must terminate")
		if firstCursor == "" {
			firstCursor = *page.NextCursor
		}
		next = target + "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	require.Len(t, all, 3)
	ids := make([]uuid.UUID, 0, len(all))
	for _, token := range all {
		ids = append(ids, token.ID)
		if token.ID == first.ID {
			assert.NotNil(t, token.RevokedAt)
		}
	}
	assert.ElementsMatch(t, []uuid.UUID{first.ID, second.ID, replacement.ID}, ids)
	otherProject := e.tokenProject(t, e.orgID)
	for _, target := range []string{
		"/api/v1/upload-tokens", "/api/v1/upload-tokens?project_id=" + project.String() + "&limit=0",
		"/api/v1/upload-tokens?project_id=" + project.String() + "&cursor=invalid",
		"/api/v1/upload-tokens?project_id=" + otherProject.String() + "&cursor=" + url.QueryEscape(firstCursor),
	} {
		assert.Equal(t, http.StatusBadRequest, e.do(t, admin, http.MethodGet, target, nil).Code, target)
	}
	var issued, revoked int
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE operation LIKE '%create%'),count(*) FILTER(WHERE operation LIKE '%revoke%') FROM audit.event WHERE object_kind='upload_token'`).Scan(&issued, &revoked))
	assert.Equal(t, 3, issued)
	assert.Equal(t, 1, revoked, "repeat revoke has no second state change")
}

func TestUploadTokens_ScopedRolesAndIsolation(t *testing.T) {
	e := newLoginEnv(t)
	admin, foreignAdmin := e.admin(t, "demo"), e.admin(t, "other")
	own, hidden, foreign := e.tokenProject(t, e.orgID), e.tokenProject(t, e.orgID), e.tokenProject(t, e.otherID)
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	user := e.userID(t, e.orgID, _otherSub)
	require.Equal(t, http.StatusOK, e.roles(t, admin, user, `"0"`, []roleInput{{Role: "developer", ProjectID: &own}}).Code)
	assert.Equal(t, http.StatusForbidden, e.createUpload(t, member, own).Code)
	require.Equal(t, http.StatusOK, e.roles(t, admin, user, `"1"`, []roleInput{{Role: "tech_lead", ProjectID: &own}}).Code)
	token := e.issueUpload(t, member, own)
	for _, project := range []uuid.UUID{hidden, foreign} {
		assert.Equal(t, http.StatusNotFound, e.createUpload(t, member, project).Code)
		assert.Equal(t, http.StatusNotFound, e.do(t, member, http.MethodGet, "/api/v1/upload-tokens?project_id="+project.String(), nil).Code)
	}
	hiddenToken := e.issueUpload(t, admin, hidden)
	foreignToken := e.issueUpload(t, foreignAdmin, foreign)
	for _, id := range []uuid.UUID{hiddenToken.ID, foreignToken.ID} {
		assert.Equal(t, http.StatusNotFound, e.do(t, member, http.MethodDelete, "/api/v1/upload-tokens/"+id.String(), _changing).Code)
	}
	assert.Equal(t, http.StatusNotFound, e.do(t, foreignAdmin, http.MethodDelete, "/api/v1/upload-tokens/"+token.ID.String(), _changing).Code)
	assert.Equal(t, http.StatusForbidden, e.bearer(t, token.Value, "/api/v1/me").Code)
	assert.Equal(t, http.StatusForbidden, e.bearer(t, token.Value, "/api/v1/upload-tokens?project_id="+own.String()).Code)
	assert.Equal(t, http.StatusBadRequest, e.bearer(t, token.Value, "/api/v1/ci/check-profile?project_id="+hidden.String()).Code)
	assert.Equal(t, http.StatusNotImplemented, e.bearer(t, token.Value, "/api/v1/ci/check-profile").Code)
	assert.Equal(t, http.StatusUnauthorized, e.bearer(t, "cpt_"+strings.Repeat("A", 43), "/api/v1/ci/check-profile").Code)
}

func TestUploadTokens_QueryLeakRevokesBeforeAuthentication(t *testing.T) {
	e := newLoginEnv(t)
	admin := e.admin(t, "demo")
	project := e.tokenProject(t, e.orgID)
	for _, variant := range []string{"value", "key", "malformed sibling", "encoded prefix", "unknown route"} {
		token := e.issueUpload(t, admin, project)
		query := url.Values{"arbitrary": {token.Value}}
		if variant == "key" {
			query = url.Values{token.Value: {"anything"}}
		}
		rawQuery := query.Encode()
		if variant == "malformed sibling" {
			rawQuery += "&broken=%ZZ"
		}
		if variant == "encoded prefix" {
			rawQuery = strings.ReplaceAll(rawQuery, "cpt_", "c%70t_") + "&broken=%ZZ"
		}
		path := "/api/v1/ci/check-profile"
		if variant == "unknown route" {
			path = "/api/v1/nonexistent"
		}
		rec := e.do(t, admin, http.MethodGet, path+"?"+rawQuery, nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotContains(t, rec.Body.String(), token.Value)
		assert.NotContains(t, rec.Header().Get("Location"), token.Value)
		assert.Equal(t, http.StatusUnauthorized, e.bearer(t, token.Value, "/api/v1/ci/check-profile").Code)
		var revoked bool
		require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL FROM auth.upload_token WHERE organization_id=$1 AND id=$2`, uuid.UUID(e.orgID), token.ID).Scan(&revoked))
		assert.True(t, revoked)
		var audited int
		require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT count(*) FROM audit.event WHERE object_id=$1 AND outcome='succeeded' AND operation LIKE '%revoke%'`, token.ID).Scan(&audited))
		assert.Equal(t, 1, audited)
	}
}

func TestUploadTokens_ConcurrentIssueRespectsTwoSlots(t *testing.T) {
	e := newLoginEnv(t)
	admin := e.admin(t, "demo")
	project := e.tokenProject(t, e.orgID)
	reg := prometheus.NewRegistry()
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, nil, nil)
	e.router = httpctrl.NewRouter(zerolog.New(zerolog.SyncWriter(e.logs)), reg, health.Probes{Ready: &health.Readiness{}}, api)
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 6)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/upload-tokens", strings.NewReader(`{"project_id":"`+project.String()+`"}`))
			req.Header = _changing.Clone()
			for name, value := range admin.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: value})
			}
			rec := httptest.NewRecorder()
			<-start
			e.router.ServeHTTP(rec, req)
			responses <- rec
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	successes, conflicts := 0, 0
	for rec := range responses {
		switch rec.Code {
		case http.StatusCreated:
			successes++
			var token uploadTokenBody
			require.NoError(t, json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&token))
			e.secrets = append(e.secrets, token.Value)
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("concurrent create status %d", rec.Code)
		}
	}
	assert.Equal(t, 2, successes)
	assert.Equal(t, 4, conflicts)
	var active int
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT count(*) FROM auth.upload_token WHERE organization_id=$1 AND project_id=$2 AND revoked_at IS NULL`, uuid.UUID(e.orgID), project).Scan(&active))
	assert.Equal(t, 2, active)
}
