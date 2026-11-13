package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	workerctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/worker"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type syncEnv struct {
	app     *pgxpool.Pool
	owner   *pgxpool.Pool
	clock   *clock.Manual
	adapter *adapterServer
	connect *connect.UseCase
	uc      *sync.UseCase
	norm    *normalize.UseCase
	intake  *ingest.UseCase
	metrics syncMetrics
	reg     *prometheus.Registry
	org     entity.ID
	jitter  atomic.Value
}

func newSyncEnv(t *testing.T, scenario testadapter.Config) *syncEnv {
	t.Helper()
	db := newDB(t)
	db.migrate(t)
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	e := &syncEnv{
		app: db.pool(t, "craftpulse_app"), owner: db.pool(t, "craftpulse_owner"),
		clock: clock.NewManual(time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)), adapter: testAdapter(t, cfg, scenario, _adapterKey),
		org: entity.ID(uuid.New()),
	}
	e.jitter.Store(0.0)
	require.NoError(t, platform.NewRepo(e.owner).CreateTenant(t.Context(), e.org, e.clock.Now()), "the organization has its knowledge counter")
	reg := prometheus.NewRegistry()
	caller, err := adapterapi.NewCaller(adapterapi.Config{
		InternalCA: ca.pool, Registerer: reg,
		Builtin: map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: e.adapter.URL, Key: _adapterKey}},
	})
	require.NoError(t, err)
	e.connect, err = newConnect(e.app, adapterKeyring(t), caller, e.clock)
	require.NoError(t, err)
	orgs, rights := newOrgAccess(e.app, e.clock)
	intake, err := newIngest(e.app, sharedStore(t), e.connect, rights, e.clock, reg)
	require.NoError(t, err)
	e.intake = intake
	e.norm, err = newNormalize(e.app, sharedStore(t), intake, orgs, e.clock)
	require.NoError(t, err)
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	e.metrics, err = newSyncMetrics(reg)
	require.NoError(t, err)
	e.reg = reg
	e.uc = sync.New(sync.Deps{
		Repo: syncrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Connect: e.connect, Ingest: intake, Normalize: e.norm,
		Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics,
		Clock: e.clock, Rand: func() float64 { return e.jitter.Load().(float64) }, PageSize: 3,
	})
	return e
}

// connection writes an active connection to a project and queues its first page; the test adapter
// serves project 10000.
func (e *syncEnv) connection(t *testing.T, project string) entity.ID {
	t.Helper()
	inst, err := e.connect.EnsureInstance(t.Context(), e.org, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	id := entity.ID(uuid.Must(uuid.NewV7()))
	r := connectrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		err := r.CreateConnection(ctx, entity.Connection{
			ID: id, OrganizationID: e.org, InstanceID: inst.ID, Name: "Jira", AdapterKind: entity.AdapterBuiltin,
			ContractVersion: "1", AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionDraft, RowVersion: 1,
			CreatedBy: entity.ID(uuid.New()), CreatedAt: e.clock.Now(),
		})
		if err != nil {
			return err
		}
		_, err = r.AddScope(ctx, e.org, id, inst.ID, entity.NewScope{Kind: entity.ScopeTrackerProject, NativeScopeID: project, DisplayName: project}, e.clock.Now())
		return err
	}))
	_, err = e.owner.Exec(t.Context(), "UPDATE connect.connection SET status = 'active' WHERE id = $1", uuid.UUID(id))
	require.NoError(t, err)
	e.enqueue(t, entity.SyncPageJob{OrganizationID: e.org, ConnectionID: id, DataKind: "tasks", CorrelationID: entity.ID(uuid.New())})
	return id
}

func (e *syncEnv) enqueue(t *testing.T, job entity.Job) {
	t.Helper()
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		return queue.New(jobs).Enqueue(ctx, job, entity.QueueBulk)
	}))
}

type queued struct {
	id          int64
	kind        string
	args        []byte
	scheduledAt time.Time
}

// next is the oldest sync job not completed yet, whatever its time; nil when none is left.
func (e *syncEnv) next(t *testing.T, conn entity.ID) *queued {
	t.Helper()
	var q queued
	err := e.owner.QueryRow(t.Context(), `SELECT id, kind, args, scheduled_at FROM river_job
		WHERE kind IN ('sync.page', 'sync.history') AND state NOT IN ('completed', 'discarded', 'cancelled') AND args->>'connection_id' = $1
		ORDER BY id LIMIT 1`, conn.String()).Scan(&q.id, &q.kind, &q.args, &q.scheduledAt)
	if errors.Is(err, context.Canceled) || err != nil {
		return nil
	}
	return &q
}

// run executes the job as the worker does; fail stands for a crash right before the commit.
func (e *syncEnv) run(t *testing.T, q *queued, fail bool) error {
	t.Helper()
	complete := func(ctx context.Context) error {
		if fail {
			return errors.New("worker stopped before the commit")
		}
		tx, _ := pg.TxFrom(ctx)
		_, err := tx.Exec(ctx, "UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1", q.id)
		return err
	}
	holder := "river-job-" + uuid.NewString()
	if q.kind == entity.KindSyncHistory {
		var job entity.SyncHistoryJob
		require.NoError(t, json.Unmarshal(q.args, &job))
		return e.uc.RunHistory(t.Context(), job, holder, complete)
	}
	var job entity.SyncPageJob
	require.NoError(t, json.Unmarshal(q.args, &job))
	return e.uc.RunPage(t.Context(), job, holder, complete)
}

// drain runs the jobs of the connection until none is left, moving the clock to each scheduled time.
func (e *syncEnv) drain(t *testing.T, conn entity.ID) int {
	t.Helper()
	n := 0
	for q := e.next(t, conn); q != nil; q = e.next(t, conn) {
		if wait := q.scheduledAt.Sub(e.clock.Now()); wait > 0 {
			e.clock.Advance(wait)
		}
		require.NoError(t, e.run(t, q, false))
		n++
		require.Less(t, n, 100, "collection does not end")
	}
	return n
}

func (e *syncEnv) state(t *testing.T, conn entity.ID) entity.SyncState {
	t.Helper()
	var st entity.SyncState
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		var err error
		st, _, err = syncrepo.NewRepo(e.app).State(ctx, e.org, conn, "tasks")
		return err
	}))
	return st
}

func (e *syncEnv) accepted(t *testing.T, kind string) []string {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT native_id FROM ingest.record WHERE outcome = 'accepted' AND record_kind = $1 ORDER BY native_id", kind)
	require.NoError(t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	return ids
}

// manifest is the list of IDs the source reports for tasks (V-06).
func (e *syncEnv) manifest(t *testing.T, conn entity.ID) []string {
	t.Helper()
	var ids []string
	in := entity.ManifestInput{SchemaID: "tasks", Scope: entity.ScopeRef{Kind: entity.ScopeTrackerProject, NativeScopeID: "10000"}, Limit: 100}
	for {
		page, err := e.connect.Manifest(t.Context(), e.org, conn, in)
		require.NoError(t, err)
		for _, it := range page.Items {
			if !slices.Contains(ids, it.NativeID) {
				ids = append(ids, it.NativeID)
			}
		}
		if page.NextCursor == "" {
			break
		}
		in.Cursor = page.NextCursor
	}
	slices.Sort(ids)
	return ids
}

func unique(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func TestSync_PagesCommitWithTheirCheckpoint(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")

	require.NoError(t, e.run(t, e.next(t, conn), false))
	first := e.state(t, conn)
	assert.Equal(t, entity.SyncBackfill, first.Phase)
	require.NotNil(t, first.CheckpointAt)
	assert.Contains(t, first.Cursor, `"modified_at":"2026-03-01T10:00:00Z"`, "the checkpoint is the cursor after page 1")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt WHERE status = 'accepted'"), "the page is in the journal with it")

	e.drain(t, conn)
	done := e.state(t, conn)
	assert.Equal(t, entity.SyncIncremental, done.Phase, "the end of the selection ends the first load")
	assert.Nil(t, done.LastSuccessAt, "the last success waits for the reconciliation")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.reconcile'"))
	assert.Equal(t, 3, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt WHERE manifest->>'schema_id' = 'tasks'"), "three pages of tasks")
	assert.Equal(t, e.manifest(t, conn), unique(e.accepted(t, "task")), "collected IDs are the IDs of the manifest (V-06)")
	assert.Len(t, e.accepted(t, "task_event"), 10)

	history := count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.history' AND state = 'completed' AND args->>'native_id' = '20003'")
	assert.Equal(t, 1, history, "the cut history of task 20003 is read on to its end")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.history' AND state <> 'completed'"))

	accepted := count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'accepted'")
	e.enqueue(t, entity.SyncPageJob{OrganizationID: e.org, ConnectionID: conn, DataKind: "tasks", CorrelationID: entity.ID(uuid.New())})
	e.drain(t, conn)
	assert.Equal(t, accepted, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'accepted'"),
		"a new round reads back over its last window and adds nothing twice")
	assert.Positive(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'replay'"), "what it reads again is a replay")
}

func TestSync_InterruptedPageIsReadAgainWithoutDuplicates(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	require.NoError(t, e.run(t, e.next(t, conn), false))
	require.NoError(t, e.run(t, e.next(t, conn), false))
	afterTwo := e.state(t, conn)

	third := e.next(t, conn)
	require.Error(t, e.run(t, third, true), "the worker stops before the commit of page 3")
	assert.Equal(t, afterTwo.Cursor, e.state(t, conn).Cursor, "nothing of page 3 is committed")
	assert.Equal(t, third.id, e.next(t, conn).id, "the same job repeats page 3")

	e.drain(t, conn)
	assert.Equal(t, 3, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.receipt WHERE manifest->>'schema_id' = 'tasks'"),
		"page 2 is not read again and page 3 is in the journal once")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'replay' AND record_kind = 'task'"))
	assert.Equal(t, e.manifest(t, conn), unique(e.accepted(t, "task")))
}

func TestSync_RateLimitHoldsEveryRequest(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Page: 2, Attempts: []int{1}, Kind: testadapter.FaultRateLimited, RetryAfterSeconds: 30},
	}})
	conn := e.connection(t, "10000")
	require.NoError(t, e.run(t, e.next(t, conn), false))
	start := e.clock.Now()
	require.NoError(t, e.run(t, e.next(t, conn), false))
	retry := e.next(t, conn)
	assert.True(t, start.Add(30*time.Second).Equal(retry.scheduledAt), "the page waits for the end of the pause: %s", retry.scheduledAt)
	assert.Equal(t, "rate_limited", e.state(t, conn).LastErrorCode)

	hits := e.adapter.hits.Load()
	require.NoError(t, e.run(t, retry, false), "a run before the end of the pause")
	assert.Equal(t, hits, e.adapter.hits.Load(), "no request reaches the adapter during the pause")
	e.drain(t, conn)
	assert.Equal(t, e.manifest(t, conn), unique(e.accepted(t, "task")))
}

func TestSync_TimeoutKeepsTheCursor(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Page: 2, Attempts: []int{1}, Kind: testadapter.FaultHang, DelayMS: 10},
	}})
	conn := e.connection(t, "10000")
	require.NoError(t, e.run(t, e.next(t, conn), false))
	before := e.state(t, conn)
	require.NoError(t, e.run(t, e.next(t, conn), false))
	failed := e.state(t, conn)
	assert.Equal(t, before.Cursor, failed.Cursor, "the cursor stays")
	assert.Equal(t, before.Cursor, failed.FailedCursor)
	assert.Equal(t, "timeout", failed.LastErrorCode)
	assert.Equal(t, "failed", failed.LastOutcome)
	assert.Equal(t, before.LastSuccessAt, failed.LastSuccessAt)

	e.drain(t, conn)
	assert.Equal(t, e.manifest(t, conn), unique(e.accepted(t, "task")), "collection goes on from the same cursor")
}

func TestSync_AuthStopsCollection(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Faults: []testadapter.Fault{{Operation: testadapter.OpFetch, Kind: testadapter.FaultAuth}}})
	conn := e.connection(t, "10000")
	require.NoError(t, e.run(t, e.next(t, conn), false))
	st := e.state(t, conn)
	assert.Equal(t, entity.SyncStopped, st.Phase)
	assert.Equal(t, "auth", st.LastErrorCode)
	assert.Nil(t, e.next(t, conn), "no repeat after 401")
	assert.Equal(t, int64(1), e.adapter.hits.Load())
}

func TestSync_UnavailableSourceIsRepeatedWithBackoff(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Faults: []testadapter.Fault{{Operation: testadapter.OpFetch, Kind: testadapter.FaultUnavailable}}})
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	moments := map[float64][]time.Duration{}
	for _, jitter := range []float64{0.1, 0.9} {
		e.jitter.Store(jitter)
		conn := e.connection(t, "P-"+strconv.FormatFloat(jitter, 'f', 1, 64))
		for i, base := range want {
			q := e.next(t, conn)
			if wait := q.scheduledAt.Sub(e.clock.Now()); wait > 0 {
				e.clock.Advance(wait)
			}
			require.NoError(t, e.run(t, q, false))
			pause := e.next(t, conn).scheduledAt.Sub(e.clock.Now())
			assert.GreaterOrEqual(t, pause, base, "pause %d", i)
			assert.LessOrEqual(t, pause, base+base/2, "pause %d", i)
			moments[jitter] = append(moments[jitter], pause)
		}
		assert.Equal(t, "unavailable", e.state(t, conn).LastErrorCode)
	}
	assert.NotEqual(t, moments[0.1], moments[0.9], "connections do not repeat together")
}

func TestSync_OneRunPerConnection(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Page: 1, Attempts: []int{1}, Kind: testadapter.FaultHang, DelayMS: 500},
	}})
	conn := e.connection(t, "10000")
	q := e.next(t, conn)
	var inflight, peak atomic.Int64
	e.adapter.Config.Handler = countingInflight(e.adapter.Config.Handler, &inflight, &peak)
	errs := make([]error, 2)
	var wg gosync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = e.run(t, q, false) })
		time.Sleep(50 * time.Millisecond)
	}
	wg.Wait()
	var retry *entity.RetryLaterError
	assert.True(t, errors.As(errs[0], &retry) || errors.As(errs[1], &retry), "the second run waits for the lease: %v", errs)
	assert.Equal(t, int64(1), peak.Load(), "the adapter is never called twice at once for one connection")

	_, err := e.owner.Exec(t.Context(), "INSERT INTO sync.lease (organization_id, connection_id, holder, lease_until) VALUES ($1, $2, 'dead-process', $3) ON CONFLICT (organization_id, connection_id) DO UPDATE SET holder = 'dead-process', lease_until = EXCLUDED.lease_until",
		uuid.UUID(e.org), uuid.UUID(conn), e.clock.Now().Add(120*time.Second))
	require.NoError(t, err)
	require.ErrorAs(t, e.run(t, e.next(t, conn), false), &retry, "a lease of a dead process holds")
	e.clock.Advance(121 * time.Second)
	require.NoError(t, e.run(t, e.next(t, conn), false), "it runs out by itself")
	assert.LessOrEqual(t, _rescueAfter+_jobTimeout, 5*time.Minute, "a job of a dead process comes back within the 300 s of NFR-009")
}

// countingInflight counts the calls of the contract served at once and keeps the peak.
func countingInflight(next http.Handler, inflight, peak *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

func TestSync_WorkerCompletesJobsInTheTransactionOfTheirResult(t *testing.T) {
	t.Parallel()
	e := newSyncEnv(t, testadapter.Config{})
	workers := river.NewWorkers()
	workerctrl.Register(workers, workerctrl.Deps{Collection: e.uc, Normalizer: e.norm})
	client, err := river.NewClient(riverpgxv5.New(e.app), &river.Config{
		Queues: map[string]river.QueueConfig{entity.QueueBulk: {MaxWorkers: 2}}, Workers: workers,
		JobTimeout: _jobTimeout, RescueStuckJobsAfter: _rescueAfter, FetchCooldown: 20 * time.Millisecond, FetchPollInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(context.WithoutCancel(t.Context())))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(ctx)
	})
	conn := e.connection(t, "10000")

	require.Eventually(t, func() bool { return e.state(t, conn).Phase == entity.SyncIncremental }, 30*time.Second, 100*time.Millisecond)
	require.Eventually(t, func() bool {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind IN ('sync.page', 'sync.history') AND state <> 'completed'") == 0
	}, 30*time.Second, 100*time.Millisecond, "every job of collection is completed")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.reconcile'"))
	assert.Equal(t, e.manifest(t, conn), unique(e.accepted(t, "task")))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM sync.lease"), "the lease is released with the last page")
	require.Eventually(t, func() bool {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'normalize.batch' AND state <> 'completed'") == 0
	}, 30*time.Second, 100*time.Millisecond, "every job of normalization is completed")
	assert.Equal(t, len(unique(e.accepted(t, "task"))), count(t, t.Context(), e.owner,
		"SELECT count(*) FROM normalize.task_version WHERE superseded_seq IS NULL"), "every collected task has its current version")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM normalize.task_event WHERE observed_via NOT IN ('backfill', 'incremental')"))
}
