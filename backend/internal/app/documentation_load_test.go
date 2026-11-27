package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestDocumentationAPI_LoadProfileD(t *testing.T) {
	if os.Getenv("CRAFTPULSE_LOAD") != "1" {
		t.Skip("set CRAFTPULSE_LOAD=1 to run the documentation load profile")
	}
	pinSpectralFixture(t)
	e, docs, scope := docsCalcEnv(t)
	api, owner := documentationAPI(t, e)
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	profile, err := ci.LoadDocsProfile(filepath.Join("..", "ci", "profiles", "git-markdown-openapi"))
	require.NoError(t, err)
	checkout := documentationCheckout(t)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)
	m := findingsLoadManifest{BaseURL: srv.URL, Paths: map[string][]string{}}
	ids := make([]string, 0, 5)
	for i := range 5 {
		if i > 0 {
			scope.ServiceID = newDocumentationService(t, e, fmt.Sprintf("Documentation %d", i))
			roles, err := orgs.UserRoles(t.Context(), e.admin, e.admin.UserID)
			require.NoError(t, err)
			_, err = orgs.ReplaceUserRoles(t.Context(), e.admin, e.admin.UserID, roles.Version,
				append(roles.Roles, entity.RoleAssignment{Role: entity.RoleServiceOwner, ServiceID: scope.ServiceID}))
			require.NoError(t, err)
			set, err := docs.ReplaceRoots(t.Context(), e.admin, scope.ServiceID, 0, entity.ID(uuid.New()), []entity.DocumentationRoot{{
				AreaID: scope.RepositoryID, PathPrefix: "docs", ProfileID: profile.ID, Audience: "organization",
			}})
			require.NoError(t, err)
			scope.RootID = set.Roots[0].ID
		}
		scope.SHA = strings.Repeat(strconv.Itoa(i+1), 40)
		e.clock.Advance(time.Minute)
		at := e.clock.Now().Add(-time.Second)
		rc := e.imported(t, e.admin, docsProfilePackage(t, profile, at, at, scope, checkout, i))
		require.Equal(t, entity.ReceiptAccepted, rc.Status)
		e.drain(t)
		require.Equal(t, "ASSESSED", string(documentPage(t, api, owner, scope.ServiceID, "").DocumentationState))
		ids = append(ids, scope.ServiceID.String())
		m.Paths["listServiceDocuments"] = append(m.Paths["listServiceDocuments"], documentPath(scope.ServiceID))

		session := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, session, _adminSub).Code)
		cookies := make([]string, 0, len(session.cookies))
		for name, value := range session.cookies {
			cookies = append(cookies, name+"="+value)
		}
		m.Sessions = append(m.Sessions, strings.Join(cookies, "; "))
	}

	k6, err := osexec.LookPath("k6")
	if err != nil {
		t.Log("k6 unavailable, measuring the fixture with Go HTTP and the same iterations and thresholds")
		measureFindingReads(t, srv.Client(), &m)
		return
	}
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	require.NoError(t, os.WriteFile(manifest, mustMarshal(t, map[string]any{
		"base_url": m.BaseURL, "sessions": m.Sessions, "service_ids": ids,
	}), 0o600))
	cmd := osexec.CommandContext(t.Context(), k6, "run", "--quiet", "testdata/k6/service-documents.js") //nolint:gosec // the test runs the k6 it found
	cmd.Env = append(os.Environ(), "MANIFEST="+manifest)
	out, err := cmd.CombinedOutput()
	t.Logf("k6 output:\n%s", out)
	require.NoError(t, err)
}
