package docs

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Documents is the normalized inventory of a repository or wiki space.
type Documents interface {
	WikiDocumentsAt(ctx context.Context, org, area entity.ID, prefix string, seq int64) ([]entity.DocumentVersion, error)
	DocumentsAt(ctx context.Context, org, area entity.ID, prefix, sha string, seq int64) ([]entity.DocumentVersion, error)
}

// InventoryAt lists the documents of a permitted, confirmed root at the given knowledge cutoff.
func (u *UseCase) InventoryAt(ctx context.Context, org entity.ID, access entity.Scope, scope entity.DocumentationScope, seq int64) (*entity.DocumentationInventory, error) {
	if !slices.Contains(access.Services(), scope.ServiceID) || !access.Area(scope.RepositoryID) {
		return nil, entity.ErrNotFound
	}
	out, err := u.CalculationInventoryAt(ctx, org, scope, seq)
	if err != nil || out == nil {
		return out, err
	}
	allowed := access.Objects()
	out.Documents = slices.DeleteFunc(out.Documents, func(v entity.DocumentVersion) bool {
		return v.Restricted && !slices.Contains(allowed, v.DocumentID)
	})
	return out, nil
}

// WikiInventoryAt asks the source about each restricted page before returning it.
func (u *UseCase) WikiInventoryAt(ctx context.Context, subject entity.Subject, access entity.Scope, scope entity.DocumentationScope, seq int64) (*entity.DocumentationInventory, []entity.AccessNote, error) {
	if !slices.Contains(access.Services(), scope.ServiceID) || !access.Area(scope.RepositoryID) {
		return nil, nil, entity.ErrNotFound
	}
	out, err := u.CalculationInventoryAt(ctx, subject.OrganizationID, scope, seq)
	if err != nil || out == nil || out.Root.ProfileID != entity.WikiInventoryProfile {
		return nil, nil, err
	}
	var restricted []entity.RestrictedObject
	for i := range out.Documents {
		doc := &out.Documents[i]
		if doc.Restricted {
			restricted = append(restricted, entity.RestrictedObject{
				ID: doc.DocumentID, AreaID: scope.RepositoryID, Kind: entity.ObjectDocument, NativeID: doc.PageNativeID,
			})
		}
	}
	if len(restricted) == 0 {
		return out, nil, nil
	}
	allowed, err := u.d.Access.Decide(ctx, subject, access, entity.SourceQuery{
		Operation: "listServiceDocuments", Areas: []entity.ID{scope.RepositoryID}, Objects: restricted,
	})
	if err != nil {
		return nil, nil, err
	}
	if !allowed.Area(scope.RepositoryID) {
		return nil, allowed.Notes(), nil
	}
	objects := allowed.Objects()
	out.Documents = slices.DeleteFunc(out.Documents, func(v entity.DocumentVersion) bool {
		return v.Restricted && !slices.Contains(objects, v.DocumentID)
	})
	return out, allowed.Notes(), nil
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
		if out.Root.ProfileID == entity.WikiInventoryProfile {
			out.Documents, err = u.d.Documents.WikiDocumentsAt(ctx, org, scope.RepositoryID, out.Root.PathPrefix, seq)
		} else {
			out.Documents, err = u.d.Documents.DocumentsAt(ctx, org, scope.RepositoryID, out.Root.PathPrefix, scope.SHA, seq)
		}
		return err
	})
	return out, err
}
