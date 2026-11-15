-- name: EnsureSeries :exec
INSERT INTO calc.series (organization_id, id, scope_hash, scope, metric_set, step, consumed_seq)
VALUES (sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(scope_hash), sqlc.arg(scope), sqlc.arg(metric_set), sqlc.arg(step), 0)
ON CONFLICT (organization_id, scope_hash, metric_set, step) DO NOTHING;

-- name: SeriesByScope :one
SELECT * FROM calc.series
WHERE organization_id = sqlc.arg(organization_id) AND scope_hash = sqlc.arg(scope_hash) AND metric_set = sqlc.arg(metric_set)
    AND step = sqlc.arg(step);

-- name: SeriesByID :one
SELECT * FROM calc.series WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: OrgSeries :many
SELECT * FROM calc.series WHERE organization_id = sqlc.arg(organization_id) AND metric_set = sqlc.arg(metric_set) ORDER BY id;

-- name: LastRevisionID :one
SELECT id FROM calc.revision WHERE organization_id = sqlc.arg(organization_id) AND series_id = sqlc.arg(series_id)
ORDER BY revision_no DESC LIMIT 1;

-- name: RevisionIDAfter :one
SELECT id FROM calc.revision
WHERE organization_id = sqlc.arg(organization_id) AND series_id = sqlc.arg(series_id) AND revision_no > sqlc.arg(after_no)::integer
ORDER BY revision_no LIMIT 1;

-- name: PartByHash :one
SELECT id FROM calc.part WHERE organization_id = sqlc.arg(organization_id) AND input_hash = sqlc.arg(input_hash);

-- name: FailStaleRevisions :exec
UPDATE calc.revision SET status = 'failed'
WHERE organization_id = sqlc.arg(organization_id) AND series_id = sqlc.arg(series_id) AND status = 'building'
    AND progress_at < sqlc.arg(before);

-- name: CreateRevision :one
INSERT INTO calc.revision (organization_id, id, series_id, revision_no, status, progress_at, window_range, as_of, knowledge_seq,
    timezone, input_hash, loading, started_at, previous_revision_id)
SELECT sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(series_id), COALESCE(max(r.revision_no), 0) + 1, 'building', sqlc.arg(now),
    tstzrange(sqlc.arg(window_from)::timestamptz, sqlc.arg(window_to)::timestamptz, '[)'), sqlc.arg(as_of), sqlc.arg(knowledge_seq),
    sqlc.arg(timezone), sqlc.arg(input_hash), sqlc.arg(loading), sqlc.arg(now), sqlc.narg(previous_revision_id)
FROM calc.revision r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.series_id = sqlc.arg(series_id)
ON CONFLICT DO NOTHING
RETURNING revision_no;

-- name: SetRevisionPrevious :exec
UPDATE calc.revision SET previous_revision_id = sqlc.narg(previous_revision_id)::uuid
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'building';

-- name: InsertRevisionInputs :exec
INSERT INTO calc.revision_input (organization_id, revision_id, input_kind, input_id, version, watermark, completeness)
SELECT sqlc.arg(organization_id), sqlc.arg(revision_id), (sqlc.arg(kinds)::text[])[i], (sqlc.arg(input_ids)::text[])[i],
    NULLIF((sqlc.arg(versions)::text[])[i], ''), NULLIF((sqlc.arg(watermarks)::text[])[i], '')::timestamptz,
    NULLIF((sqlc.arg(completeness)::text[])[i], '')
FROM generate_subscripts(sqlc.arg(kinds)::text[], 1) AS i;

-- name: RevisionInputs :many
SELECT input_kind, input_id, version, watermark, completeness FROM calc.revision_input
WHERE organization_id = sqlc.arg(organization_id) AND revision_id = sqlc.arg(revision_id)
ORDER BY input_kind, input_id;

-- name: GetRevision :one
SELECT id, series_id, revision_no, status, progress_at, lower(window_range)::timestamptz AS window_from,
    upper(window_range)::timestamptz AS window_to, as_of, knowledge_seq, timezone, input_hash, loading, started_at, published_at,
    superseded_at, irreproducible_reason, previous_revision_id
FROM calc.revision WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: TouchRevision :execrows
UPDATE calc.revision SET progress_at = sqlc.arg(now)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'building';

-- name: FailRevision :execrows
UPDATE calc.revision SET status = 'failed'
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'building';

-- name: PublishRevision :execrows
UPDATE calc.revision SET status = 'published', published_at = sqlc.arg(now)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'building';

-- name: SupersedeRevision :exec
UPDATE calc.revision SET status = 'superseded', superseded_at = sqlc.arg(now)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'published';

-- name: MoveCurrentRevision :execrows
UPDATE calc.series SET current_revision_id = sqlc.arg(revision_id), consumed_seq = GREATEST(consumed_seq, sqlc.arg(knowledge_seq)::bigint)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(series_id)
    AND current_revision_id IS NOT DISTINCT FROM sqlc.narg(previous_revision_id)::uuid;

-- name: UpsertPart :one
INSERT INTO calc.part (organization_id, id, input_hash, metric_set, part_range, created_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(input_hash), sqlc.arg(metric_set),
    tstzrange(sqlc.arg(part_from)::timestamptz, sqlc.arg(part_to)::timestamptz, '[)'), sqlc.arg(created_at))
ON CONFLICT (organization_id, input_hash) DO UPDATE SET input_hash = EXCLUDED.input_hash
RETURNING id;

-- name: LinkPart :exec
INSERT INTO calc.revision_part (organization_id, revision_id, part_no, part_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(revision_id), sqlc.arg(part_no), sqlc.arg(part_id))
ON CONFLICT DO NOTHING;

-- name: InsertPoints :exec
INSERT INTO calc.point (organization_id, part_id, metric_id, estimator, point_start, point_end, value, unit, numerator, denominator, n,
    state, reason_code, bound_kind, bound_value, excluded_count, missing_count, low_sample)
SELECT sqlc.arg(organization_id), sqlc.arg(part_id), (sqlc.arg(metric_ids)::text[])[i], (sqlc.arg(estimators)::text[])[i],
    (sqlc.arg(starts)::timestamptz[])[i], (sqlc.arg(ends)::timestamptz[])[i],
    CASE WHEN (sqlc.arg(has_values)::boolean[])[i] THEN (sqlc.arg(point_values)::text[])[i]::numeric END,
    (sqlc.arg(units)::text[])[i],
    CASE WHEN (sqlc.arg(has_numerators)::boolean[])[i] THEN (sqlc.arg(numerators)::text[])[i]::numeric END,
    CASE WHEN (sqlc.arg(has_denominators)::boolean[])[i] THEN (sqlc.arg(denominators)::text[])[i]::numeric END,
    CASE WHEN (sqlc.arg(has_ns)::boolean[])[i] THEN (sqlc.arg(ns)::integer[])[i] END,
    (sqlc.arg(states)::text[])[i], NULLIF((sqlc.arg(reasons)::text[])[i], ''), NULLIF((sqlc.arg(bound_kinds)::text[])[i], ''),
    CASE WHEN (sqlc.arg(bound_kinds)::text[])[i] <> '' THEN (sqlc.arg(bound_values)::text[])[i]::numeric END,
    (sqlc.arg(excluded)::integer[])[i], (sqlc.arg(missing)::integer[])[i], (sqlc.arg(low_samples)::boolean[])[i]
FROM generate_subscripts(sqlc.arg(metric_ids)::text[], 1) AS i;

-- name: InsertContributions :exec
INSERT INTO calc.contribution (organization_id, part_id, metric_id, estimator, point_start, object_id, item_no, object_kind, role, value,
    reason_code)
SELECT sqlc.arg(organization_id), sqlc.arg(part_id), (sqlc.arg(metric_ids)::text[])[i], (sqlc.arg(estimators)::text[])[i],
    (sqlc.arg(starts)::timestamptz[])[i], (sqlc.arg(object_ids)::uuid[])[i], (sqlc.arg(item_nos)::smallint[])[i],
    (sqlc.arg(kinds)::text[])[i], (sqlc.arg(roles)::text[])[i],
    CASE WHEN (sqlc.arg(has_values)::boolean[])[i] THEN (sqlc.arg(contribution_values)::text[])[i]::numeric END,
    NULLIF((sqlc.arg(reasons)::text[])[i], '')
FROM generate_subscripts(sqlc.arg(metric_ids)::text[], 1) AS i;

-- name: InsertIntervals :exec
INSERT INTO calc.interval (organization_id, part_id, task_id, episode_no, interval_no, class, during, source_event_ids)
SELECT sqlc.arg(organization_id), sqlc.arg(part_id), (sqlc.arg(task_ids)::uuid[])[i], (sqlc.arg(episode_nos)::integer[])[i],
    (sqlc.arg(interval_nos)::integer[])[i], (sqlc.arg(classes)::text[])[i],
    tstzrange((sqlc.arg(froms)::timestamptz[])[i], (sqlc.arg(tos)::timestamptz[])[i], '[)'),
    ARRAY(SELECT jsonb_array_elements_text(((sqlc.arg(sources)::text[])[i])::jsonb)::uuid)
FROM generate_subscripts(sqlc.arg(task_ids)::uuid[], 1) AS i;

-- name: InsertTaskSlices :exec
INSERT INTO calc.task_slice (organization_id, part_id, task_id, project_id, task_type, size_label)
SELECT sqlc.arg(organization_id), sqlc.arg(part_id), (sqlc.arg(task_ids)::uuid[])[i], (sqlc.arg(project_ids)::uuid[])[i],
    NULLIF((sqlc.arg(task_types)::text[])[i], ''), NULLIF((sqlc.arg(size_labels)::text[])[i], '')
FROM generate_subscripts(sqlc.arg(task_ids)::uuid[], 1) AS i;

-- name: RevisionPoints :many
SELECT p.metric_id, p.estimator, p.point_start, p.point_end, COALESCE(p.value, 0)::float8 AS value, (p.value IS NOT NULL)::boolean AS has_value,
    p.unit, COALESCE(p.numerator, 0)::float8 AS numerator, (p.numerator IS NOT NULL)::boolean AS has_numerator,
    COALESCE(p.denominator, 0)::float8 AS denominator, (p.denominator IS NOT NULL)::boolean AS has_denominator, p.n, p.state, p.reason_code,
    p.bound_kind, COALESCE(p.bound_value, 0)::float8 AS bound_value, p.excluded_count, p.missing_count, p.low_sample
FROM calc.revision_part rp
JOIN calc.point p ON p.organization_id = rp.organization_id AND p.part_id = rp.part_id
WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.revision_id = sqlc.arg(revision_id)
ORDER BY p.metric_id, p.estimator, p.point_start;

-- name: PointContributions :many
SELECT c.object_id, c.item_no, c.object_kind, c.role, COALESCE(c.value, 0)::float8 AS value, (c.value IS NOT NULL)::boolean AS has_value,
    c.reason_code
FROM calc.revision_part rp
JOIN calc.contribution c ON c.organization_id = rp.organization_id AND c.part_id = rp.part_id
WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.revision_id = sqlc.arg(revision_id) AND c.metric_id = sqlc.arg(metric_id)
    AND c.estimator = sqlc.arg(estimator) AND c.point_start = sqlc.arg(point_start)
    AND (c.object_id, c.item_no) > (sqlc.arg(after_object_id)::uuid, sqlc.arg(after_item_no)::smallint)
ORDER BY c.object_id, c.item_no
LIMIT sqlc.arg(row_limit);

-- name: TaskIntervals :many
SELECT i.episode_no, i.interval_no, i.class, lower(i.during)::timestamptz AS during_from, upper(i.during)::timestamptz AS during_to,
    i.source_event_ids
FROM calc.revision_part rp
JOIN calc.interval i ON i.organization_id = rp.organization_id AND i.part_id = rp.part_id
WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.revision_id = sqlc.arg(revision_id) AND i.task_id = sqlc.arg(task_id)
ORDER BY i.episode_no, i.interval_no, lower(i.during);

-- name: MarkBasisDeleted :execrows
UPDATE calc.revision r SET irreproducible_reason = 'basis_deleted'
WHERE r.organization_id = sqlc.arg(organization_id) AND r.irreproducible_reason IS NULL AND r.id IN (
    SELECT rp.revision_id FROM calc.contribution c
    JOIN calc.revision_part rp ON rp.organization_id = c.organization_id AND rp.part_id = c.part_id
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.object_id = sqlc.arg(object_id));

-- name: MarkBasisExpired :execrows
UPDATE calc.revision SET irreproducible_reason = 'basis_expired'
WHERE organization_id = sqlc.arg(organization_id) AND irreproducible_reason IS NULL AND status IN ('published', 'superseded')
    AND knowledge_seq < sqlc.arg(reproducible_from)::bigint;

-- name: MarkDefinitionRetired :execrows
UPDATE calc.revision r SET irreproducible_reason = 'definition_retired'
WHERE r.organization_id = sqlc.arg(organization_id) AND r.irreproducible_reason IS NULL AND EXISTS (
    SELECT 1 FROM calc.revision_input i
    WHERE i.organization_id = r.organization_id AND i.revision_id = r.id AND i.input_kind = 'definition'
        AND NOT (i.input_id || '@' || COALESCE(i.version, '') = ANY (sqlc.arg(current)::text[])));

-- name: UnusedParts :many
SELECT p.id FROM calc.part p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.id > sqlc.arg(after_id)::uuid AND p.created_at < sqlc.arg(before)
    AND NOT EXISTS (
        SELECT 1 FROM calc.revision_part rp
        JOIN calc.revision r ON r.organization_id = rp.organization_id AND r.id = rp.revision_id
        WHERE rp.organization_id = p.organization_id AND rp.part_id = p.id AND r.status <> 'failed')
ORDER BY p.id
LIMIT sqlc.arg(row_limit)
FOR UPDATE OF p SKIP LOCKED;

-- name: DeletePartLinks :exec
DELETE FROM calc.revision_part WHERE organization_id = sqlc.arg(organization_id) AND part_id = ANY (sqlc.arg(part_ids)::uuid[]);

-- name: DeleteParts :exec
DELETE FROM calc.part WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(part_ids)::uuid[]);

-- name: ScopeSeries :many
SELECT * FROM calc.series
WHERE organization_id = sqlc.arg(organization_id) AND scope_hash = sqlc.arg(scope_hash) AND metric_set = sqlc.arg(metric_set)
ORDER BY step;

-- name: PointSamples :many
SELECT c.object_id, c.role, COALESCE(c.value, 0)::float8 AS value, (c.value IS NOT NULL)::boolean AS has_value, c.reason_code,
    s.task_type, s.size_label
FROM calc.revision_part rp
JOIN calc.point p ON p.organization_id = rp.organization_id AND p.part_id = rp.part_id
JOIN calc.contribution c ON c.organization_id = p.organization_id AND c.part_id = p.part_id AND c.metric_id = p.metric_id
    AND c.estimator = p.estimator AND c.point_start = p.point_start
LEFT JOIN calc.task_slice s ON s.organization_id = c.organization_id AND s.part_id = c.part_id AND s.task_id = c.object_id
WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.revision_id = sqlc.arg(revision_id) AND p.metric_id = sqlc.arg(metric_id)
    AND p.estimator = sqlc.arg(estimator) AND p.point_start >= sqlc.arg(from_at) AND p.point_end <= sqlc.arg(to_at)
ORDER BY p.point_start, c.object_id, c.item_no;
