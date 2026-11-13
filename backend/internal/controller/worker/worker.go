package worker

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const _minSnooze = time.Second

type Deps struct {
	Probes   usecase.ProbeRunner
	Packages usecase.PackageProcessor
}

// Register adds the handlers of the job kinds; the count matters: River refuses to fetch jobs without a
// registered kind.
func Register(w *river.Workers, d Deps) int {
	river.AddWorker(w, &probeWorker{runner: d.Probes})
	river.AddWorker(w, &packageWorker{packages: d.Packages})
	return 2
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
