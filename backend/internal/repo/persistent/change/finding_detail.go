package change

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// FindingObservation prefers the latest complete observation of the canonical finding.
func (r *Repo) FindingObservation(ctx context.Context, org, id entity.ID) (entity.FindingDetail, error) {
	var out entity.FindingDetail
	o, report := &out.Observation, &out.Report
	var reportID uuid.UUID
	var component *uuid.UUID
	var mapping *int
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `
		SELECT o.rule_id, COALESCE(o.path, ''), o.line_start, o.line_end,
		       COALESCE(o.package_purl, ''), COALESCE(o.native_severity, ''), COALESCE(o.native_confidence, ''),
		       COALESCE(o.mapped_severity, ''), o.severity_mapping_version,
		       CASE WHEN f.finding_kind <> 'secret' THEN o.evidence END,
		       r.id, r.component_id, r.report_kind, COALESCE(r.sha, ''), COALESCE(r.profile_id, ''),
		       COALESCE(r.producer, ''), COALESCE(r.producer_version, ''), r.mode, r.rules_hash,
		       COALESCE(r.db_version, ''), r.completeness
		FROM change.occurrence_identity i
		JOIN change.finding_identity f ON (f.organization_id, f.id) = (i.organization_id, i.finding_identity_id)
		JOIN normalize.finding_occurrence o ON (o.organization_id, o.id) = (i.organization_id, i.occurrence_id)
		JOIN normalize.report r ON (r.organization_id, r.id, r.area_id) = (o.organization_id, o.report_id, f.area_id)
		LEFT JOIN normalize.ci_attempt_version a ON a.organization_id = r.organization_id
		    AND a.attempt_id = r.attempt_id AND a.superseded_seq IS NULL
		WHERE i.organization_id = $1 AND i.finding_identity_id = $2 AND o.finding_kind = f.finding_kind
		ORDER BY (r.completeness = 'complete' AND r.exit_class IN ('success', 'findings')
		          AND NOT COALESCE(a.outcome = 'canceled', false)) DESC,
		         COALESCE(r.finished_at, r.started_at, i.reported_at) DESC,
		         r.known_seq DESC, r.record_no DESC, r.id DESC, o.id
		LIMIT 1`, uuid.UUID(org), uuid.UUID(id)).
		Scan(&o.RuleID, &o.Path, &o.LineStart, &o.LineEnd, &o.PackagePURL, &o.NativeSeverity, &o.NativeConfidence,
			&o.MappedSeverity, &mapping, &o.Evidence, &reportID, &component, &out.ReportKind, &report.SHA, &report.ProfileID,
			&report.Producer, &report.ProducerVersion, &report.Mode, &report.RulesHash, &report.DBVersion, &report.Completeness)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, entity.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("change: read finding observation: %w", err)
	}
	if mapping != nil {
		o.SeverityMappingVersion = *mapping
	}
	out.ComponentID, report.ID = (*entity.ID)(component), entity.ID(reportID)
	return out, nil
}
