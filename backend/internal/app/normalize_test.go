package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var (
	_created = time.Date(2026, 2, 2, 9, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	_t1      = time.Date(2026, 3, 1, 10, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	_t2      = time.Date(2026, 3, 5, 10, 0, 0, 0, time.FixedZone("MSK", 3*3600))
)

type normalizeEnv struct {
	*ingestEnv
	norm *normalize.UseCase
}

func newNormalizeEnv(t *testing.T) *normalizeEnv {
	t.Helper()
	e := &normalizeEnv{ingestEnv: newIngestEnv(t)}
	orgs, _ := newOrgAccess(e.app, e.clock)
	var err error
	e.norm, err = newNormalize(e.app, e.store, e.uc, orgs, e.clock)
	require.NoError(t, err)
	return e
}

type pendingJob struct {
	id   int64
	args entity.NormalizeBatchJob
}

// pending reads the normalize jobs waiting in the queue in the order they were queued.
func (e *normalizeEnv) pending(t *testing.T) []pendingJob {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT id, args FROM river_job WHERE kind = $1 AND state = 'available' ORDER BY id", entity.KindNormalizeBatch)
	require.NoError(t, err)
	defer rows.Close()
	var out []pendingJob
	for rows.Next() {
		var (
			j   pendingJob
			raw []byte
		)
		require.NoError(t, rows.Scan(&j.id, &raw))
		require.NoError(t, json.Unmarshal(raw, &j.args))
		out = append(out, j)
	}
	require.NoError(t, rows.Err())
	return out
}

// completeJob ends a job in the transaction of its part, as the worker does with JobCompleteTx.
func completeJob(id int64) func(context.Context) error {
	return func(ctx context.Context) error {
		tx, ok := pg.TxFrom(ctx)
		if !ok {
			return errors.New("no transaction")
		}
		_, err := tx.Exec(ctx, "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1", id)
		return err
	}
}

func (e *normalizeEnv) run(t *testing.T, j pendingJob) error {
	t.Helper()
	return e.norm.RunBatch(t.Context(), j.args, completeJob(j.id))
}

// drain runs the waiting normalize jobs, and the parts they queue, until none is left.
func (e *normalizeEnv) drain(t *testing.T) int {
	t.Helper()
	runs := 0
	for jobs := e.pending(t); len(jobs) > 0; jobs = e.pending(t) {
		for _, j := range jobs {
			require.NoError(t, e.run(t, j))
			runs++
		}
	}
	return runs
}

// renormalize queues the normalization of a receipt once more, as a repeated job would.
func (e *normalizeEnv) renormalize(t *testing.T, receiptID entity.ID) {
	t.Helper()
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		return queue.New(jobs).Enqueue(ctx, entity.NormalizeBatchJob{OrganizationID: e.orgID, ReceiptID: receiptID}, entity.QueueBulk)
	}))
}

func (e *normalizeEnv) n(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.owner.QueryRow(t.Context(), sql, args...).Scan(&n))
	return n
}

func manifestOf(schema envelope.ManifestSchemaId, provider envelope.SourceProvider, instance, area string) *envelope.Manifest {
	m := importManifest("1")
	m.SchemaId, m.Source.Provider, m.Source.InstanceKey, m.Area.NativeId = schema, provider, instance, area
	return m
}

func jiraTasks(area string) *envelope.Manifest {
	return manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderJiraCloud, _jiraInstance, area)
}

func jiraEvents(area string) *envelope.Manifest {
	return manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, area)
}

type taskSpec struct {
	id, project, key, title string
	updated                 *time.Time
	revision                string
	restriction, assignee   string
}

func jiraTask(t *testing.T, s taskSpec) *envelope.Record {
	t.Helper()
	p := envelope.TaskV1{
		ProjectNativeId: s.project, TaskKey: &s.key, Title: &s.title, TypeNativeId: "10001", StatusNativeId: "3",
		CreatedAt: _created, UpdatedAt: s.updated,
	}
	if s.restriction != "" {
		p.Restriction = &s.restriction
	}
	if s.assignee != "" {
		p.AssigneeAccountNativeId = &s.assignee
	}
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	rec := &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task", PayloadSchema: "task/1", NativeId: s.id, Payload: raw}
	if s.revision != "" {
		rec.SourceRevision = &s.revision
	}
	return rec
}

func statusChange(t *testing.T, task, changelog string, at time.Time, actor string) *envelope.Record {
	t.Helper()
	field, before, after := "status", "1", "3"
	raw, err := json.Marshal(envelope.TaskEventV1{
		TaskNativeId: task, EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: &before,
		AfterValue: &after, ActorAccountNativeId: &actor,
	})
	require.NoError(t, err)
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: task,
		NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &at, Payload: raw,
	}
}

func accountRecord(t *testing.T, id, name string) *envelope.Record {
	t.Helper()
	raw, err := json.Marshal(envelope.AccountV1{Login: &id, DisplayName: &name, AccountKind: "human", Active: true})
	require.NoError(t, err)
	return &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "account", PayloadSchema: "account/1", NativeId: id, Payload: raw}
}

func statusRecord(t *testing.T, id, name string) *envelope.Record {
	t.Helper()
	raw, err := json.Marshal(envelope.TaskStatusV1{Name: name, Category: "indeterminate"})
	require.NoError(t, err)
	return &envelope.Record{RecordClass: envelope.RecordRecordClassObject, RecordKind: "task_status", PayloadSchema: "task_status/1", NativeId: id, Payload: raw}
}

func TestNormalize_SameKeyInJiraAndUniversalImportGivesTwoObjects(t *testing.T) {
	e := newNormalizeEnv(t)
	conn, scope := e.scope(t, "10000")
	rc := e.page(t, conn, scope, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PROJ-1", title: "A", updated: &_t1})))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)

	snapshot := manifestOf(envelope.ManifestSchemaIdTasks, envelope.SourceProviderUniversal, "import.example.test", "PROJ")
	snapshot.Snapshot = true
	read := "read-1"
	snapshot.ReadId = &read
	csv := []byte("native_id,project_native_id,task_key,type_native_id,status_native_id,created_at\nPROJ-1,PROJ,PROJ-1,Story,Done,2026-02-03T09:00:00+03:00\n")
	imported := e.imported(t, e.admin, packCSV(t, snapshot, csv))
	require.Equal(t, entity.ReceiptAccepted, imported.Status)
	e.drain(t)

	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task'"))
	assert.Equal(t, 2, e.n(t, "SELECT count(DISTINCT instance_id) FROM normalize.source_object WHERE object_kind = 'task'"), "AC-06, V-17: the instance tells them apart")
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.task_version WHERE task_key = 'PROJ-1' AND superseded_seq IS NULL"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task' AND native_ns = 'PROJ' AND native_id = 'PROJ-1'"),
		"the universal import keeps the namespace of its manifest")
	assert.Equal(t, 0, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE identity_quality <> 'stable'"))
}

func packCSV(t *testing.T, m *envelope.Manifest, rows []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	require.NoError(t, w.AddTasksCSV(rows))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestNormalize_RepeatsAddNothing(t *testing.T) {
	e := newNormalizeEnv(t)
	tasks := pack(t, jiraTasks("10000"),
		statusRecord(t, "3", "In Progress"), accountRecord(t, "u-1", "Same Name"),
		jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "A", updated: &_t1, assignee: "u-1"}),
		jiraTask(t, taskSpec{id: "20002", project: "10000", key: "PA-2", title: "B", updated: &_t1}))
	events := pack(t, jiraEvents("10000"), statusChange(t, "20001", "30010", _t1, "u-1"), statusChange(t, "20002", "30011", _t1, "u-1"))
	first := e.imported(t, e.admin, tasks)
	history := e.imported(t, e.admin, events)
	e.drain(t)
	versions := "SELECT count(*) FROM normalize.task_version"
	facts := "SELECT count(*) FROM normalize.task_event"
	require.Equal(t, 2, e.n(t, versions))
	require.Equal(t, 2, e.n(t, facts))

	for range 3 {
		again := e.imported(t, e.admin, tasks)
		assert.Equal(t, entity.ReceiptDuplicate, again.Status)
	}
	e.renormalize(t, first.ID)
	e.renormalize(t, history.ID)
	assert.Equal(t, 2, e.drain(t), "the repeated jobs ran")
	assert.Equal(t, 2, e.n(t, versions), "three repeats and a repeated normalization add no version")
	assert.Equal(t, 2, e.n(t, facts))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.dictionary_entry"))

	area, err := e.norm.AreaOf(t.Context(), e.orgID, *first.SourceInstanceID, entity.ObjectTrackerProject, "10000")
	require.NoError(t, err)
	statuses, err := e.norm.Dictionary(t.Context(), e.orgID, area, entity.DictionaryStatus)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "In Progress", statuses[0].Name, "the dictionaries of the package are read by area")
	current, err := e.norm.CurrentTasks(t.Context(), e.orgID, area, []string{"20001", "20002"})
	require.NoError(t, err)
	require.Len(t, current, 2)
	require.NotNil(t, current["20001"].SourceUpdatedAt)
	assert.True(t, current["20001"].SourceUpdatedAt.Equal(_t1))
	assert.NotEmpty(t, current["20001"].Revision.Source)
	assert.NotNil(t, current["20001"].AssigneeAccountID)
}

func TestNormalize_MovedTaskKeepsItsIdentity(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "A", updated: &_t1})))
	e.drain(t)
	moved := e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "20000", key: "PB-7", title: "A", updated: &_t2})))
	e.drain(t)

	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task'"), "the key and the project are not the identity")
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.task_version"))
	assert.Equal(t, 1, e.n(t, "SELECT count(DISTINCT task_id) FROM normalize.task_version"))
	newArea, err := e.norm.AreaOf(t.Context(), e.orgID, *moved.SourceInstanceID, entity.ObjectTrackerProject, "20000")
	require.NoError(t, err)
	oldArea, err := e.norm.AreaOf(t.Context(), e.orgID, *moved.SourceInstanceID, entity.ObjectTrackerProject, "10000")
	require.NoError(t, err)
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.task_version WHERE superseded_seq IS NULL AND area_id = $1 AND task_key = 'PB-7'", uuid.UUID(newArea)))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task' AND current_area_id = $1", uuid.UUID(newArea)),
		"access decides by the new area")
	seq := e.n(t, "SELECT max(known_seq) FROM normalize.task_version")
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.area_change WHERE known_seq = $1 AND area_id IN ($2, $3)", seq, uuid.UUID(newArea), uuid.UUID(oldArea)),
		"both areas are recounted")
}

func TestNormalize_LateEventKeepsItsEventTime(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraEvents("10000"), statusChange(t, "20001", "30020", _t2, "u-1")))
	e.drain(t)
	late := _t1.Add(-48 * time.Hour)
	rc := e.imported(t, e.admin, pack(t, jiraEvents("10000"), statusChange(t, "20001", "30010", late, "u-1"), statusChange(t, "20001", "30030", _t2.Add(time.Hour), "u-1")))
	e.drain(t)

	var (
		known, first int64
		at           time.Time
		offset       int
	)
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT known_seq, event_time, event_offset_s FROM normalize.task_event WHERE event_key = '30010/status'").Scan(&known, &at, &offset))
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT known_seq FROM normalize.task_event WHERE event_key = '30020/status'").Scan(&first))
	assert.Greater(t, known, first, "the late event has its own knowledge number")
	assert.True(t, at.Equal(late), "and its early event time")
	assert.Equal(t, 3*3600, offset, "the offset of the source is kept")
	area, err := e.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectTrackerProject, "10000")
	require.NoError(t, err)
	changes, err := e.norm.AreaChanges(t.Context(), e.orgID, area, first)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NotNil(t, changes[0].EarliestEventTime)
	assert.True(t, changes[0].EarliestEventTime.Equal(late), "area_change keeps the earliest time")
	assert.Equal(t, []string{"task_event"}, changes[0].FactKinds)
	assert.Equal(t, int(known), e.n(t, "SELECT last_change_seq FROM normalize.area WHERE area_id = $1", uuid.UUID(area)))
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.task_event WHERE observed_via = 'import'"))
}

func TestNormalize_OlderStateDoesNotReplaceTheCurrentOne(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "new", updated: &_t2})))
	e.drain(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "old", updated: &_t1})))
	e.drain(t)

	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.task_version"), "the older state stays a record of the journal")
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.task_version WHERE superseded_seq IS NULL AND title = 'new'"))
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM ingest.record WHERE outcome = 'accepted' AND record_kind = 'task'"))
}

func TestNormalize_SliceSeesTheVersionOfItsKnowledgeNumber(t *testing.T) {
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "v1", updated: &_t1})))
	e.drain(t)
	k1 := int64(e.n(t, "SELECT last_seq FROM platform.org_clock WHERE organization_id = $1", uuid.UUID(e.orgID)))
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "v2", updated: &_t2})))
	e.drain(t)
	k2 := int64(e.n(t, "SELECT last_seq FROM platform.org_clock WHERE organization_id = $1", uuid.UUID(e.orgID)))
	area, err := e.norm.AreaOf(t.Context(), e.orgID, *rc.SourceInstanceID, entity.ObjectTrackerProject, "10000")
	require.NoError(t, err)

	tests := []struct {
		name      string
		giveSlice int64
		wantTitle string
	}{
		{name: "before the second version", giveSlice: k1, wantTitle: "v1"},
		{name: "after it", giveSlice: k2, wantTitle: "v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.norm.TasksAt(t.Context(), e.orgID, area, tt.giveSlice)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tt.wantTitle, got[0].Title)
		})
	}
	empty, err := e.norm.TasksAt(t.Context(), e.orgID, area, k1-1)
	require.NoError(t, err)
	assert.Empty(t, empty, "a slice before the first version sees nothing")
}

func TestNormalize_KnowledgeNumbersFollowCommits(t *testing.T) {
	e := newNormalizeEnv(t)
	receipts := make([]entity.ID, 0, 3)
	for i := range 3 {
		rc := e.imported(t, e.admin, pack(t, jiraTasks("10000"),
			jiraTask(t, taskSpec{id: strconv.Itoa(30000 + i), project: "10000", key: fmt.Sprintf("PA-%d", i), title: "t", updated: &_t1})))
		receipts = append(receipts, rc.ID)
	}
	jobs := e.pending(t)
	require.Len(t, jobs, 3)

	holder, err := e.owner.Begin(t.Context())
	require.NoError(t, err)
	var held int64
	require.NoError(t, holder.QueryRow(t.Context(), "UPDATE platform.org_clock SET last_seq = last_seq + 1 WHERE organization_id = $1 RETURNING last_seq",
		uuid.UUID(e.orgID)).Scan(&held))
	first := make(chan error, 1)
	go func() { first <- e.run(t, jobs[0]) }()
	select {
	case err := <-first:
		t.Fatalf("normalization took a number while another transaction held the counter: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, holder.Commit(t.Context()))
	require.NoError(t, <-first)
	assert.Equal(t, int(held)+1, e.n(t, "SELECT known_seq FROM normalize.task_version WHERE receipt_id = $1", uuid.UUID(receipts[0])),
		"the number comes after the commit of the transaction that held it")

	var wg sync.WaitGroup
	for _, j := range jobs[1:] {
		wg.Go(func() { assert.NoError(t, e.run(t, j)) })
	}
	wg.Wait()
	assert.Equal(t, 3, e.n(t, "SELECT count(DISTINCT known_seq) FROM normalize.task_version"), "parallel normalizations take numbers one after another")
	assert.Equal(t, int(held)+3, e.n(t, "SELECT max(known_seq) FROM normalize.task_version"))
}

func TestNormalize_AccountsAreJoinedByNativeIDOnly(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"),
		accountRecord(t, "JIRAUSER10100", "Иван Петров"), accountRecord(t, "JIRAUSER10200", "Иван Петров"),
		jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "A", updated: &_t1, assignee: "JIRAUSER10200"})))
	e.imported(t, e.admin, pack(t, jiraEvents("10000"), statusChange(t, "20001", "30010", _t1, "JIRAUSER10300")))
	e.drain(t)

	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM org.source_account WHERE display_name = 'Иван Петров'"), "the same name joins nothing")
	assert.Equal(t, 1, e.n(t, `SELECT count(*) FROM normalize.task_version v JOIN org.source_account a ON a.id = v.assignee_account_id
		WHERE a.native_account_id = 'JIRAUSER10200'`))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM org.source_account WHERE native_account_id = 'JIRAUSER10300' AND proposed_kind = 'unknown' AND display_name IS NULL"),
		"an account an event names appears without a description")
}

func TestNormalize_TaskWithSecurityLevelIsRestricted(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"),
		jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "open", updated: &_t1}),
		jiraTask(t, taskSpec{id: "20002", project: "10000", key: "PA-2", title: "closed", updated: &_t1, restriction: "10100"})))
	e.drain(t)

	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task' AND restricted AND native_id = '20002'"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.source_object WHERE object_kind = 'task' AND NOT restricted AND native_id = '20001'"))
}

func TestNormalize_AcceptedNewMakesANewVersion(t *testing.T) {
	e := newNormalizeEnv(t)
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "A", updated: &_t1, revision: "7"})))
	e.drain(t)
	before := e.n(t, "SELECT known_seq FROM normalize.task_version")
	e.imported(t, e.admin, pack(t, jiraTasks("10000"), jiraTask(t, taskSpec{id: "20001", project: "10000", key: "PA-1", title: "B", updated: &_t1, revision: "7"})))
	assert.Zero(t, e.drain(t), "a conflict goes to no normalization")

	var conflict uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM ingest.conflict WHERE status = 'open'").Scan(&conflict))
	_, err := e.uc.ResolveConflict(t.Context(), e.admin, entity.ID(conflict), entity.ResolutionAcceptedNew)
	require.NoError(t, err)
	e.drain(t)

	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.task_version WHERE source_revision = '7'"))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.task_version WHERE superseded_seq IS NULL AND title = 'B' AND known_seq > $1", before),
		"the accepted record is the current version with a new knowledge number")
}

func TestNormalize_InterruptedBatchContinuesFromItsCursor(t *testing.T) {
	recs := make([]*envelope.Record, 0, 450)
	for i := range 450 {
		recs = append(recs, jiraTask(t, taskSpec{id: strconv.Itoa(40000 + i), project: "10000", key: fmt.Sprintf("PA-%d", i), title: "t", updated: &_t1}))
	}

	whole := newNormalizeEnv(t)
	data := pack(t, jiraTasks("10000"), recs...)
	whole.imported(t, whole.admin, data)
	assert.Equal(t, 3, whole.drain(t), "450 records take three parts of 200")

	cut := newNormalizeEnv(t)
	cut.imported(t, cut.admin, data)
	require.NoError(t, cut.run(t, cut.pending(t)[0]))
	second := cut.pending(t)[0]
	stopped := cut.norm.RunBatch(t.Context(), second.args, func(context.Context) error { return errors.New("worker stopped") })
	require.Error(t, stopped)
	assert.Equal(t, 200, cut.n(t, "SELECT count(*) FROM normalize.task_version"), "the interrupted part left nothing")
	assert.Equal(t, 2, cut.drain(t), "the job of the part repeats from its cursor")

	assert.Equal(t, snapshotOf(t, whole), snapshotOf(t, cut), "the result is that of an uninterrupted run")
	assert.Equal(t, 450, cut.n(t, "SELECT count(*) FROM normalize.task_version"))
}

// snapshotOf lists the versions of tasks by native ID with their knowledge number and revision.
func snapshotOf(t *testing.T, e *normalizeEnv) []string {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT o.native_id, v.known_seq - c.first, v.source_revision
		FROM normalize.task_version v JOIN normalize.source_object o ON o.id = v.task_id
		CROSS JOIN (SELECT min(known_seq) AS first FROM normalize.task_version) c ORDER BY 1`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			native, revision string
			seq              int64
		)
		require.NoError(t, rows.Scan(&native, &seq, &revision))
		out = append(out, fmt.Sprintf("%s %d %s", native, seq, revision))
	}
	require.NoError(t, rows.Err())
	return out
}
