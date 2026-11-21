package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type historyPages struct {
	sync.Connect
	first, last []byte
}

func (h *historyPages) Fetch(_ context.Context, _, _ entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	if in.Cursor == nil {
		return io.NopCloser(bytes.NewReader(h.first)), nil
	}
	return io.NopCloser(bytes.NewReader(h.last)), nil
}

func TestSync_RecoveryValidatesEveryHistoryPage(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	e.runner().settle(t)
	scopes, err := e.connect.Scopes(t.Context(), e.org, conn)
	require.NoError(t, err)
	require.Len(t, scopes, 1)
	require.NotNil(t, scopes[0].AreaID)
	const taskID = "20003"
	job := entity.SyncHistoryJob{
		OrganizationID: e.org, ConnectionID: conn, ScopeID: scopes[0].ID,
		SchemaID: "task_events", ObjectKind: "task", NativeID: taskID,
	}
	adapter := &historyPages{Connect: e.connect}
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	reader := sync.New(sync.Deps{
		Repo: syncrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Connect: adapter, Ingest: e.intake, Normalize: e.norm,
		Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics, Clock: e.clock,
	})
	run := func(first, last *envelope.Record) entity.ID {
		t.Helper()
		e.clock.Advance(time.Second)
		cursor := &envelope.Cursor{ModifiedAt: *first.EventTime, NativeId: first.NativeId}
		manifest := importManifest("1")
		manifest.SchemaId = "task_events"
		manifest.Selection.End, manifest.Selection.NextCursor = false, cursor
		adapter.first = pack(t, manifest, first)
		manifest.Selection.End, manifest.Selection.Cursor, manifest.Selection.NextCursor = true, cursor, nil
		adapter.last = pack(t, manifest, last)
		require.NoError(t, reader.RunHistory(t.Context(), job, "history-test-first", nil))
		var id int64
		var raw []byte
		require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id,args FROM river_job WHERE kind = 'sync.history' AND state IN ('available','scheduled')").Scan(&id, &raw))
		var next entity.SyncHistoryJob
		require.NoError(t, json.Unmarshal(raw, &next))
		require.Len(t, next.PageReceiptIDs, 1)
		require.NoError(t, reader.RunHistory(t.Context(), next, "history-test-last", completeJob(id)))
		e.runner().settle(t)
		return next.PageReceiptIDs[0]
	}
	countRows := func(query string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, e.owner.QueryRow(t.Context(), query, args...).Scan(&n))
		return n
	}
	recoveries := func() int {
		return countRows("SELECT count(*) FROM sync.activity_recovery WHERE native_id = $1", taskID)
	}
	before := recoveries()
	conflict := statusChange(t, taskID, "40001", _t1, "another-actor")
	badReceipt := run(conflict, statusChange(t, taskID, "history-clean-final", _t2, "demo-person-05"))
	assert.Equal(t, 1, countRows("SELECT count(*) FROM ingest.record WHERE receipt_id = $1 AND outcome = 'conflict'", uuid.UUID(badReceipt)))
	assert.Equal(t, before, recoveries(), "a clean final page cannot confirm an earlier page with a journal conflict")

	firstReceipt := run(statusChange(t, taskID, "history-page-one", _t1, "demo-person-05"),
		statusChange(t, taskID, "history-page-two", _t2, "demo-person-05"))
	require.Equal(t, before+1, recoveries())
	assert.Equal(t, 1, countRows(
		"SELECT count(*) FROM sync.activity_recovery WHERE cardinality(page_receipt_ids) = 2 AND page_receipt_ids[1] = $1", uuid.UUID(firstReceipt)))
	recoveryOfTask := func() *entity.ActivityHistoryNotice {
		t.Helper()
		history, err := reader.ActivityHistory(t.Context(), e.org, []entity.ID{*scopes[0].AreaID}, conn, e.clock.Now())
		require.NoError(t, err)
		for i := range history {
			if history[i].Key.NativeID == taskID {
				require.NotNil(t, history[i].Cut)
				return history[i].Recovery
			}
		}
		t.Fatal("missing history evidence")
		return nil
	}
	require.NotNil(t, recoveryOfTask())
	require.NoError(t, e.intake.RejectRecords(t.Context(), e.org, firstReceipt, map[int]string{1: entity.RecordMissingSourceID}))
	assert.Nil(t, recoveryOfTask(), "a later rejection on the first page invalidates recovery evidence")
}

// jobRunner runs the jobs of collection and normalization as the worker does, each completed in the
// transaction of its result, in the order of their scheduled time; the clock moves to that time.
type jobRunner struct {
	owner *pgxpool.Pool
	clock *clock.Manual
	sync  *sync.UseCase
	norm  *normalize.UseCase
}

func (r jobRunner) next(t *testing.T) *queued {
	t.Helper()
	var q queued
	err := r.owner.QueryRow(t.Context(), `SELECT id, kind, args, scheduled_at FROM river_job
		WHERE kind IN ('sync.page', 'sync.history', 'sync.reconcile', 'sync.dirty', 'sync.schedule', 'sync.webhook', 'sync.mr', 'normalize.batch')
			AND state NOT IN ('completed', 'discarded', 'cancelled')
		ORDER BY scheduled_at, id LIMIT 1`).Scan(&q.id, &q.kind, &q.args, &q.scheduledAt)
	if err != nil {
		return nil
	}
	return &q
}

func (r jobRunner) run(t *testing.T, q *queued) error {
	t.Helper()
	complete := func(ctx context.Context) error {
		tx, ok := pg.TxFrom(ctx)
		if !ok {
			return errors.New("no transaction")
		}
		_, err := tx.Exec(ctx, "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1", q.id)
		return err
	}
	holder := "river-job-" + strconv.FormatInt(q.id, 10)
	switch q.kind {
	case entity.KindSyncWebhook:
		return r.sync.RunWebhook(t.Context(), args[entity.SyncWebhookJob](t, q), complete)
	case entity.KindSyncMR:
		return r.sync.RunMR(t.Context(), args[entity.SyncMRJob](t, q), holder, complete)
	case entity.KindSyncPage:
		return r.sync.RunPage(t.Context(), args[entity.SyncPageJob](t, q), holder, complete)
	case entity.KindSyncHistory:
		return r.sync.RunHistory(t.Context(), args[entity.SyncHistoryJob](t, q), holder, complete)
	case entity.KindSyncReconcile:
		return r.sync.RunReconcile(t.Context(), args[entity.SyncReconcileJob](t, q), holder, complete)
	case entity.KindSyncDirty:
		return r.sync.RunDirty(t.Context(), args[entity.SyncDirtyJob](t, q), holder, complete)
	case entity.KindSyncSchedule:
		if err := r.sync.Schedule(t.Context(), args[entity.SyncScheduleJob](t, q).OrganizationID); err != nil {
			return err
		}
		_, err := r.owner.Exec(t.Context(), "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1", q.id)
		return err
	}
	return r.norm.RunBatch(t.Context(), args[entity.NormalizeBatchJob](t, q), complete)
}

func args[T any](t *testing.T, q *queued) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(q.args, &out))
	return out
}

// settle runs jobs until none is left.
func (r jobRunner) settle(t *testing.T) int {
	t.Helper()
	n := 0
	for q := r.next(t); q != nil; q = r.next(t) {
		if wait := q.scheduledAt.Sub(r.clock.Now()); wait > 0 {
			r.clock.Advance(wait)
		}
		require.NoError(t, r.run(t, q), q.kind)
		n++
		require.Less(t, n, 300, "jobs do not end")
	}
	return n
}

func (e *syncEnv) runner() jobRunner {
	return jobRunner{owner: e.owner, clock: e.clock, sync: e.uc, norm: e.norm}
}

func (e *syncEnv) status(t *testing.T, conn entity.ID) (entity.SyncStatus, entity.SyncDataKindStatus) {
	t.Helper()
	st, err := e.uc.Status(t.Context(), e.org, conn)
	require.NoError(t, err)
	require.Len(t, st.DataKinds, 1)
	return st, st.DataKinds[0]
}

func (e *syncEnv) runs(t *testing.T, conn entity.ID) []entity.ReconcileRun {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT id FROM sync.reconcile_run WHERE connection_id = $1 ORDER BY started_at, id", uuid.UUID(conn))
	require.NoError(t, err)
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	out := make([]entity.ReconcileRun, 0, len(ids))
	for _, id := range ids {
		require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
			run, _, err := syncrepo.NewRepo(e.app).Reconcile(ctx, e.org, entity.ID(id))
			out = append(out, run)
			return err
		}))
	}
	return out
}

func TestSync_V06_StatusChangesOnlyAfterReconcile(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	e.runner().settle(t)
	_, first := e.status(t, conn)
	require.Equal(t, "complete", first.Completeness, "the first load and its reconciliation")
	require.NotNil(t, first.LastSuccessAt)
	lastSuccess := *first.LastSuccessAt

	e.clock.Advance(24 * time.Hour)
	require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Page: 0, Attempts: []int{1}, Kind: testadapter.FaultUnavailable},
	}}))
	require.NoError(t, e.uc.Schedule(t.Context(), e.org))
	q := e.runner().next(t)
	require.NotNil(t, q)
	require.Equal(t, entity.KindSyncPage, q.kind, "the daily increment starts by itself")
	require.NoError(t, e.runner().run(t, q))

	st, failed := e.status(t, conn)
	assert.True(t, failed.FailedPagePresent, "the page the collection stopped at")
	assert.Equal(t, "failed", failed.LastOutcome)
	assert.Equal(t, "unavailable", failed.LastErrorCode)
	require.NotNil(t, failed.LastSuccessAt)
	assert.True(t, failed.LastSuccessAt.Equal(lastSuccess), "the last success stays")
	assert.Equal(t, "partial", failed.Completeness)
	assert.Contains(t, failed.AffectedMetrics, "T08", "the metrics the gap touches")
	assert.Equal(t, entity.ConnectionActive, st.ConnectionStatus)

	_, err := e.owner.Exec(t.Context(), "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = $1", entity.KindCalcRecount)
	require.NoError(t, err)
	e.runner().settle(t)
	recounts := count(t, t.Context(), e.owner, `SELECT count(*) FROM river_job j JOIN connect.connection_scope s
		ON s.area_id::text = j.args->>'area_id' WHERE j.kind = '`+entity.KindCalcRecount+`' AND j.state = 'available' AND s.connection_id = '`+conn.String()+`'`)
	assert.Equal(t, 1, recounts, "the reconciliation queues the recount of its area, the pages read again changed no fact")
	st, done := e.status(t, conn)
	assert.False(t, done.FailedPagePresent)
	assert.Equal(t, "complete", done.Completeness)
	require.NotNil(t, done.LastSuccessAt)
	assert.True(t, done.LastSuccessAt.After(lastSuccess), "a new last success after the reconciliation")
	require.NotNil(t, done.Watermark)
	assert.True(t, done.Watermark.After(lastSuccess))
	require.NotNil(t, done.LagSeconds)
	require.NotNil(t, st.LastReconcile)
	assert.Equal(t, entity.ReconcileComplete, st.LastReconcile.Outcome)
	assert.Equal(t, 0, *st.LastReconcile.MissingCount)
	assert.Equal(t, len(e.manifest(t, conn)), *st.LastReconcile.ExpectedCount)
	assert.Equal(t, e.state(t, conn).LastSuccessAt, done.LastSuccessAt, "the status is the state of sync")
}

func TestSync_ReconcileFindsMissedAndStaleObjects(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Missed: []string{"20007"}, Stale: []string{"20005"}})
	conn := e.connection(t, "10000")
	e.runner().settle(t)

	runs := e.runs(t, conn)
	require.Len(t, runs, 2)
	first, second := runs[0], runs[1]
	assert.Equal(t, entity.ReconcileIncomplete, first.Outcome)
	assert.Equal(t, 1, *first.MissingCount, "the task the pages skipped")
	assert.Equal(t, 1, *first.ChangedCount, "the task changed without a new time of change")
	assert.Equal(t, entity.ReconcileComplete, second.Outcome, "the objects were read again")
	assert.Equal(t, 0, *second.MissingCount)
	assert.Equal(t, 0, *second.ChangedCount)
	assert.Equal(t, "complete", e.state(t, conn).Completeness)
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM sync.dirty_key"))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, `SELECT count(*) FROM normalize.task_version v JOIN normalize.source_object o ON o.id = v.task_id
		WHERE o.native_id = '20005' AND v.superseded_seq IS NULL AND v.source_updated_at = '2026-03-03T10:00:00Z'`))
	keyed := func(schema string) int {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt WHERE manifest->'selection'->>'total_count' IS NULL"+
			" AND manifest->>'schema_id' = '"+schema+"'")
	}
	assert.Equal(t, 1, keyed("tasks"), "the tasks found are read again by key")
	assert.Equal(t, 2, keyed("task_events"), "with their history; the other read is the cut history of 20003")
}

// hookConnect adds keys to the set while the set is read.
type hookConnect struct {
	sync.Connect
	onFetch func(ctx context.Context)
}

func (h hookConnect) Fetch(ctx context.Context, orgID, connectionID entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	if h.onFetch != nil {
		h.onFetch(ctx)
	}
	return h.Connect.Fetch(ctx, orgID, connectionID, in)
}

func TestSync_KeyAddedDuringTheReadIsKept(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	e.runner().settle(t)
	scopes, err := e.connect.Scopes(t.Context(), e.org, conn)
	require.NoError(t, err)
	key := func(id string) entity.DirtyKey {
		return entity.DirtyKey{ScopeID: scopes[0].ID, DataKind: "tasks", Object: entity.ObjectKey{ObjectKind: "task", NativeID: id}}
	}
	repo := syncrepo.NewRepo(e.app)
	add := func(ctx context.Context, keys ...entity.DirtyKey) {
		require.NoError(t, pg.InOrgTx(ctx, e.app, uuid.UUID(e.org), func(ctx context.Context) error {
			return repo.AddDirtyKeys(ctx, e.org, conn, keys, e.clock.Now())
		}))
	}
	add(t.Context(), key("20001"), key("20002"))
	once := false
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	hooked := sync.New(sync.Deps{
		Repo: repo, Tenants: platform.NewRepo(e.app), Connect: hookConnect{Connect: e.connect, onFetch: func(ctx context.Context) {
			if !once {
				once = true
				e.clock.Advance(time.Second)
				add(ctx, key("20002"), key("20003"))
			}
		}},
		Ingest: e.intake, Normalize: e.norm, Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics,
		Clock: e.clock, Rand: func() float64 { return 0 }, PageSize: 3,
	})
	require.NoError(t, hooked.RunDirty(t.Context(), entity.SyncDirtyJob{OrganizationID: e.org, ConnectionID: conn}, "test", nil))

	left := count(t, t.Context(), e.owner, "SELECT count(*) FROM sync.dirty_key")
	assert.Equal(t, 2, left, "20002 added again and 20003 added during the read stay; 20001 is read")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, `SELECT count(*) FROM sync.dirty_key WHERE native_ref LIKE '%"native_id":"20002"%'`))
	require.NoError(t, e.uc.RunDirty(t.Context(), entity.SyncDirtyJob{OrganizationID: e.org, ConnectionID: conn}, "test", nil))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM sync.dirty_key"), "the next read takes them")
}

func TestSync_ScheduleStartsTheDailyIncrement(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	e.runner().settle(t)
	pages := func() int {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.page' AND state <> 'completed'")
	}
	before := e.state(t, conn)
	require.NotNil(t, before.LastAttemptAt)

	e.clock.Advance(before.LastAttemptAt.Add(22 * time.Hour).Sub(e.clock.Now()))
	require.NoError(t, e.uc.Tick(t.Context()))
	e.runner().settle(t)
	assert.Equal(t, before.LastAttemptAt, e.state(t, conn).LastAttemptAt, "nothing is due yet")

	e.clock.Advance(before.LastAttemptAt.Add(24 * time.Hour).Sub(e.clock.Now()))
	require.NoError(t, e.uc.Tick(t.Context()))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.schedule' AND state <> 'completed'"),
		"the tick plans every organization in its own job")
	require.NoError(t, e.runner().run(t, e.runner().next(t)))
	assert.Equal(t, 1, pages(), "the increment is queued without a manual start")
	require.NoError(t, e.uc.Schedule(t.Context(), e.org))
	assert.Equal(t, 1, pages(), "an active attempt is not queued twice")

	e.runner().settle(t)
	after := e.state(t, conn)
	require.NotNil(t, after.LastAttemptAt)
	assert.False(t, after.LastAttemptAt.After(before.LastAttemptAt.Add(24*time.Hour+time.Hour)), "within 24 h of the previous one")
	st, kind := e.status(t, conn)
	require.NotNil(t, st.LastReconcile.FinishedAt)
	assert.False(t, st.LastReconcile.FinishedAt.Before(st.LastReconcile.StartedAt), "the reconciliation shows its start and end")
	require.NotNil(t, kind.Watermark)
	assert.True(t, kind.Watermark.After(*before.Watermark), "a new watermark")
	assert.InDelta(t, 0, gauge(t, e, "craftpulse_sync_seconds_since_success"), 1, "the freshness is reported by connection")
}

func gauge(t *testing.T, e *syncEnv, name string) float64 {
	t.Helper()
	require.NoError(t, e.uc.Schedule(t.Context(), e.org))
	families, err := e.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name && len(f.GetMetric()) > 0 {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("metric %s is not reported", name)
	return 0
}
