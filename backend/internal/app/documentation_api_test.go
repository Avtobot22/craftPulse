package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

func TestDocumentationAPI_CheckStateAndRecheck(t *testing.T) {
	pinSpectralFixture(t)
	e, docs, scope := docsCalcEnv(t)
	api, owner := documentationAPI(t, e)
	empty := newDocumentationService(t, e, "Empty")
	emptyPage := documentPage(t, api, owner, empty, "")
	assert.Equal(t, openapi.DataState("UNKNOWN"), emptyPage.DocumentationState)
	assert.Empty(t, emptyPage.Data)
	assert.Nil(t, emptyPage.NextCursor)
	assert.Equal(t, openapi.DataState("NOT_EVALUATED"), documentPage(t, api, owner, scope.ServiceID, "").DocumentationState)

	path := "/api/v1/services/" + scope.ServiceID.String() + "/documentation-checks"
	request := func(root entity.ID, b *browser, status int) *openapi.BackgroundOperation {
		t.Helper()
		input := map[string]string{"root_id": root.String()}
		schema := api.contract.Components.Schemas["DocumentationCheckRequest"].Value
		require.NoError(t, schema.VisitJSON(map[string]any{"root_id": input["root_id"]}))
		rec := api.call(t, b, "", http.MethodPost, path, _changing, mustMarshal(t, input))
		require.Equal(t, status, rec.Code, rec.Body.String())
		if status != http.StatusAccepted {
			return nil
		}
		op := decode[openapi.BackgroundOperation](t, rec)
		assert.Equal(t, "/api/v1/operations/"+op.Id.String(), rec.Header().Get("Location"))
		return &op
	}
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	roles, err := orgs.UserRoles(t.Context(), e.admin, e.admin.UserID)
	require.NoError(t, err)
	before := roles.Roles
	_, err = orgs.ReplaceUserRoles(t.Context(), e.admin, e.admin.UserID, roles.Version,
		append(roles.Roles, entity.RoleAssignment{Role: entity.RoleServiceOwner, ServiceID: empty}))
	require.NoError(t, err)
	other, err := docs.ReplaceRoots(t.Context(), e.admin, empty, 0, entity.ID(uuid.New()), []entity.DocumentationRoot{{
		AreaID: scope.RepositoryID, PathPrefix: "docs", ProfileID: "git-markdown-openapi", Audience: "organization",
	}})
	require.NoError(t, err)
	request(other.Roots[0].ID, owner, http.StatusUnprocessableEntity)

	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	rec := api.call(t, member, "", http.MethodGet, documentPath(scope.ServiceID), nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	// The administrator can read the other service but lacks docs.manage there.
	roles, err = orgs.UserRoles(t.Context(), e.admin, e.admin.UserID)
	require.NoError(t, err)
	_, err = orgs.ReplaceUserRoles(t.Context(), e.admin, e.admin.UserID, roles.Version, before)
	require.NoError(t, err)
	rec = api.call(t, owner, "", http.MethodPost, "/api/v1/services/"+empty.String()+"/documentation-checks",
		_changing, mustMarshal(t, openapi.DocumentationCheckRequest{RootId: uuid.UUID(other.Roots[0].ID)}))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	profile, err := ci.LoadDocsProfile(filepath.Join("..", "ci", "profiles", "git-markdown-openapi"))
	require.NoError(t, err)
	checkout := documentationCheckout(t)
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "docs", "README.md"), []byte("# Service documentation\n\n[Guide](guide.md#absent)\n"), 0o600))
	identifier := newIdentifier(e.app, e.norm, orgs, e.clock)
	normalized := &identityEnv{checksEnv: api, normalized: e.normalizeEnv, identifier: identifier}
	var pending *openapi.BackgroundOperation
	var anchorID uuid.UUID
	var firstSeen time.Time
	for i, shaDigit := range []string{"1", "2", "3", "4", "5", "6"} {
		switch i {
		case 1:
			updateDocFixture(t, checkout, "README.md", "# Service documentation", "---\nupdated: 2026-11-20\n---\n# Service documentation")
		case 2, 5:
			updateDocFixture(t, checkout, "guide.md", "# Guide\n", "# Guide\n\n## Absent\n")
		case 3:
			updateDocFixture(t, checkout, "guide.md", "\n## Absent\n", "")
		}
		e.clock.Advance(time.Minute)
		scope.SHA = strings.Repeat(shaDigit, 40)
		at := e.clock.Now().Add(-time.Second)
		rc := e.imported(t, e.admin, docsProfilePackage(t, profile, at, at, scope, checkout, i))
		require.Equal(t, entity.ReceiptAccepted, rc.Status, rc.ReasonCode)
		e.drain(t)
		wantState := openapi.DataState("ASSESSED")
		switch i {
		case 2:
			// Model an upload whose docs check skipped input, even though it saw the fixed anchor.
			_, err := e.owner.Exec(t.Context(), `UPDATE normalize.report SET completeness='partial'
				WHERE organization_id=$1 AND receipt_id=$2 AND report_kind='docs'`, uuid.UUID(e.orgID), uuid.UUID(rc.ID))
			require.NoError(t, err)
			wantState = "PROVISIONAL"
		case 3, 4:
			// Model a full check that omits the still-broken anchor after rules or configuration changes.
			omitAnchorWithChangedProfile(t, e, rc.ID, i == 3)
		}
		normalized.drain(t)
		page := documentPage(t, api, owner, scope.ServiceID, "")
		require.Equal(t, wantState, page.DocumentationState)
		doc := readmeDocument(t, page)
		assert.Equal(t, scope.SHA, doc.VersionRef)
		assert.Equal(t, "markdown", string(doc.DocFormat))
		require.NotNil(t, doc.LastCheckedAt)
		assert.Equal(t, at.UTC(), doc.LastCheckedAt.UTC())
		require.NotNil(t, doc.SourceUrl)
		assert.Equal(t, _gitlabInstance+"/demo/docs/-/blob/"+scope.SHA+"/docs/README.md", *doc.SourceUrl)
		require.NotNil(t, doc.OpenViolations)
		if i < 5 {
			assert.Equal(t, 1, *doc.OpenViolations)
			var id uuid.UUID
			var since time.Time
			require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id, first_seen_at FROM change.finding_identity
				WHERE organization_id=$1 AND area_id=$2 AND rule_id='missing-anchor'`,
				uuid.UUID(e.orgID), uuid.UUID(scope.RepositoryID)).Scan(&id, &since))
			if i == 0 {
				anchorID, firstSeen = id, since
			} else {
				assert.Equal(t, anchorID, id)
				assert.Equal(t, firstSeen, since)
			}
		} else {
			assert.Zero(t, *doc.OpenViolations)
		}
		if pending != nil {
			rec := api.call(t, owner, "", http.MethodGet, "/api/v1/operations/"+pending.Id.String(), nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			got := decode[openapi.BackgroundOperation](t, rec)
			if i == 2 {
				assert.Equal(t, "queued", string(got.Status))
				assert.Equal(t, pending.Id, request(scope.RootID, owner, http.StatusAccepted).Id)
				continue
			}
			assert.Equal(t, "succeeded", string(got.Status))
			require.NotNil(t, got.ResultUri)
			assert.Equal(t, documentPath(scope.ServiceID), *got.ResultUri)
		}
		pending = request(scope.RootID, owner, http.StatusAccepted)
		assert.Equal(t, "queued", string(pending.Status))
		assert.Equal(t, pending.Id, request(scope.RootID, owner, http.StatusAccepted).Id)
	}
	page := documentPage(t, api, owner, scope.ServiceID, "?limit=1")
	require.Len(t, page.Data, 1)
	require.NotNil(t, page.NextCursor)
	next := documentPage(t, api, owner, scope.ServiceID, "?limit=1&cursor="+url.QueryEscape(*page.NextCursor))
	require.Len(t, next.Data, 1)
	assert.NotEqual(t, page.Data[0].Id, next.Data[0].Id)

	// A version with unknown ACL stays out of the corpus even for the service owner.
	readme := readmeDocument(t, documentPage(t, api, owner, scope.ServiceID, ""))
	_, err = e.owner.Exec(t.Context(), `UPDATE normalize.document_version SET acl_state='unknown'
		WHERE organization_id=$1 AND document_id=$2 AND version_ref=$3`, uuid.UUID(e.orgID), readme.Id, readme.VersionRef)
	require.NoError(t, err)
	for _, doc := range documentPage(t, api, owner, scope.ServiceID, "").Data {
		assert.NotEqual(t, readme.Id, doc.Id)
	}
}

func omitAnchorWithChangedProfile(t *testing.T, e *reviewEnv, receipt entity.ID, rulesChanged bool) {
	t.Helper()
	result, err := e.owner.Exec(t.Context(), `WITH changed AS (
		UPDATE normalize.report SET
			rules_hash = CASE WHEN $3 THEN decode(repeat('ab', 32), 'hex') ELSE rules_hash END,
			config_hash = CASE WHEN $3 THEN config_hash ELSE decode(repeat('cd', 32), 'hex') END
		WHERE organization_id=$1 AND receipt_id=$2 AND report_kind='docs'
		RETURNING id
	)
	DELETE FROM normalize.finding_occurrence
	WHERE organization_id=$1 AND report_id IN (SELECT id FROM changed)
		AND path='docs/README.md' AND rule_id='missing-anchor'`, uuid.UUID(e.orgID), uuid.UUID(receipt), rulesChanged)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RowsAffected())
}

func TestDocumentationAPI_ConcurrentRequestsReuseOperation(t *testing.T) {
	e, _, scope := docsCalcEnv(t)
	_, owner := documentationAPI(t, e)
	path := "/api/v1/services/" + scope.ServiceID.String() + "/documentation-checks"
	body := mustMarshal(t, openapi.DocumentationCheckRequest{RootId: uuid.UUID(scope.RootID)})
	var requests sync.WaitGroup
	start := make(chan struct{})
	var responses [5]*httptest.ResponseRecorder
	for i := range responses {
		requests.Go(func() {
			<-start
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(body))
			req.Header = _changing.Clone()
			for name, value := range owner.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: value})
			}
			rec := httptest.NewRecorder()
			e.router.ServeHTTP(rec, req)
			responses[i] = rec
		})
	}
	close(start)
	requests.Wait()
	for _, rec := range responses {
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Equal(t, responses[0].Header().Get("Location"), rec.Header().Get("Location"))
	}
}

func documentationAPI(t *testing.T, e *reviewEnv) (*checksEnv, *browser) {
	t.Helper()
	m := e.modules(t)
	m.documentationAreas, m.findingReports = e.norm, e.norm
	reg := prometheus.NewRegistry()
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, m)
	e.router = httpctrl.NewRouter(zerolog.New(zerolog.SyncWriter(e.logs)), reg, health.Probes{Ready: &health.Readiness{}}, api)
	owner := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, owner, _adminSub).Code)
	contract, err := openapi3.NewLoader().LoadFromFile("../../../docs/technical-specification/openapi.yaml")
	require.NoError(t, err)
	return &checksEnv{ciEnv: &ciEnv{ingestEnv: e.ingestEnv, contract: contract}}, owner
}

func documentPath(service entity.ID) string {
	return "/api/v1/services/" + service.String() + "/documents"
}

func documentPage(t *testing.T, api *checksEnv, b *browser, service entity.ID, query string) openapi.DocumentPage {
	t.Helper()
	rec := api.call(t, b, "", http.MethodGet, documentPath(service)+query, nil, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[openapi.DocumentPage](t, rec)
}

func readmeDocument(t *testing.T, page openapi.DocumentPage) openapi.Document {
	t.Helper()
	for i := range page.Data {
		if d := &page.Data[i]; d.Path != nil && *d.Path == "docs/README.md" {
			return *d
		}
	}
	t.Fatal("README.md is missing")
	return openapi.Document{}
}
