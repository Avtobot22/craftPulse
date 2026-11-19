package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
)

func linkedTask(t *testing.T, e *normalizeEnv, instance, native, key string) entity.ID {
	t.Helper()
	m := importManifest("1")
	m.Source.InstanceKey = instance
	record := task(t, native, "Task "+key, "v1")
	var p map[string]any
	require.NoError(t, json.Unmarshal(record.Payload, &p))
	p["task_key"] = key
	record.Payload = mustMarshal(t, p)
	rc := e.imported(t, e.admin, pack(t, m, record))
	e.drain(t)
	area, err := e.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectTrackerProject, "10000")
	require.NoError(t, err)
	return area
}

func bindLinkArea(t *testing.T, e *normalizeEnv, area entity.ID) {
	t.Helper()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
 VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, uuid.UUID(e.orgID), uuid.UUID(e.project), uuid.UUID(area), e.clock.Now())
	require.NoError(t, err)
}

func candidateMR(t *testing.T, e *normalizeEnv, id, title, branch, revision string, updated time.Time) entity.Receipt {
	t.Helper()
	record := mrState(t, id, "42", title, revision, updated, nil)
	var p map[string]any
	require.NoError(t, json.Unmarshal(record.Payload, &p))
	p["source_branch"] = branch
	record.Payload = mustMarshal(t, p)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), record))
	e.drain(t)
	return rc
}

func linkCandidateCount(t *testing.T, e *normalizeEnv, where string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM link.edge WHERE organization_id=$1 AND "+where, append([]any{uuid.UUID(e.orgID)}, args...)...).Scan(&n))
	return n
}

func TestNormalizeLinks_ProjectScopeAmbiguityAndRejection(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	j1 := linkedTask(t, e, "jira-one.test", "1", "PA-7")
	j2 := linkedTask(t, e, "jira-two.test", "1", "PA-7")
	repo := e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "team/repo", "r1")))
	e.drain(t)
	bindLinkArea(t, e, j1)
	bindLinkArea(t, e, mrArea(t, e, repo, "42"))
	first := candidateMR(t, e, "9001", "No task key", "feature/PA-7-cache", "v1", _t1)
	assert.Equal(t, 1, linkCandidateCount(t, e, "status='candidate' AND superseded_seq IS NULL"))
	var evidence string
	var edge, taskID uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id,from_object_id,evidence::text FROM link.edge WHERE superseded_seq IS NULL`).Scan(&edge, &taskID, &evidence))
	assert.JSONEq(t, `{"field":"source_branch","fragment":"PA-7","ambiguous":false}`, evidence)
	var taskArea uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT current_area_id FROM normalize.source_object WHERE id=$1", taskID).Scan(&taskArea))
	assert.Equal(t, uuid.UUID(j1), taskArea)
	bindLinkArea(t, e, j2)
	candidateMR(t, e, "9001", "Still no key", "feature/PA-7-cache", "v2", _t2)
	assert.Equal(t, 2, linkCandidateCount(t, e, "status='candidate' AND superseded_seq IS NULL"))
	assert.Equal(t, 2, linkCandidateCount(t, e, "status='candidate' AND superseded_seq IS NULL AND evidence->>'ambiguous'='true'"))
	assert.Zero(t, linkCandidateCount(t, e, "status='confirmed'"))
	// Reject the first task. A later basis change must preserve that decision.
	var rejected uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM link.edge WHERE from_object_id=$1 AND superseded_seq IS NULL", taskID).Scan(&rejected))
	tx := persistent.NewTransactor(e.app)
	links := linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Auditor: audit.New(auditrepo.NewRepo(e.app), tx, e.clock), Clock: e.clock, NewID: newID})
	require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		_, err := links.Decide(ctx, e.orgID, e.admin.UserID, entity.ID(rejected), entity.LinkReject, entity.ID(uuid.New()))
		return err
	}))
	candidateMR(t, e, "9001", "No task key", "feature/no-key", "v3", _t2.Add(time.Hour))
	assert.Zero(t, linkCandidateCount(t, e, "status='candidate' AND superseded_seq IS NULL"))
	candidateMR(t, e, "9001", "No task key", "feature/PA-7-cache", "v4", _t2.Add(2*time.Hour))
	assert.Equal(t, 1, linkCandidateCount(t, e, "status='rejected' AND superseded_seq IS NULL"))
	assert.Equal(t, 1, linkCandidateCount(t, e, "status='candidate' AND superseded_seq IS NULL"))
	before := e.n(t, "SELECT count(*) FROM link.edge")
	e.renormalize(t, first.ID)
	e.drain(t)
	assert.Equal(t, before, e.n(t, "SELECT count(*) FROM link.edge"))
}

func TestNormalizeLinks_TitleLateTaskAndClosedBasis(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	tracker := linkedTask(t, e, "jira-one.test", "1", "PA-7")
	repo := e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "team/repo", "r1")))
	e.drain(t)
	bindLinkArea(t, e, tracker)
	bindLinkArea(t, e, mrArea(t, e, repo, "42"))
	candidateMR(t, e, "9001", "Fix PA-7", "feature/plain", "v1", _t1)
	candidateMR(t, e, "9002", "Fix LATE-9", "feature/plain", "v1", _t1)
	candidateMR(t, e, "9003", "No key", "feature/plain", "v1", _t1)
	assert.Equal(t, 1, linkCandidateCount(t, e, "method='title_key' AND superseded_seq IS NULL"))
	linkedTask(t, e, "jira-one.test", "2", "LATE-9")
	assert.Equal(t, 2, linkCandidateCount(t, e, "method='title_key' AND superseded_seq IS NULL"))
	candidateMR(t, e, "9001", "No key now", "feature/plain", "v2", _t2)
	assert.Equal(t, 1, linkCandidateCount(t, e, "method='title_key' AND superseded_seq IS NULL"))
	assert.Equal(t, 1, linkCandidateCount(t, e, "method='title_key' AND superseded_seq IS NOT NULL"))
}

func TestNormalizeLinks_VerifiedPipelineOnly(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	candidateMR(t, e, "9001", "No key", "feature/plain", "v1", _t1)
	rc := e.imported(t, e.admin, pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _headA, status: "success", revision: "r1", updated: _t1, mr: "9001"}),
		ciRun(t, runSpec{id: "5002", project: "42", sha: _headB, status: "success", revision: "r1", updated: _t1, mr: "9001"}),
		ciRun(t, runSpec{id: "5003", project: "43", sha: _headA, status: "success", revision: "r1", updated: _t1, mr: "9001"})))
	e.drain(t)
	e.renormalize(t, rc.ID)
	e.drain(t)
	assert.Equal(t, 1, linkCandidateCount(t, e, "link_type='mr_run' AND status='confirmed' AND method='source_explicit' AND superseded_seq IS NULL"))
	var native string
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT o.native_id FROM link.edge e JOIN normalize.source_object o ON o.id=e.to_object_id WHERE e.link_type='mr_run'`).Scan(&native))
	assert.Equal(t, "5001", native)
}
