package app

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var (
	_linux   = strings.Repeat("1", 64)
	_windows = strings.Repeat("2", 64)
	_headA   = strings.Repeat("a", 40)
	_headB   = strings.Repeat("e", 40)
)

func ciManifest(area string, truncated ...string) *envelope.Manifest {
	m := manifestOf(envelope.ManifestSchemaIdCiRuns, envelope.SourceProviderGitlab, _gitlabInstance, area)
	m.Area.Kind = envelope.AreaRefKindRepository
	for _, id := range truncated {
		m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{RecordKind: "ci_run", NativeId: id})
	}
	return m
}

type runSpec struct {
	id, project, sha, status, revision string
	updated                            time.Time
	mr, parent                         string
}

func ciRun(t *testing.T, s runSpec) *envelope.Record {
	t.Helper()
	p := map[string]any{
		"project_native_id": s.project, "ref_name": "refs/merge-requests/7/head", "sha": s.sha, "native_status": s.status,
		"outcome": "unknown", "started_at": _t1, "updated_at": s.updated,
	}
	if s.mr != "" {
		p["mr_native_id"] = s.mr
	}
	if s.parent != "" {
		p["parent_run_native_id"] = s.parent
	}
	return mrRecord(t, "ci_run", s.id, "", s.revision, p)
}

type attemptSpec struct {
	id, run, job, fingerprint, status string
	no, seconds                       int
	failure                           string
}

func ciAttempt(t *testing.T, s attemptSpec) *envelope.Record {
	t.Helper()
	p := map[string]any{
		"run_native_id": s.run, "job_key": s.job, "job_name": "test", "config_fingerprint": s.fingerprint, "attempt_no": s.no,
		"native_status": s.status, "outcome": "unknown", "allow_failure": false, "runner_class": "shared",
		"started_at": _t1, "finished_at": _t1.Add(time.Duration(s.seconds) * time.Second), "duration_s": s.seconds,
	}
	if s.failure != "" {
		p["failure_kind"] = s.failure
	}
	return mrRecord(t, "ci_attempt", s.id, "", "r1", p)
}

func runID(t *testing.T, e *normalizeEnv, native string) entity.ID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind='ci_run' AND native_id=$1", native).Scan(&id))
	return entity.ID(id)
}

// v28 is the run of V-28: linux fails in 60 s and passes on retry in 30 s, windows of the same job passes in 40 s.
func v28(t *testing.T) []*envelope.Record {
	t.Helper()
	return []*envelope.Record{
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "test", fingerprint: _linux, status: "failed", no: 1, seconds: 60, failure: "script_failure"}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "test", fingerprint: _linux, status: "success", no: 2, seconds: 30}),
		ciAttempt(t, attemptSpec{id: "7003", run: "5001", job: "test", fingerprint: _windows, status: "success", no: 1, seconds: 40}),
	}
}

func TestNormalizeCI_V28AttemptsAndRepeats(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	data := pack(t, ciManifest("42"), v28(t)...)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	for range 3 {
		assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	again := ciManifest("42")
	again.CreatedAt = again.CreatedAt.Add(time.Hour)
	assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, pack(t, again, v28(t)...)).Status)
	e.drain(t)
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.ci_run_version"))
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"))

	area := mrArea(t, e, rc, "42")
	scope := e.areaScope(t, e.admin, area)
	run, err := e.norm.RunAt(t.Context(), scope, e.orgID, runID(t, e, "5001"), math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.CISuccess, run.Run.Outcome)
	assert.Equal(t, "success", run.Run.NativeStatus)
	assert.Equal(t, area, run.Run.AreaID)
	assert.Empty(t, run.Children)
	assert.Equal(t, entity.AttemptHistoryComplete, run.History)
	require.Len(t, run.Attempts, 3)
	type attempt struct {
		fingerprint string
		no          int
		outcome     entity.CIOutcome
		seconds     int64
	}
	got := make([]attempt, 0, len(run.Attempts))
	var total int64
	for _, a := range run.Attempts {
		require.NotNil(t, a.DurationS)
		got = append(got, attempt{fingerprint: string(a.ConfigFingerprint[:1]), no: a.AttemptNo, outcome: a.Outcome, seconds: *a.DurationS})
		total += *a.DurationS
	}
	assert.Equal(t, []attempt{{"\x11", 1, entity.CIFailed, 60}, {"\x11", 2, entity.CISuccess, 30}, {"\x22", 1, entity.CISuccess, 40}}, got)
	assert.Equal(t, int64(130), total)
	assert.Equal(t, "script_failure", run.Attempts[0].FailureKind)

	_, err = e.norm.RunAt(t.Context(), entity.Scope{}, e.orgID, runID(t, e, "5001"), math.MaxInt64)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	_, err = e.norm.RunAt(t.Context(), scope, e.otherID, runID(t, e, "5001"), math.MaxInt64)
	assert.ErrorIs(t, err, entity.ErrNotFound)
}

func TestNormalizeCI_RetryAfterTheRunFinished(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	first := e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "failed", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "test", fingerprint: _linux, status: "failed", no: 1, seconds: 60}),
	))
	e.drain(t)
	k1 := int64(e.n(t, "SELECT max(known_seq) FROM normalize.ci_run_version"))
	e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p2", updated: _t2}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "test", fingerprint: _linux, status: "success", no: 2, seconds: 30}),
	))
	e.drain(t)
	e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "running", revision: "p0", updated: _t1.Add(-time.Minute)}),
	))
	e.drain(t)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.ci_run_version"), "an older state does not replace the current one")
	scope := e.areaScope(t, e.admin, mrArea(t, e, first, "42"))
	id := runID(t, e, "5001")
	run, err := e.norm.RunAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.CISuccess, run.Run.Outcome)
	require.Len(t, run.Attempts, 2)
	assert.Equal(t, entity.CIFailed, run.Attempts[0].Outcome, "the final pass keeps the failed attempt")
	assert.Equal(t, entity.CISuccess, run.Attempts[1].Outcome)
	past, err := e.norm.RunAt(t.Context(), scope, e.orgID, id, k1)
	require.NoError(t, err)
	assert.Equal(t, entity.CIFailed, past.Run.Outcome)
	assert.Len(t, past.Attempts, 1)
}

func TestNormalizeCI_ChildPipelineJobsCountOnce(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciAttempt(t, attemptSpec{id: "7102", run: "5002", job: "trigger:deploy/test", fingerprint: _linux, status: "success", no: 1, seconds: 20}),
		ciRun(t, runSpec{id: "5002", project: "42", sha: _headA, status: "success", revision: "c1", updated: _t1, parent: "5001"}),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7101", run: "5001", job: "build", fingerprint: _linux, status: "success", no: 1, seconds: 10}),
	))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	scope := e.areaScope(t, e.admin, mrArea(t, e, rc, "42"))
	parent, child := runID(t, e, "5001"), runID(t, e, "5002")
	run, err := e.norm.RunAt(t.Context(), scope, e.orgID, parent, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, run.Children, 1)
	assert.Equal(t, child, run.Children[0].RunID)
	require.NotNil(t, run.Children[0].ParentRunID)
	assert.Equal(t, parent, *run.Children[0].ParentRunID)
	require.Len(t, run.Attempts, 2)
	assert.Equal(t, []string{"build", "trigger:deploy/test"}, []string{run.Attempts[0].JobKey, run.Attempts[1].JobKey})
	own, err := e.norm.AttemptsAt(t.Context(), scope, e.orgID, child, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, own.Attempts, 1)
	assert.Equal(t, child, own.Attempts[0].RunID)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"))
	assert.Zero(t, e.n(t, "SELECT count(*) FROM normalize.area WHERE area_kind NOT IN ('repository')"), "runs and attempts are not areas")
}

func TestNormalizeCI_StatusesAndUnknownFailure(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "failed", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "a", fingerprint: _linux, status: "failed", no: 1, seconds: 1}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "b", fingerprint: _linux, status: "failed", no: 1, seconds: 1, failure: "unknown_failure"}),
		ciAttempt(t, attemptSpec{id: "7003", run: "5001", job: "c", fingerprint: _linux, status: "manual", no: 1, seconds: 0}),
		ciAttempt(t, attemptSpec{id: "7004", run: "5001", job: "d", fingerprint: _linux, status: "canceling", no: 1, seconds: 0}),
		ciAttempt(t, attemptSpec{id: "7005", run: "5001", job: "e", fingerprint: _linux, status: "bogus", no: 1, seconds: 0}),
	))
	e.drain(t)
	scope := e.areaScope(t, e.admin, mrArea(t, e, rc, "42"))
	got, err := e.norm.AttemptsAt(t.Context(), scope, e.orgID, runID(t, e, "5001"), math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, got.Attempts, 5)
	want := []entity.CIOutcome{entity.CIFailed, entity.CIFailed, entity.CISkipped, entity.CICanceled, entity.CIUnknown}
	for i, a := range got.Attempts {
		assert.Equal(t, want[i], a.Outcome, a.JobKey)
		assert.Empty(t, a.FailureKind, "an unknown cause is not attributed to the code")
	}
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version WHERE failure_kind IS NULL AND outcome = 'failed'"))
	assert.Equal(t, "bogus", got.Attempts[4].NativeStatus)
}

func TestNormalizeCI_RunGetsMROnlyOfItsRepositoryAndHead(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	mrs := e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "MR", "v1", _t1, nil),
		mrRevision(t, "9001", "rev1", "rev1", strings.Repeat("c", 40))))
	require.Equal(t, entity.ReceiptAccepted, mrs.Status)
	e.drain(t)
	e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1, mr: "9001"}),
		ciRun(t, runSpec{id: "5002", project: "42", sha: strings.Repeat("c", 40), status: "success", revision: "p1", updated: _t1, mr: "9001"}),
		ciRun(t, runSpec{id: "5003", project: "42", sha: _headB, status: "success", revision: "p1", updated: _t1, mr: "9001"}),
		ciRun(t, runSpec{id: "5004", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1, mr: "9999"}),
	))
	e.imported(t, e.admin, pack(t, ciManifest("43"),
		ciRun(t, runSpec{id: "5005", project: "43", sha: _headA, status: "success", revision: "p1", updated: _t1, mr: "9001"}),
	))
	e.drain(t)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.ci_run_version WHERE mr_id IS NOT NULL"))
	assert.Zero(t, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind='merge_request' AND native_id='9999'"), "a run does not invent its MR")
	area := mrArea(t, e, mrs, "42")
	scope := e.areaScope(t, e.admin, area, mrArea(t, e, mrs, "43"))
	runs, err := e.norm.MRRunsAt(t.Context(), scope, e.orgID, mrID(t, e, "9001"), math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.ElementsMatch(t, []entity.ID{runID(t, e, "5001"), runID(t, e, "5002")}, []entity.ID{runs[0].RunID, runs[1].RunID})
	window, err := e.norm.AreaRunsAt(t.Context(), scope, e.orgID, area, _t1, _t1.Add(time.Second), math.MaxInt64)
	require.NoError(t, err)
	assert.Len(t, window, 4)
	empty, err := e.norm.AreaRunsAt(t.Context(), scope, e.orgID, area, _t2, _t2.Add(time.Hour), math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, empty)
	hidden, err := e.norm.MRRunsAt(t.Context(), entity.Scope{}, e.orgID, mrID(t, e, "9001"), math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, hidden)
}

func TestNormalizeCI_TruncatedAttemptHistory(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, pack(t, ciManifest("42", "5001"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7002", run: "5001", job: "test", fingerprint: _linux, status: "success", no: 2, seconds: 30}),
		ciRun(t, runSpec{id: "5002", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1}),
	))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	scope := e.areaScope(t, e.admin, mrArea(t, e, rc, "42"))
	cut, err := e.norm.AttemptsAt(t.Context(), scope, e.orgID, runID(t, e, "5001"), math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.AttemptHistoryTruncated, cut.History)
	assert.Len(t, cut.Attempts, 1)
	whole, err := e.norm.AttemptsAt(t.Context(), scope, e.orgID, runID(t, e, "5002"), math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.AttemptHistoryComplete, whole.History)
	assert.Empty(t, whole.Attempts)
	changes, err := e.norm.AreaChanges(t.Context(), e.orgID, mrArea(t, e, rc, "42"), 0)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, []string{"ci_attempt", "ci_run"}, changes[0].FactKinds)
}
