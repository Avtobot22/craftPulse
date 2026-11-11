package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

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
	tx, ok := postgres.TxFrom(ctx)
	if !ok {
		return errNoTx
	}
	if _, err := q.client.InsertTx(ctx, tx, job, &river.InsertOpts{Queue: queue}); err != nil {
		return fmt.Errorf("queue: insert %s: %w", job.Kind(), err)
	}
	return nil
}
