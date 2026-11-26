package change

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// TechnicalSummaries loads revisions on the report cut in descending revision order for each MR.
// Older statuses are mutable, so the selection at a point is determined by head and build time.
func (r *Repo) TechnicalSummaries(ctx context.Context, org entity.ID, mrs []entity.ID, seq int64, at time.Time) ([]entity.TechnicalSummaryRevision, error) {
	ids := make([]uuid.UUID, len(mrs))
	for i, id := range mrs {
		ids[i] = uuid.UUID(id)
	}
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		SELECT s.mr_id, p.project_id, s.head_sha, s.built_at, c.calculation->'technical',
		       (ci.value->>'pipeline_id')::uuid,
		       COALESCE((ci.value->'metrics' @? '$[*] ? (@.MetricID == "I08" && @.Numerator > 0)'), false),
		       COALESCE((ci.value->'public_checks'->>'Numerator')::numeric > 0, false)
		FROM change.summary_revision s
		JOIN change.profile_version p ON (p.organization_id,p.id) = (s.organization_id,s.profile_version_id)
		JOIN change.summary_input c ON (c.organization_id,c.summary_revision_id) = (s.organization_id,s.id) AND c.input_kind = 'calc_revision'
		LEFT JOIN change.summary_item ci ON (ci.organization_id,ci.summary_revision_id) = (s.organization_id,s.id) AND ci.item_kind = 'ci_attempts'
		WHERE s.organization_id = $1 AND s.mr_id = ANY($2::uuid[]) AND s.knowledge_seq <= $3 AND s.built_at <= $4
		ORDER BY s.mr_id, s.revision_no DESC`, uuid.UUID(org), ids, seq, at)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.TechnicalSummaryRevision, error) {
		var v entity.TechnicalSummaryRevision
		var technical *entity.TechnicalSummary
		var run *uuid.UUID
		err := row.Scan((*uuid.UUID)(&v.MRID), (*uuid.UUID)(&v.ProjectID), &v.HeadSHA, &v.BuiltAt, &technical,
			&run, &v.MissingChecks, &v.PublicMissingChecks)
		if technical != nil {
			v.TechnicalSummary = *technical
		}
		if run != nil {
			v.PipelineID = entity.ID(*run)
		}
		return v, err
	})
}
