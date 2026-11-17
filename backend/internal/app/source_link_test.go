package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
)

const _sourceCode = "source-code-235-MARKER"

func newSourceLinkEnv(t *testing.T) *accessEnv {
	t.Helper()
	e := newAccessEnv(t)
	e.rights(t, testadapter.Access{Mode: "user_token", Default: "allowed", Rules: []testadapter.AccessRule{
		{ItemKind: "object", NativeID: "20003", Decision: "denied"},
	}})
	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection SET access_mode = 'user_token' WHERE id = $1", e.conn)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "UPDATE normalize.source_object SET restricted = true WHERE object_kind = 'task' AND native_id = '20003'")
	require.NoError(t, err)
	e.secrets = append(e.secrets, _sourceCode, "test-user-token:"+_sourceCode)
	return e
}

type sourceLinksBody struct {
	Data []struct {
		ConnectionID uuid.UUID  `json:"connection_id"`
		State        string     `json:"state"`
		LinkedAt     *time.Time `json:"linked_at"`
	} `json:"data"`
}

func (e *accessEnv) sourceLinks(t *testing.T, b *browser) sourceLinksBody {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/me/source-links", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	validateSourceResponse(t, rec, "SourceLinkList")
	return decode[sourceLinksBody](t, rec)
}

func (e *accessEnv) startSource(t *testing.T, b *browser, returnTo string) string {
	t.Helper()
	rec := e.do(t, b, http.MethodPost, "/api/v1/me/source-links/"+e.conn.String()+"/authorization?return_to="+url.QueryEscape(returnTo),
		http.Header{"Origin": {_site}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	validateSourceResponse(t, rec, "AuthorizationRedirect")
	var result struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	auth, err := url.Parse(result.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "test-source-client", auth.Query().Get("client_id"))
	assert.Equal(t, "READ", auth.Query().Get("scope"))
	assert.Equal(t, _site+"/source-links/callback", auth.Query().Get("redirect_uri"))
	state := auth.Query().Get("state")
	require.Len(t, state, 43)
	e.secrets = append(e.secrets, state)
	return state
}

func (e *accessEnv) completeSource(t *testing.T, b *browser, state, sourceError string) *httptest.ResponseRecorder {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/source-links/callback?"+url.Values{
		"state": {state}, "code": {_sourceCode}, "error": {sourceError},
	}.Encode(), nil)
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Header().Get("Location"), _sourceCode)
	assert.NotContains(t, rec.Header().Get("Location"), state)
	return rec
}

func TestSourceLinks_LinkReadRevokeAndRelink(t *testing.T) {
	t.Parallel()
	e := newSourceLinkEnv(t)
	links := e.sourceLinks(t, e.member)
	require.Len(t, links.Data, 1)
	assert.Equal(t, "not_linked", links.Data[0].State)
	assert.Nil(t, links.Data[0].LinkedAt)
	assert.False(t, e.sees(t, e.member), "a user_token source starts closed without a linked identity")
	state := e.startSource(t, e.member, "/projects?tab=source#details")
	rec := e.completeSource(t, e.member, state, "")
	assert.Equal(t, "/projects?source_link_result=linked&tab=source#details", rec.Header().Get("Location"))
	links = e.sourceLinks(t, e.member)
	assert.Equal(t, "linked", links.Data[0].State)
	assert.NotNil(t, links.Data[0].LinkedAt)
	require.True(t, e.sees(t, e.member), "no source_account mapping is needed for user_token")
	tasks, _ := e.tasks(t, e.memberID)
	assert.NotEmpty(t, tasks)
	assert.NotContains(t, tasks, "20003", "an object's own restriction is checked even when its area is open")
	before := e.adapter.hits.Load()
	assert.Contains(t, e.completeSource(t, e.member, state, "").Header().Get("Location"), "link_failed")
	assert.Equal(t, before, e.adapter.hits.Load(), "replay must not call the adapter")
	for range 2 {
		rec = e.do(t, e.member, http.MethodDelete, "/api/v1/me/source-links/"+e.conn.String()+"/authorization", http.Header{"Origin": {_site}})
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	}
	assert.False(t, e.sees(t, e.member))
	assert.Equal(t, "revoked", e.sourceLinks(t, e.member).Data[0].State)
	assert.Len(t, e.audit(t, "source_link.link"), 1)
	assert.Len(t, e.audit(t, "source_link.revoke"), 1)
	var count int
	require.NoError(t, e.adapter.db.pool(t, "owner").QueryRow(t.Context(), "SELECT count(*) FROM adapter.user_token WHERE connection_id = $1", e.conn).Scan(&count))
	assert.Zero(t, count)
	state = e.startSource(t, e.member, "//evil.test")
	assert.Equal(t, "/?source_link_result=linked", e.completeSource(t, e.member, state, "").Header().Get("Location"))
	assert.True(t, e.sees(t, e.member))
	assert.NotContains(t, e.adapter.journal.String(), _sourceCode)
	assert.NotContains(t, e.adapter.journal.String(), "test-user-token:"+_sourceCode)
	var dump string
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT coalesce(jsonb_agg(to_jsonb(l))::text,'') FROM access.source_link l`).Scan(&dump))
	assert.NotContains(t, dump, _sourceCode)
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT coalesce(jsonb_agg(to_jsonb(a))::text,'') FROM access.source_link_attempt a`).Scan(&dump))
	assert.NotContains(t, dump, _sourceCode)
}

func TestSourceLinks_InvalidStateNeverExchangesCode(t *testing.T) {
	t.Parallel()
	e := newSourceLinkEnv(t)
	state := e.startSource(t, e.member, "/projects")
	before := e.adapter.hits.Load()
	assert.Contains(t, e.completeSource(t, e.admin, state, "").Header().Get("Location"), "link_failed")
	otherSession := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, otherSession, _otherSub).Code)
	assert.Contains(t, e.completeSource(t, otherSession, state, "").Header().Get("Location"), "link_failed")
	assert.Contains(t, e.completeSource(t, e.member, "unknown-state", "").Header().Get("Location"), "link_failed")
	assert.Equal(t, before, e.adapter.hits.Load())
	e.clock.Advance(10 * time.Minute)
	assert.Contains(t, e.completeSource(t, e.member, state, "").Header().Get("Location"), "link_failed")
	assert.Equal(t, before, e.adapter.hits.Load())
	state = e.startSource(t, e.member, "/projects")
	before = e.adapter.hits.Load()
	assert.Contains(t, e.completeSource(t, e.member, state, "access_denied").Header().Get("Location"), "link_failed")
	assert.Contains(t, e.completeSource(t, e.member, state, "").Header().Get("Location"), "link_failed")
	assert.Equal(t, before, e.adapter.hits.Load())
	assert.Empty(t, e.audit(t, "source_link.link"))
}

func TestSourceLinks_VisibilityModesAndExpiry(t *testing.T) {
	t.Parallel()
	e := newSourceLinkEnv(t)
	outsider := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, outsider, "outsider").Code)
	assert.Empty(t, e.sourceLinks(t, outsider).Data)
	path := "/api/v1/me/source-links/" + e.conn.String() + "/authorization"
	assert.Equal(t, http.StatusNotFound, e.do(t, outsider, http.MethodPost, path, http.Header{"Origin": {_site}}).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, newBrowser(), http.MethodGet, "/api/v1/me/source-links", nil).Code)
	state := e.startSource(t, e.member, "")
	e.completeSource(t, e.member, state, "")
	assert.True(t, e.sees(t, e.member))
	e.rights(t, testadapter.Access{Mode: "user_token", Default: "allowed", Expired: true})
	e.clock.Advance(61 * time.Second)
	assert.False(t, e.sees(t, e.member))
	assert.Equal(t, "expired", e.sourceLinks(t, e.member).Data[0].State)
	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection SET access_mode = 'service_account' WHERE id = $1", e.conn)
	require.NoError(t, err)
	assert.Empty(t, e.sourceLinks(t, e.member).Data)
	assert.Equal(t, http.StatusConflict, e.do(t, e.member, http.MethodPost, path, http.Header{"Origin": {_site}}).Code)
	_, err = e.owner.Exec(t.Context(), "UPDATE connect.connection SET access_mode = 'user_token', status = 'access_lost', access_lost_at = now() WHERE id = $1", e.conn)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, e.do(t, e.member, http.MethodPost, path, http.Header{"Origin": {_site}}).Code)
	assert.False(t, strings.Contains(e.logs.String(), _sourceCode))
}

func TestSourceLinks_RevocationInvalidatesPendingState(t *testing.T) {
	t.Parallel()
	e := newSourceLinkEnv(t)
	state := e.startSource(t, e.member, "/projects")
	rec := e.do(t, e.member, http.MethodDelete, "/api/v1/me/source-links/"+e.conn.String()+"/authorization", http.Header{"Origin": {_site}})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	before := e.adapter.hits.Load()
	assert.Contains(t, e.completeSource(t, e.member, state, "").Header().Get("Location"), "link_failed")
	assert.Equal(t, before, e.adapter.hits.Load())
	assert.Equal(t, "revoked", e.sourceLinks(t, e.member).Data[0].State)
	// RLS prevents the application role in another organization from seeing the link.
	var visible int
	require.NoError(t, e.app.QueryRow(t.Context(), "SELECT count(*) FROM access.source_link").Scan(&visible))
	assert.Zero(t, visible)
}

var _sourceSpec = sync.OnceValues(func() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	return loader.LoadFromFile("../../../docs/technical-specification/openapi.yaml")
})

func validateSourceResponse(t *testing.T, rec *httptest.ResponseRecorder, name string) {
	t.Helper()
	spec, err := _sourceSpec()
	require.NoError(t, err)
	var body any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NoError(t, spec.Components.Schemas[name].Value.VisitJSON(body))
}
