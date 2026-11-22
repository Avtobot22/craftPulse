package change

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// FindingKindsForException finds classes with at least one occurrence matching the requested scope.
func (r *Repo) FindingKindsForException(ctx context.Context, org entity.ID, s entity.ExceptionScope) ([]string, error) {
	id := func(v entity.ID) uuid.NullUUID { return uuid.NullUUID{UUID: uuid.UUID(v), Valid: !v.IsZero()} }
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
	out, err := r.q(ctx).ExceptionFindingKinds(ctx, ExceptionFindingKindsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(s.AreaID), FindingID: id(s.FindingID), RuleID: text(s.RuleID),
		ComponentID: id(s.ComponentID), PathPrefix: text(s.PathPrefix), PackagePurl: text(s.PackagePURL),
	})
	if err != nil {
		return nil, fmt.Errorf("change: find exception scope: %w", err)
	}
	return out, nil
}

// ProjectExceptions saves the effective decision versions of an area for change.
func (r *Repo) ProjectExceptions(ctx context.Context, org, area entity.ID, seq int64, at time.Time, versions []entity.ID) error {
	err := r.q(ctx).StoreExceptionProjection(ctx, StoreExceptionProjectionParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), KnownSeq: seq, AsOf: at, ActiveVersionIds: uuids(versions),
	})
	if err != nil {
		return fmt.Errorf("change: store exception projection: %w", err)
	}
	return nil
}
