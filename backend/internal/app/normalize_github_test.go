package app

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _githubInstance = "github.com"

var (
	_prHead  = strings.Repeat("c", 40)
	_prHead2 = strings.Repeat("f", 40)
)

func githubManifest(schema envelope.ManifestSchemaId, repository string) *envelope.Manifest {
	m := manifestOf(schema, envelope.SourceProviderGithub, _githubInstance, repository)
	m.Area.Kind = envelope.AreaRefKindRepository
	return m
}

func githubAccount(t *testing.T, id, login string, kind envelope.AccountV1AccountKind) *envelope.Record {
	t.Helper()
	return mrRecord(t, "account", id, "", "a1", envelope.AccountV1{Login: &login, DisplayName: &login, AccountKind: kind, Active: true})
}

func githubPR(t *testing.T, id, repository, revision string, draft bool, reviewers []string) *envelope.Record {
	t.Helper()
	number, author := 12, "100"
	p := envelope.MergeRequestV1{
		ProjectNativeId: repository, Iid: &number, Title: ptr("Cache orders"), State: "open", Draft: draft, AuthorAccountNativeId: &author,
		SourceBranch: "feature/PA-7", TargetBranch: "main", HeadSha: _prHead, BaseSha: ptr(strings.Repeat("b", 40)), OpenedAt: _created,
		UpdatedAt: ptr(_t1.Add(time.Hour)),
	}
	if reviewers != nil {
		p.ReviewerField, p.ReviewerAccountNativeIds = ptr(true), &reviewers
	}
	return mrRecord(t, "merge_request", id, "", revision, p)
}

type prEvent struct {
	id, kind, actor, state string
	minute                 int
	system                 bool
}

func githubEvent(t *testing.T, mr string, e prEvent) *envelope.Record {
	t.Helper()
	p := map[string]any{"mr_native_id": mr, "event_kind": e.kind, "actor_account_native_id": e.actor, "system_note": e.system}
	if e.state != "" {
		p["review_state"] = e.state
	}
	if e.kind == "head_pushed" {
		p["head_sha"] = _prHead
	}
	at := _t1.Add(time.Duration(e.minute) * time.Minute)
	r := mrRecord(t, "mr_event", e.id, mr, e.id, p)
	r.RecordClass, r.PayloadSchema, r.EventTime = "event", "mr_event/2", &at
	return r
}

func TestNormalizeGitHub_PullRequestEvents(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	events := []prEvent{
		{id: "ready-1", kind: "ready", actor: "100", minute: 1},
		{id: "rr-1", kind: "reviewer_assigned", actor: "100", minute: 2},
		{id: "review-1", kind: "review", actor: "200", state: "commented", minute: 3},
		{id: "review-2", kind: "review", actor: "300", state: "approved", minute: 4},
		{id: "review-3", kind: "review", actor: "300", state: "changes_requested", minute: 5},
		{id: "comment-1", kind: "comment", actor: "100", minute: 6},
		{id: "note-1", kind: "comment", actor: "100", minute: 7, system: true},
		{id: "draft-1", kind: "draft", actor: "100", minute: 8},
		{id: "push-1", kind: "head_pushed", actor: "100", minute: 9},
	}
	records := make([]*envelope.Record, 0, 4+len(events))
	records = append(records,
		githubAccount(t, "100", "author", envelope.AccountV1AccountKindHuman),
		githubAccount(t, "200", "review-bot[bot]", envelope.AccountV1AccountKindBot),
		githubAccount(t, "300", "reviewer", envelope.AccountV1AccountKindHuman),
		githubPR(t, "pr-1", "777", "v1", false, nil),
	)
	for _, ev := range events {
		records = append(records, githubEvent(t, "pr-1", ev))
	}
	data := pack(t, githubManifest(envelope.ManifestSchemaIdMergeRequests, "777"), records...)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)

	area := mrArea(t, e, rc, "777")
	scope := e.areaScope(t, e.admin, area)
	mr := mrID(t, e, "pr-1")
	got, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, mr, math.MaxInt64)
	require.NoError(t, err)
	type row struct {
		key, kind string
		minute    int
		actor     entity.PersonKind
	}
	rows := make([]row, 0, len(got))
	for _, ev := range got {
		rows = append(rows, row{key: ev.EventKey, kind: ev.Kind, minute: int(ev.EventTime.Sub(_t1) / time.Minute), actor: ev.ActorKind})
	}
	assert.ElementsMatch(t, []row{
		{"ready-1", "ready", 1, entity.PersonUnknown},
		{"rr-1", "reviewer_assigned", 2, entity.PersonUnknown},
		{"review-1", "comment", 3, entity.PersonUnknown},
		{"review-2", "review", 4, entity.PersonUnknown},
		{"review-2/approval", "approval", 4, entity.PersonUnknown},
		{"review-3", "review", 5, entity.PersonUnknown},
		{"comment-1", "comment", 6, entity.PersonUnknown},
		{"draft-1", "draft", 8, entity.PersonUnknown},
		{"push-1", "head_pushed", 9, entity.PersonUnknown},
	}, rows, "a comment-only review is a comment, an approval is a review and an approval, a system note is no answer")

	var proposed string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT proposed_kind FROM org.source_account WHERE native_account_id = '200'").Scan(&proposed))
	assert.Equal(t, "bot", proposed, "a Bot account is proposed as a bot, the event still has an unknown participant")

	versions, err := e.norm.MRsAt(t.Context(), scope, e.orgID, area, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.False(t, versions[0].ReviewerField, "a PR without the reviewers field has an unknown assignment")
	assert.Empty(t, versions[0].ReviewerAccountIDs)

	for range 3 {
		assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.mr_version"))
	assert.Equal(t, 9, e.n(t, "SELECT count(*) FROM normalize.mr_event"))
}

type actionsJob struct {
	run, id, job, fingerprint, status string
	attempt, seconds                  int
}

func githubJob(t *testing.T, j actionsJob) *envelope.Record {
	t.Helper()
	p := map[string]any{
		"run_native_id": j.run, "job_key": j.job, "job_name": j.job, "config_fingerprint": j.fingerprint, "attempt_no": j.attempt,
		"native_status": j.status, "outcome": "unknown", "allow_failure": false, "runner_class": "ubuntu-latest",
		"started_at": _t1, "finished_at": _t1.Add(time.Duration(j.seconds) * time.Second), "duration_s": j.seconds,
	}
	return mrRecord(t, "ci_attempt", strconv.Itoa(j.attempt)+"/"+j.id, j.run, "r1", p)
}

func ciManifestOf(provider envelope.SourceProvider, instance, area string) *envelope.Manifest {
	m := manifestOf(envelope.ManifestSchemaIdCiRuns, provider, instance, area)
	m.Area.Kind = envelope.AreaRefKindRepository
	return m
}

func githubRun(t *testing.T, id, repository, sha, status, pr string) *envelope.Record {
	t.Helper()
	return ciRun(t, runSpec{id: id, project: repository, sha: sha, status: status, revision: "p-" + status, updated: _t1, mr: pr})
}

func runAt(t *testing.T, e *normalizeEnv, area entity.ID, native string) entity.CIRun {
	t.Helper()
	run, err := e.norm.RunAt(t.Context(), e.areaScope(t, e.admin, area), e.orgID, runID(t, e, native), math.MaxInt64)
	require.NoError(t, err)
	return run
}

func TestNormalizeGitHub_WorkflowRunsAndRerun(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	prs := e.imported(t, e.admin, pack(t, githubManifest(envelope.ManifestSchemaIdMergeRequests, "777"),
		githubPR(t, "pr-1", "777", "v1", false, []string{"300"}),
		mrRecord(t, "mr_revision", "rev-1", "pr-1", "rv1", envelope.MrRevisionV1{MrNativeId: "pr-1", HeadSha: _prHead, CreatedAt: _t1}),
	))
	require.Equal(t, entity.ReceiptAccepted, prs.Status)
	e.drain(t)

	statuses := []string{"timed_out", "cancelled", "skipped", "neutral", "action_required", "in_progress", "something_new"}
	records := make([]*envelope.Record, 0, 7+len(statuses))
	records = append(records,
		githubRun(t, "5001", "777", _prHead, "success", "pr-1"),
		githubJob(t, actionsJob{run: "5001", id: "7001", job: "test", fingerprint: _linux, status: "failure", attempt: 1, seconds: 60}),
		githubJob(t, actionsJob{run: "5001", id: "7002", job: "test", fingerprint: _linux, status: "success", attempt: 2, seconds: 30}),
		githubJob(t, actionsJob{run: "5001", id: "7003", job: "test", fingerprint: _windows, status: "success", attempt: 1, seconds: 40}),
		githubRun(t, "5002", "777", _prHead2, "success", "pr-1"),
		githubRun(t, "5003", "888", _prHead, "success", "pr-1"),
	)
	for i, s := range statuses {
		records = append(records, githubJob(t, actionsJob{run: "5004", id: "80" + strconv.Itoa(i), job: "job-" + s, fingerprint: _linux, status: s, attempt: 1, seconds: 5}))
	}
	records = append(records, githubRun(t, "5004", "777", _prHead, "timed_out", ""))
	data := pack(t, ciManifestOf(envelope.SourceProviderGithub, _githubInstance, "777"), records...)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	area := mrArea(t, e, prs, "777")

	v28 := runAt(t, e, area, "5001")
	assert.Equal(t, entity.CISuccess, v28.Run.Outcome)
	assert.False(t, v28.Run.AllowFailureSemantics, "Actions has no allowed failure of a job")
	require.NotNil(t, v28.Run.MRID, "the run of the PR in its repository on a head SHA")
	assert.Equal(t, mrID(t, e, "pr-1"), *v28.Run.MRID)
	require.Len(t, v28.Attempts, 3)
	type attempt struct {
		fingerprint string
		no          int
		outcome     entity.CIOutcome
	}
	got := make([]attempt, 0, 3)
	for _, a := range v28.Attempts {
		got = append(got, attempt{string(a.ConfigFingerprint[:1]), a.AttemptNo, a.Outcome})
	}
	assert.ElementsMatch(t, []attempt{{"\x11", 1, entity.CIFailed}, {"\x11", 2, entity.CISuccess}, {"\x22", 1, entity.CISuccess}}, got,
		"the rerun is attempt 2 of the same job, the final pass keeps the failed attempt 1")

	assert.Nil(t, runAt(t, e, area, "5002").Run.MRID, "a SHA that is not a head of the PR")
	var foreign *uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT v.mr_id FROM normalize.ci_run_version v JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.run_id WHERE o.native_id = '5003'").Scan(&foreign))
	assert.Nil(t, foreign, "a run of another repository")

	jobs := runAt(t, e, area, "5004")
	assert.Equal(t, entity.CIFailed, jobs.Run.Outcome)
	assert.Equal(t, "timed_out", jobs.Run.NativeStatus)
	want := map[string]entity.CIOutcome{
		"timed_out": entity.CIFailed, "cancelled": entity.CICanceled, "skipped": entity.CISkipped, "neutral": entity.CISkipped,
		"action_required": entity.CIUnknown, "in_progress": entity.CIRunning, "something_new": entity.CIUnknown,
	}
	require.Len(t, jobs.Attempts, len(want))
	for _, a := range jobs.Attempts {
		assert.Equal(t, want[a.NativeStatus], a.Outcome, a.NativeStatus)
		assert.Equal(t, "job-"+a.NativeStatus, a.JobKey, "the native value stays with the attempt")
	}

	for range 3 {
		assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	again := pack(t, ciManifestOf(envelope.SourceProviderGithub, _githubInstance, "777"), records[1], records[2])
	assert.Equal(t, entity.ReceiptAccepted, e.imported(t, e.admin, again).Status)
	e.drain(t)
	assert.Equal(t, 4, e.n(t, "SELECT count(*) FROM normalize.ci_run_version"))
	assert.Equal(t, 3+len(statuses), e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"), "a repeated delivery of a job is the same attempt")
}
