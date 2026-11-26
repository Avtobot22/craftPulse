package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// The version at the cut supplies the current source timestamps and outcome. For each timestamp,
// use the first version that carried it, so a later snapshot cannot change when the start was learned.
const _ciTimeline = `
WITH runs AS MATERIALIZED (
	SELECT v.*, o.current_area_id, o.instance_id, o.native_id
	FROM normalize.ci_run_version v
	JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.run_id
	WHERE v.organization_id = $1 AND o.current_area_id = ANY($2::uuid[]) AND o.deleted_at IS NULL
	  AND (NOT o.restricted OR o.id = ANY($3::uuid[]))
	  AND v.known_seq <= $4 AND (v.superseded_seq IS NULL OR v.superseded_seq > $4)
	  AND ($7::uuid[] IS NULL OR v.run_id = ANY($7))
), versions AS (
	SELECT v.run_id AS object_id, v.run_id, v.current_area_id, v.instance_id, v.native_id,
	       '' AS parent_native_id, '#' || v.native_id AS display_key, 'ci_run' AS object_kind,
	       v.started_at, v.finished_at, v.outcome
	FROM runs v
	UNION ALL
	SELECT a.attempt_id, v.run_id, v.current_area_id, v.instance_id, o.native_id, v.native_id,
	       a.job_key, 'ci_attempt', a.started_at, a.finished_at, a.outcome
	FROM runs v
	JOIN normalize.ci_attempt_version a ON a.organization_id = v.organization_id AND a.run_id = v.run_id
	JOIN normalize.source_object o ON o.organization_id = a.organization_id AND o.id = a.attempt_id
	WHERE o.deleted_at IS NULL AND (o.current_area_id IS NULL OR o.current_area_id = v.current_area_id)
	  AND (NOT o.restricted OR o.id = ANY($3::uuid[]))
	  AND a.known_seq <= $4 AND (a.superseded_seq IS NULL OR a.superseded_seq > $4)
)
SELECT v.object_id, v.run_id, v.current_area_id, v.instance_id, v.native_id, v.parent_native_id,
       v.display_key, v.object_kind, e.kind, e.at, e.outcome, first.observed_at, first.offset_s,
       COALESCE(first.observed_via, CASE WHEN rec.channel = 'import' THEN 'import' ELSE 'incremental' END)
FROM versions v
CROSS JOIN LATERAL (VALUES
	(v.started_at, '', CASE WHEN v.object_kind = 'ci_run' THEN 'pipeline_started' ELSE 'attempt_started' END, true),
	(v.finished_at, v.outcome, CASE WHEN v.object_kind = 'ci_run' THEN 'pipeline_finished' ELSE 'attempt_finished' END, false)
) e(at, outcome, kind, is_start)
CROSS JOIN LATERAL (
	SELECT h.receipt_id, h.observed_at, h.observed_via, h.offset_s
	FROM (
		SELECT h.receipt_id, h.observed_at, h.observed_via, h.known_seq, h.id,
		       CASE WHEN e.is_start THEN h.start_offset_s ELSE h.finish_offset_s END AS offset_s
		FROM normalize.ci_run_version h
		WHERE v.object_kind = 'ci_run' AND h.organization_id = $1 AND h.run_id = v.object_id AND h.known_seq <= $4
		  AND (e.is_start AND h.started_at = e.at OR NOT e.is_start AND h.finished_at = e.at AND h.outcome = e.outcome)
		UNION ALL
		SELECT h.receipt_id, h.observed_at, h.observed_via, h.known_seq, h.id,
		       CASE WHEN e.is_start THEN h.start_offset_s ELSE h.finish_offset_s END
		FROM normalize.ci_attempt_version h
		WHERE v.object_kind = 'ci_attempt' AND h.organization_id = $1 AND h.attempt_id = v.object_id AND h.known_seq <= $4
		  AND (e.is_start AND h.started_at = e.at OR NOT e.is_start AND h.finished_at = e.at AND h.outcome = e.outcome)
	) h
	ORDER BY h.known_seq, h.id LIMIT 1
) first
LEFT JOIN ingest.receipt rec ON rec.organization_id = $1 AND rec.id = first.receipt_id
WHERE e.at >= $5 AND e.at < $6`

// Git inventories do not carry the commit timestamp. The scan time, when present, dates the
// creation of the inventoried version; its receipt supplies the separate time of observation.
const _documentTimeline = `
SELECT d.id, d.document_id, o.current_area_id, o.instance_id, d.path, COALESCE(d.title, ''), d.version_ref,
       f.event_time, f.received_at, CASE WHEN f.channel = 'import' THEN 'import' ELSE 'incremental' END
FROM normalize.document_version d
JOIN normalize.source_object o ON o.organization_id = d.organization_id AND o.id = d.document_id
CROSS JOIN LATERAL (
	SELECT COALESCE(r.finished_at, r.started_at, i.received_at) AS event_time, i.received_at, i.channel
	FROM normalize.docs_report_document l
	JOIN normalize.report r ON r.organization_id = l.organization_id AND r.id = l.report_id
	JOIN ingest.receipt i ON i.organization_id = r.organization_id AND i.id = r.receipt_id
	WHERE l.organization_id = d.organization_id AND l.document_version_id = d.id AND r.known_seq <= $4
	ORDER BY r.known_seq, r.id LIMIT 1
) f
WHERE d.organization_id = $1 AND o.current_area_id = ANY($2::uuid[]) AND o.deleted_at IS NULL
  AND d.acl_state = 'known' AND (NOT o.restricted OR o.id = ANY($3::uuid[]))
  AND d.known_seq <= $4 AND f.event_time >= $5 AND f.event_time < $6`

func (r *Repo) TimelineFacts(ctx context.Context, org entity.ID, areas, allowed []entity.ID, q entity.TimelineFactQuery) ([]entity.TimelineFact, error) {
	args := []any{uuid.UUID(org), ids(areas), ids(allowed), q.Knowledge, q.From, q.To}
	sql, scan := _documentTimeline, scanDocumentTimeline
	if q.Kind == entity.ObjectCIRun {
		sql, scan = _ciTimeline, scanCITimeline
		var runs []uuid.UUID
		if q.Runs != nil {
			runs = ids(q.Runs)
		}
		args = append(args, runs)
	}
	rows, err := postgres.Conn(ctx, r.pool).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("normalize: read timeline facts: %w", err)
	}
	defer rows.Close()
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return nil, fmt.Errorf("normalize: read timeline facts: %w", err)
	}
	return out, nil
}

func scanCITimeline(row pgx.CollectableRow) (entity.TimelineFact, error) {
	var f entity.TimelineFact
	var object, run, area, instance uuid.UUID
	err := row.Scan(&object, &run, &area, &instance, &f.NativeID, &f.ParentNativeID, &f.DisplayKey,
		&f.ObjectKind, &f.Kind, &f.EventTime, &f.Value, &f.ObservedAt, &f.EventOffsetS, &f.ObservedVia)
	f.ObjectID, f.RunID, f.AreaID, f.InstanceID = entity.ID(object), entity.ID(run), entity.ID(area), entity.ID(instance)
	f.ID = entity.ID(uuid.NewSHA1(object, []byte(f.Kind)))
	return f, err
}

func scanDocumentTimeline(row pgx.CollectableRow) (entity.TimelineFact, error) {
	f := entity.TimelineFact{Kind: "version_created", ObjectKind: entity.ObjectDocument}
	var id, object, area, instance uuid.UUID
	err := row.Scan(&id, &object, &area, &instance, &f.DisplayKey, &f.Title, &f.Value, &f.EventTime, &f.ObservedAt, &f.ObservedVia)
	f.ID, f.ObjectID, f.AreaID, f.InstanceID = entity.ID(id), entity.ID(object), entity.ID(area), entity.ID(instance)
	return f, err
}
