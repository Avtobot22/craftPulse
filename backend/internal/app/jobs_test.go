package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type testArgs struct {
	Name string        `json:"name"`
	Hold time.Duration `json:"hold"`
}

func (testArgs) Kind() string { return "test" }

type done struct {
	name, queue string
	err         error
}

type testWorker struct {
	river.WorkerDefaults[testArgs]
	done chan done
}

func (w *testWorker) Work(ctx context.Context, job *river.Job[testArgs]) error {
	select {
	case <-time.After(job.Args.Hold):
		w.done <- done{name: job.Args.Name, queue: job.Queue}
		return nil
	case <-ctx.Done():
		w.done <- done{name: job.Args.Name, queue: job.Queue, err: ctx.Err()}
		return ctx.Err()
	}
}

type jobsFixture struct {
	pool   *pgxpool.Pool
	queue  *queue.Queue
	done   chan done
	cancel context.CancelFunc
	result chan error
}

func startJobs(t *testing.T, wd *watchdog, traced bool, stopTimeout time.Duration) jobsFixture {
	t.Helper()
	db := newDB(t)
	db.migrate(t)
	var opts []pg.Option
	if traced {
		opts = append(opts, pg.Tracer(wd))
	}
	pool := db.pool(t, "craftpulse_app", opts...)
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)

	f := jobsFixture{pool: pool, queue: queue.New(insertOnly), done: make(chan done, 8), result: make(chan error, 1)}
	workers := river.NewWorkers()
	river.AddWorker(workers, &testWorker{done: f.done})
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	t.Cleanup(cancel)
	go func() {
		f.result <- runJobs(ctx, jobsConfig{
			log: zerolog.Nop(), pool: pool, workers: workers,
			ready: databaseProbes(pool).Ready, watchdog: wd, stopTimeout: stopTimeout,
		})
	}()
	return f
}

func (f jobsFixture) enqueue(t *testing.T, job testArgs, queueName string, commit bool) {
	t.Helper()
	errRollback := errors.New("rollback")
	err := pg.InTx(t.Context(), f.pool, func(ctx context.Context) error {
		if err := f.queue.Enqueue(ctx, job, queueName); err != nil {
			return err
		}
		if !commit {
			return errRollback
		}
		return nil
	})
	if commit {
		require.NoError(t, err)
	}
}

func TestJobs_OnlyCommittedJobRunsInItsQueue(t *testing.T) {
	f := startJobs(t, newWatchdog(time.Minute), true, time.Second)

	f.enqueue(t, testArgs{Name: "rolled back"}, entity.QueueInteractive, false)
	f.enqueue(t, testArgs{Name: "committed"}, entity.QueueBulk, true)

	select {
	case d := <-f.done:
		assert.Equal(t, done{name: "committed", queue: entity.QueueBulk}, d)
	case <-time.After(10 * time.Second):
		t.Fatal("committed job did not run")
	}
	select {
	case d := <-f.done:
		t.Fatalf("unexpected job %q", d.name)
	case <-time.After(2 * time.Second):
	}
}

func TestQueue_RequiresTransaction(t *testing.T) {
	f := startJobs(t, newWatchdog(time.Minute), true, time.Second)
	assert.Error(t, f.queue.Enqueue(t.Context(), testArgs{}, entity.QueueBulk))
}

func TestJobs_StopFinishesCurrentJobThenCancelsOverdue(t *testing.T) {
	f := startJobs(t, newWatchdog(time.Minute), true, 2*time.Second)
	f.enqueue(t, testArgs{Name: "short", Hold: 500 * time.Millisecond}, entity.QueueInteractive, true)
	f.enqueue(t, testArgs{Name: "long", Hold: time.Minute}, entity.QueueBulk, true)
	time.Sleep(300 * time.Millisecond)

	start := time.Now()
	f.cancel()
	select {
	case err := <-f.result:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not stop")
	}
	assert.Less(t, time.Since(start), 2*time.Second+_cancelTimeout)

	got := map[string]error{}
	for range 2 {
		d := <-f.done
		got[d.name] = d.err
	}
	assert.NoError(t, got["short"], "the current job finishes")
	assert.ErrorIs(t, got["long"], context.Canceled, "a job over the stop timeout is cancelled")
}

func TestJobs_WatchdogSeesRiverFetchLoop(t *testing.T) {
	wd := newWatchdog(1500 * time.Millisecond)
	f := startJobs(t, wd, true, time.Second)

	select {
	case err := <-f.result:
		t.Fatalf("worker stopped: %v", err)
	case <-time.After(4 * time.Second):
	}
	assert.NoError(t, wd.Check(t.Context()))
}

func TestJobs_WatchdogStopsStalledFetchLoop(t *testing.T) {
	wd := newWatchdog(1500 * time.Millisecond)
	f := startJobs(t, wd, false, time.Second)

	select {
	case err := <-f.result:
		assert.ErrorIs(t, err, errStalled)
		assert.ErrorIs(t, wd.Check(t.Context()), errStalled, "/livez reports the stall")
	case <-time.After(10 * time.Second):
		t.Fatal("stalled loop was not detected")
	}
}

func TestWaitReady_DoesNotStartWhileUnready(t *testing.T) {
	ready := &health.Readiness{}
	ready.Register(func(context.Context) error { return errors.New("schema is behind") })
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	assert.False(t, waitReady(ctx, ready))
}
