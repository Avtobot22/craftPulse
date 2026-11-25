package normalize

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// CIReceiptTarget resolves a receipt's coordinates against collected source objects.
func (r *Repo) CIReceiptTarget(ctx context.Context, org entity.ID, target *entity.CIResultCoordinates) (
	pipeline, mr *entity.ID, head string, err error,
) {
	var runID, mrID uuid.NullUUID
	area := uuid.NullUUID{UUID: uuid.UUID(target.AreaID), Valid: !target.AreaID.IsZero()}
	err = postgres.Conn(ctx, r.pool).QueryRow(ctx, `
		SELECT CASE WHEN $4 = '' AND $6::uuid IS NULL AND v.mr_id IS NULL THEN v.run_id END,
		       m.id, COALESCE(NULLIF($5, ''), v.sha, '')
		FROM normalize.source_object p
		LEFT JOIN normalize.ci_run_version v ON (v.organization_id, v.run_id) = (p.organization_id, p.id)
			AND v.superseded_seq IS NULL AND p.deleted_at IS NULL AND v.area_id = p.current_area_id
			AND ($3::uuid IS NULL OR v.area_id = $3) AND ($5 = '' OR v.sha = $5)
		LEFT JOIN normalize.source_object named ON named.organization_id = p.organization_id
			AND named.instance_id = p.instance_id AND named.object_kind = 'merge_request'
			AND named.native_ns = '' AND named.native_id = NULLIF($4, '')
		LEFT JOIN normalize.source_object m ON m.organization_id = p.organization_id
			AND m.id = COALESCE($6, CASE WHEN $4 <> '' THEN named.id ELSE v.mr_id END)
			AND m.object_kind = 'merge_request' AND m.deleted_at IS NULL AND m.current_area_id IS NOT NULL
			AND (m.current_area_id = COALESCE($3, v.area_id) OR ($3 IS NULL AND m.id = $6))
		WHERE (p.organization_id, p.id) = ($1, $2) AND p.object_kind = 'ci_run'`,
		uuid.UUID(org), uuid.UUID(target.PipelineID), area, target.MRNativeID, target.HeadSHA, nullID(target.MRID)).
		Scan(&runID, &mrID, &head)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, "", nil
	}
	return idPtr(runID), idPtr(mrID), head, err
}
