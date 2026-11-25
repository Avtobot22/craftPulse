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
	var v entity.SummaryRevision
	var ready *time.Time
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT id, revision_no, head_sha, COALESCE(base_sha, ''),
		status, pipeline_state, built_at, reports_ready_at, publication_status
		FROM change.summary_revision WHERE organization_id = $1 AND mr_id = $2
		AND status IN ('ready', 'stale') ORDER BY revision_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(mr)).
		Scan((*uuid.UUID)(&v.ID), &v.No, &v.HeadSHA, &v.BaseSHA, &v.Status, &v.PipelineState, &v.BuiltAt,
			&ready, &v.Publication.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = entity.ErrNotFound
	}
	if ready != nil {
		v.ReportsReadyAt = *ready
	}
	v.MRID = mr
	return v, err
}

func (r *Repo) PublicationItems(ctx context.Context, org, revision entity.ID) ([]entity.SummaryItem, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `SELECT item_kind, finding_identity_id, occurrence_id,
		required_right, rank, COALESCE(analysis_kind,''), COALESCE(reason_code,''), COALESCE(severity,''), value
		FROM change.summary_item WHERE organization_id = $1 AND summary_revision_id = $2
		AND (required_right = 'security' OR item_kind IN ('new','resolved','not_evaluated','ci_attempts','coverage','tests'))
		ORDER BY rank NULLS LAST, item_no`, uuid.UUID(org), uuid.UUID(revision))
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

func (r *Repo) Publication(ctx context.Context, org, mr, revision entity.ID) (entity.Publication, error) {
	var p entity.Publication
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT publication_status, COALESCE(publication_external_id,''),
		COALESCE(publication_error_code,''), published_at FROM change.summary_revision
		WHERE organization_id = $1 AND mr_id = $2 AND id = $3`, uuid.UUID(org), uuid.UUID(mr), uuid.UUID(revision)).
		Scan(&p.Status, &p.ExternalID, &p.ErrorCode, &p.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = entity.ErrNotFound
	}
	return p, err
}
