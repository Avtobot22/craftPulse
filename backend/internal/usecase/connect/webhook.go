package connect

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) WebhookRoute(ctx context.Context, id entity.ID) (entity.WebhookRoute, error) {
	return u.d.Repo.WebhookRoute(ctx, id)
}

func (u *UseCase) ParseWebhook(ctx context.Context, org, conn entity.ID, in entity.WebhookInput) (entity.WebhookResult, error) {
	var out entity.WebhookResult
	err := u.call(ctx, org, conn, entity.OpParseWebhook, func(ctx context.Context, t entity.AdapterTarget) error {
		var err error
		out, err = u.d.Adapters.ParseWebhook(ctx, t, in)
		return err
	})
	return out, err
}
