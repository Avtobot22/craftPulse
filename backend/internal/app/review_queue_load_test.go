package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// TestReviewQueue_LoadProfileD measures the D-data volume relevant to this query (six GitLab MRs and four
// GitHub PRs) with the D-load session count and warmup. It runs against a local API and PostgreSQL.
func TestReviewQueue_LoadProfileD(t *testing.T) {
	if os.Getenv("CRAFTPULSE_LOAD") != "1" {
		t.Skip("set CRAFTPULSE_LOAD=1 to run the D-load profile with k6")
	}
	k6, err := osexec.LookPath("k6")
	require.NoError(t, err)
	e, b := codeEnv(t)
	gitlab, github := seedQueueVolume(t, e)
	project := e.project.String()
	filters := []map[string]string{
		{"project_id": project, "limit": "10", "sort": "-wait"},
		{"project_id": project, "limit": "10", "review_wait": "no_reviewer"},
		{"project_id": project, "limit": "10", "review_wait": "awaiting_first_response"},
		{"project_id": project, "area_id": gitlab.String(), "limit": "10", "sort": "opened_at"},
		{"project_id": project, "area_id": github.String(), "limit": "10", "sort": "wait"},
	}
	for i, count := range []int{10, 5, 5, 6, 4} {
		q := url.Values{}
		for key, value := range filters[i] {
			q.Set(key, value)
		}
		require.Len(t, reviewQueue(t, e.loginEnv, b, q).Data, count)
	}
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)
	sessions := make([]string, 0, 5)
	for range 5 {
		browser := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, browser, _adminSub).Code)
		cookies := make([]string, 0, len(browser.cookies))
		for name, value := range browser.cookies {
			cookies = append(cookies, name+"="+value)
		}
		sessions = append(sessions, strings.Join(cookies, "; "))
	}
	dir := t.TempDir()
	manifest, summary := filepath.Join(dir, "manifest.json"), filepath.Join(dir, "summary.json")
	require.NoError(t, os.WriteFile(manifest, mustMarshal(t, map[string]any{"base_url": srv.URL, "sessions": sessions, "filters": filters}), 0o600))
	cmd := osexec.CommandContext(t.Context(), k6, "run", "--quiet", "testdata/k6/review-queue.js") //nolint:gosec // the test runs the k6 it found
	cmd.Env = append(os.Environ(), "MANIFEST="+manifest, "SUMMARY="+summary)
	out, err := cmd.CombinedOutput()
	t.Logf("Environment: %s/%s, Go %s, %d logical CPUs, PostgreSQL 18 container", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU())
	t.Logf("k6 output:\n%s", out)
	if _, statErr := os.Stat(summary); statErr == nil {
		logSummary(t, summary)
	}
	require.NoError(t, err, "a threshold of D-load failed")
}

func seedQueueVolume(t *testing.T, e *normalizeEnv) (gitlabArea, githubArea entity.ID) {
	t.Helper()
	gitlab := make([]*envelope.Record, 0, 7)
	gitlab = append(gitlab, mrRepository(t, "42", "team/queue", "r1"))
	for i := range 6 {
		reviewers := []string{}
		if i%2 == 0 {
			reviewers = []string{"2"}
		}
		gitlab = append(gitlab, queueMR(t, strconv.Itoa(9001+i), "42", _created.Add(time.Duration(i)*time.Hour), ptr(true), reviewers, false))
	}
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), gitlab...))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	gitlabArea = mrArea(t, e, rc, "42")
	bindLinkArea(t, e, gitlabArea)
	github := make([]*envelope.Record, 0, 4)
	for i := range 4 {
		reviewers := []string{}
		if i%2 == 0 {
			reviewers = []string{"200"}
		}
		github = append(github, githubPR(t, strconv.Itoa(9501+i), "81", "r1", false, reviewers))
	}
	rc = e.imported(t, e.admin, pack(t, githubManifest(envelope.ManifestSchemaIdMergeRequests, "81"), github...))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	githubArea = mrArea(t, e, rc, "81")
	bindLinkArea(t, e, githubArea)
	return gitlabArea, githubArea
}
