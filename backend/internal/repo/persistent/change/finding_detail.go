package change

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) ReadFindingSnapshot(ctx context.Context, org entity.ID, read func(context.Context) error) error {
	return postgres.InOrgSnapshot(ctx, r.pool, uuid.UUID(org), read)
}

// FindingObservations prefers a complete observation within the requested component scope.
func (r *Repo) FindingObservations(ctx context.Context, org, id entity.ID, sel entity.FindingSelection) ([]entity.FindingDetail, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		SELECT f.id, f.area_id, f.finding_kind, f.first_seen_at, o.*
		FROM change.finding_identity f
		CROSS JOIN LATERAL (
			SELECT o.rule_id, COALESCE(o.path, ''), o.line_start, o.line_end,
			       COALESCE(o.package_purl, ''), COALESCE(o.native_severity, ''), COALESCE(o.native_confidence, ''),
			       COALESCE(o.mapped_severity, ''), COALESCE(o.severity_mapping_version, 0),
			       CASE WHEN f.finding_kind = 'secret' THEN
			           jsonb_build_object('external_suppressions', o.evidence->'external_suppressions')
			        ELSE o.evidence END,
			       r.id, r.component_id, r.report_kind, COALESCE(r.sha, ''), COALESCE(r.profile_id, ''),
			       COALESCE(r.producer, ''), COALESCE(r.producer_version, ''), r.mode, r.rules_hash,
			       COALESCE(r.db_version, ''), r.completeness
			FROM change.occurrence_identity i
			JOIN normalize.finding_occurrence o ON (o.organization_id, o.id) = (i.organization_id, i.occurrence_id)
			JOIN normalize.report r ON (r.organization_id, r.id, r.area_id) = (o.organization_id, o.report_id, f.area_id)
			LEFT JOIN normalize.ci_attempt_version a ON a.organization_id = r.organization_id
			    AND a.attempt_id = r.attempt_id AND a.superseded_seq IS NULL
			WHERE i.organization_id = f.organization_id AND i.finding_identity_id = f.id AND o.finding_kind = f.finding_kind
			  AND (NOT $7::boolean OR r.component_id = ANY($6::uuid[]))
			ORDER BY (r.completeness = 'complete' AND r.exit_class IN ('success', 'findings')
			          AND NOT COALESCE(a.outcome = 'canceled', false)) DESC,
			         COALESCE(r.finished_at, r.started_at, i.reported_at) DESC,
			         r.known_seq DESC, r.record_no DESC, r.id DESC, o.id
			LIMIT 1
		) o
		WHERE f.organization_id = $1 AND (NULLIF($2::uuid, '00000000-0000-0000-0000-000000000000') IS NULL OR f.id = $2)
		  AND f.area_id = ANY($3::uuid[])
		  AND (f.finding_kind IN ('code', 'docs', 'openapi') OR f.area_id = ANY($4::uuid[]))
		  AND ($5 = '' OR f.finding_kind = $5)`,
		uuid.UUID(org), uuid.UUID(id), sel.Areas, sel.SecurityAreas, sel.Kind, sel.Components, len(sel.Components) > 0)
	if err != nil {
		return nil, fmt.Errorf("change: read finding observations: %w", err)
	}
	out, err := pgx.CollectRows(rows, scanFinding)
	if err != nil {
		return nil, fmt.Errorf("change: scan finding observations: %w", err)
	}
	return out, nil
}

func scanFinding(row pgx.CollectableRow) (entity.FindingDetail, error) {
	var out entity.FindingDetail
	o, report := &out.Observation, &out.Report
	err := row.Scan(&out.ID, &out.AreaID, &out.Kind, &out.FirstSeenAt,
		&o.RuleID, &o.Path, &o.LineStart, &o.LineEnd, &o.PackagePURL, &o.NativeSeverity, &o.NativeConfidence,
		&o.MappedSeverity, &o.SeverityMappingVersion, &o.Evidence, &report.ID, &out.ComponentID, &out.ReportKind, &report.SHA, &report.ProfileID,
		&report.Producer, &report.ProducerVersion, &report.Mode, &report.RulesHash, &report.DBVersion, &report.Completeness)
	return out, err
}
