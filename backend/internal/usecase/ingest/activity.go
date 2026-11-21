package ingest

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) ActivityReceiptConnections(ctx context.Context, org entity.ID, ids []entity.ID) (map[entity.ID]entity.ID, error) {
	var out map[entity.ID]entity.ID
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ActivityReceiptConnections(ctx, org, ids)
		return err
	})
	return out, err
}

func (u *UseCase) ActivityTruncations(ctx context.Context, org entity.ID, scopes []entity.ID, at time.Time) ([]entity.ActivityHistoryNotice, error) {
	var out []entity.ActivityHistoryNotice
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ActivityTruncations(ctx, org, scopes, at)
		return err
	})
	return out, err
}

func (u *UseCase) ActivityCompleteReceipts(ctx context.Context, org entity.ID, receipts []entity.ID) (map[entity.ID]bool, error) {
	var out map[entity.ID]bool
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ActivityCompleteReceipts(ctx, org, receipts)
		return err
	})
	return out, err
}
