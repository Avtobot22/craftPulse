-- name: InsertObjects :exec
INSERT INTO normalize.source_object (organization_id, instance_id, object_kind, native_ns, native_id, identity_quality, first_known_seq, restricted)
SELECT sqlc.arg(organization_id)::uuid, (sqlc.arg(instance_ids)::uuid[])[i], (sqlc.arg(kinds)::text[])[i], (sqlc.arg(native_nss)::text[])[i],
    (sqlc.arg(native_ids)::text[])[i], (sqlc.arg(qualities)::text[])[i], sqlc.arg(known_seq)::bigint, false
FROM generate_subscripts(sqlc.arg(native_ids)::text[], 1) AS i
ORDER BY 2, 3, 4, 5
ON CONFLICT (organization_id, instance_id, object_kind, native_ns, native_id) DO NOTHING;

-- name: GetObjects :many
SELECT o.id, o.instance_id, o.object_kind, o.native_ns, o.native_id, o.identity_quality, o.first_known_seq, o.current_area_id,
    o.restricted, o.deleted_at
FROM normalize.source_object o
JOIN (
    SELECT (sqlc.arg(instance_ids)::uuid[])[i] AS instance_id, (sqlc.arg(kinds)::text[])[i] AS object_kind,
        (sqlc.arg(native_nss)::text[])[i] AS native_ns, (sqlc.arg(native_ids)::text[])[i] AS native_id
    FROM generate_subscripts(sqlc.arg(native_ids)::text[], 1) AS i
) k ON o.instance_id = k.instance_id AND o.object_kind = k.object_kind AND o.native_ns = k.native_ns AND o.native_id = k.native_id
WHERE o.organization_id = sqlc.arg(organization_id);

-- name: InsertAreas :exec
INSERT INTO normalize.area (organization_id, area_id, area_kind, last_change_seq)
SELECT sqlc.arg(organization_id)::uuid, (sqlc.arg(area_ids)::uuid[])[i], (sqlc.arg(kinds)::text[])[i], 0
FROM generate_subscripts(sqlc.arg(area_ids)::uuid[], 1) AS i
ORDER BY 2
ON CONFLICT (organization_id, area_id) DO NOTHING;

-- name: PlaceObjects :exec
UPDATE normalize.source_object o SET current_area_id = k.area_id, restricted = k.restricted
FROM (
    SELECT (sqlc.arg(object_ids)::uuid[])[i] AS id, (sqlc.arg(area_ids)::uuid[])[i] AS area_id, (sqlc.arg(restricted)::bool[])[i] AS restricted
    FROM generate_subscripts(sqlc.arg(object_ids)::uuid[], 1) AS i
) k
WHERE o.organization_id = sqlc.arg(organization_id) AND o.id = k.id;

-- name: RecordAreaChange :batchexec
INSERT INTO normalize.area_change (organization_id, area_id, known_seq, earliest_event_time, fact_kinds, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (organization_id, area_id, known_seq) DO UPDATE
SET earliest_event_time = LEAST(normalize.area_change.earliest_event_time, EXCLUDED.earliest_event_time),
    fact_kinds = ARRAY(SELECT DISTINCT k FROM unnest(normalize.area_change.fact_kinds || EXCLUDED.fact_kinds) AS k ORDER BY k);

-- name: SetAreaChangeSeq :exec
UPDATE normalize.area SET last_change_seq = sqlc.arg(known_seq)
WHERE organization_id = sqlc.arg(organization_id) AND area_id = ANY (sqlc.arg(area_ids)::uuid[]);

-- name: AreaChanges :many
SELECT area_id, known_seq, earliest_event_time, fact_kinds FROM normalize.area_change
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND known_seq > sqlc.arg(after_seq)
ORDER BY known_seq;

-- name: CurrentTaskVersions :many
SELECT * FROM normalize.task_version
WHERE organization_id = sqlc.arg(organization_id) AND task_id = ANY (sqlc.arg(task_ids)::uuid[]) AND superseded_seq IS NULL;

-- name: TaskRevisions :many
SELECT task_id, source_revision, content_hash FROM normalize.task_version
WHERE organization_id = sqlc.arg(organization_id) AND task_id = ANY (sqlc.arg(task_ids)::uuid[]);

-- name: SupersedeTaskVersions :exec
UPDATE normalize.task_version SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertTaskVersion :batchexec
INSERT INTO normalize.task_version (organization_id, task_id, area_id, known_seq, receipt_id, record_no, source_revision, content_hash,
    observed_at, source_updated_at, task_key, title, type_native_id, status_native_id, resolution_native_id, created_src_at, size_native,
    team_native, assignee_account_id, parent_task_id, restriction_native, raw_fields)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22);

-- name: CurrentTaskEvents :many
SELECT e.* FROM normalize.task_event e
JOIN (
    SELECT (sqlc.arg(task_ids)::uuid[])[i] AS task_id, (sqlc.arg(event_keys)::text[])[i] AS event_key
    FROM generate_subscripts(sqlc.arg(task_ids)::uuid[], 1) AS i
) k ON e.task_id = k.task_id AND e.event_key = k.event_key
WHERE e.organization_id = sqlc.arg(organization_id) AND e.superseded_seq IS NULL;

-- name: TaskEventRevisions :many
SELECT e.task_id, e.event_key, e.source_revision, e.content_hash FROM normalize.task_event e
JOIN (
    SELECT (sqlc.arg(task_ids)::uuid[])[i] AS task_id, (sqlc.arg(event_keys)::text[])[i] AS event_key
    FROM generate_subscripts(sqlc.arg(task_ids)::uuid[], 1) AS i
) k ON e.task_id = k.task_id AND e.event_key = k.event_key
WHERE e.organization_id = sqlc.arg(organization_id);

-- name: SupersedeTaskEvents :exec
UPDATE normalize.task_event SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertTaskEvent :batchexec
INSERT INTO normalize.task_event (organization_id, task_id, area_id, event_key, source_revision, content_hash, known_seq, receipt_id,
    record_no, event_kind, field_native_id, before_value, after_value, before_account_id, after_account_id, actor_account_id, event_time,
    event_offset_s, observed_at, observed_via)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20);

-- name: CurrentDictionaryEntries :many
SELECT d.* FROM normalize.dictionary_entry d
JOIN (
    SELECT (sqlc.arg(area_ids)::uuid[])[i] AS area_id, (sqlc.arg(kinds)::text[])[i] AS entry_kind, (sqlc.arg(native_ids)::text[])[i] AS native_id
    FROM generate_subscripts(sqlc.arg(area_ids)::uuid[], 1) AS i
) k ON d.area_id = k.area_id AND d.entry_kind = k.entry_kind AND d.native_id = k.native_id
WHERE d.organization_id = sqlc.arg(organization_id) AND d.superseded_seq IS NULL;

-- name: DictionaryRevisions :many
SELECT d.area_id, d.entry_kind, d.native_id, d.source_revision, d.content_hash FROM normalize.dictionary_entry d
JOIN (
    SELECT (sqlc.arg(area_ids)::uuid[])[i] AS area_id, (sqlc.arg(kinds)::text[])[i] AS entry_kind, (sqlc.arg(native_ids)::text[])[i] AS native_id
    FROM generate_subscripts(sqlc.arg(area_ids)::uuid[], 1) AS i
) k ON d.area_id = k.area_id AND d.entry_kind = k.entry_kind AND d.native_id = k.native_id
WHERE d.organization_id = sqlc.arg(organization_id);

-- name: SupersedeDictionaryEntries :exec
UPDATE normalize.dictionary_entry SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertDictionaryEntry :batchexec
INSERT INTO normalize.dictionary_entry (organization_id, area_id, entry_kind, native_id, source_revision, content_hash, known_seq, receipt_id,
    record_no, name, attributes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: DictionaryOfArea :many
SELECT * FROM normalize.dictionary_entry
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND entry_kind = sqlc.arg(entry_kind)
    AND superseded_seq IS NULL
ORDER BY native_id;

-- name: TaskVersionsAt :many
SELECT * FROM normalize.task_version
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND known_seq <= sqlc.arg(known_seq)
    AND (superseded_seq IS NULL OR superseded_seq > sqlc.arg(known_seq))
ORDER BY task_id;

-- name: TaskEventsAt :many
SELECT * FROM normalize.task_event
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND known_seq <= sqlc.arg(known_seq)
    AND (superseded_seq IS NULL OR superseded_seq > sqlc.arg(known_seq))
ORDER BY event_time, id;

-- name: CurrentTasksByNative :many
SELECT sqlc.embed(v), o.native_id FROM normalize.task_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.task_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id) AND v.superseded_seq IS NULL
    AND o.native_id = ANY (sqlc.arg(native_ids)::text[])
ORDER BY o.native_id;

-- name: AreaOf :one
SELECT a.area_id FROM normalize.area a
JOIN normalize.source_object o ON o.organization_id = a.organization_id AND o.id = a.area_id
WHERE o.organization_id = sqlc.arg(organization_id) AND o.instance_id = sqlc.arg(instance_id) AND o.object_kind = sqlc.arg(object_kind)
    AND o.native_ns = '' AND o.native_id = sqlc.arg(native_id);
