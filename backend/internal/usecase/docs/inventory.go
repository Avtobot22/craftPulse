package docs

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Documents is the normalized inventory of a Git commit in a repository.
type Documents interface {
	DocumentsAt(ctx context.Context, org, area entity.ID, prefix, sha string, seq int64) ([]entity.DocumentVersion, error)
}

// InventoryAt lists the documents of a permitted, confirmed root at the given knowledge cutoff.
func (u *UseCase) InventoryAt(ctx context.Context, org entity.ID, access entity.Scope, scope entity.DocumentationScope, seq int64) (*entity.DocumentationInventory, error) {
	if !slices.Contains(access.Services(), scope.ServiceID) || !access.Area(scope.RepositoryID) {
		return nil, entity.ErrNotFound
	}
	return u.CalculationInventoryAt(ctx, org, scope, seq)
}

// CalculationInventoryAt is the internal projection calc uses. A missing or superseded root returns nil.
func (u *UseCase) CalculationInventoryAt(ctx context.Context, org entity.ID, scope entity.DocumentationScope, seq int64) (*entity.DocumentationInventory, error) {
	var out *entity.DocumentationInventory
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		roots, err := u.d.Repo.RootsAt(ctx, org, scope.ServiceID, seq)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(roots, func(r entity.DocumentationRoot) bool {
			return r.ID == scope.RootID && r.AreaID == scope.RepositoryID
		})
		if i < 0 {
			return nil
		}
		out = &entity.DocumentationInventory{Root: roots[i]}
		out.Documents, err = u.d.Documents.DocumentsAt(ctx, org, scope.RepositoryID, roots[i].PathPrefix, scope.SHA, seq)
		return err
	})
	return out, err
}
