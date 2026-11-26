package calc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) ComponentCodeRevisions(ctx context.Context, org entity.ID, c entity.Component, profiles []string) ([]entity.ComponentCodeRevision, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		SELECT r.id, i.input_id::uuid, s.scope->>'format', s.scope->>'producer', s.scope->>'profile_id',
		       lower(r.window_range), upper(r.window_range)
		FROM calc.series s
		JOIN calc.revision r ON (r.organization_id, r.id) = (s.organization_id, s.current_revision_id)
		JOIN calc.revision_input i ON (i.organization_id, i.revision_id) = (r.organization_id, r.id)
		WHERE s.organization_id = $1 AND s.metric_set = 'code' AND r.status = 'published'
		  AND s.scope->>'repository_id' = $2 AND s.scope->>'component_id' = $3
		  AND s.scope->>'profile_id' = ANY($4::text[]) AND i.input_kind = 'code_report'
		ORDER BY r.knowledge_seq DESC, r.as_of DESC, r.id DESC`,
		uuid.UUID(org), c.RepositoryAreaID.String(), c.ID.String(), profiles)
	if err != nil {
		return nil, fmt.Errorf("calc: read component code revisions: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.ComponentCodeRevision, error) {
		var out entity.ComponentCodeRevision
		var revision, report uuid.UUID
		err := row.Scan(&revision, &report, &out.Format, &out.Producer, &out.ProfileID, &out.Window.From, &out.Window.To)
		out.ID, out.ReportID = entity.ID(revision), entity.ID(report)
		return out, err
	})
}
