package connect

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// AreaSources returns every scope that collected the areas, also removed ones, the newest of an area first.
func (r *Repo) AreaSources(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaSource, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).AreaSources(ctx, AreaSourcesParams{OrganizationID: uuid.UUID(orgID), AreaIds: uuids(areaIDs)})
	if err != nil {
		return nil, fmt.Errorf("connect: area sources: %w", err)
	}
	out := make([]entity.AreaSource, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.AreaSource{
			AreaID: entity.ID(row.AreaID), ConnectionID: entity.ID(row.ConnectionID), InstanceID: entity.ID(row.InstanceID),
			Kind: entity.ScopeKind(row.ScopeKind), NativeScopeID: row.NativeScopeID, Active: row.ScopeStatus == "active",
			ConnectionStatus: entity.ConnectionStatus(row.ConnectionStatus), PausedUntil: timePtr(row.PausedUntil),
		}
	}
	return out, nil
}

// PageSourceAreas reads one item more than the limit, so the caller knows whether a next page exists.
func (r *Repo) PageSourceAreas(ctx context.Context, orgID entity.ID, f entity.SourceAreaFilter) ([]entity.SourceArea, error) {
	if f.Limit < 1 || f.Limit >= 1<<30 {
		return nil, fmt.Errorf("connect: page limit %d is out of range", f.Limit)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageSourceAreas(ctx, PageSourceAreasParams{
		OrganizationID: uuid.UUID(orgID),
		ScopeKind:      pgtype.Text{String: string(f.Kind), Valid: f.Kind != ""},
		ConnectionID:   uuid.NullUUID{UUID: uuid.UUID(f.ConnectionID), Valid: !f.ConnectionID.IsZero()},
		After:          uuid.NullUUID{UUID: uuid.UUID(f.After), Valid: !f.After.IsZero()},
		RowLimit:       int32(f.Limit + 1), //nolint:gosec // bounded by the check above
	})
	if err != nil {
		return nil, fmt.Errorf("connect: page source areas: %w", err)
	}
	out := make([]entity.SourceArea, len(rows))
	for i, row := range rows {
		out[i] = entity.SourceArea{
			ID: entity.ID(row.AreaID), Kind: entity.ScopeKind(row.ScopeKind), DisplayName: row.DisplayName,
			ConnectionID: entity.ID(row.ConnectionID),
		}
	}
	return out, nil
}
