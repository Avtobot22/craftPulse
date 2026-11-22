package normalize

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) CodeReportAt(ctx context.Context, org entity.ID, scope entity.CodeScope, seq int64) (*entity.CodeReport, error) {
	var out *entity.CodeReport
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CodeReportAt(ctx, org, scope, seq)
		return err
	})
	return out, err
}

func (u *UseCase) ReceiptCodeScopes(ctx context.Context, org, receipt entity.ID) ([]entity.CodeScope, error) {
	var out []entity.CodeScope
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReceiptCodeScopes(ctx, org, receipt)
		return err
	})
	return out, err
}
