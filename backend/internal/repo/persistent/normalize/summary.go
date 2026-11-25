package normalize

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// SummaryTargets uses the facts written at seq, including rejected imports and runs with no report files.
func (r *Repo) SummaryTargets(ctx context.Context, org entity.ID, seq int64) ([]entity.ID, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		WITH RECURSIVE scans AS (
			SELECT area_id, sha, run_id FROM normalize.report WHERE organization_id = $1 AND known_seq = $2
			UNION ALL SELECT area_id, sha, run_id FROM normalize.rejected_report WHERE organization_id = $1 AND known_seq = $2
			UNION ALL SELECT area_id, sha, NULL FROM normalize.analysis_run WHERE organization_id = $1 AND known_seq = $2
		), changed_runs AS (
			SELECT run_id FROM normalize.ci_run_version WHERE organization_id = $1 AND known_seq = $2
				AND outcome IN ('success','failed','canceled','skipped')
			UNION SELECT run_id FROM normalize.ci_attempt_version WHERE organization_id = $1 AND known_seq = $2
				AND outcome IN ('success','failed','canceled','skipped')
			UNION SELECT run_id FROM scans WHERE run_id IS NOT NULL
		), runs AS (
			SELECT run_id FROM changed_runs
			UNION
			SELECT c.parent_run_id FROM normalize.ci_run_version c JOIN runs r ON r.run_id = c.run_id
			WHERE c.organization_id = $1 AND c.superseded_seq IS NULL AND c.parent_run_id IS NOT NULL
		)
		SELECT m.mr_id FROM normalize.mr_version m
		WHERE m.organization_id = $1 AND m.superseded_seq IS NULL AND (
			m.known_seq = $2 OR EXISTS (
				SELECT 1 FROM normalize.mr_diff d JOIN normalize.mr_revision r
					ON (r.organization_id,r.id) = (d.organization_id,d.mr_revision_id)
				WHERE d.organization_id = $1 AND d.known_seq = $2 AND r.mr_id = m.mr_id AND r.head_sha = m.head_sha
			) OR EXISTS (
				SELECT 1 FROM scans s WHERE s.area_id = m.area_id AND s.sha IN (m.head_sha, m.base_sha, m.merge_base_sha)
			) OR EXISTS (
				SELECT 1 FROM normalize.ci_run_version c
				WHERE c.organization_id = $1 AND c.superseded_seq IS NULL AND c.sha = m.head_sha
				AND (c.mr_id = m.mr_id OR c.area_id = m.area_id AND c.mr_id IS NULL)
				AND c.run_id IN (SELECT run_id FROM runs)
			)
		) ORDER BY m.mr_id`, uuid.UUID(org), seq)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.ID, error) {
		var id uuid.UUID
		err := row.Scan(&id)
		return entity.ID(id), err
	})
}

// SummaryMR reads the current normalized head and the knowledge cut used by a summary.
func (r *Repo) SummaryMR(ctx context.Context, org, mr entity.ID, lock bool) (entity.SummaryMR, error) {
	conn := postgres.Conn(ctx, r.pool)
	var out entity.SummaryMR
	var versionID, mrID, areaID, instanceID uuid.UUID
	query := `SELECT last_seq FROM platform.org_clock WHERE organization_id = $1`
	if lock {
		query += " FOR UPDATE"
	}
	if err := conn.QueryRow(ctx, query, uuid.UUID(org)).Scan(&out.KnowledgeSeq); err != nil {
		return out, err
	}
	err := conn.QueryRow(ctx, `SELECT m.id, m.mr_id, m.area_id, m.head_sha, COALESCE(m.base_sha,''),
		COALESCE(m.merge_base_sha,''), m.observed_at, m.state, o.instance_id, o.native_ns, o.native_id,
		GREATEST(m.known_seq, (
			SELECT max(f.known_seq)
			FROM (
				WITH RECURSIVE pipeline AS (
					SELECT c.run_id, c.known_seq FROM normalize.ci_run_version c
					WHERE c.organization_id = $1 AND c.sha = m.head_sha AND c.superseded_seq IS NULL
						AND c.mr_id = $2 AND c.parent_run_id IS NULL
					UNION
					SELECT c.run_id, c.known_seq FROM normalize.ci_run_version c JOIN pipeline p ON p.run_id = c.parent_run_id
					WHERE c.organization_id = $1 AND c.area_id = m.area_id AND c.superseded_seq IS NULL
				)
				SELECT r.known_seq FROM normalize.report r
				WHERE r.organization_id = $1 AND r.area_id = m.area_id AND r.sha IN (m.head_sha, m.base_sha, m.merge_base_sha)
				UNION ALL
				SELECT r.known_seq FROM normalize.rejected_report r
				WHERE r.organization_id = $1 AND r.area_id = m.area_id AND r.sha IN (m.head_sha, m.base_sha, m.merge_base_sha)
				UNION ALL
				SELECT r.known_seq FROM normalize.analysis_run r
				WHERE r.organization_id = $1 AND r.area_id = m.area_id AND r.sha IN (m.head_sha, m.base_sha, m.merge_base_sha)
				UNION ALL
				SELECT d.known_seq FROM normalize.mr_diff d JOIN normalize.mr_revision r
					ON (r.organization_id, r.id) = (d.organization_id, d.mr_revision_id)
				WHERE r.organization_id = $1 AND r.mr_id = $2 AND r.head_sha = m.head_sha
				UNION ALL
				SELECT GREATEST(c.known_seq, a.known_seq)
				FROM pipeline c LEFT JOIN normalize.ci_attempt_version a
					ON a.organization_id = $1 AND a.run_id = c.run_id AND a.superseded_seq IS NULL
			) f
		))
		FROM normalize.mr_version m JOIN normalize.source_object o
			ON (o.organization_id, o.id) = (m.organization_id, m.mr_id)
		WHERE m.organization_id = $1 AND m.mr_id = $2 AND m.superseded_seq IS NULL AND o.deleted_at IS NULL`, uuid.UUID(org), uuid.UUID(mr)).
		Scan(&versionID, &mrID, &areaID, &out.HeadSHA, &out.BaseSHA, &out.MergeBaseSHA, &out.ObservedAt,
			&out.State, &instanceID, &out.Source.NativeNS, &out.Source.NativeID, &out.FactsSeq)
	out.ID, out.MRID, out.AreaID, out.InstanceID = entity.ID(versionID), entity.ID(mrID), entity.ID(areaID), entity.ID(instanceID)
	out.Source.ObjectKind = "merge_request"
	if errors.Is(err, pgx.ErrNoRows) {
		err = entity.ErrNotFound
	}
	return out, err
}

// SummarySeverities reads the mapped priority of the occurrences of one comparison.
func (r *Repo) SummarySeverities(ctx context.Context, org entity.ID, occurrences []entity.ID) (map[entity.ID]string, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `SELECT id, COALESCE(mapped_severity, lower(native_severity), '')
		FROM normalize.finding_occurrence WHERE organization_id = $1 AND id = ANY($2::uuid[])`, uuid.UUID(org), ids(occurrences))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[entity.ID]string, len(occurrences))
	for rows.Next() {
		var id uuid.UUID
		var severity string
		if err := rows.Scan(&id, &severity); err != nil {
			return nil, err
		}
		out[entity.ID(id)] = severity
	}
	return out, rows.Err()
}

// SummaryReportReadyAt returns the time a report became a normalized fact.
func (r *Repo) SummaryReportReadyAt(ctx context.Context, org, report entity.ID) (time.Time, error) {
	var at time.Time
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT COALESCE(c.recorded_at, i.received_at)
		FROM normalize.report r JOIN ingest.receipt i ON (i.organization_id, i.id) = (r.organization_id, r.receipt_id)
		LEFT JOIN normalize.area_change c ON (c.organization_id,c.area_id,c.known_seq) = (r.organization_id,r.area_id,r.known_seq)
		WHERE r.organization_id = $1 AND r.id = $2`, uuid.UUID(org), uuid.UUID(report)).Scan(&at)
	return at, err
}

// SummaryReports reads the complete history of one exact stream. Completeness and configured rules
// are checked before choosing an observation, so a newer partial report cannot hide a valid one.
func (r *Repo) SummaryReports(ctx context.Context, org entity.ID, mr entity.SummaryMR, e entity.ProfileEntry) ([]entity.TechnicalReport, error) {
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, `
		WITH observations AS (
			SELECT r.id, r.sha, r.report_format AS format, COALESCE(r.language,'') AS language, COALESCE(r.producer,'') AS producer,
				COALESCE(r.producer_version,'') AS producer_version, r.rules_hash, r.config_hash, r.scope_hash,
				COALESCE(r.db_version,'') AS db_version, r.completeness, r.exit_class,
				COALESCE(a.outcome = 'canceled', false) AS interrupted, false AS rejected,
				COALESCE(r.dependency_delta,'') AS delta, COALESCE(r.dependency_base_sha,'') AS delta_base,
				COALESCE(r.finished_at,r.started_at,i.received_at) AS observed_at, r.known_seq, r.record_no
			FROM normalize.report r
			JOIN ingest.receipt i ON (i.organization_id, i.id) = (r.organization_id, r.receipt_id)
			LEFT JOIN normalize.ci_attempt_version a ON (a.organization_id, a.attempt_id) = (r.organization_id, r.attempt_id)
				AND a.known_seq <= $9 AND (a.superseded_seq IS NULL OR a.superseded_seq > $9)
			WHERE r.organization_id = $1 AND r.area_id = $2 AND r.component_id = $3 AND r.report_kind = $4
				AND ($5 = '' OR r.language = $5) AND COALESCE(r.profile_id,'') = $6 AND r.mode = $7
				AND r.sha = ANY($8::text[]) AND r.known_seq <= $9
				AND ($10::bytea IS NULL OR r.rules_hash = $10)
			UNION ALL
			SELECT r.id, r.sha, COALESCE(r.report_format,''), r.language, COALESCE(r.producer,''), '',
				CAST(NULL AS bytea), NULL, r.scope_hash, '', 'unknown', '', false, true, '', '',
				i.received_at, r.known_seq, r.record_no
			FROM normalize.rejected_report r
			JOIN ingest.receipt i ON (i.organization_id, i.id) = (r.organization_id, r.receipt_id)
			WHERE r.organization_id = $1 AND r.area_id = $2 AND r.component_id = $3 AND r.report_kind = $4
				AND ($5 = '' OR r.language = $5) AND COALESCE(r.profile_id,'') = $6 AND r.mode = $7
				AND r.sha = ANY($8::text[]) AND r.known_seq <= $9
		)
		SELECT id, sha, format, language, producer, producer_version, rules_hash, config_hash, scope_hash, db_version,
			completeness, exit_class, interrupted, rejected, delta, delta_base
		FROM observations ORDER BY observed_at DESC, known_seq DESC, record_no DESC, id DESC`,
		uuid.UUID(org), uuid.UUID(mr.AreaID), uuid.UUID(e.ComponentID), e.AnalysisKind, e.Language, e.ProfileID, e.Mode,
		[]string{mr.HeadSHA, mr.BaseSHA}, mr.KnowledgeSeq, e.RulesHash)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entity.TechnicalReport, error) {
		v := entity.TechnicalReport{ProfileID: e.ProfileID, Mode: e.Mode}
		var id uuid.UUID
		err := row.Scan(&id, &v.SHA, &v.Format, &v.Language, &v.Producer, &v.ProducerVersion, &v.RulesHash, &v.ConfigHash, &v.ScopeHash,
			&v.DBVersion, &v.Completeness, &v.ExitClass, &v.Interrupted, &v.Rejected, &v.DependencyDelta, &v.DependencyBaseSHA)
		v.ID = entity.ID(id)
		return v, err
	})
}

// SummaryCodeFacts reads the changed-code input by immutable report ID.
func (r *Repo) SummaryCodeFacts(ctx context.Context, org, id entity.ID) (*entity.CodeReport, error) {
	out := &entity.CodeReport{ID: id}
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `
		SELECT f.facts, r.completeness, COALESCE(r.skipped_count,0), r.skipped_targets
		FROM normalize.code_report_fact f JOIN normalize.report r ON (r.organization_id, r.id) = (f.organization_id, f.report_id)
		WHERE f.organization_id = $1 AND f.report_id = $2`, uuid.UUID(org), uuid.UUID(id)).
		Scan(&out.Facts, &out.Completeness, &out.SkippedCount, &out.Skipped)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return out, err
}
