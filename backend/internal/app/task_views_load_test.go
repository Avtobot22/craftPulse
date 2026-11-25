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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// TestTaskViews_LoadProfileD adds the card as the fourth D-load operation and measures the first page separately.
func TestTaskViews_LoadProfileD(t *testing.T) {
	if os.Getenv("CRAFTPULSE_LOAD") != "1" {
		t.Skip("set CRAFTPULSE_LOAD=1 to run the D-load profile with k6")
	}
	k6, err := osexec.LookPath("k6")
	require.NoError(t, err)
	e := newReportEnv(t, true)
	var initial int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task'").Scan(&initial))
	e.collectLoadTasks(t, _loadTasks-initial)
	task, _, _, _, _ := seedTaskCard(t, e)
	seedTaskLoadEvents(t, e)
	paths := seedTaskLinkVolume(t, e, task)
	month := url.Values{"step": {"month"}}
	require.Equal(t, http.StatusNotFound, e.report(t, e.admin, month).Code)
	revision := e.publish(t)
	require.Equal(t, http.StatusOK, e.report(t, e.admin, month).Code)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)
	m := loadManifest{
		BaseURL: srv.URL, TeamID: e.team.String(), RevisionID: revision.Revision.ID.String(),
		From: e.week.AddDate(0, 0, -7*30).Format(time.RFC3339), To: e.next.AddDate(0, 0, 7).Format(time.RFC3339),
		Filters:   []map[string]string{{}, {"size_label": "S"}, {"task_type": "Story"}, {"group_by": "size_label"}, {"size_label": "unknown"}},
		TaskPaths: paths,
	}
	for _, s := range revision.Metrics {
		for _, p := range s.Points {
			if p.Value.Value == nil || *p.Value.Value == 0 || s.Group != nil {
				continue
			}
			point := map[string]string{"metric_id": s.MetricID, "point_start": p.PointStart.Format(time.RFC3339)}
			if s.Estimator != nil {
				point["estimator"] = *s.Estimator
			}
			m.Points = append(m.Points, point)
		}
	}
	require.NotEmpty(t, m.Points)
	for range 5 {
		browser := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, browser, _adminSub).Code)
		cookies := make([]string, 0, len(browser.cookies))
		for name, value := range browser.cookies {
			cookies = append(cookies, name+"="+value)
		}
		m.Sessions = append(m.Sessions, strings.Join(cookies, "; "))
	}
	t.Logf("Environment: %s/%s, Go %s, %d logical CPUs, PostgreSQL 18 container", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU())
	var tasks, events, mrs, runs int
	for _, count := range []struct {
		query string
		out   *int
	}{
		{"SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task'", &tasks},
		{"SELECT count(*) FROM normalize.task_event WHERE superseded_seq IS NULL", &events},
		{"SELECT count(*) FROM normalize.source_object WHERE object_kind = 'merge_request'", &mrs},
		{"SELECT count(*) FROM normalize.source_object WHERE object_kind = 'ci_run'", &runs},
	} {
		require.NoError(t, e.owner.QueryRow(t.Context(), count.query).Scan(count.out))
	}
	require.Equal(t, 100, tasks)
	require.Equal(t, 1000, events)
	require.Equal(t, 10, mrs)
	require.Equal(t, 20, runs)
	t.Logf("Fixture: %d tasks, %d events, %d MRs/PRs, %d CI runs", tasks, events, mrs, runs)
	runTaskLoad(t, k6, "testdata/k6/report.js", &m)
	p, team := e.project.String(), e.team.String()
	m.Filters = []map[string]string{
		{"project_id": p, "limit": "10", "sort": "-age"},
		{"project_id": p, "limit": "10", "sort": "key"},
		{"project_id": p, "limit": "10", "sort": "-updated_at", "status_meaning": "done"},
		{"team_id": team, "limit": "10", "status_meaning": "active"},
		{"team_id": team, "limit": "10", "sort": "age"},
	}
	runTaskLoad(t, k6, "testdata/k6/task-list.js", &m)
}

func seedTaskLoadEvents(t *testing.T, e *reportEnv) {
	t.Helper()
	var count int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM normalize.task_event WHERE superseded_seq IS NULL").Scan(&count))
	events := make([]*envelope.Record, 0, 1000-count)
	for i := range 1000 - count {
		id := strconv.Itoa(32000 + i%80)
		key := "load-comment-" + strconv.Itoa(i)
		event := mrRecord(t, "task_event", key, id, key, envelope.TaskEventV1{
			TaskNativeId: id, EventKind: envelope.TaskEventV1EventKindComment,
		})
		event.RecordClass, event.EventTime = "event", &_linkMerged
		events = append(events, event)
	}
	e.collect(t, jiraEvents("10000"), events...)
}

func runTaskLoad(t *testing.T, k6, script string, m *loadManifest) {
	t.Helper()
	dir := t.TempDir()
	manifest, summary := filepath.Join(dir, "manifest.json"), filepath.Join(dir, "summary.json")
	require.NoError(t, os.WriteFile(manifest, mustMarshal(t, m), 0o600))
	cmd := osexec.CommandContext(t.Context(), k6, "run", "--quiet", script) //nolint:gosec // gated test with repository scripts
	cmd.Env = append(os.Environ(), "MANIFEST="+manifest, "SUMMARY="+summary)
	out, err := cmd.CombinedOutput()
	t.Logf("k6 output:\n%s", out)
	if _, statErr := os.Stat(summary); statErr == nil {
		logSummary(t, summary)
	}
	require.NoError(t, err, "a threshold of D-load failed")
}

func seedTaskLinkVolume(t *testing.T, e *reportEnv, first uuid.UUID) []string {
	t.Helper()
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	importer := e.codeImporter(t, admin)
	tasks := make([]uuid.UUID, 1, 6)
	tasks[0] = first
	mrs := make([]*envelope.Record, 0, 5)
	runs := make([]*envelope.Record, 0, 10)
	at := _linkMerged
	for i := range 5 {
		id := strconv.Itoa(9010 + i)
		tasks = append(tasks, uuid.UUID(e.objectID(t, "task", strconv.Itoa(32000+i))))
		mrs = append(mrs, mrRecord(t, "merge_request", id, "", "v1", envelope.MergeRequestV1{
			ProjectNativeId: "42", Iid: new(10 + i), Title: new("Load merge request"), State: "open",
			SourceBranch: "feature/L-" + strconv.Itoa(i+1), TargetBranch: "main", HeadSha: _headA,
			OpenedAt: at.Add(-time.Hour), ReadyAt: &at, UpdatedAt: &at, ReviewerField: new(true), ReviewerAccountNativeIds: new([]string{}),
		}))
		for n := range 2 {
			runs = append(runs, ciRun(t, runSpec{id: strconv.Itoa(5020 + i*2 + n), project: "42", sha: _headA, status: "success", revision: "p1", updated: at, mr: id}))
		}
	}
	importer(mrManifest("42"), mrs...)
	importer(ciManifest("42"), runs...)
	prs := make([]*envelope.Record, 0, 4)
	workflows := make([]*envelope.Record, 0, 8)
	for i := range 4 {
		id := strconv.Itoa(9100 + i)
		prs = append(prs, githubPR(t, id, "81", "r1", false, nil))
		for n := range 2 {
			workflows = append(workflows, githubRun(t, strconv.Itoa(5100+i*2+n), "81", _prHead, "success", id))
		}
	}
	importer(githubManifest(envelope.ManifestSchemaIdMergeRequests, "81"), prs...)
	importer(githubManifest(envelope.ManifestSchemaIdCiRuns, "81"), workflows...)
	e.grantManager(t)
	paths := make([]string, len(tasks))
	for i, id := range tasks {
		paths[i] = _tasksPath + "/" + id.String()
		detail := decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, paths[i], "TaskDetail"))
		require.Len(t, detail.Links, 1)
		rec := e.json(t, e.admin, http.MethodPost, "/api/v1/links/"+detail.Links[0].Id.String()+"/decisions",
			http.Header{"Idempotency-Key": {uuid.NewString()}}, map[string]string{"decision": "confirm"})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		detail = decode[openapi.TaskDetail](t, codeRead(t, e.loginEnv, e.admin, paths[i], "TaskDetail"))
		require.GreaterOrEqual(t, len(detail.Links), 2)
	}
	return paths
}
