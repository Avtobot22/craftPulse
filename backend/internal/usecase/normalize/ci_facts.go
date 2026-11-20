package normalize

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CIFacts reads each run and its own attempts once for calc, including restricted objects.
func (u *UseCase) CIFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.CIFacts, error) {
	out := entity.CIFacts{AreaID: areaID}
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		areas := []entity.ID{areaID}
		restricted, err := u.d.Repo.RestrictedObjects(ctx, orgID, areas)
		if err != nil {
			return err
		}
		allowed := make([]entity.ID, len(restricted))
		for i := range restricted {
			allowed[i] = restricted[i].ID
		}
		if out.Provider, err = u.d.Repo.AreaProvider(ctx, orgID, areaID); err != nil {
			return err
		}
		ids, err := u.d.Repo.FindCIRunIDs(ctx, orgID, areas, allowed, entity.RunsQuery{}, seq)
		if err != nil {
			return err
		}
		if out.Runs, err = u.d.Repo.ReadCIRunVersions(ctx, orgID, areas, allowed, ids, seq); err != nil {
			return err
		}
		out.Attempts, err = u.d.Repo.ReadCIAttemptVersions(ctx, orgID, areas, allowed, ids, seq)
		return err
	})
	return out, err
}
