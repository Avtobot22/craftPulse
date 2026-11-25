package connect

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) FindPublished(ctx context.Context, org, connection entity.ID, in entity.PublicationTarget) (string, error) {
	var id string
	err := u.call(ctx, org, connection, entity.OpFindPublished, func(ctx context.Context, target entity.AdapterTarget) error {
		var err error
		id, err = u.d.Adapters.FindPublished(ctx, target, in)
		return err
	})
	return id, err
}

func (u *UseCase) Publish(ctx context.Context, org, connection entity.ID, in entity.PublishInput) (entity.Publication, error) {
	var result entity.Publication
	err := u.call(ctx, org, connection, entity.OpPublish, func(ctx context.Context, target entity.AdapterTarget) error {
		var err error
		result, err = u.d.Adapters.Publish(ctx, target, in)
		return err
	})
	return result, err
}
