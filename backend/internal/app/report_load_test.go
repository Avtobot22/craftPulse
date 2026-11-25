package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _loadTasks is the volume of the tasks of D-data (TZ 8.2): 100 tasks with about 1 000 events.
const _loadTasks = 100

type loadManifest struct {
	BaseURL    string              `json:"base_url"`
	TeamID     string              `json:"team_id"`
	RevisionID string              `json:"revision_id"`
	From       string              `json:"from"`
	To         string              `json:"to"`
	Sessions   []string            `json:"sessions"`
	Filters    []map[string]string `json:"filters"`
	Points     []map[string]string `json:"points"`
	TaskPaths  []string            `json:"task_paths,omitempty"`
}

// collectLoad writes 100 tasks over the window of the series, about ten events each: the start, three
// blockings by the flag, a resize and the completion, every fifth task cancelled instead.
func (e *reportEnv) collectLoad(t *testing.T) {
	t.Helper()
	e.collectLoadTasks(t, _loadTasks)
}

func (e *reportEnv) collectLoadTasks(t *testing.T, count int) {
	t.Helper()
	first := e.week.AddDate(0, 0, -7*30)
	tasks, events := make([]*envelope.Record, 0, count), make([]*envelope.Record, 0, 10*count)
	for i := range count {
		id := strconv.Itoa(32000 + i)
		key := "L-" + strconv.Itoa(i+1)
		created := first.Add(time.Duration(i) * 50 * time.Hour)
		at := func(h int) time.Time { return created.Add(time.Duration(h) * time.Hour) }
		kind, size, done := "10001", "S", "5"
		if i%2 == 1 {
			kind, size = "10002", "M"
		}
		if i%5 == 4 {
			done = _cancelStatus
		}
		p := envelope.TaskV1{ProjectNativeId: "10000", TaskKey: &key, Title: &key, TypeNativeId: kind, StatusNativeId: done, Size: &size, CreatedAt: created}
		if done == "5" {
			resolution := "10000"
			p.ResolutionNativeId = &resolution
		}
		raw, err := json.Marshal(p)
		require.NoError(t, err)
		tasks = append(tasks, &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/1", NativeId: id, Payload: raw})
		change := func(n, h int, field string, before, after *string) {
			changelog := id + "-" + strconv.Itoa(n)
			raw, err := json.Marshal(envelope.TaskEventV1{
				TaskNativeId: id, EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: before, AfterValue: after,
			})
			require.NoError(t, err)
			when := at(h)
			events = append(events, &envelope.Record{
				RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: id,
				NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &when, Payload: raw,
			})
		}
		open, active, flag, other := "1", "3", "Impediment", "S"
		change(1, 2, "status", &open, &active)
		for b := range 3 {
			change(2+2*b, 8+10*b, "customfield_10020", nil, &flag)
			change(3+2*b, 12+10*b, "customfield_10020", &flag, nil)
		}
		if size == "M" {
			change(8, 40, "customfield_10016", &other, &size)
		} else {
			change(8, 40, "customfield_10016", &other, &other)
		}
		change(9, 48+(i%7)*24, "status", &active, &done)
		if done == "5" {
			resolution := "10000"
			change(9, 48+(i%7)*24, "resolution", nil, &resolution)
		}
	}
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"), tasks...)
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"), events...)
}

// TestReport_LoadProfileD runs the D-load profile of TZ 8.2 with k6 against the API on the volume of D-data and
// fails when an operation breaks p95 <= 2 s, max <= 5 s or has an error. It needs k6 and CRAFTPULSE_LOAD=1.
func TestReport_LoadProfileD(t *testing.T) {
	if os.Getenv("CRAFTPULSE_LOAD") != "1" {
		t.Skip("set CRAFTPULSE_LOAD=1 to run the D-load profile with k6")
	}
	k6, err := osexec.LookPath("k6")
	if err != nil {
		t.Skip("k6 is not installed")
	}
	e := newReportEnv(t, true)
	e.collectLoad(t)
	month := url.Values{"step": {"month"}}
	require.Equal(t, http.StatusNotFound, e.report(t, e.admin, month).Code)
	r := e.publish(t)
	require.Equal(t, http.StatusOK, e.report(t, e.admin, month).Code)

	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)
	m := loadManifest{
		BaseURL: srv.URL, TeamID: e.team.String(), RevisionID: r.Revision.ID.String(),
		From: e.week.AddDate(0, 0, -7*30).Format(time.RFC3339), To: e.next.AddDate(0, 0, 7).Format(time.RFC3339),
		Filters: []map[string]string{{}, {"size_label": "S"}, {"task_type": "Story"}, {"group_by": "size_label"}, {"size_label": "unknown"}},
	}
	for range 5 {
		b := newBrowser()
		require.Equal(t, http.StatusFound, e.login(t, b, _adminSub).Code)
		cookies := make([]string, 0, len(b.cookies))
		for name, value := range b.cookies {
			cookies = append(cookies, name+"="+value)
		}
		m.Sessions = append(m.Sessions, strings.Join(cookies, "; "))
	}
	for _, s := range r.Metrics {
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
	// A k6 of a snap does not see the temporary directory of the system; the files stay next to the script.
	dir, err := os.MkdirTemp("testdata/k6", "run-")
	require.NoError(t, err)
	dir, err = filepath.Abs(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // a leftover run directory is harmless
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	manifest, summary := filepath.Join(dir, "manifest.json"), filepath.Join(dir, "summary.json")
	require.NoError(t, os.WriteFile(manifest, raw, 0o600))

	cmd := osexec.CommandContext(t.Context(), k6, "run", "--quiet", "testdata/k6/report.js") //nolint:gosec // the test runs the k6 it found
	cmd.Env = append(os.Environ(), "MANIFEST="+manifest, "SUMMARY="+summary)
	out, runErr := cmd.CombinedOutput()
	t.Logf("k6 output:\n%s", out)
	logSummary(t, summary)
	require.NoError(t, runErr, "a threshold of D-load failed")
}

// logSummary prints p50, p95, p99, max and the share of errors of every measured operation.
func logSummary(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // the file is in the temporary directory of the test
	require.NoError(t, err)
	var s struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(raw, &s))
	var lines []string
	for name, metric := range s.Metrics {
		if strings.HasPrefix(name, "http_req_duration{phase:measure") {
			v := metric.Values
			lines = append(lines, fmt.Sprintf("%s n=%.0f p50=%.1fms p95=%.1fms p99=%.1fms max=%.1fms", name, v["count"], v["med"], v["p(95)"], v["p(99)"], v["max"]))
		}
		if strings.HasPrefix(name, "http_req_failed{phase:measure") {
			lines = append(lines, fmt.Sprintf("%s rate=%.4f", name, metric.Values["rate"]))
		}
	}
	sort.Strings(lines)
	t.Logf("D-load, measured cycles:\n%s", strings.Join(lines, "\n"))
}
