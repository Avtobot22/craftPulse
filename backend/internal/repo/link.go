package repo

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// LinkRepo keeps the versions of links (TZ 12.5.6); a read of one link gives entity.ErrNotFound without it.
type LinkRepo interface {
	// LockEdgeWrites serializes the changes of links of an organization within its transaction.
	LockEdgeWrites(ctx context.Context, orgID entity.ID) error
	Edge(ctx context.Context, orgID, id entity.ID) (entity.LinkEdge, error)
	CurrentEdge(ctx context.Context, orgID entity.ID, t entity.LinkType, from, to entity.ID, m entity.LinkMethod) (entity.LinkEdge, error)
	// CurrentConfirmedEdge prefers the explicit link of the source to the one a user confirmed.
	CurrentConfirmedEdge(ctx context.Context, orgID entity.ID, t entity.LinkType, from, to entity.ID) (entity.LinkEdge, error)
	EdgeByConfirmation(ctx context.Context, orgID, key entity.ID) (entity.LinkEdge, error)
	EdgesAt(ctx context.Context, orgID, objectID entity.ID, dir entity.LinkDirection, seq int64) ([]entity.LinkEdge, error)
	TaskMREdgesAt(ctx context.Context, orgID entity.ID, objectIDs []entity.ID, seq int64) ([]entity.LinkEdge, error)
	MREdgesAt(ctx context.Context, orgID entity.ID, mrIDs []entity.ID, seq int64) ([]entity.LinkEdge, error)
	InsertEdge(ctx context.Context, orgID entity.ID, e *entity.LinkEdge) error
	SupersedeEdge(ctx context.Context, orgID, id entity.ID, seq int64, at time.Time) error
}
