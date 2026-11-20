package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) AreaProvider(ctx context.Context, orgID, areaID entity.ID) (entity.Provider, error) {
	p, err := r.q(ctx).AreaProvider(ctx, AreaProviderParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID)})
	if err != nil {
		return "", fmt.Errorf("normalize: area provider: %w", err)
	}
	return entity.Provider(p), nil
}
