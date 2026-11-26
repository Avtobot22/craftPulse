package app

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
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
	Name string `json:"name"`
}

func (testArgs) Kind() string { return "test" }

type done struct {
	name, queue string
	err         error
}

type testWorker struct {
	river.WorkerDefaults[testArgs]
	started chan string
	done    chan done
	hold    map[string]<-chan struct{}
}

func (w *testWorker) Work(ctx context.Context, job *river.Job[testArgs]) error {
	w.started <- job.Args.Name
	if release, held := w.hold[job.Args.Name]; held {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	err := ctx.Err()
	w.done <- done{name: job.Args.Name, queue: job.Queue, err: err}
	return err
}

type jobsFixture struct {
	pool    *pgxpool.Pool
	queue   *queue.Queue
	started chan string
	done    chan done
	cancel  context.CancelFunc
	stopped chan struct{}
	err     error
}

func startJobs(t *testing.T, wd *watchdog, traced bool, stopTimeout time.Duration, hold map[string]<-chan struct{}) *jobsFixture {
	t.Helper()
	db := newMigratedDB(t)
	var opts []pg.Option
	if traced {
		opts = append(opts, pg.Tracer(wd))
	}
	pool := db.pool(t, "craftpulse_app", opts...)
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	f := &jobsFixture{
		pool: pool, queue: queue.New(insertOnly), started: make(chan string, 8), done: make(chan done, 8),
		cancel: cancel, stopped: make(chan struct{}),
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &testWorker{started: f.started, done: f.done, hold: hold})
	t.Cleanup(func() {
		cancel()
		f.wait(t)
	})
	go func() {
		defer close(f.stopped)
		f.err = runJobs(ctx, jobsConfig{
			log: zerolog.Nop(), pool: pool, workers: workers,
			ready: databaseProbes(pool).Ready, watchdog: wd, stopTimeout: stopTimeout,
		})
	}()
	return f
}

func (f *jobsFixture) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-f.stopped:
		return f.err
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not stop")
		return nil
	}
}

func (f *jobsFixture) enqueue(t *testing.T, job testArgs, queueName string, commit bool) {
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
	} else {
		require.ErrorIs(t, err, errRollback)
	}
}

func TestJobs_OnlyCommittedJobRunsInItsQueue(t *testing.T) {
	t.Parallel()
	f := startJobs(t, newWatchdog(time.Minute), true, time.Second, nil)

	f.enqueue(t, testArgs{Name: "rolled back"}, entity.QueueInteractive, false)
	f.enqueue(t, testArgs{Name: "committed"}, entity.QueueBulk, true)

	select {
	case d := <-f.done:
		assert.Equal(t, done{name: "committed", queue: entity.QueueBulk}, d)
	case <-time.After(10 * time.Second):
		t.Fatal("committed job did not run")
	}
	f.cancel()
	require.NoError(t, f.wait(t))
	assert.Zero(t, count(t, t.Context(), f.pool, `SELECT count(*) FROM river_job WHERE kind = 'test' AND args->>'name' = 'rolled back'`))
	select {
	case d := <-f.done:
		t.Fatalf("unexpected job %q", d.name)
	default:
	}
}

func TestQueue_RequiresTransaction(t *testing.T) {
	t.Parallel()
	assert.ErrorContains(t, queue.New(nil).Enqueue(t.Context(), testArgs{}, entity.QueueBulk), "transaction")
}

func TestJobs_StopFinishesCurrentJobThenCancelsOverdue(t *testing.T) {
	t.Parallel()
	const stopTimeout = time.Second
	wd := newWatchdog(time.Minute)
	finish := make(chan struct{})
	f := startJobs(t, wd, true, stopTimeout, map[string]<-chan struct{}{"short": finish, "long": nil})
	f.enqueue(t, testArgs{Name: "short"}, entity.QueueInteractive, true)
	f.enqueue(t, testArgs{Name: "long"}, entity.QueueBulk, true)
	var started []string
	deadline := time.After(10 * time.Second)
	for range 2 {
		select {
		case name := <-f.started:
			started = append(started, name)
		case <-deadline:
			t.Fatal("both jobs did not start")
		}
	}
	require.ElementsMatch(t, []string{"short", "long"}, started)

	start := time.Now()
	f.cancel()
	require.Eventually(t, func() bool { return !wd.armed.Load() }, stopTimeout, 10*time.Millisecond, "the worker begins graceful shutdown")
	close(finish)
	require.NoError(t, f.wait(t))
	assert.Less(t, time.Since(start), stopTimeout+_cancelTimeout)

	got := map[string]error{}
	for range 2 {
		select {
		case d := <-f.done:
			got[d.name] = d.err
		default:
			t.Fatal("worker stopped before both jobs finished")
		}
	}
	assert.Contains(t, got, "short")
	assert.NoError(t, got["short"], "the current job finishes")
	assert.ErrorIs(t, got["long"], context.Canceled, "a job over the stop timeout is cancelled")
}

func TestJobs_WatchdogSeesRiverFetchLoop(t *testing.T) {
	t.Parallel()
	wd := newWatchdog(1500 * time.Millisecond)
	f := startJobs(t, wd, true, time.Second, nil)

	require.Eventually(t, wd.armed.Load, 10*time.Second, 10*time.Millisecond)
	first := wd.last.Load()
	require.Eventually(t, func() bool {
		return time.Duration(wd.last.Load()-first) > wd.timeout
	}, 10*time.Second, 10*time.Millisecond, "fetches continue to refresh the watchdog beyond its timeout")
	select {
	case <-f.stopped:
		t.Fatalf("worker stopped: %v", f.err)
	default:
	}
	assert.NoError(t, wd.Check(t.Context()))
}

func TestJobs_WatchdogStopsStalledFetchLoop(t *testing.T) {
	t.Parallel()
	wd := newWatchdog(250 * time.Millisecond)
	f := startJobs(t, wd, false, time.Second, nil)

	assert.ErrorIs(t, f.wait(t), errStalled)
	assert.ErrorIs(t, wd.Check(t.Context()), errStalled, "/livez reports the stall")
}

func TestWaitReady_DoesNotStartWhileUnready(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ready := &health.Readiness{}
		ready.Register(func(context.Context) error { return errors.New("schema is behind") })
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		assert.False(t, waitReady(ctx, ready))
		require.NoError(t, runJobs(ctx, jobsConfig{ready: ready}))
	})
}

type failingJobStopper struct{ err error }

func (failingJobStopper) Stop(context.Context) error { return context.DeadlineExceeded }

func (f failingJobStopper) StopAndCancel(context.Context) error { return f.err }

func TestStopJobs_ReportsCancellationError(t *testing.T) {
	t.Parallel()
	stopErr := errors.New("river could not cancel running jobs")
	err := stopJobs(t.Context(), jobsConfig{log: zerolog.Nop(), stopTimeout: time.Second}, failingJobStopper{err: stopErr})
	require.ErrorIs(t, err, stopErr)
}
