package change

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) PublicationRevision(ctx context.Context, org, mr entity.ID) (entity.SummaryRevision, error) {
	return r.SummaryRevision(ctx, org, mr, nil)
}

func (r *Repo) PublicationItems(ctx context.Context, org, revision entity.ID) ([]entity.SummaryItem, error) {
	return r.loadSummaryItems(ctx, org, revision, true)
}

func (r *Repo) SummaryItems(ctx context.Context, org, revision entity.ID) ([]entity.SummaryItem, error) {
	return r.loadSummaryItems(ctx, org, revision, false)
}

func (r *Repo) loadSummaryItems(ctx context.Context, org, revision entity.ID, forPublication bool) ([]entity.SummaryItem, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `SELECT i.item_kind, i.finding_identity_id, i.occurrence_id,
		i.required_right, i.rank, COALESCE(i.analysis_kind,''), COALESCE(i.reason_code,''), COALESCE(i.severity,''),
		CASE WHEN $3 AND i.required_right = 'security' THEN NULL
		     WHEN NOT $3 AND o.id IS NOT NULL THEN jsonb_strip_nulls(
		        jsonb_build_object('rule', o.rule_id, 'path', o.path, 'package', o.package_purl, 'line', o.line_start, 'line_end', o.line_end))
		     WHEN NOT $3 AND run.id IS NOT NULL THEN i.value || jsonb_build_object(
		        'outcome', run.outcome, 'parent_pipeline_id', run.parent_run_id,
		        'retried_jobs', (SELECT count(DISTINCT (a.run_id, a.job_key, a.config_fingerprint))
		            FROM normalize.ci_attempt_version AS a
		            JOIN jsonb_array_elements_text(i.value->'attempt_versions') AS ids(id) ON a.id = ids.id::uuid
		            WHERE a.organization_id = i.organization_id AND a.attempt_no > 1))
		     ELSE i.value END
		FROM change.summary_item AS i
		LEFT JOIN normalize.finding_occurrence AS o ON NOT $3 AND (i.organization_id, i.occurrence_id) = (o.organization_id, o.id)
		LEFT JOIN normalize.ci_run_version AS run ON NOT $3 AND i.item_kind = 'ci_attempts'
		    AND (run.organization_id, run.id) = (i.organization_id, (i.value->'run_versions'->>0)::uuid)
		WHERE i.organization_id = $1 AND i.summary_revision_id = $2
		AND (NOT $3 OR i.required_right = 'security' OR i.item_kind IN ('new','resolved','not_evaluated','ci_attempts','coverage','tests'))
		ORDER BY i.rank NULLS LAST, i.item_no`, uuid.UUID(org), uuid.UUID(revision), forPublication)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.SummaryItem, error) {
		var v entity.SummaryItem
		var finding, occurrence *uuid.UUID
		err := row.Scan(&v.Kind, &finding, &occurrence, &v.RequiredRight, &v.Rank, &v.AnalysisKind, &v.ReasonCode, &v.Severity, &v.Value)
		v.FindingID, v.OccurrenceID = (*entity.ID)(finding), (*entity.ID)(occurrence)
		return v, err
	})
}

func (r *Repo) SavePublication(ctx context.Context, org, revision entity.ID, p entity.Publication) error {
	_, err := postgres.Conn(ctx, r.pool).Exec(ctx, `UPDATE change.summary_revision
		SET publication_status = $3, publication_external_id = NULLIF($4,''),
		    publication_error_code = NULLIF($5,''), published_at = $6
		WHERE organization_id = $1 AND id = $2`, uuid.UUID(org), uuid.UUID(revision), p.Status, p.ExternalID, p.ErrorCode, p.PublishedAt)
	return err
}

func (r *Repo) SummaryRevision(ctx context.Context, org, mr entity.ID, no *int) (entity.SummaryRevision, error) {
	v := entity.SummaryRevision{MRID: mr}
	var built, ready *time.Time
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT id, revision_no, head_sha, COALESCE(base_sha,''),
		status, pipeline_state, built_at, reports_ready_at, COALESCE(stale_reason,''), publication_status,
		COALESCE(publication_external_id,''), COALESCE(publication_error_code,''), published_at
		FROM change.summary_revision WHERE organization_id = $1 AND mr_id = $2
		AND (($3::integer IS NULL AND status IN ('ready','stale')) OR revision_no = $3)
		ORDER BY revision_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(mr), no).
		Scan((*uuid.UUID)(&v.ID), &v.No, &v.HeadSHA, &v.BaseSHA, &v.Status, &v.PipelineState, &built, &ready, &v.StaleReason,
			&v.Publication.Status, &v.Publication.ExternalID, &v.Publication.ErrorCode, &v.Publication.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = entity.ErrNotFound
	}
	if built != nil {
		v.BuiltAt = *built
	}
	if ready != nil {
		v.ReportsReadyAt = *ready
	}
	return v, err
}
