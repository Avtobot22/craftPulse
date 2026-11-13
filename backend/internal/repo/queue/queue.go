package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var errNoTx = errors.New("queue: job must be enqueued inside a data transaction")

type Queue struct {
	client *river.Client[pgx.Tx]
}

func New(client *river.Client[pgx.Tx]) *Queue {
	return &Queue{client: client}
}

func (q *Queue) Enqueue(ctx context.Context, job entity.Job, queue string) error {
	return q.insert(ctx, job, &river.InsertOpts{Queue: queue})
}

func (q *Queue) EnqueueAt(ctx context.Context, job entity.Job, queue string, at time.Time) error {
	return q.insert(ctx, job, &river.InsertOpts{Queue: queue, ScheduledAt: at})
}

var _activeStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
	rivertype.JobStateRetryable, rivertype.JobStateScheduled,
}

func (q *Queue) insert(ctx context.Context, job entity.Job, opts *river.InsertOpts) error {
	tx, ok := postgres.TxFrom(ctx)
	if !ok {
		return errNoTx
	}
	if _, unique := job.(entity.ActiveUnique); unique {
		opts.UniqueOpts = river.UniqueOpts{ByArgs: true, ByState: _activeStates}
	}
	if _, err := q.client.InsertTx(ctx, tx, job, opts); err != nil {
		return fmt.Errorf("queue: insert %s: %w", job.Kind(), err)
	}
	return nil
}
