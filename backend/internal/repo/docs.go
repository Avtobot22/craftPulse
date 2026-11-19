package repo

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type DocumentationRepo interface {
	LockRootWrites(ctx context.Context, orgID entity.ID) error
	LatestRootSet(ctx context.Context, orgID, serviceID entity.ID) (entity.DocumentationRootSet, error)
	RootSetByConfirmation(ctx context.Context, orgID, key entity.ID) (entity.DocumentationRootSet, entity.ID, error)
	RememberRootConfirmation(ctx context.Context, orgID, key, actor entity.ID, set entity.DocumentationRootSet, at time.Time) error
	RootsAt(ctx context.Context, orgID, serviceID entity.ID, seq int64) ([]entity.DocumentationRoot, error)
	CurrentAreaRoots(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.DocumentationRoot, error)
	InsertRootSet(ctx context.Context, orgID entity.ID, set entity.DocumentationRootSet) error
	SupersedeRoots(ctx context.Context, orgID, serviceID entity.ID, seq int64, at time.Time) error
}
