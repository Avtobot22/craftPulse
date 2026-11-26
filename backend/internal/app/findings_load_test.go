package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type findingsLoadManifest struct {
	BaseURL  string              `json:"base_url"`
	Sessions []string            `json:"sessions"`
	Paths    map[string][]string `json:"paths"`
}

func TestFindings_LoadProfileD(t *testing.T) {
	if os.Getenv("CRAFTPULSE_LOAD") != "1" {
		t.Skip("set CRAFTPULSE_LOAD=1 to run the findings load profile")
	}
	e := newIdentityEnv(t)
	ids := e.importOverviewFindings(t)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)
	m := findingsLoadManifest{BaseURL: srv.URL, Paths: map[string][]string{
		"listFindings": {
			_findingsPath + "?limit=1",
			_findingsPath + "?finding_kind=security",
			_findingsPath + "?finding_kind=dependency",
			_findingsPath + "?component_id=" + e.docs.String(),
			_findingsPath + "?component_id=" + e.goComponent.String(),
		},
		"getComponentOverview": {
			"/api/v1/components/" + e.goComponent.String() + "/overview",
			"/api/v1/components/" + e.docs.String() + "/overview",
		},
	}}
	for _, id := range ids {
		m.Paths["getFinding"] = append(m.Paths["getFinding"], _findingsPath+"/"+id.String())
	}
	slices.Sort(m.Paths["getFinding"])
	require.GreaterOrEqual(t, len(m.Paths["getFinding"]), 5)

	project := uuid.UUID(e.project)
	e.adminBrowser = newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, e.adminBrowser, _adminSub).Code)
	for i := range 5 {
		b := e.exceptionActor(t, fmt.Sprintf("load-findings-%d", i), []roleInput{
			{Role: "developer", ProjectID: &project}, {Role: "security_access", ProjectID: &project},
		})
		cookies := make([]string, 0, len(b.cookies))
		for name, value := range b.cookies {
			cookies = append(cookies, name+"="+value)
		}
		m.Sessions = append(m.Sessions, strings.Join(cookies, "; "))
	}
	k6, err := osexec.LookPath("k6")
	if err != nil {
		t.Log("k6 is unavailable. Using the same fixture, iterations and thresholds via Go HTTP.")
		measureFindingReads(t, srv.Client(), &m)
		return
	}
	dir := t.TempDir()
	manifest, summary := filepath.Join(dir, "manifest.json"), filepath.Join(dir, "summary.json")
	require.NoError(t, os.WriteFile(manifest, mustMarshal(t, m), 0o600))
	cmd := osexec.CommandContext(t.Context(), k6, "run", "--quiet", "testdata/k6/findings.js") //nolint:gosec // the test runs the k6 it found
	cmd.Env = append(os.Environ(), "MANIFEST="+manifest, "SUMMARY="+summary)
	out, runErr := cmd.CombinedOutput()
	t.Logf("k6 output:\n%s", out)
	logSummary(t, summary)
	require.NoError(t, runErr)
}

func measureFindingReads(t *testing.T, client *http.Client, m *findingsLoadManifest) {
	t.Helper()
	const warmup, measured = 5, 20
	measurements := make([]map[string][]time.Duration, len(m.Sessions))
	sessions, ctx := errgroup.WithContext(t.Context())
	for vu, session := range m.Sessions {
		sessions.Go(func() error {
			results := map[string][]time.Duration{}
			for cycle := range warmup + measured {
				for op, paths := range m.Paths {
					path := paths[(vu+cycle)%len(paths)]
					start := time.Now()
					if err := timedFindingRead(ctx, client, m.BaseURL+path, session); err != nil {
						return fmt.Errorf("%s: %w", op, err)
					}
					if cycle >= warmup {
						results[op] = append(results[op], time.Since(start))
					}
				}
			}
			measurements[vu] = results
			return nil
		})
	}
	require.NoError(t, sessions.Wait())
	for op := range m.Paths {
		times := make([]time.Duration, 0, len(m.Sessions)*measured)
		for _, perSession := range measurements {
			times = append(times, perSession[op]...)
		}
		require.Len(t, times, len(m.Sessions)*measured)
		slices.Sort(times)
		p95, maxTime := times[(95*len(times)+99)/100-1], times[len(times)-1]
		t.Logf("%s n=%d p95=%s max=%s errors=0", op, len(times), p95, maxTime)
		assert.LessOrEqual(t, p95, 2*time.Second)
		assert.LessOrEqual(t, maxTime, 5*time.Second)
	}
}

func timedFindingRead(ctx context.Context, client *http.Client, endpoint, cookie string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}
