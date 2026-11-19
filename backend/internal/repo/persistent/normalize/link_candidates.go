package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) LinkMRVersions(ctx context.Context, org entity.ID, areas []entity.ID) ([]entity.MRVersion, error) {
	rows, err := r.q(ctx).LinkMRVersions(ctx, LinkMRVersionsParams{OrganizationID: uuid.UUID(org), Areas: ids(areas)})
	if err != nil {
		return nil, fmt.Errorf("normalize: link MR versions: %w", err)
	}
	return mapMRVersions(rows), nil
}

func (r *Repo) LinkTaskVersions(ctx context.Context, org entity.ID, areas []entity.ID, keys []string) ([]entity.TaskVersion, error) {
	rows, err := r.q(ctx).LinkTaskVersions(ctx, LinkTaskVersionsParams{OrganizationID: uuid.UUID(org), Areas: ids(areas), Keys: keys})
	if err != nil {
		return nil, fmt.Errorf("normalize: link task versions: %w", err)
	}
	return taskVersions(rows), nil
}
