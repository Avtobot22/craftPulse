package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const _gitlabInstance = "https://gitlab.example.test"

func mrManifest(area string) *envelope.Manifest {
	m := manifestOf(envelope.ManifestSchemaIdMergeRequests, envelope.SourceProviderGitlab, _gitlabInstance, area)
	m.Area.Kind = envelope.AreaRefKindRepository
	return m
}

func mrRecord(t *testing.T, kind, id, ns, revision string, p any) *envelope.Record {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return &envelope.Record{RecordClass: "object", RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: kind + "/1", NativeId: id, NativeNs: ns, SourceRevision: &revision, Payload: raw}
}

func mrState(t *testing.T, id, project, title, revision string, updated time.Time, reviewer *bool) *envelope.Record {
	t.Helper()
	iid, author, reviewers := 7, "1", []string{"2"}
	return mrRecord(t, "merge_request", id, "", revision, envelope.MergeRequestV1{
		ProjectNativeId: project, Iid: &iid, Title: &title, State: "open", Draft: false, AuthorAccountNativeId: &author,
		SourceBranch: "feature/PA-7", TargetBranch: "main", HeadSha: strings.Repeat("a", 40), BaseSha: ptr(strings.Repeat("b", 40)), MergeBaseSha: ptr(strings.Repeat("b", 40)),
		ReviewerField: reviewer, ReviewerAccountNativeIds: &reviewers, OpenedAt: _created, UpdatedAt: &updated,
	})
}

func mrEvent(t *testing.T, mr, id, revision, kind string, at time.Time, system bool) *envelope.Record {
	t.Helper()
	r := mrRecord(t, "mr_event", id, mr, revision, map[string]any{"mr_native_id": mr, "event_kind": kind, "actor_account_native_id": "1", "head_sha": strings.Repeat("a", 40), "system_note": system})
	r.RecordClass = "event"
	r.PayloadSchema = "mr_event/2"
	r.EventTime = &at
	return r
}

func mrRevision(t *testing.T, mr, id, revision, sha string) *envelope.Record {
	t.Helper()
	return mrRecord(t, "mr_revision", id, mr, revision, envelope.MrRevisionV1{MrNativeId: mr, HeadSha: sha, BaseSha: ptr(strings.Repeat("b", 40)), CreatedAt: _t1})
}

func mrRepository(t *testing.T, id, path, revision string) *envelope.Record {
	t.Helper()
	return mrRecord(t, "repository", id, "", revision, map[string]any{"path": path, "name": "Repository " + id, "default_branch": "main", "visibility": "private", "web_url": _gitlabInstance + "/" + path, "updated_at": _t1})
}

func mrID(t *testing.T, e *normalizeEnv, native string) entity.ID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind='merge_request' AND native_id=$1", native).Scan(&id))
	return entity.ID(id)
}

func mrArea(t *testing.T, e *normalizeEnv, rc entity.Receipt, native string) entity.ID {
	t.Helper()
	id, err := e.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectRepository, native)
	require.NoError(t, err)
	return id
}

func TestNormalizeMR_IdentitiesRepeatsAndScopedReads(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	yes := true
	data := pack(t, mrManifest("42"), mrRepository(t, "42", "demo/one", "repo1"), accountRecord(t, "1", "Same Name"), accountRecord(t, "2", "Same Name"),
		mrState(t, "9001", "42", "First", "v1", _t1, &yes), mrRevision(t, "9001", "rev1", "rev1", strings.Repeat("c", 40)), mrEvent(t, "9001", "ready1", "e1", "ready", _t1, false))
	first := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, first.Status)
	e.drain(t)
	second := e.imported(t, e.admin, pack(t, mrManifest("43"), mrRepository(t, "43", "demo/two", "repo1"), mrState(t, "9002", "43", "Second", "v1", _t1, nil)))
	require.Equal(t, entity.ReceiptAccepted, second.Status)
	e.drain(t)
	for range 3 {
		assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
	}
	e.renormalize(t, first.ID)
	e.drain(t)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.mr_version"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.mr_event"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.mr_revision"))
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind='merge_request'"))
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM org.source_account WHERE display_name='Same Name'"))
	area := mrArea(t, e, first, "42")
	scope := e.areaScope(t, e.admin, area)
	id := mrID(t, e, "9001")
	v, err := e.norm.CurrentMR(t.Context(), scope, e.orgID, id)
	require.NoError(t, err)
	assert.Equal(t, "First", v.Title)
	assert.Equal(t, area, v.AreaID)
	require.NotNil(t, v.AuthorAccountID)
	assert.Len(t, v.ReviewerAccountIDs, 1)
	assert.True(t, v.ReviewerField)
	repo, err := e.norm.Repository(t.Context(), scope, e.orgID, area)
	require.NoError(t, err)
	assert.Equal(t, "demo/one", repo.Path)
	assert.Equal(t, "main", repo.DefaultBranch)
	native, err := e.norm.MRByNative(t.Context(), scope, e.orgID, *first.SourceInstanceID, "9001")
	require.NoError(t, err)
	assert.Equal(t, id, native.MRID)
	all, err := e.norm.MRsAt(t.Context(), scope, e.orgID, area, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, all, 1)
	byHead, err := e.norm.MRsByHeadSHA(t.Context(), scope, e.orgID, area, strings.Repeat("c", 40), math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, byHead, 1)
	assert.Equal(t, id, byHead[0].MRID)
	events, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, entity.PersonUnknown, events[0].ActorKind, "a proposed human is not a confirmed kind")
	revisions, err := e.norm.MRRevisionsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	_, err = e.norm.CurrentMR(t.Context(), scope, e.orgID, mrID(t, e, "9002"))
	assert.ErrorIs(t, err, entity.ErrNotFound)
	_, err = e.norm.CurrentMR(t.Context(), entity.Scope{}, e.orgID, id)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	_, err = e.norm.Repository(t.Context(), entity.Scope{}, e.orgID, area)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	hidden, err := e.norm.MREventsAt(t.Context(), entity.Scope{}, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, hidden)
	hiddenRevs, err := e.norm.MRRevisionsAt(t.Context(), entity.Scope{}, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, hiddenRevs)
	_, err = e.norm.MRByNative(t.Context(), entity.Scope{}, e.orgID, *first.SourceInstanceID, "9001")
	assert.ErrorIs(t, err, entity.ErrNotFound)
	all, err = e.norm.MRsAt(t.Context(), entity.Scope{}, e.orgID, area, math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, all)
	byHead, err = e.norm.MRsByHeadSHA(t.Context(), entity.Scope{}, e.orgID, area, strings.Repeat("c", 40), math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, byHead)
	// The application role must not read a different tenant, even with a scope from this one.
	_, err = e.norm.CurrentMR(t.Context(), scope, e.otherID, id)
	assert.ErrorIs(t, err, entity.ErrNotFound)
}

func TestNormalizeMR_OldStateSnapshotsAndKnowledge(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	first := e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "First", "v1", _t1, nil)))
	e.drain(t)
	k1 := int64(e.n(t, "SELECT max(known_seq) FROM normalize.mr_version"))
	e.clock.Advance(time.Hour)
	e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "New", "v2", _t2, nil)))
	e.drain(t)
	e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "Old", "old", _t1, nil)))
	e.drain(t)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.mr_version"))
	area := mrArea(t, e, first, "42")
	scope := e.areaScope(t, e.admin, area)
	past, err := e.norm.MRsAt(t.Context(), scope, e.orgID, area, k1)
	require.NoError(t, err)
	require.Len(t, past, 1)
	assert.Equal(t, "First", past[0].Title)
	current, err := e.norm.CurrentMR(t.Context(), scope, e.orgID, mrID(t, e, "9001"))
	require.NoError(t, err)
	assert.Equal(t, "New", current.Title)
	assert.False(t, current.ReviewerField)
	for i := range 2 {
		m := mrManifest("42")
		m.Snapshot = true
		m.ReadId = ptr("read-" + strconv.Itoa(i))
		r := mrState(t, "9001", "42", "New", "snapshot", _t2, nil)
		r.RecordClass = "snapshot"
		r.SourceRevision = nil
		rc := e.imported(t, e.admin, pack(t, m, r))
		require.Equal(t, entity.ReceiptAccepted, rc.Status)
		e.drain(t)
	}
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.mr_version"), "unchanged snapshots add no version")
}

func TestNormalizeMR_LateCorrectedEventsAndSystemNotes(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	first := e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "First", "v1", _t1, nil), mrEvent(t, "9001", "ready1", "e1", "ready", _t2, true)))
	e.drain(t)
	before := int64(e.n(t, "SELECT max(known_seq) FROM normalize.mr_event"))
	late := _t1.Add(-time.Hour)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), mrEvent(t, "9001", "ready2", "e2", "ready", late, true), mrEvent(t, "9001", "system1", "s1", "comment", _t1, true), mrEvent(t, "9001", "system2", "s2", "review", _t1, true)))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	area := mrArea(t, e, first, "42")
	scope := e.areaScope(t, e.admin, area)
	id := mrID(t, e, "9001")
	events, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Greater(t, events[0].KnownSeq, before)
	assert.True(t, events[0].EventTime.Equal(late))
	assert.Equal(t, 10800, events[0].EventOffsetS)
	assert.Equal(t, entity.ViaImport, events[0].ObservedVia)
	changes, err := e.norm.AreaChanges(t.Context(), e.orgID, area, before)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NotNil(t, changes[0].EarliestEventTime)
	assert.True(t, changes[0].EarliestEventTime.Equal(late))
	corrected := late.Add(time.Hour)
	e.imported(t, e.admin, pack(t, mrManifest("42"), mrEvent(t, "9001", "ready2", "e3", "ready", corrected, true)))
	e.drain(t)
	old, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, events[0].KnownSeq)
	require.NoError(t, err)
	require.Len(t, old, 2)
	assert.True(t, old[0].EventTime.Equal(late))
	events, err = e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.True(t, events[0].EventTime.Equal(corrected))
	changes, err = e.norm.AreaChanges(t.Context(), e.orgID, area, old[0].KnownSeq)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.True(t, changes[0].EarliestEventTime.Equal(late), "a correction invalidates both old and new windows")
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.mr_event"))
	assert.Zero(t, e.n(t, "SELECT count(*) FROM normalize.mr_event WHERE event_kind IN ('comment','review')"))
}

func TestNormalizeMR_EventsBeforeStateAndCurrentAccess(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), mrEvent(t, "9001", "ready1", "e1", "ready", _t1, false)))
	e.drain(t)
	area := mrArea(t, e, rc, "42")
	scope := e.areaScope(t, e.admin, area)
	id := mrID(t, e, "9001")
	events, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, events, 1)
	e.imported(t, e.admin, pack(t, mrManifest("43"), mrState(t, "9001", "43", "Moved", "v2", _t2, nil)))
	e.drain(t)
	events, err = e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, events, "access checks current placement even for historical events")
}

func TestNormalizeMR_InterruptedBatchContinues(t *testing.T) {
	t.Parallel()
	const partSize, recordCount = 3, 7
	records := make([]*envelope.Record, 0, recordCount)
	for i := range recordCount {
		records = append(records, mrState(t, strconv.Itoa(9000+i), "42", "MR", "v1", _t1, nil))
	}
	data := pack(t, mrManifest("42"), records...)
	whole := newNormalizeEnvWithPartSize(t, partSize)
	whole.imported(t, whole.admin, data)
	assert.Equal(t, 3, whole.drain(t))
	cut := newNormalizeEnvWithPartSize(t, partSize)
	cut.imported(t, cut.admin, data)
	require.NoError(t, cut.run(t, cut.pending(t)[0]))
	second := cut.pending(t)[0]
	err := cut.norm.RunBatch(t.Context(), second.args, func(context.Context) error { return errors.New("worker stopped") })
	require.Error(t, err)
	assert.Equal(t, partSize, cut.n(t, "SELECT count(*) FROM normalize.mr_version"))
	assert.Equal(t, 2, cut.drain(t))
	snapshot := func(e *normalizeEnv) []string {
		rows, err := e.owner.Query(t.Context(), `SELECT o.native_id,v.source_revision,v.known_seq-(SELECT min(known_seq) FROM normalize.mr_version) FROM normalize.mr_version v JOIN normalize.source_object o ON o.id=v.mr_id ORDER BY o.native_id`)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var id, revision string
			var seq int64
			require.NoError(t, rows.Scan(&id, &revision, &seq))
			out = append(out, fmt.Sprintf("%s %s %d", id, revision, seq))
		}
		require.NoError(t, rows.Err())
		return out
	}
	assert.Equal(t, snapshot(whole), snapshot(cut))
	assert.Equal(t, recordCount, cut.n(t, "SELECT count(*) FROM normalize.mr_version"))
}

func ptr[T any](value T) *T { return &value }

func TestNormalizeMR_AllEventKindsAndConfirmedActor(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	kinds := []string{"ready", "reviewer_assigned", "reviewer_removed", "review", "comment", "approval", "head_pushed", "merged", "closed", "reopened"}
	records := make([]*envelope.Record, 0, 3+len(kinds))
	records = append(records, mrState(t, "9001", "42", "MR", "v1", _t1, nil), accountRecord(t, "1", "Author"))
	for i, kind := range kinds {
		records = append(records, mrEvent(t, "9001", strconv.Itoa(i), "e1", kind, _t1.Add(time.Duration(i)*time.Minute), false))
	}
	// A v1 event remains readable alongside v2.
	records = append(records, mrRecord(t, "mr_event", "legacy", "9001", "l1", envelope.MrEventV1{MrNativeId: "9001", EventKind: "reviewer_assigned", TargetAccountNativeId: ptr("2")}))
	records[len(records)-1].RecordClass = "event"
	records[len(records)-1].EventTime = &_t1
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), records...))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	area := mrArea(t, e, rc, "42")
	scope := e.areaScope(t, e.admin, area)
	id := mrID(t, e, "9001")
	before := int64(e.n(t, "SELECT max(known_seq) FROM normalize.mr_event"))
	events, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, before)
	require.NoError(t, err)
	require.Len(t, events, len(kinds)+1)
	var actor entity.ID
	for _, ev := range events {
		assert.Equal(t, entity.PersonUnknown, ev.ActorKind)
		if ev.ActorAccountID != nil {
			actor = *ev.ActorAccountID
		}
	}
	require.False(t, actor.IsZero())
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	outcomes, err := orgs.AssignAccounts(t.Context(), e.admin, entity.ID(uuid.New()), []entity.AssignmentRequest{{SourceAccountID: actor, CreatePerson: true, ActorKind: entity.PersonHuman}})
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, entity.AssignmentSaved, outcomes[0].Outcome)
	now, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, math.MaxInt64)
	require.NoError(t, err)
	for _, ev := range now {
		if ev.ActorAccountID != nil {
			assert.Equal(t, entity.PersonHuman, ev.ActorKind)
		}
	}
	old, err := e.norm.MREventsAt(t.Context(), scope, e.orgID, id, before)
	require.NoError(t, err)
	for _, ev := range old {
		assert.Equal(t, entity.PersonUnknown, ev.ActorKind)
	}
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.mr_event WHERE target_account_id IS NOT NULL"))
}

func TestNormalizeMR_ConflictResolutionAndRepositoryRename(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	first := e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "demo/one", "repo1"), mrState(t, "9001", "42", "First", "v1", _t1, nil)))
	e.drain(t)
	e.imported(t, e.admin, pack(t, mrManifest("42"), mrState(t, "9001", "42", "Corrected", "v1", _t1, nil)))
	assert.Zero(t, e.drain(t))
	var conflict uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM ingest.conflict WHERE status='open'").Scan(&conflict))
	_, err := e.uc.ResolveConflict(t.Context(), e.admin, entity.ID(conflict), entity.ResolutionAcceptedNew)
	require.NoError(t, err)
	e.drain(t)
	area := mrArea(t, e, first, "42")
	scope := e.areaScope(t, e.admin, area)
	current, err := e.norm.CurrentMR(t.Context(), scope, e.orgID, mrID(t, e, "9001"))
	require.NoError(t, err)
	assert.Equal(t, "Corrected", current.Title)
	e.imported(t, e.admin, pack(t, mrManifest("42"), mrRepository(t, "42", "renamed/one", "repo2")))
	e.drain(t)
	repo, err := e.norm.Repository(t.Context(), scope, e.orgID, area)
	require.NoError(t, err)
	assert.Equal(t, "renamed/one", repo.Path)
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind='repository'"))
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.repository_version"))
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.mr_version"))
}
