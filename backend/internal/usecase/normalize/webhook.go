package normalize

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) WebhookTargets(ctx context.Context, org, instance entity.ID, keys []entity.ObjectKey) (map[entity.ObjectKey]entity.WebhookMRTarget, error) {
	var out map[entity.ObjectKey]entity.WebhookMRTarget
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.WebhookTargets(ctx, org, instance, keys)
		return err
	})
	return out, err
}

func (u *UseCase) CurrentSyncVersions(ctx context.Context, org, area entity.ID, nativeIDs []string) (map[entity.ObjectKey]entity.SyncObjectVersion, error) {
	var out map[entity.ObjectKey]entity.SyncObjectVersion
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CurrentSyncVersions(ctx, org, area, nativeIDs)
		return err
	})
	return out, err
}
