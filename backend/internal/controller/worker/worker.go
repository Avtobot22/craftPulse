package worker

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _minSnooze = time.Second

type Deps struct {
	Probes     usecase.ProbeRunner
	Packages   usecase.PackageProcessor
	Collection usecase.Collector
	Normalizer usecase.Normalizer
}

// Register adds the handlers of the job kinds; the count matters: River refuses to fetch jobs without a
// registered kind.
func Register(w *river.Workers, d Deps) int {
	river.AddWorker(w, &probeWorker{runner: d.Probes})
	river.AddWorker(w, &packageWorker{packages: d.Packages})
	river.AddWorker(w, &pageWorker{collector: d.Collection})
	river.AddWorker(w, &historyWorker{collector: d.Collection})
	river.AddWorker(w, &normalizeWorker{normalizer: d.Normalizer})
	river.AddWorker(w, &reconcileWorker{collector: d.Collection})
	river.AddWorker(w, &dirtyWorker{collector: d.Collection})
	river.AddWorker(w, &tickWorker{collector: d.Collection})
	river.AddWorker(w, &scheduleWorker{collector: d.Collection})
	return 9
}

// _tickEvery plans collection every hour, so a data kind waits no more than 24 h for its increment.
const _tickEvery = time.Hour

// Periodic are the periodic jobs; River runs them on the leader only, so several workers do not repeat them.
func Periodic() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(_tickEvery), func() (river.JobArgs, *river.InsertOpts) {
			return entity.SyncTickJob{}, &river.InsertOpts{Queue: entity.QueueBulk}
		}, &river.PeriodicJobOpts{RunOnStart: true}),
	}
}

type reconcileWorker struct {
	river.WorkerDefaults[entity.SyncReconcileJob]
	collector usecase.Collector
}

func (w *reconcileWorker) Work(ctx context.Context, job *river.Job[entity.SyncReconcileJob]) error {
	return snooze(w.collector.RunReconcile(ctx, job.Args, holder(job.JobRow), completeIn(job)))
}

type dirtyWorker struct {
	river.WorkerDefaults[entity.SyncDirtyJob]
	collector usecase.Collector
}

func (w *dirtyWorker) Work(ctx context.Context, job *river.Job[entity.SyncDirtyJob]) error {
	return snooze(w.collector.RunDirty(ctx, job.Args, holder(job.JobRow), completeIn(job)))
}

type tickWorker struct {
	river.WorkerDefaults[entity.SyncTickJob]
	collector usecase.Collector
}

func (w *tickWorker) Work(ctx context.Context, _ *river.Job[entity.SyncTickJob]) error {
	return w.collector.Tick(ctx)
}

type scheduleWorker struct {
	river.WorkerDefaults[entity.SyncScheduleJob]
	collector usecase.Collector
}

func (w *scheduleWorker) Work(ctx context.Context, job *river.Job[entity.SyncScheduleJob]) error {
	return w.collector.Schedule(ctx, job.Args.OrganizationID)
}

type normalizeWorker struct {
	river.WorkerDefaults[entity.NormalizeBatchJob]
	normalizer usecase.Normalizer
}

func (w *normalizeWorker) Work(ctx context.Context, job *river.Job[entity.NormalizeBatchJob]) error {
	return w.normalizer.RunBatch(ctx, job.Args, completeIn(job))
}

type pageWorker struct {
	river.WorkerDefaults[entity.SyncPageJob]
	collector usecase.Collector
}

func (w *pageWorker) Work(ctx context.Context, job *river.Job[entity.SyncPageJob]) error {
	return snooze(w.collector.RunPage(ctx, job.Args, holder(job.JobRow), completeIn(job)))
}

type historyWorker struct {
	river.WorkerDefaults[entity.SyncHistoryJob]
	collector usecase.Collector
}

func (w *historyWorker) Work(ctx context.Context, job *river.Job[entity.SyncHistoryJob]) error {
	return snooze(w.collector.RunHistory(ctx, job.Args, holder(job.JobRow), completeIn(job)))
}

// holder names the lease after the job, so its own repeat takes the lease back at once.
func holder(row *rivertype.JobRow) string { return "river-job-" + strconv.FormatInt(row.ID, 10) }

// completeIn ends the job in the transaction of its result: a crash before the commit repeats the job,
// a crash after it finds the job done (TZ 10.5.1).
func completeIn[T river.JobArgs](job *river.Job[T]) func(context.Context) error {
	return func(ctx context.Context) error {
		tx, ok := postgres.TxFrom(ctx)
		if !ok {
			return errors.New("worker: the job is completed in the transaction of its result")
		}
		_, err := river.JobCompleteTx[*riverpgxv5.Driver](ctx, tx, job)
		return err
	}
}

type packageWorker struct {
	river.WorkerDefaults[entity.IngestPackageJob]
	packages usecase.PackageProcessor
}

func (w *packageWorker) Work(ctx context.Context, job *river.Job[entity.IngestPackageJob]) error {
	return w.packages.ProcessPackage(ctx, job.Args.OrganizationID, job.Args.ReceiptID)
}

type probeWorker struct {
	river.WorkerDefaults[entity.ConnectProbeJob]
	runner usecase.ProbeRunner
}

func (w *probeWorker) Work(ctx context.Context, job *river.Job[entity.ConnectProbeJob]) error {
	a := job.Args
	return snooze(w.runner.RunProbe(ctx, a.OrganizationID, a.ConnectionID, a.ProbeID))
}

// snooze waits out a rate limit or a preparing source without spending an attempt (TZ 10.6).
func snooze(err error) error {
	if rl, ok := errors.AsType[*entity.RetryLaterError](err); ok {
		return river.JobSnooze(max(time.Until(rl.At), _minSnooze))
	}
	return err
}
