package change

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// StaleSummaries invalidates the ready revision when the source advances to another head.
func (r *Repo) StaleSummaries(ctx context.Context, org, mr entity.ID, head string) error {
	_, err := postgres.Conn(ctx, r.pool).Exec(ctx, `UPDATE change.summary_revision
		SET status = 'stale', stale_reason = 'new_head',
			publication_error_code = CASE WHEN publication_status = 'pending' THEN 'head_changed' ELSE publication_error_code END,
			publication_status = CASE WHEN publication_status = 'pending' THEN 'stale' ELSE publication_status END
		WHERE organization_id = $1 AND mr_id = $2 AND status = 'ready' AND head_sha <> $3`, uuid.UUID(org), uuid.UUID(mr), head)
	return err
}

// SaveSummary replaces the ready revision when the logical input hash changed.
func (r *Repo) SaveSummary(ctx context.Context, org entity.ID, v *entity.SummaryRevision) (bool, error) {
	conn := postgres.Conn(ctx, r.pool)
	var prior []byte
	err := conn.QueryRow(ctx, `SELECT input_hash FROM change.summary_revision
		WHERE organization_id = $1 AND mr_id = $2 AND status = 'ready'`, uuid.UUID(org), uuid.UUID(v.MRID)).Scan(&prior)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if bytes.Equal(prior, v.InputHash) {
		return false, nil
	}
	_, err = conn.Exec(ctx, `UPDATE change.summary_revision SET status = 'superseded',
		publication_status = CASE WHEN publication_status = 'pending' THEN 'stale' ELSE publication_status END
		WHERE organization_id = $1 AND mr_id = $2 AND status = 'ready'`, uuid.UUID(org), uuid.UUID(v.MRID))
	if err != nil {
		return false, err
	}
	var revisionID uuid.UUID
	err = conn.QueryRow(ctx, `INSERT INTO change.summary_revision
		(organization_id, mr_id, revision_no, head_sha, base_sha, profile_version_id, status,
		 pipeline_state, input_hash, knowledge_seq, built_at, reports_ready_at, publication_status)
		SELECT $1, $2, COALESCE(max(revision_no), 0) + 1, $3, NULLIF($4, ''), $5, 'ready',
			$6, $7, $8, $9, $10, CASE WHEN $6::text = 'running' THEN 'not_requested' ELSE 'pending' END
		FROM change.summary_revision WHERE organization_id = $1 AND mr_id = $2
		RETURNING id`, uuid.UUID(org), uuid.UUID(v.MRID), v.HeadSHA, v.BaseSHA, uuid.UUID(v.ProfileVersionID), v.PipelineState, v.InputHash,
		v.KnowledgeSeq, v.BuiltAt, reportReadyAt(v)).Scan(&revisionID)
	if err != nil {
		return false, err
	}
	v.ID = entity.ID(revisionID)
	calculation, err := json.Marshal(struct {
		Definitions []entity.RevisionInput        `json:"definitions"`
		Findings    []entity.SummaryFindingValues `json:"findings"`
		Technical   entity.TechnicalSummary       `json:"technical"`
	}{v.Definitions, v.FindingValues, v.Technical})
	if err != nil {
		return false, err
	}
	_, err = conn.Exec(ctx, `INSERT INTO change.summary_input
		(organization_id, summary_revision_id, input_kind, input_id, calculation)
		VALUES ($1, $2, 'calc_revision', uuidv7(), $3)`, uuid.UUID(org), revisionID, calculation)
	if err != nil {
		return false, err
	}
	batch := &pgx.Batch{}
	for _, i := range v.Inputs {
		batch.Queue(`INSERT INTO change.summary_input
			(organization_id, summary_revision_id, input_kind, input_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			uuid.UUID(org), revisionID, i.Kind, uuid.UUID(i.ID))
	}
	for n := range v.Items {
		i := &v.Items[n]
		batch.Queue(`INSERT INTO change.summary_item
			(organization_id, summary_revision_id, item_no, item_kind, finding_identity_id, occurrence_id,
			 required_right, rank, component_id, analysis_kind, reason_code, severity, value, mode)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), $13, NULLIF($14,''))`,
			uuid.UUID(org), revisionID, n+1, i.Kind, (*uuid.UUID)(i.FindingID), (*uuid.UUID)(i.OccurrenceID),
			i.RequiredRight, i.Rank, (*uuid.UUID)(i.ComponentID),
			i.AnalysisKind, i.ReasonCode, i.Severity, i.Value, i.Mode)
	}
	return true, conn.SendBatch(ctx, batch).Close()
}

// LastGoodSummary identifies the last complete revision and its build time, including history.
func (r *Repo) LastGoodSummary(ctx context.Context, org, mr entity.ID) (entity.SummaryRevision, error) {
	var v entity.SummaryRevision
	var id, mrID, profileID uuid.UUID
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT id, mr_id, revision_no, head_sha, COALESCE(base_sha,''),
		profile_version_id, status, pipeline_state, input_hash, knowledge_seq, built_at, COALESCE(stale_reason,'')
		FROM change.summary_revision WHERE organization_id = $1 AND mr_id = $2
		AND pipeline_state = 'complete' AND built_at IS NOT NULL ORDER BY revision_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(mr)).
		Scan(&id, &mrID, &v.No, &v.HeadSHA, &v.BaseSHA, &profileID, &v.Status, &v.PipelineState,
			&v.InputHash, &v.KnowledgeSeq, &v.BuiltAt, &v.StaleReason)
	if errors.Is(err, pgx.ErrNoRows) {
		err = entity.ErrNotFound
	}
	v.ID, v.MRID, v.ProfileVersionID = entity.ID(id), entity.ID(mrID), entity.ID(profileID)
	return v, err
}

// SummaryProfileAt selects the matrix in force at the start of the pipeline.
func (r *Repo) SummaryProfileAt(ctx context.Context, org, project entity.ID, at time.Time, seq int64) (entity.CheckProfileVersion, error) {
	row, err := scanSummaryProfile(postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT organization_id, id, project_id,
		version_no, base_version_no, content_hash, known_seq, confirmed_by, confirmed_at, confirmation_id
		FROM change.profile_version WHERE organization_id = $1 AND project_id = $2 AND confirmed_at <= $3 AND ($4::bigint = 0 OR known_seq <= $4)
		ORDER BY version_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(project), at, seq))
	return r.withEntries(ctx, row, err)
}

func scanSummaryProfile(row pgx.Row) (ChangeProfileVersion, error) {
	var v ChangeProfileVersion
	err := row.Scan(&v.OrganizationID, &v.ID, &v.ProjectID, &v.VersionNo, &v.BaseVersionNo,
		&v.ContentHash, &v.KnownSeq, &v.ConfirmedBy, &v.ConfirmedAt, &v.ConfirmationID)
	return v, err
}

// ReadSummary selects the latest built revision matching the optional head and revision number.
// Last good is relative to the selected revision.
func (r *Repo) ReadSummary(ctx context.Context, org, mr entity.ID, no *int, head *string) (entity.SummaryRead, error) {
	var v entity.SummaryRead
	var id, project uuid.UUID
	var goodNo *int
	var goodHead *string
	var goodAt *time.Time
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `
		SELECT s.id, p.project_id, s.revision_no, s.head_sha, COALESCE(s.base_sha, ''),
		       CASE WHEN live.new_head THEN 'stale' ELSE s.status END,
		       s.pipeline_state, s.built_at,
		       CASE WHEN live.new_head THEN 'new_head' ELSE COALESCE(s.stale_reason, '') END,
		       s.publication_status, COALESCE(s.publication_external_id,''), COALESCE(s.publication_error_code,''), s.published_at,
		       g.revision_no, g.head_sha, g.built_at
		FROM change.summary_revision s
		JOIN change.profile_version p ON (p.organization_id,p.id) = (s.organization_id,s.profile_version_id)
		LEFT JOIN normalize.mr_version m ON (m.organization_id,m.mr_id) = (s.organization_id,s.mr_id)
			AND m.superseded_seq IS NULL
		CROSS JOIN LATERAL (SELECT s.status = 'ready' AND m.head_sha <> s.head_sha AS new_head) live
		LEFT JOIN LATERAL (
			SELECT revision_no, head_sha, built_at FROM change.summary_revision
			WHERE organization_id = s.organization_id AND mr_id = s.mr_id AND revision_no <= s.revision_no
			  AND pipeline_state = 'complete' AND built_at IS NOT NULL
			ORDER BY revision_no DESC LIMIT 1
		) g ON true
		WHERE s.organization_id = $1 AND s.mr_id = $2
		  AND (($3::integer IS NULL AND ($4::text IS NOT NULL OR s.status IN ('ready','stale'))) OR s.revision_no = $3)
		  AND ($4::text IS NULL OR s.head_sha = $4)
		ORDER BY s.revision_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(mr), no, head).
		Scan(&id, &project, &v.No, &v.HeadSHA, &v.BaseSHA, &v.Status, &v.PipelineState, &v.BuiltAt, &v.StaleReason,
			&v.Publication.Status, &v.Publication.ExternalID, &v.Publication.ErrorCode, &v.Publication.PublishedAt,
			&goodNo, &goodHead, &goodAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, entity.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.ID, v.MRID, v.ProjectID = entity.ID(id), mr, entity.ID(project)
	if goodNo != nil {
		v.LastGood = &entity.SummaryCheckpoint{No: *goodNo, HeadSHA: *goodHead, BuiltAt: *goodAt}
	}
	return v, nil
}

// ReadSummaryItems loads immutable occurrence coordinates and falls back to the original profile for older items.
func (r *Repo) ReadSummaryItems(ctx context.Context, org, id entity.ID) ([]entity.SummaryItem, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		SELECT i.item_kind, i.finding_identity_id, i.occurrence_id, i.required_right, i.component_id,
		       COALESCE(i.analysis_kind,''), COALESCE(i.reason_code,''), COALESCE(i.severity,''), i.value,
		       COALESCE(i.mode, orep.mode, p.mode, ''), COALESCE(o.rule_id, f.rule_id, i.value->>'rule', ''),
		       COALESCE(o.path, i.value->>'path', ''), COALESCE(o.line_start, (i.value->>'line')::integer), o.line_end
		FROM change.summary_item i
		JOIN change.summary_revision s ON (s.organization_id,s.id) = (i.organization_id,i.summary_revision_id)
		LEFT JOIN normalize.finding_occurrence o ON (o.organization_id,o.id) = (i.organization_id,i.occurrence_id)
		LEFT JOIN normalize.report orep ON (orep.organization_id,orep.id) = (o.organization_id,o.report_id)
		LEFT JOIN change.finding_identity f ON (f.organization_id,f.id) = (i.organization_id,i.finding_identity_id)
		LEFT JOIN LATERAL (
			SELECT min(mode) AS mode FROM change.profile_entry p
			WHERE (p.organization_id,p.profile_version_id,p.component_id,p.analysis_kind) =
			      (s.organization_id,s.profile_version_id,i.component_id,i.analysis_kind)
			HAVING min(mode) = max(mode)
		) p ON i.mode IS NULL AND orep.mode IS NULL
		WHERE i.organization_id = $1 AND i.summary_revision_id = $2 ORDER BY i.item_no`, uuid.UUID(org), uuid.UUID(id))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.SummaryItem, error) {
		var i entity.SummaryItem
		var finding, occurrence, component *uuid.UUID
		err := row.Scan(&i.Kind, &finding, &occurrence, &i.RequiredRight,
			&component, &i.AnalysisKind, &i.ReasonCode, &i.Severity, &i.Value,
			&i.Mode, &i.RuleID, &i.Path, &i.LineStart, &i.LineEnd)
		i.FindingID, i.OccurrenceID, i.ComponentID = (*entity.ID)(finding), (*entity.ID)(occurrence), (*entity.ID)(component)
		return i, err
	})
}

// HasBuiltSummary reports whether there is a revision for this exact head.
func (r *Repo) HasBuiltSummary(ctx context.Context, org, mr entity.ID, head string) (bool, error) {
	var exists bool
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM change.summary_revision
		WHERE (organization_id, mr_id, head_sha) = ($1, $2, $3) AND built_at IS NOT NULL
	)`, uuid.UUID(org), uuid.UUID(mr), head).Scan(&exists)
	return exists, err
}

func reportReadyAt(v *entity.SummaryRevision) *time.Time {
	if v.PipelineState != "complete" || v.ReportsReadyAt.IsZero() {
		return nil
	}
	return &v.ReportsReadyAt
}
