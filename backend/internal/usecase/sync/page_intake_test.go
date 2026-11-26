package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
)

type pageIntake struct {
	Ingest
	committed []entity.Receipt
}

func (*pageIntake) RecordPage(_ context.Context, page *entity.PreparedPackage) (entity.Receipt, error) {
	return page.Receipt, nil
}

func (i *pageIntake) PageCommitted(rc entity.Receipt) {
	i.committed = append(i.committed, rc)
}

type pageTx struct {
	repo.Transactor
	run func(context.Context, func(context.Context) error) error
}

func (tx pageTx) InOrgTx(ctx context.Context, _ entity.ID, work func(context.Context) error) error {
	return tx.run(ctx, work)
}

func TestPageIntakeCountsOnlyCommittedAttempt(t *testing.T) {
	t.Parallel()
	rollback := errors.New("transaction rolled back")
	for _, retry := range []bool{false, true} {
		intake := &pageIntake{}
		tx := pageTx{run: func(ctx context.Context, work func(context.Context) error) error {
			if err := work(ctx); err != nil {
				return err
			}
			assert.Empty(t, intake.committed)
			if retry {
				return work(ctx)
			}
			return rollback
		}}
		u := New(Deps{Ingest: intake, Tx: tx})
		page := &entity.PreparedPackage{Receipt: entity.Receipt{Channel: entity.ChannelSync, Status: entity.ReceiptRejected}}
		err := u.withPageIntake(t.Context(), entity.ID{}, func(_ context.Context, record pageRecorder) error {
			_, err := record(page)
			return err
		})
		if retry {
			require.NoError(t, err)
			assert.Equal(t, []entity.Receipt{page.Receipt}, intake.committed)
		} else {
			require.ErrorIs(t, err, rollback)
			assert.Empty(t, intake.committed)
		}
	}
}
