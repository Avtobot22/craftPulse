package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	docsrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/docs"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/docs"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type docsEnv struct {
	*loginEnv
	admin, member                  *browser
	service, project, area, person uuid.UUID
	uc                             usecase.Documentation
	rights                         usecase.Access
}

func newDocsEnv(t *testing.T) *docsEnv {
	t.Helper()
	e := &docsEnv{loginEnv: newLoginEnv(t), service: uuid.New(), project: uuid.New(), area: uuid.New(), person: uuid.New()}
	e.admin = e.loginEnv.admin(t, "demo")
	e.member = newBrowser()
	require.Equal(t, http.StatusFound, e.loginTo(t, e.member, "demo", _otherSub).Code)
	org := uuid.UUID(e.orgID)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.service (organization_id,id,name,status,created_at) VALUES ($1,$2,'API','active',$3)`, org, e.service, e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.project (organization_id,id,name,status,created_at) VALUES ($1,$2,'Core','active',$3)`, org, e.project, e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.person (organization_id,id,kind,created_at) VALUES ($1,$2,'human',$3)`, org, e.person, e.clock.Now())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.source_object (organization_id,id,instance_id,object_kind,native_ns,native_id,identity_quality,first_known_seq,restricted) VALUES ($1,$2,$3,'repository','','repo','stable',1,false)`, org, e.area, uuid.New())
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO normalize.area (organization_id,area_id,area_kind,last_change_seq) VALUES ($1,$2,'repository',1)`, org, e.area)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, org, e.project, e.area, e.clock.Now())
	require.NoError(t, err)
	e.rolesFor(t, []roleInput{{Role: "service_owner", ServiceID: &e.service}, {Role: "developer", ProjectID: &e.project}})
	m := e.modules(t)
	normalizer, err := newNormalize(e.app, nil, nil, m.orgs, nil, e.clock)
	require.NoError(t, err)
	m.documentationAreas = normalizer
	reg := prometheus.NewRegistry()
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, m)
	e.router = httpctrl.NewRouter(zerolog.New(zerolog.SyncWriter(e.logs)), reg, health.Probes{Ready: &health.Readiness{}}, api)
	e.rights = m.access
	e.uc = docs.New(docs.Deps{
		Repo: docsrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: persistent.NewTransactor(e.app),
		Access: m.access, References: m.orgs, Areas: normalizer, Clock: e.clock,
	})
	return e
}

func (e *docsEnv) rolesFor(t *testing.T, roles []roleInput) {
	t.Helper()
	target := "/api/v1/users/" + e.userID(t, e.orgID, _otherSub).String() + "/roles"
	rec := e.do(t, e.admin, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = e.roles(t, e.admin, e.userID(t, e.orgID, _otherSub), rec.Header().Get("ETag"), roles)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func (e *docsEnv) rootsURL() string {
	return "/api/v1/services/" + e.service.String() + "/documentation-roots"
}

func (e *docsEnv) root(path string) map[string]any {
	return map[string]any{"area_id": e.area, "path_prefix": path, "profile_id": "git-markdown-openapi", "owner_person_id": nil, "audience": "organization"}
}

func (e *docsEnv) putRoots(t *testing.T, b *browser, tag string, key uuid.UUID, roots ...map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if roots == nil {
		roots = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"data": roots})
	require.NoError(t, err)
	h := _changing.Clone()
	if tag != "" {
		h.Set("If-Match", tag)
	}
	h.Set("Idempotency-Key", key.String())
	return e.send(t, b, http.MethodPut, e.rootsURL(), h, bytes.NewReader(raw))
}

func (e *docsEnv) rootCounts(t *testing.T) [4]int64 {
	t.Helper()
	var out [4]int64
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM docs.root_set WHERE organization_id=$1),
 (SELECT count(*) FROM docs.root WHERE organization_id=$1),(SELECT count(*) FROM audit.event WHERE organization_id=$1 AND operation='docs.roots.replace'),
 (SELECT last_seq FROM platform.org_clock WHERE organization_id=$1)`, uuid.UUID(e.orgID)).Scan(&out[0], &out[1], &out[2], &out[3]))
	return out
}

func TestDocumentationRoots_LifecycleHistoryAndIdempotency(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	rec := e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"data":[]}`, rec.Body.String())
	require.Equal(t, `"0"`, rec.Header().Get("ETag"))
	key := uuid.New()
	root := e.root("docs")
	root["owner_person_id"] = e.person
	rec = e.putRoots(t, e.member, `"0"`, key, root, e.root("api"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	first := decode[openapi.DocumentationRootList](t, rec)
	require.Len(t, first.Data, 2)
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	var seq int64
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT known_seq FROM docs.root_set WHERE organization_id=$1 AND version_no=1`, uuid.UUID(e.orgID)).Scan(&seq))
	before := e.rootCounts(t)
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("api"), root)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	assert.Equal(t, before, e.rootCounts(t))
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("guide"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	second := decode[openapi.DocumentationRootList](t, rec)
	require.Len(t, second.Data, 1)
	assert.Nil(t, second.Data[0].OwnerPersonId)
	assert.Equal(t, `"2"`, rec.Header().Get("ETag"))
	before = e.rootCounts(t)
	rec = e.putRoots(t, e.member, `"0"`, key, root, e.root("api"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, first, decode[openapi.DocumentationRootList](t, rec))
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	assert.Equal(t, before, e.rootCounts(t))
	rec = e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	assert.Equal(t, second, decode[openapi.DocumentationRootList](t, rec))
	rec = e.putRoots(t, e.member, `"2"`, key, e.root("other"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, before, e.rootCounts(t))
	s := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.userID(t, e.orgID, _otherSub))}
	scope, err := e.rights.Authorize(t.Context(), s, entity.AccessRequest{Action: "docs.read", Area: entity.AreaRef{Kind: entity.AreaService, ID: entity.ID(e.service)}})
	require.NoError(t, err)
	scope, err = e.rights.Decide(t.Context(), s, scope, entity.SourceQuery{Areas: []entity.ID{entity.ID(e.area)}})
	require.NoError(t, err)
	history, err := e.uc.RootsAt(t.Context(), e.orgID, scope, entity.ID(e.service), entity.ID(e.area), seq)
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, entity.ID(e.person), *history[1].OwnerPersonID)
	require.NotNil(t, history[0].SupersededSeq)
	rec = e.putRoots(t, e.member, `"2"`, uuid.New())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"data":[]}`, rec.Body.String())
	assert.Equal(t, `"3"`, rec.Header().Get("ETag"))
	rec = e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	assert.JSONEq(t, `{"data":[]}`, rec.Body.String())
	assert.Equal(t, `"3"`, rec.Header().Get("ETag"))
	counts := e.rootCounts(t)
	assert.Equal(t, [3]int64{3, 3, 3}, [3]int64{counts[0], counts[1], counts[2]})
}

func TestDocumentationRoots_ValidationAndAccess(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	rec := e.putRoots(t, e.member, `"0"`, uuid.New(), e.root("docs"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	before := e.rootCounts(t)
	cases := []struct {
		name, tag, field, value string
		status                  int
	}{
		{"missing precondition", "", "", "", 428},
		{"stale precondition", `"0"`, "", "", 412},
		{"invalid precondition", "bad", "", "", 412},
		{"outside owner scope", `"1"`, "area_id", uuid.NewString(), 422},
		{"unknown profile", `"1"`, "profile_id", "other", 422},
		{"traversal", `"1"`, "path_prefix", "../secrets", 422},
		{"byte limit", `"1"`, "path_prefix", string(bytes.Repeat([]byte("я"), 513)), 422},
		{"audience", `"1"`, "audience", "public", 422},
		{"unknown owner", `"1"`, "owner_person_id", uuid.NewString(), 422},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := e.root("guide")
			if tt.field != "" {
				root[tt.field] = tt.value
			}
			rec := e.putRoots(t, e.member, tt.tag, uuid.New(), root)
			assert.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.name == "outside owner scope" {
				assert.Contains(t, rec.Body.String(), "outside_scope")
			}
			assert.Equal(t, before, e.rootCounts(t))
		})
	}
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("docs"), e.root("docs"))
	assert.Equal(t, 422, rec.Code, rec.Body.String())
	rec = e.putRoots(t, e.admin, `"1"`, uuid.New(), e.root("guide"))
	assert.Equal(t, 403, rec.Code, rec.Body.String())
	e.rolesFor(t, []roleInput{{Role: "developer", ProjectID: &e.project}})
	rec = e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	assert.Equal(t, 404, rec.Code, rec.Body.String())
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("guide"))
	assert.Equal(t, 404, rec.Code, rec.Body.String())
	e.rolesFor(t, []roleInput{{Role: "developer", ServiceID: &e.service}})
	rec = e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	assert.Equal(t, 200, rec.Code, rec.Body.String())
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("guide"))
	assert.Equal(t, 403, rec.Code, rec.Body.String())
	other := e.loginEnv.admin(t, "other")
	rec = e.do(t, other, http.MethodGet, e.rootsURL(), nil)
	assert.Equal(t, 404, rec.Code, rec.Body.String())
	var count int
	require.NoError(t, persistent.NewTransactor(e.app).InOrgTx(t.Context(), e.otherID, func(ctx context.Context) error {
		return postgres.Conn(ctx, e.app).QueryRow(ctx, "SELECT count(*) FROM docs.root").Scan(&count)
	}))
	// Row-level security hides the roots of the other organization.
	assert.Zero(t, count)
	assert.Equal(t, before, e.rootCounts(t))
}

func TestDocumentationRoots_ConcurrentConfirmation(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	raw, err := json.Marshal(map[string]any{"data": []map[string]any{e.root("docs")}})
	require.NoError(t, err)
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "same key", false: "stale forms"}[sameKey], func(t *testing.T) {
			rec := e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
			tag := rec.Header().Get("ETag")
			key := uuid.New()
			codes := make([]int, 2)
			var wg sync.WaitGroup
			for i := range codes {
				h := _changing.Clone()
				h.Set("If-Match", tag)
				h.Set("Idempotency-Key", key.String())
				if !sameKey {
					h.Set("Idempotency-Key", uuid.NewString())
				}
				// Both requests get an immutable cookie snapshot; response cookies cannot race.
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, e.rootsURL(), bytes.NewReader(raw))
				req.Header = h
				for name, value := range e.member.cookies {
					req.AddCookie(&http.Cookie{Name: name, Value: value})
				}
				wg.Go(func() { rec := httptest.NewRecorder(); e.router.ServeHTTP(rec, req); codes[i] = rec.Code })
			}
			wg.Wait()
			if sameKey {
				assert.Equal(t, []int{200, 200}, codes)
			} else {
				assert.ElementsMatch(t, []int{200, 412}, codes)
			}
			// The next round has a different body, so it must try a new version.
			raw, err = json.Marshal(map[string]any{"data": []map[string]any{e.root("guide")}})
			require.NoError(t, err)
		})
	}
}

func TestDocumentationRoots_NoopKeyKeepsItsResult(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	first := e.putRoots(t, e.member, `"0"`, uuid.New(), e.root("docs"))
	require.Equal(t, 200, first.Code, first.Body.String())
	noopKey := uuid.New()
	rec := e.putRoots(t, e.member, `"1"`, noopKey, e.root("docs"))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, 200, e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("guide")).Code)
	before := e.rootCounts(t)
	rec = e.putRoots(t, e.member, `"2"`, noopKey, e.root("guide"))
	assert.Equal(t, 422, rec.Code, rec.Body.String())
	rec = e.putRoots(t, e.member, `"1"`, noopKey, e.root("docs"))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Equal(t, first.Body.String(), rec.Body.String())
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	assert.Equal(t, before, e.rootCounts(t))
}

func TestDocumentationRoots_ErrorIndexMatchesSubmittedOrder(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	rec := e.putRoots(t, e.member, `"0"`, uuid.New(), e.root("docs"))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	// The inaccessible area sorts before the allowed one, but occupies row 1 in the request.
	denied := e.root("private")
	denied["area_id"] = "00000000-0000-0000-0000-000000000001"
	rec = e.putRoots(t, e.member, `"1"`, uuid.New(), e.root("docs"), denied)
	require.Equal(t, 422, rec.Code, rec.Body.String())
	problem := decode[problemBody](t, rec)
	require.NotEmpty(t, problem.Errors)
	assert.Equal(t, "/data/1/area_id", problem.Errors[0].Field)
}

var errDocsAudit = errors.New("audit unavailable")

type failedDocsAudit struct{}

func (failedDocsAudit) Record(context.Context, entity.AuditEvent) error { return errDocsAudit }

func TestDocumentationRoots_AuditFailureRollsBack(t *testing.T) {
	t.Parallel()
	e := newDocsEnv(t)
	require.Equal(t, 200, e.putRoots(t, e.member, `"0"`, uuid.New(), e.root("docs")).Code)
	before := e.rootCounts(t)
	m := e.modules(t)
	n, err := newNormalize(e.app, nil, nil, m.orgs, nil, e.clock)
	require.NoError(t, err)
	u := docs.New(docs.Deps{
		Repo: docsrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: persistent.NewTransactor(e.app), Access: m.access,
		References: m.orgs, Areas: n, Auditor: failedDocsAudit{}, Clock: e.clock, NewID: newID,
	})
	s := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.userID(t, e.orgID, _otherSub))}
	_, err = u.ReplaceRoots(t.Context(), s, entity.ID(e.service), 1, entity.ID(uuid.New()), []entity.DocumentationRoot{{AreaID: entity.ID(e.area), PathPrefix: "guide", ProfileID: "git-markdown-openapi", Audience: "team"}})
	require.ErrorIs(t, err, errDocsAudit)
	assert.Equal(t, before, e.rootCounts(t))
	rec := e.do(t, e.member, http.MethodGet, e.rootsURL(), nil)
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	assert.Equal(t, "docs", decode[openapi.DocumentationRootList](t, rec).Data[0].PathPrefix)
}
