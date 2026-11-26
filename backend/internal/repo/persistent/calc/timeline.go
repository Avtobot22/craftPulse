package calc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) TimelinePublications(ctx context.Context, org entity.ID, teams []entity.ID, from, observedBefore time.Time) ([]entity.TimelinePublication, error) {
	teamIDs := make([]string, len(teams))
	for i, id := range teams {
		teamIDs[i] = id.String()
	}
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
SELECT (s.scope->>'team_id')::uuid, r.as_of, r.published_at, r.knowledge_seq
FROM calc.series s
JOIN calc.revision r ON r.organization_id = s.organization_id AND r.series_id = s.id
WHERE s.organization_id = $1 AND s.scope->>'kind' = 'team' AND s.scope->>'team_id' = ANY($2::text[])
  AND r.as_of >= $3 AND r.published_at < $4`, uuid.UUID(org), teamIDs, from, observedBefore)
	if err != nil {
		return nil, fmt.Errorf("calc: read timeline publications: %w", err)
	}
	defer rows.Close()
	var out []entity.TimelinePublication
	for rows.Next() {
		var item entity.TimelinePublication
		var team uuid.UUID
		if err := rows.Scan(&team, &item.AsOf, &item.PublishedAt, &item.Knowledge); err != nil {
			return nil, fmt.Errorf("calc: scan timeline publication: %w", err)
		}
		item.TeamID = entity.ID(team)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("calc: read timeline publications: %w", err)
	}
	return out, nil
}
