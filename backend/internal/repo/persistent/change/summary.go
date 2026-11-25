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
		SET status = 'stale', stale_reason = 'new_head'
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
	_, err = conn.Exec(ctx, `UPDATE change.summary_revision SET status = 'superseded'
		WHERE organization_id = $1 AND mr_id = $2 AND status = 'ready'`, uuid.UUID(org), uuid.UUID(v.MRID))
	if err != nil {
		return false, err
	}
	var revisionID uuid.UUID
	err = conn.QueryRow(ctx, `INSERT INTO change.summary_revision
		(organization_id, mr_id, revision_no, head_sha, base_sha, profile_version_id, status,
		 pipeline_state, input_hash, knowledge_seq, built_at)
		SELECT $1, $2, COALESCE(max(revision_no), 0) + 1, $3, NULLIF($4, ''), $5, 'ready',
			$6, $7, $8, $9 FROM change.summary_revision WHERE organization_id = $1 AND mr_id = $2
		RETURNING id`, uuid.UUID(org), uuid.UUID(v.MRID), v.HeadSHA, v.BaseSHA, uuid.UUID(v.ProfileVersionID), v.PipelineState, v.InputHash,
		v.KnowledgeSeq, v.BuiltAt).Scan(&revisionID)
	if err != nil {
		return false, err
	}
	v.ID = entity.ID(revisionID)
	calculation, err := json.Marshal(struct {
		Definitions []entity.RevisionInput        `json:"definitions"`
		Findings    []entity.SummaryFindingValues `json:"findings"`
	}{v.Definitions, v.FindingValues})
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
			 required_right, rank, component_id, analysis_kind, reason_code, severity, value)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), $13)`,
			uuid.UUID(org), revisionID, n+1, i.Kind, (*uuid.UUID)(i.FindingID), (*uuid.UUID)(i.OccurrenceID),
			i.RequiredRight, i.Rank, (*uuid.UUID)(i.ComponentID),
			i.AnalysisKind, i.ReasonCode, i.Severity, i.Value)
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
func (r *Repo) SummaryProfileAt(ctx context.Context, org, project entity.ID, at time.Time) (entity.CheckProfileVersion, error) {
	row, err := scanSummaryProfile(postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT organization_id, id, project_id,
		version_no, base_version_no, content_hash, known_seq, confirmed_by, confirmed_at, confirmation_id
		FROM change.profile_version WHERE organization_id = $1 AND project_id = $2 AND confirmed_at <= $3
		ORDER BY version_no DESC LIMIT 1`, uuid.UUID(org), uuid.UUID(project), at))
	return r.withEntries(ctx, row, err)
}

func scanSummaryProfile(row pgx.Row) (ChangeProfileVersion, error) {
	var v ChangeProfileVersion
	err := row.Scan(&v.OrganizationID, &v.ID, &v.ProjectID, &v.VersionNo, &v.BaseVersionNo,
		&v.ContentHash, &v.KnownSeq, &v.ConfirmedBy, &v.ConfirmedAt, &v.ConfirmationID)
	return v, err
}
