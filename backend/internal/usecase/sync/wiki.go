package sync

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// WikiCollectionAt gives calc the per-space manifest result and the last complete synchronization.
func (u *UseCase) WikiCollectionAt(ctx context.Context, org, area entity.ID, seq int64, asOf time.Time) (entity.WikiCollection, error) {
	sources, err := u.d.Connect.AreaSources(ctx, org, []entity.ID{area})
	if err != nil {
		return entity.WikiCollection{}, err
	}
	var scopes []entity.ID
	for i := range sources {
		src := &sources[i]
		if src.Active && src.Kind == entity.ScopeWikiSpace {
			scopes = append(scopes, src.ScopeID)
		}
	}
	var result entity.WikiCollection
	err = u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if len(scopes) > 0 {
			result, err = u.d.Repo.ReadWikiCollection(ctx, org, scopes, seq, asOf)
			if err != nil {
				return err
			}
		}
		result.EarliestVersion, err = u.d.Normalize.WikiVersionBoundary(ctx, org, area, seq, asOf)
		return err
	})
	return result, err
}
