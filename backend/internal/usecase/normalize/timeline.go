package normalize

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) TimelineFacts(ctx context.Context, scope entity.Scope, org entity.ID, areas []entity.ID, q entity.TimelineFactQuery) ([]entity.TimelineFact, error) {
	areas = slices.DeleteFunc(slices.Clone(areas), func(id entity.ID) bool { return !scope.Area(id) })
	if len(areas) == 0 {
		return nil, nil
	}
	var out []entity.TimelineFact
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TimelineFacts(ctx, org, areas, scope.Objects(), q)
		return err
	})
	return out, err
}
