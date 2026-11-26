package change

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// ReadTaskSummaryRows reads observation evidence from the latest built revision per MR at the cut.
func (r *Repo) ReadTaskSummaryRows(ctx context.Context, org entity.ID, mrs []entity.MRVersion, seq int64, at time.Time) ([]entity.TaskSummaryRow, error) {
	ids, heads := make([]uuid.UUID, len(mrs)), make([]string, len(mrs))
	for i := range mrs {
		ids[i], heads[i] = uuid.UUID(mrs[i].MRID), mrs[i].HeadSHA
	}
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		SELECT s.mr_id, p.project_id, s.head_sha, s.built_at, s.pipeline_state, s.base_sha IS NOT NULL,
		       i.item_kind, i.finding_identity_id, i.occurrence_id, i.required_right, i.rank, i.component_id,
		       COALESCE(i.analysis_kind, ''), COALESCE(i.reason_code, ''), COALESCE(i.severity, ''),
		       COALESCE(f.rule_id, i.value->>'rule', ''), c.path_prefix
		FROM unnest($2::uuid[], $3::text[]) AS mr(id, head)
		CROSS JOIN LATERAL (
			SELECT id, organization_id, mr_id, profile_version_id, head_sha, base_sha, built_at, pipeline_state, status
			FROM change.summary_revision
			WHERE organization_id = $1 AND mr_id = mr.id AND knowledge_seq <= $4 AND built_at <= $5
			ORDER BY revision_no DESC LIMIT 1
		) s
		JOIN change.profile_version p ON (p.organization_id,p.id) = (s.organization_id,s.profile_version_id)
		JOIN change.summary_item i ON (i.organization_id,i.summary_revision_id) = (s.organization_id,s.id)
		LEFT JOIN change.finding_identity f ON (f.organization_id,f.id) = (i.organization_id,i.finding_identity_id)
		LEFT JOIN org.component c ON i.item_kind = 'not_evaluated'
			AND (c.organization_id,c.id) = (i.organization_id,i.component_id)
			AND c.known_seq <= $4 AND (c.superseded_seq IS NULL OR c.superseded_seq > $4)
		WHERE s.head_sha = mr.head AND s.status IN ('ready','superseded') AND i.item_kind IN ('not_evaluated','new')
		ORDER BY s.mr_id, i.item_no`, uuid.UUID(org), ids, heads, seq, at)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.TaskSummaryRow, error) {
		var v entity.TaskSummaryRow
		var finding, occurrence, component *uuid.UUID
		err := row.Scan((*uuid.UUID)(&v.MRID), (*uuid.UUID)(&v.ProjectID), &v.HeadSHA, &v.BuiltAt, &v.PipelineState, &v.HasBase,
			&v.Kind, &finding, &occurrence, &v.RequiredRight, &v.Rank, &component, &v.AnalysisKind, &v.ReasonCode, &v.Severity,
			&v.RuleID, &v.ComponentPath)
		v.FindingID, v.OccurrenceID, v.ComponentID = (*entity.ID)(finding), (*entity.ID)(occurrence), (*entity.ID)(component)
		return v, err
	})
}
