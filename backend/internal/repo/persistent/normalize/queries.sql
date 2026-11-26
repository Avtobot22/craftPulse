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

-- name: AreaChangeSeq :one
SELECT COALESCE(max(last_change_seq), 0)::bigint FROM normalize.area
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
SELECT v.* FROM normalize.task_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.task_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id) AND v.known_seq <= sqlc.arg(known_seq)
    AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
    AND (NOT o.restricted OR o.id = ANY (sqlc.arg(allowed)::uuid[]))
ORDER BY v.task_id;

-- name: TaskEventsAt :many
SELECT e.* FROM normalize.task_event e
JOIN normalize.source_object o ON o.organization_id = e.organization_id AND o.id = e.task_id
WHERE e.organization_id = sqlc.arg(organization_id) AND e.area_id = sqlc.arg(area_id) AND e.known_seq <= sqlc.arg(known_seq)
    AND (e.superseded_seq IS NULL OR e.superseded_seq > sqlc.arg(known_seq))
    AND (NOT o.restricted OR o.id = ANY (sqlc.arg(allowed)::uuid[]))
ORDER BY e.event_time, e.id;

-- name: ReadTaskViewVersion :many
SELECT v.* FROM normalize.task_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.task_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id) AND v.task_id = sqlc.arg(task_id)
    AND v.known_seq <= sqlc.arg(known_seq)
    AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
    AND o.current_area_id = v.area_id AND o.deleted_at IS NULL
    AND (NOT o.restricted OR o.id = ANY (sqlc.arg(allowed)::uuid[]));

-- name: ReadTaskViewEvents :many
SELECT e.* FROM normalize.task_event e
JOIN normalize.source_object o ON o.organization_id = e.organization_id AND o.id = e.task_id
WHERE e.organization_id = sqlc.arg(organization_id) AND e.area_id = sqlc.arg(area_id) AND e.task_id = sqlc.arg(task_id)
    AND e.known_seq <= sqlc.arg(known_seq)
    AND (e.superseded_seq IS NULL OR e.superseded_seq > sqlc.arg(known_seq))
    AND o.current_area_id = e.area_id AND o.deleted_at IS NULL
    AND (NOT o.restricted OR o.id = ANY (sqlc.arg(allowed)::uuid[]))
ORDER BY e.event_time, e.id;

-- name: RestrictedObjects :many
SELECT id, current_area_id::uuid AS area_id, object_kind, native_ns, native_id FROM normalize.source_object
WHERE organization_id = sqlc.arg(organization_id) AND current_area_id = ANY (sqlc.arg(area_ids)::uuid[])
    AND restricted AND deleted_at IS NULL
ORDER BY current_area_id, id;

-- name: LinkEnds :many
SELECT o.id, o.object_kind, o.instance_id, o.current_area_id, o.restricted, o.native_ns, o.native_id, t.task_key, t.title
FROM normalize.source_object o
LEFT JOIN normalize.task_version t ON t.organization_id = o.organization_id AND t.task_id = o.id AND t.superseded_seq IS NULL
WHERE o.organization_id = sqlc.arg(organization_id) AND o.id = ANY (sqlc.arg(ids)::uuid[]) AND o.deleted_at IS NULL;

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

-- name: AreaInstances :many
SELECT id, instance_id FROM normalize.source_object
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(area_ids)::uuid[]);

-- name: AreaProvider :one
SELECT i.provider FROM normalize.source_object o
JOIN connect.source_instance i ON i.organization_id = o.organization_id AND i.id = o.instance_id
WHERE o.organization_id = sqlc.arg(organization_id) AND o.id = sqlc.arg(area_id);

-- name: DictionariesOfAreas :many
SELECT area_id, entry_kind, native_id, name, attributes FROM normalize.dictionary_entry
WHERE organization_id = sqlc.arg(organization_id) AND area_id = ANY (sqlc.arg(area_ids)::uuid[]) AND superseded_seq IS NULL
ORDER BY area_id, entry_kind, native_id;

-- name: RepositoryAreas :many
SELECT area_id FROM normalize.area WHERE organization_id = $1 AND area_id = ANY(sqlc.arg(ids)::uuid[]) AND area_kind = 'repository';

-- name: CurrentRepositoryVersions :many
SELECT * FROM normalize.repository_version WHERE organization_id = sqlc.arg(organization_id) AND repository_id = ANY (sqlc.arg(object_ids)::uuid[]) AND superseded_seq IS NULL;

-- name: KnownRepositoryVersions :many
SELECT repository_id, source_revision, content_hash FROM normalize.repository_version
WHERE organization_id = sqlc.arg(organization_id) AND repository_id = ANY (sqlc.arg(object_ids)::uuid[]);

-- name: SupersedeRepositoryVersions :exec
UPDATE normalize.repository_version SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertRepositoryVersions :batchexec
INSERT INTO normalize.repository_version (organization_id, repository_id, source_updated_at, path, name, default_branch, visibility, web_url, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: CurrentMRVersions :many
SELECT * FROM normalize.mr_version WHERE organization_id = sqlc.arg(organization_id) AND mr_id = ANY (sqlc.arg(object_ids)::uuid[]) AND superseded_seq IS NULL;

-- name: KnownMRVersions :many
SELECT mr_id, source_revision, content_hash FROM normalize.mr_version
WHERE organization_id = sqlc.arg(organization_id) AND mr_id = ANY (sqlc.arg(object_ids)::uuid[]);

-- name: SupersedeMRVersions :exec
UPDATE normalize.mr_version SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertMRVersions :batchexec
INSERT INTO normalize.mr_version (organization_id, mr_id, area_id, source_updated_at, iid, title, state, draft, author_account_id, source_branch, target_branch, head_sha, base_sha, merge_base_sha, reviewer_field, reviewer_account_ids, opened_at, ready_at, merged_at, closed_at, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26);

-- name: CurrentMREvents :many
SELECT * FROM normalize.mr_event WHERE organization_id = sqlc.arg(organization_id) AND (mr_id, event_key) IN (
    SELECT (sqlc.arg(mr_ids)::uuid[])[i], (sqlc.arg(fact_keys)::text[])[i]
    FROM generate_subscripts(sqlc.arg(mr_ids)::uuid[], 1) AS i) AND superseded_seq IS NULL;

-- name: KnownMREvents :many
SELECT mr_id, event_key, source_revision, content_hash FROM normalize.mr_event
WHERE organization_id = sqlc.arg(organization_id) AND (mr_id, event_key) IN (
    SELECT (sqlc.arg(mr_ids)::uuid[])[i], (sqlc.arg(fact_keys)::text[])[i]
    FROM generate_subscripts(sqlc.arg(mr_ids)::uuid[], 1) AS i);

-- name: SupersedeMREvents :exec
UPDATE normalize.mr_event SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertMREvents :batchexec
INSERT INTO normalize.mr_event (organization_id, mr_id, event_key, event_kind, actor_account_id, target_account_id, head_sha, review_state, event_time, event_offset_s, observed_via, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

-- name: CurrentMRRevisions :many
SELECT * FROM normalize.mr_revision WHERE organization_id = sqlc.arg(organization_id) AND (mr_id, revision_key) IN (
    SELECT (sqlc.arg(mr_ids)::uuid[])[i], (sqlc.arg(fact_keys)::text[])[i]
    FROM generate_subscripts(sqlc.arg(mr_ids)::uuid[], 1) AS i) AND superseded_seq IS NULL;

-- name: KnownMRRevisions :many
SELECT mr_id, revision_key, source_revision, content_hash FROM normalize.mr_revision
WHERE organization_id = sqlc.arg(organization_id) AND (mr_id, revision_key) IN (
    SELECT (sqlc.arg(mr_ids)::uuid[])[i], (sqlc.arg(fact_keys)::text[])[i]
    FROM generate_subscripts(sqlc.arg(mr_ids)::uuid[], 1) AS i);

-- name: SupersedeMRRevisions :exec
UPDATE normalize.mr_revision SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertMRRevisions :batchexec
INSERT INTO normalize.mr_revision (organization_id, mr_id, revision_key, head_sha, base_sha, start_sha, created_at, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ReadMRVersions :many
SELECT v.* FROM normalize.mr_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.mr_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.mr_id = ANY(sqlc.arg(mr_ids)::uuid[])
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
ORDER BY v.mr_id;

-- name: ReadMRRevisions :many
SELECT v.* FROM normalize.mr_revision v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.mr_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.mr_id = ANY(sqlc.arg(mr_ids)::uuid[])
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
ORDER BY v.created_at, v.id;

-- name: ReadMREvents :many
SELECT v.* FROM normalize.mr_event v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.mr_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.mr_id = ANY(sqlc.arg(mr_ids)::uuid[])
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
ORDER BY v.event_time, v.id;

-- name: FindMRIDs :many
SELECT o.id FROM normalize.source_object o
WHERE o.organization_id = sqlc.arg(organization_id) AND o.object_kind = 'merge_request'
    AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND (sqlc.arg(native_id)::text = '' OR (o.instance_id = sqlc.arg(instance_id)::uuid AND o.native_ns = sqlc.arg(native_ns)::text AND o.native_id = sqlc.arg(native_id)))
    AND (sqlc.arg(head_sha)::text = '' OR EXISTS (
        SELECT 1 FROM normalize.mr_version v WHERE v.organization_id = o.organization_id AND v.mr_id = o.id
            AND v.head_sha = sqlc.arg(head_sha) AND v.known_seq <= sqlc.arg(known_seq)
            AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
    ) OR EXISTS (
        SELECT 1 FROM normalize.mr_revision r WHERE r.organization_id = o.organization_id AND r.mr_id = o.id
            AND r.head_sha = sqlc.arg(head_sha) AND r.known_seq <= sqlc.arg(known_seq)
            AND (r.superseded_seq IS NULL OR r.superseded_seq > sqlc.arg(known_seq))
    ))
ORDER BY o.id;

-- name: ReadRepositoryVersion :one
SELECT v.* FROM normalize.repository_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.repository_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.repository_id = sqlc.arg(repository_id) AND o.deleted_at IS NULL
    AND v.superseded_seq IS NULL;

-- name: CurrentCIRunVersions :many
SELECT * FROM normalize.ci_run_version WHERE organization_id = sqlc.arg(organization_id) AND run_id = ANY (sqlc.arg(object_ids)::uuid[]) AND superseded_seq IS NULL;

-- name: KnownCIRunVersions :many
SELECT run_id, source_revision, content_hash FROM normalize.ci_run_version
WHERE organization_id = sqlc.arg(organization_id) AND run_id = ANY (sqlc.arg(object_ids)::uuid[]);

-- name: SupersedeCIRunVersions :exec
UPDATE normalize.ci_run_version SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertCIRunVersions :batchexec
INSERT INTO normalize.ci_run_version (organization_id, run_id, area_id, mr_id, source_updated_at, ref_name, sha, parent_run_id, definition_version, native_status, outcome, started_at, finished_at, attempt_history, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at, allow_failure_semantics, observed_via, start_offset_s, finish_offset_s)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24);

-- name: CurrentCIAttemptVersions :many
SELECT * FROM normalize.ci_attempt_version WHERE organization_id = sqlc.arg(organization_id) AND attempt_id = ANY (sqlc.arg(object_ids)::uuid[]) AND superseded_seq IS NULL;

-- name: KnownCIAttemptVersions :many
SELECT attempt_id, source_revision, content_hash FROM normalize.ci_attempt_version
WHERE organization_id = sqlc.arg(organization_id) AND attempt_id = ANY (sqlc.arg(object_ids)::uuid[]);

-- name: SupersedeCIAttemptVersions :exec
UPDATE normalize.ci_attempt_version SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertCIAttemptVersions :batchexec
INSERT INTO normalize.ci_attempt_version (organization_id, attempt_id, run_id, job_key, config_fingerprint, attempt_no, native_status, outcome, allow_failure, failure_kind, runner_class, started_at, finished_at, duration_s, known_seq, receipt_id, record_no, source_revision, content_hash, observed_at, observed_via, start_offset_s, finish_offset_s)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23);

-- name: MatchRunMRs :many
SELECT i::integer AS ord, o.id FROM generate_subscripts(sqlc.arg(mr_native_ids)::text[], 1) AS i
JOIN normalize.source_object o ON o.organization_id = sqlc.arg(organization_id) AND o.instance_id = sqlc.arg(instance_id)::uuid
    AND o.object_kind = 'merge_request' AND o.native_ns = '' AND o.native_id = (sqlc.arg(mr_native_ids)::text[])[i]
WHERE o.deleted_at IS NULL AND o.current_area_id = (sqlc.arg(area_ids)::uuid[])[i]
    AND (EXISTS (
        SELECT 1 FROM normalize.mr_revision r WHERE r.organization_id = o.organization_id AND r.mr_id = o.id AND r.head_sha = (sqlc.arg(shas)::text[])[i]
    ) OR EXISTS (
        SELECT 1 FROM normalize.mr_version v WHERE v.organization_id = o.organization_id AND v.mr_id = o.id AND v.head_sha = (sqlc.arg(shas)::text[])[i]
    ));

-- name: ReadCIRunVersions :many
SELECT v.* FROM normalize.ci_run_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.run_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.run_id = ANY(sqlc.arg(run_ids)::uuid[])
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
ORDER BY v.started_at NULLS LAST, v.run_id;

-- name: FindCIRunIDs :many
SELECT v.run_id FROM normalize.ci_run_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.run_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
    AND (sqlc.narg(mr_id)::uuid IS NULL OR v.mr_id = sqlc.narg(mr_id))
    AND (sqlc.narg(parent_run_ids)::uuid[] IS NULL OR v.parent_run_id = ANY(sqlc.narg(parent_run_ids)::uuid[]))
    AND (sqlc.narg(started_from)::timestamptz IS NULL OR v.started_at >= sqlc.narg(started_from))
    AND (sqlc.narg(started_before)::timestamptz IS NULL OR v.started_at < sqlc.narg(started_before))
ORDER BY v.run_id;

-- name: ReadCIAttemptVersions :many
SELECT v.* FROM normalize.ci_attempt_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.run_id
WHERE v.organization_id = sqlc.arg(organization_id) AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
    AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    AND v.run_id = ANY(sqlc.arg(run_ids)::uuid[])
    AND v.known_seq <= sqlc.arg(known_seq) AND (v.superseded_seq IS NULL OR v.superseded_seq > sqlc.arg(known_seq))
ORDER BY v.job_key, v.config_fingerprint, v.attempt_no, v.attempt_id;

-- name: LinkMRVersions :many
SELECT v.* FROM normalize.mr_version v
JOIN normalize.source_object o ON o.organization_id=v.organization_id AND o.id=v.mr_id
WHERE v.organization_id=$1 AND v.area_id=ANY(sqlc.arg(areas)::uuid[])
 AND v.superseded_seq IS NULL AND o.deleted_at IS NULL
 AND (v.mr_id=ANY(sqlc.arg(mr_ids)::uuid[]) OR EXISTS (
  SELECT 1 FROM unnest(sqlc.arg(task_keys)::text[]) AS k(value)
  WHERE strpos(v.title,k.value)>0 OR strpos(v.source_branch,k.value)>0
 )) ORDER BY v.mr_id;

-- name: LinkTaskVersions :many
SELECT v.* FROM normalize.task_version v
JOIN normalize.source_object o ON o.organization_id=v.organization_id AND o.id=v.task_id
WHERE v.organization_id=$1 AND v.area_id=ANY(sqlc.arg(areas)::uuid[])
 AND v.task_key=ANY(sqlc.arg(keys)::text[]) AND v.superseded_seq IS NULL AND o.deleted_at IS NULL ORDER BY v.task_id;

-- name: WebhookTargets :many
SELECT (k.key->>'object_kind')::text AS object_kind, (k.key->>'native_ns')::text AS input_ns, (k.key->>'native_id')::text AS input_id,
    COALESCE(mr.native_ns, (k.key->>'native_ns'))::text AS native_ns, COALESCE(mr.native_id, (k.key->>'native_id'))::text AS native_id,
    COALESCE(v.state, '')::text AS state, COALESCE(area.native_id, (k.key->>'native_ns'))::text AS scope_native_id
FROM jsonb_array_elements(sqlc.arg(keys)::jsonb) AS k(key)
LEFT JOIN normalize.source_object obj ON obj.organization_id = sqlc.arg(organization_id) AND obj.instance_id = sqlc.arg(instance_id)
    AND obj.object_kind = (k.key->>'object_kind') AND obj.native_ns = (k.key->>'native_ns') AND obj.native_id = (k.key->>'native_id') AND obj.deleted_at IS NULL
LEFT JOIN normalize.ci_run_version run ON run.organization_id = obj.organization_id AND run.run_id = obj.id AND run.superseded_seq IS NULL
LEFT JOIN normalize.source_object mr ON mr.organization_id = obj.organization_id AND mr.instance_id = obj.instance_id
    AND mr.id = CASE WHEN obj.object_kind = 'merge_request' THEN obj.id ELSE run.mr_id END AND mr.deleted_at IS NULL
LEFT JOIN normalize.mr_version v ON v.organization_id = mr.organization_id AND v.mr_id = mr.id AND v.superseded_seq IS NULL
LEFT JOIN normalize.source_object area ON area.organization_id = obj.organization_id AND area.id = COALESCE(v.area_id, obj.current_area_id);

-- name: CurrentSyncVersions :many
SELECT obj.object_kind, obj.native_ns, obj.native_id, v.source_revision, v.source_updated_at
FROM normalize.source_object obj
JOIN normalize.mr_version v ON v.organization_id = obj.organization_id AND v.mr_id = obj.id AND v.superseded_seq IS NULL
WHERE obj.organization_id = $1 AND v.area_id = $2 AND obj.native_id = ANY($3::text[]) AND obj.deleted_at IS NULL
UNION ALL
SELECT obj.object_kind, obj.native_ns, obj.native_id, v.source_revision, v.source_updated_at
FROM normalize.source_object obj
JOIN normalize.ci_run_version v ON v.organization_id = obj.organization_id AND v.run_id = obj.id AND v.superseded_seq IS NULL
WHERE obj.organization_id = $1 AND v.area_id = $2 AND obj.native_id = ANY($3::text[]) AND obj.deleted_at IS NULL;

-- name: ReportedRecords :many
SELECT record_no FROM normalize.report
WHERE organization_id = sqlc.arg(organization_id) AND receipt_id = sqlc.arg(receipt_id) AND record_no = ANY (sqlc.arg(record_nos)::integer[]);

-- name: InsertDocsReport :exec
INSERT INTO normalize.docs_report (organization_id, report_id, root_id, path_prefix, owner_rule, checks, links, sarif_hash)
VALUES (sqlc.arg(organization_id), sqlc.arg(report_id), sqlc.narg(root_id), sqlc.arg(path_prefix),
    sqlc.narg(owner_rule), sqlc.arg(checks), sqlc.narg(links), sqlc.narg(sarif_hash));

-- name: InsertDocsReportDocuments :execrows
WITH input AS (
    SELECT (doc.value->>'document_id')::uuid AS document_id, doc.value->>'path' AS path,
        doc.value->>'version_ref' AS version_ref, doc.value->>'format' AS format, doc.value->>'title' AS title,
        doc.value->>'owner' AS owner, doc.value->>'acl_state' AS acl_state, (doc.value->>'spec_valid')::boolean AS spec_valid,
        decode(doc.value->>'content_hash', 'base64') AS hash
    FROM jsonb_array_elements(sqlc.arg(documents)::jsonb) AS doc(value)
), inserted AS (
    INSERT INTO normalize.document_version
        (organization_id, document_id, area_id, known_seq, version_ref, doc_format, path, title, content_hash, owner, acl_state)
    SELECT sqlc.arg(organization_id), document_id, sqlc.arg(area_id), sqlc.arg(known_seq),
        version_ref, format, path, title, hash, owner, acl_state
    FROM input
    ON CONFLICT (organization_id, document_id, version_ref) DO NOTHING
    RETURNING id, document_id
), versions AS (
    SELECT id, document_id FROM inserted
    UNION ALL
    SELECT v.id, v.document_id FROM normalize.document_version v
    JOIN input i ON i.document_id = v.document_id AND i.version_ref = v.version_ref
        AND i.hash = v.content_hash AND i.format = v.doc_format
    WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id)
)
INSERT INTO normalize.docs_report_document (organization_id, report_id, document_version_id, spec_valid)
SELECT sqlc.arg(organization_id), sqlc.arg(report_id), v.id, i.spec_valid
FROM versions v JOIN input i USING (document_id);

-- name: DocumentsAt :many
SELECT v.* FROM normalize.document_version v
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.document_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id)
    AND v.version_ref = sqlc.arg(version_ref) AND v.known_seq <= sqlc.arg(known_seq) AND v.acl_state = 'known'
    AND NOT o.restricted AND o.deleted_at IS NULL
    AND (sqlc.arg(path_prefix)::text IN ('', v.path) OR starts_with(v.path, sqlc.arg(path_prefix)::text || '/'))
ORDER BY v.path, v.id;

-- name: ReceiptDocumentationRoots :many
SELECT DISTINCT r.area_id, d.root_id, d.path_prefix, r.profile_id, r.sha
FROM normalize.docs_report d
JOIN normalize.report r ON r.organization_id = d.organization_id AND r.id = d.report_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.receipt_id = sqlc.arg(receipt_id)
    AND r.sha IS NOT NULL AND r.profile_id IS NOT NULL
ORDER BY r.area_id, d.root_id, d.path_prefix, r.profile_id, r.sha;

-- name: DocumentationReportsAt :many
SELECT DISTINCT ON (r.report_kind)
    r.id, r.report_kind, r.completeness, r.skipped_count, d.owner_rule, d.checks, d.links, r.known_seq,
    COALESCE(r.finished_at, r.started_at, uuid_extract_timestamp(r.id))::timestamptz AS observed_at,
    COALESCE(sarif.id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS sarif_id
FROM normalize.report r
JOIN normalize.docs_report d ON d.organization_id = r.organization_id AND d.report_id = r.id
LEFT JOIN LATERAL (
    SELECT s.id FROM normalize.report s
    WHERE s.organization_id = r.organization_id AND s.area_id = r.area_id AND s.sha = r.sha
        AND s.report_kind = r.report_kind AND s.report_format = 'sarif' AND s.profile_id = r.profile_id
        AND s.docs_run_id = r.docs_run_id AND s.docs_file_hash = d.sarif_hash
        AND s.known_seq <= sqlc.arg(known_seq)
    ORDER BY (s.receipt_id = r.receipt_id) DESC, s.known_seq DESC, s.id DESC
    LIMIT 1
) sarif ON true
WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
    AND r.sha = sqlc.arg(sha) AND r.profile_id = sqlc.arg(profile_id) AND r.known_seq <= sqlc.arg(known_seq)
    AND d.path_prefix = sqlc.arg(path_prefix) AND (d.root_id = sqlc.arg(root_id) OR d.root_id IS NULL)
ORDER BY r.report_kind, COALESCE(r.finished_at, r.started_at) DESC NULLS LAST, r.known_seq DESC, r.id DESC;

-- name: DocumentationReportDocuments :many
SELECT d.report_id, v.id, v.document_id, v.version_ref, v.doc_format, v.path, v.title, v.content_hash, v.owner, v.acl_state, d.spec_valid
FROM normalize.docs_report_document d
JOIN normalize.document_version v ON v.organization_id = d.organization_id AND v.id = d.document_version_id
JOIN normalize.source_object o ON o.organization_id = v.organization_id AND o.id = v.document_id
WHERE d.organization_id = sqlc.arg(organization_id) AND d.report_id = ANY(sqlc.arg(report_ids)::uuid[])
    AND v.acl_state = 'known' AND NOT o.restricted AND o.deleted_at IS NULL
ORDER BY d.report_id, v.path, v.id;

-- name: DocumentationFindingsAt :many
SELECT f.id, f.report_id, f.finding_kind, f.rule_id, f.path, f.line_start,
    COALESCE(f.mapped_severity, f.native_severity, 'unknown')::text AS severity, f.evidence,
    first_seen.first_seen_at, COALESCE(f.evidence ? 'link', false)::boolean AS has_link_target
FROM normalize.finding_occurrence f
JOIN LATERAL (
    SELECT min(COALESCE(r.finished_at, r.started_at, rc.received_at, uuid_extract_timestamp(r.id)))::timestamptz AS first_seen_at
    FROM normalize.finding_occurrence old
    JOIN normalize.report r ON r.organization_id = old.organization_id AND r.id = old.report_id
    LEFT JOIN ingest.receipt rc ON rc.organization_id = r.organization_id AND rc.id = r.receipt_id
    WHERE old.organization_id = f.organization_id AND old.fingerprint = f.fingerprint
        AND old.finding_kind IN ('docs', 'openapi') AND old.finding_kind = f.finding_kind
        AND r.area_id = sqlc.arg(area_id) AND r.known_seq <= sqlc.arg(known_seq)
) first_seen ON true
WHERE f.organization_id = sqlc.arg(organization_id) AND f.report_id = ANY(sqlc.arg(report_ids)::uuid[])
    AND f.finding_kind IN ('docs', 'openapi') AND (f.evidence->>'suppressed') IS DISTINCT FROM 'true'
    AND (sqlc.arg(path_prefix)::text IN ('', f.path) OR f.path IS NULL OR starts_with(f.path, sqlc.arg(path_prefix)::text || '/'))
ORDER BY f.finding_kind, f.path, f.line_start, f.rule_id, f.id;

-- name: InsertReport :one
INSERT INTO normalize.report (organization_id, receipt_id, record_no, known_seq, area_id, component_id, report_kind, mode, producer,
    producer_version, report_format, schema_version, parser_version, profile_id, rules_hash, config_hash, db_version, db_published_at,
    scope_hash, sha, artifact_digest, run_id, attempt_id, started_at, finished_at, exit_class, completeness, scanned_count, skipped_count,
    skipped_targets, history_from_sha, history_to_sha, history_truncated, ref_name, language, docs_run_id, docs_file_hash,
    dependency_delta, dependency_base_sha, dependency_inventory)
VALUES (sqlc.arg(organization_id), sqlc.arg(receipt_id), sqlc.arg(record_no), sqlc.arg(known_seq), sqlc.arg(area_id), sqlc.narg(component_id),
    sqlc.arg(report_kind), sqlc.arg(mode), sqlc.narg(producer), sqlc.narg(producer_version), sqlc.arg(report_format), sqlc.arg(schema_version),
    sqlc.arg(parser_version), sqlc.narg(profile_id), sqlc.narg(rules_hash), sqlc.narg(config_hash), sqlc.narg(db_version),
    sqlc.narg(db_published_at), sqlc.narg(scope_hash), sqlc.narg(sha), sqlc.narg(artifact_digest), sqlc.narg(run_id), sqlc.narg(attempt_id),
    sqlc.narg(started_at), sqlc.narg(finished_at), sqlc.arg(exit_class), sqlc.arg(completeness), sqlc.narg(scanned_count),
    sqlc.narg(skipped_count), sqlc.arg(skipped_targets), sqlc.narg(history_from_sha), sqlc.narg(history_to_sha), sqlc.narg(history_truncated),
    sqlc.narg(ref_name), sqlc.narg(language), sqlc.narg(docs_run_id), sqlc.narg(docs_file_hash), sqlc.narg(dependency_delta),
    sqlc.narg(dependency_base_sha), sqlc.narg(dependency_inventory))
RETURNING id;

-- An empty text and a zero line stand for NULL: arrays of the statement carry no NULL elements. Set-returning
-- functions of one select list run in step, so the arrays give one row per finding.
-- name: InsertFindings :exec
INSERT INTO normalize.finding_occurrence (organization_id, report_id, known_seq, native_id, rule_id, finding_kind, path, line_start, line_end,
    package_purl, native_severity, native_confidence, mapped_severity, severity_mapping_version, fingerprint, content_fingerprint, evidence)
SELECT sqlc.arg(organization_id), sqlc.arg(report_id), sqlc.arg(known_seq), NULLIF(unnest(sqlc.arg(native_ids)::text[]), ''),
    unnest(sqlc.arg(rule_ids)::text[]), unnest(sqlc.arg(finding_kinds)::text[]), NULLIF(unnest(sqlc.arg(paths)::text[]), ''),
    NULLIF(unnest(sqlc.arg(line_starts)::integer[]), 0), NULLIF(unnest(sqlc.arg(line_ends)::integer[]), 0),
    NULLIF(unnest(sqlc.arg(package_purls)::text[]), ''), NULLIF(unnest(sqlc.arg(native_severities)::text[]), ''),
    NULLIF(unnest(sqlc.arg(native_confidences)::text[]), ''), NULLIF(unnest(sqlc.arg(mapped_severities)::text[]), ''),
    NULLIF(unnest(sqlc.arg(severity_mapping_versions)::integer[]), 0), unnest(sqlc.arg(fingerprints)::bytea[]),
    NULLIF(unnest(sqlc.arg(content_fingerprints)::bytea[]), ''::bytea), NULLIF(unnest(sqlc.arg(evidences)::text[]), '')::jsonb;

-- name: EnsureTestCases :exec
INSERT INTO normalize.test_case (organization_id, area_id, case_key, case_key_hash)
SELECT sqlc.arg(organization_id), sqlc.arg(area_id), unnest(sqlc.arg(case_keys)::text[]), unnest(sqlc.arg(case_key_hashes)::bytea[])
ON CONFLICT (organization_id, area_id, case_key_hash) DO NOTHING;

-- name: TestCaseIDs :many
SELECT id, case_key_hash FROM normalize.test_case
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND case_key_hash = ANY (sqlc.arg(case_key_hashes)::bytea[]);

-- A zero duration stands for NULL plus one: arrays of the statement carry no NULL elements, so a known duration is
-- shifted by one.
-- name: InsertTestExecutions :exec
INSERT INTO normalize.test_execution (organization_id, report_id, test_case_id, attempt_id, config_fingerprint, ordinal, outcome,
    duration_ms, run_started_at, known_seq, suite)
SELECT sqlc.arg(organization_id), sqlc.arg(report_id), unnest(sqlc.arg(test_case_ids)::uuid[]), sqlc.narg(attempt_id),
    unnest(sqlc.arg(config_fingerprints)::bytea[]), unnest(sqlc.arg(ordinals)::smallint[]), unnest(sqlc.arg(outcomes)::text[]),
    NULLIF(unnest(sqlc.arg(durations_ms)::bigint[]), 0) - 1, sqlc.arg(run_started_at), sqlc.arg(known_seq), unnest(sqlc.arg(suites)::text[]);

-- name: InsertCoverageFacts :exec
INSERT INTO normalize.coverage_fact (organization_id, report_id, path, path_hash, kind, covered, executable, line_map_key, known_seq)
SELECT sqlc.arg(organization_id), sqlc.arg(report_id), unnest(sqlc.arg(paths)::text[]), unnest(sqlc.arg(path_hashes)::bytea[]),
    unnest(sqlc.arg(kinds)::text[]), unnest(sqlc.arg(covered)::integer[]), unnest(sqlc.arg(executable)::integer[]),
    NULLIF(unnest(sqlc.arg(line_map_keys)::text[]), ''), sqlc.arg(known_seq);

-- name: InsertCodeFacts :exec
INSERT INTO normalize.code_report_fact (organization_id, report_id, facts)
VALUES (sqlc.arg(organization_id), sqlc.arg(report_id), sqlc.arg(facts));

-- name: CodeReportAt :one
SELECT r.id, r.completeness, r.skipped_count, r.skipped_targets, f.facts
FROM normalize.report r
JOIN normalize.code_report_fact f ON f.organization_id = r.organization_id AND f.report_id = r.id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.area_id = sqlc.arg(area_id) AND r.component_id IS NOT DISTINCT FROM sqlc.narg(component_id)::uuid
  AND r.sha = sqlc.arg(sha) AND r.report_format = sqlc.arg(format)
  AND r.producer IS NOT DISTINCT FROM sqlc.narg(producer)::text AND r.profile_id IS NOT DISTINCT FROM sqlc.narg(profile_id)::text
  AND r.known_seq <= sqlc.arg(known_seq)
  AND r.exit_class NOT IN ('timeout', 'failed')
  AND r.report_format IN ('lizard_csv', 'jscpd_json')
ORDER BY r.known_seq DESC, r.record_no DESC, r.id DESC
LIMIT 1;

-- name: ReceiptCodeScopes :many
SELECT DISTINCT r.area_id, r.component_id, r.sha, r.report_format, r.producer, r.profile_id
FROM normalize.report r
JOIN normalize.code_report_fact f ON f.organization_id = r.organization_id AND f.report_id = r.id
WHERE r.organization_id = $1 AND r.receipt_id = $2 AND r.sha IS NOT NULL
ORDER BY r.area_id, r.component_id, r.sha, r.report_format, r.producer, r.profile_id;

-- name: ReceiptReports :many
SELECT r.id, r.area_id, r.component_id, r.report_kind, COALESCE(r.language, '')::text AS language, COALESCE(r.ref_name, '')::text AS ref_name,
    COALESCE(r.sha, '')::text AS sha, r.exit_class, r.completeness, r.started_at, r.finished_at, r.known_seq,
    COALESCE(a.outcome = 'canceled', false)::boolean AS interrupted, COALESCE(v.default_branch, '')::text AS default_branch
FROM normalize.report r
LEFT JOIN normalize.ci_attempt_version a ON a.organization_id = r.organization_id AND a.attempt_id = r.attempt_id AND a.superseded_seq IS NULL
LEFT JOIN normalize.repository_version v ON v.organization_id = r.organization_id AND v.repository_id = r.area_id AND v.superseded_seq IS NULL
WHERE r.organization_id = sqlc.arg(organization_id) AND r.receipt_id = sqlc.arg(receipt_id)
    AND r.report_format NOT IN ('lizard_csv', 'jscpd_json')
ORDER BY r.record_no;

-- name: ReportOccurrences :many
SELECT id, report_id, rule_id, finding_kind, COALESCE(path, '')::text AS path, line_start, COALESCE(package_purl, '')::text AS package_purl, fingerprint,
    content_fingerprint, COALESCE(evidence #>> '{package,delta}', '')::text AS dependency_delta
FROM normalize.finding_occurrence
WHERE organization_id = sqlc.arg(organization_id) AND report_id = ANY (sqlc.arg(report_ids)::uuid[])
ORDER BY report_id, id;

-- name: TechnicalReports :many
WITH runs AS (
    SELECT r.id, r.sha, COALESCE(r.language, '')::text AS language, COALESCE(r.profile_id, '')::text AS profile_id,
        COALESCE(r.producer, '')::text AS producer, COALESCE(r.producer_version, '')::text AS producer_version,
        r.mode, r.rules_hash, r.config_hash, r.scope_hash, COALESCE(r.db_version, '')::text AS db_version,
        r.completeness, r.exit_class, COALESCE(a.outcome = 'canceled', false)::boolean AS interrupted,
        false AS rejected, COALESCE(r.dependency_delta, '')::text AS dependency_delta, COALESCE(r.dependency_base_sha, '')::text AS dependency_base_sha,
        COALESCE(r.finished_at, r.started_at, receipt.received_at) AS observed_at, r.known_seq, r.record_no
    FROM normalize.report r
    JOIN ingest.receipt receipt ON receipt.organization_id = r.organization_id AND receipt.id = r.receipt_id
    LEFT JOIN normalize.ci_attempt_version a ON a.organization_id = r.organization_id AND a.attempt_id = r.attempt_id AND a.superseded_seq IS NULL
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
        AND r.component_id = sqlc.arg(component_id)::uuid AND r.report_kind = sqlc.arg(report_kind)::text
        AND r.report_format NOT IN ('lizard_csv', 'jscpd_json') AND r.sha IS NOT NULL
        AND (sqlc.arg(latest)::boolean OR r.sha = ANY(sqlc.arg(shas)::text[]) OR r.id = ANY(sqlc.arg(report_ids)::uuid[]))
    UNION ALL
    SELECT r.id, r.sha, r.language, COALESCE(r.profile_id, ''), COALESCE(r.producer, ''), '',
        COALESCE(r.mode, ''), NULL::bytea, NULL::bytea, r.scope_hash, '', 'unknown', '', false, true, '', '',
        receipt.received_at, r.known_seq, r.record_no
    FROM normalize.rejected_report r
    JOIN ingest.receipt receipt ON receipt.organization_id = r.organization_id AND receipt.id = r.receipt_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
        AND r.component_id = sqlc.arg(component_id)::uuid AND r.report_kind = sqlc.arg(report_kind)::text
        AND COALESCE(r.report_format, '') NOT IN ('lizard_csv', 'jscpd_json') AND r.sha = ANY(sqlc.arg(shas)::text[])
), eligible AS (
    SELECT *, (completeness = 'complete' AND exit_class IN ('success','findings') AND NOT interrupted AND NOT rejected) AS complete
    FROM runs
    WHERE (sqlc.narg(language)::text IS NULL OR language = sqlc.narg(language))
        AND (sqlc.narg(profile_id)::text IS NULL OR profile_id = sqlc.narg(profile_id))
        AND (sqlc.narg(reference_report_id)::uuid IS NULL OR EXISTS (
            SELECT 1 FROM normalize.report ref
            WHERE ref.organization_id = sqlc.arg(organization_id) AND ref.id = sqlc.narg(reference_report_id)
                AND (COALESCE(ref.producer, ''), COALESCE(ref.producer_version, ''), ref.mode,
                    ref.rules_hash, ref.config_hash, ref.scope_hash, COALESCE(ref.db_version, ''))
                IS NOT DISTINCT FROM (runs.producer, runs.producer_version, runs.mode,
                    runs.rules_hash, runs.config_hash, runs.scope_hash, runs.db_version)
        ))
), ranked AS (
    SELECT *, row_number() OVER (
        PARTITION BY CASE WHEN sqlc.arg(latest)::boolean THEN '' ELSE sha END, language, profile_id, complete
        ORDER BY observed_at DESC, known_seq DESC, record_no DESC, id DESC
    ) AS position
    FROM eligible
    WHERE NOT sqlc.arg(latest)::boolean OR complete
)
SELECT id, COALESCE(sha, '')::text AS sha, language, profile_id, producer, producer_version, mode, rules_hash, config_hash,
    scope_hash, db_version, completeness, exit_class, interrupted, rejected, dependency_delta, dependency_base_sha
FROM ranked
WHERE position <= 2 OR id = ANY(sqlc.arg(report_ids)::uuid[])
ORDER BY observed_at DESC, known_seq DESC, record_no DESC, id DESC;

-- name: ReportsByID :many
SELECT r.id, r.area_id, r.component_id, r.report_kind, COALESCE(r.language, '')::text AS language, COALESCE(r.ref_name, '')::text AS ref_name,
    COALESCE(r.sha, '')::text AS sha, r.exit_class, r.completeness, r.started_at, r.finished_at, r.known_seq
FROM normalize.report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = ANY (sqlc.arg(ids)::uuid[]);

-- The last report of every component, language and kind of analysis on the branches asked; a report without its
-- time sorts last.
-- name: LastBranchReports :many
SELECT DISTINCT ON (r.component_id, COALESCE(r.language, ''), r.report_kind, r.ref_name)
    r.id, r.area_id, r.component_id, r.report_kind, COALESCE(r.language, '')::text AS language, COALESCE(r.ref_name, '')::text AS ref_name,
    COALESCE(r.sha, '')::text AS sha, r.exit_class, r.completeness, r.started_at, r.finished_at, r.known_seq
FROM normalize.report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.component_id = ANY (sqlc.arg(component_ids)::uuid[])
    AND r.ref_name = ANY (sqlc.arg(ref_names)::text[])
    AND r.report_format NOT IN ('lizard_csv', 'jscpd_json')
ORDER BY r.component_id, COALESCE(r.language, ''), r.report_kind, r.ref_name, COALESCE(r.finished_at, r.started_at) DESC NULLS LAST, r.known_seq DESC;

-- name: DefaultBranches :many
SELECT repository_id, default_branch::text AS default_branch FROM normalize.repository_version
WHERE organization_id = sqlc.arg(organization_id) AND repository_id = ANY (sqlc.arg(repository_ids)::uuid[]) AND superseded_seq IS NULL
    AND default_branch IS NOT NULL;

-- name: RunsWithAttempts :many
SELECT DISTINCT run_id FROM normalize.ci_attempt_version
WHERE organization_id = sqlc.arg(organization_id) AND run_id = ANY (sqlc.arg(run_ids)::uuid[]) AND superseded_seq IS NULL
    AND job_key <> sqlc.arg(except_job_key);

-- name: InsertRejectedReport :exec
INSERT INTO normalize.rejected_report (organization_id, receipt_id, record_no, known_seq, area_id,
    component_id, report_kind, language, sha, run_id, attempt_id, reason_code, report_format, producer, mode, profile_id,
    scope_hash, secret_scope, history_from_sha, history_to_sha)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (organization_id, receipt_id, record_no) DO NOTHING;

-- name: ReadReportEvidence :many
SELECT r.id, r.run_id, r.component_id, r.report_kind, COALESCE(r.language, '')::text AS language,
    r.report_format, COALESCE(r.producer, '')::text AS producer, COALESCE(r.producer_version, '')::text AS producer_version,
    COALESCE(encode(r.scope_hash, 'hex'), '')::text AS scope_hash,
    r.exit_class, r.completeness, r.known_seq, ''::text AS rejected_reason, r.skipped_targets,
    COALESCE(r.dependency_delta, '')::text AS dependency_delta,
    (CASE WHEN r.report_kind <> 'secrets' THEN ''
          WHEN r.history_from_sha <> r.history_to_sha OR r.history_truncated THEN 'diff'
          WHEN r.mode = 'builtin' AND r.profile_id = 'gitleaks-default'
               AND r.history_from_sha IS NOT NULL AND r.scanned_count IS NULL THEN 'diff'
          ELSE 'head' END)::text AS secret_scope,
    r.mode, COALESCE(r.profile_id, '')::text AS profile_id,
    COALESCE(r.history_from_sha, '')::text AS history_from_sha, COALESCE(r.history_to_sha, '')::text AS history_to_sha
FROM normalize.report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
    AND r.known_seq <= sqlc.arg(known_seq)
    AND (r.sha = sqlc.arg(sha)::text OR (r.sha IS NULL AND r.report_kind NOT IN ('junit', 'coverage')
        AND r.run_id = ANY(sqlc.arg(run_ids)::uuid[])))
    AND (NOT sqlc.arg(pipeline)::boolean OR (r.run_id = ANY(sqlc.arg(run_ids)::uuid[])
        AND (r.attempt_id IS NULL OR r.attempt_id = ANY(sqlc.arg(attempt_ids)::uuid[]))))
UNION ALL
SELECT r.id, r.run_id, r.component_id, r.report_kind, r.language, COALESCE(r.report_format, ''), COALESCE(r.producer, ''), '',
    COALESCE(encode(r.scope_hash, 'hex'), ''), '', '', r.known_seq, r.reason_code, '[]'::jsonb, '', COALESCE(r.secret_scope, ''),
    COALESCE(r.mode, ''), COALESCE(r.profile_id, ''), COALESCE(r.history_from_sha, ''), COALESCE(r.history_to_sha, '')
FROM normalize.rejected_report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
    AND r.known_seq <= sqlc.arg(known_seq)
    AND (r.sha = sqlc.arg(sha)::text OR (r.sha IS NULL AND r.report_kind NOT IN ('junit', 'coverage')
        AND r.run_id = ANY(sqlc.arg(run_ids)::uuid[])))
    AND (NOT sqlc.arg(pipeline)::boolean OR (r.run_id = ANY(sqlc.arg(run_ids)::uuid[])
        AND (r.attempt_id IS NULL OR r.attempt_id = ANY(sqlc.arg(attempt_ids)::uuid[]))))
ORDER BY known_seq DESC, id DESC;

-- name: ReadSecurityFindings :many
SELECT id, report_id, rule_id, finding_kind, COALESCE(path, '')::text AS path, line_start, line_end,
    COALESCE(package_purl, '')::text AS package_purl, COALESCE(native_severity, '')::text AS native_severity,
    COALESCE(native_confidence, '')::text AS native_confidence, COALESCE(mapped_severity, '')::text AS mapped_severity,
    COALESCE(severity_mapping_version, 0)::integer AS severity_mapping_version, fingerprint, evidence
FROM normalize.finding_occurrence
WHERE organization_id = sqlc.arg(organization_id) AND report_id = ANY(sqlc.arg(report_ids)::uuid[])
ORDER BY report_id, fingerprint, id;

-- name: ReceiptSecurityScopes :many
SELECT r.area_id, r.component_id, COALESCE(r.sha, '')::text AS sha
FROM normalize.report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.receipt_id = sqlc.arg(receipt_id) AND r.sha IS NOT NULL
    AND r.report_kind IN ('sast', 'sca', 'iac', 'secrets')
UNION
SELECT r.area_id, r.component_id, COALESCE(r.sha, '')::text AS sha
FROM normalize.rejected_report r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.receipt_id = sqlc.arg(receipt_id) AND r.sha IS NOT NULL
    AND r.report_kind IN ('sast', 'sca', 'iac', 'secrets')
ORDER BY area_id, component_id, sha;

-- Re-uploads of the same execution replace its earlier observation. Without a run or attempt there is
-- no identity across reports. Apply the outcome filter after choosing the current observation.
-- name: ReadTestExecutions :many
WITH ranked AS (
    SELECT x.id, x.report_id, x.test_case_id, x.attempt_id, x.suite, x.config_fingerprint,
        x.ordinal, x.outcome, x.duration_ms, x.run_started_at, c.case_key,
        row_number() OVER (PARTITION BY COALESCE(r.run_id, x.attempt_id, r.id), x.attempt_id,
            x.test_case_id, x.config_fingerprint, x.ordinal ORDER BY r.known_seq DESC, r.id DESC) AS position
    FROM normalize.test_execution x
    JOIN normalize.test_case c ON c.organization_id = x.organization_id AND c.id = x.test_case_id
    JOIN normalize.report r ON r.organization_id = x.organization_id AND r.id = x.report_id
    WHERE x.organization_id = sqlc.arg(organization_id) AND x.report_id = ANY(sqlc.arg(report_ids)::uuid[])
)
SELECT id, report_id, test_case_id, attempt_id, suite, config_fingerprint, ordinal, outcome, duration_ms, run_started_at,
    case_key, (position > 1)::boolean AS superseded
FROM ranked
WHERE (NOT sqlc.arg(current_only)::boolean OR position = 1)
    AND (sqlc.arg(outcome)::text = '' OR outcome = sqlc.arg(outcome))
    AND (sqlc.narg(after_id)::uuid IS NULL OR id > sqlc.narg(after_id)::uuid)
ORDER BY id
LIMIT NULLIF(sqlc.arg(page_size)::integer, 0);

-- name: ReadReportCoverage :many
SELECT id, report_id, path, kind, covered, executable, line_map_key
FROM normalize.coverage_fact
WHERE organization_id = sqlc.arg(organization_id) AND report_id = ANY(sqlc.arg(report_ids)::uuid[])
ORDER BY path, kind, id;

-- name: AllRepositoryAreas :many
SELECT area_id FROM normalize.area WHERE organization_id = $1 AND area_kind = 'repository' ORDER BY area_id;

-- name: InsertMRDiffs :execrows
INSERT INTO normalize.mr_diff (organization_id, mr_revision_id, known_seq, receipt_id, record_no, content_hash, completeness, files)
SELECT r.organization_id, r.id, sqlc.arg(known_seq), sqlc.arg(receipt_id), d.record_no, decode(d.content_hash, 'hex'), d.completeness, d.files
FROM jsonb_to_recordset(sqlc.arg(diffs)::jsonb)
    AS d(mr_id uuid, revision_key text, head_sha text, base_sha text, record_no integer, content_hash text, completeness text, files jsonb)
JOIN normalize.mr_revision r ON r.organization_id = sqlc.arg(organization_id) AND r.mr_id = d.mr_id
    AND r.revision_key = d.revision_key AND r.head_sha = d.head_sha AND r.base_sha = d.base_sha AND r.superseded_seq IS NULL
JOIN normalize.source_object o ON o.organization_id = r.organization_id AND o.id = r.mr_id
    AND o.current_area_id = sqlc.arg(area_id) AND o.deleted_at IS NULL
ON CONFLICT (organization_id, mr_revision_id, content_hash) DO NOTHING;

-- name: ReadMRDiff :one
WITH revision AS (
    SELECT r.organization_id, r.id, r.mr_id, r.revision_key, r.head_sha, r.base_sha
    FROM normalize.mr_revision r
    JOIN normalize.source_object o ON o.organization_id = r.organization_id AND o.id = r.mr_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.mr_id = sqlc.arg(mr_id) AND r.head_sha = sqlc.arg(head_sha)
        AND r.known_seq <= sqlc.arg(known_seq) AND (r.superseded_seq IS NULL OR r.superseded_seq > sqlc.arg(known_seq))
        AND o.deleted_at IS NULL AND o.current_area_id = ANY(sqlc.arg(areas)::uuid[])
        AND (NOT o.restricted OR o.id = ANY(sqlc.arg(allowed)::uuid[]))
    ORDER BY r.created_at DESC, r.known_seq DESC, r.id DESC LIMIT 1
)
SELECT r.mr_id, r.id AS revision_id, r.revision_key, r.head_sha, r.base_sha, d.known_seq, d.completeness, d.files
FROM revision r
JOIN normalize.mr_diff d ON d.organization_id = r.organization_id AND d.mr_revision_id = r.id
WHERE d.known_seq <= sqlc.arg(known_seq)
    AND NOT EXISTS (
        SELECT 1 FROM normalize.mr_version m
        WHERE m.organization_id = r.organization_id AND m.mr_id = r.mr_id AND m.head_sha = r.head_sha
            AND m.known_seq <= sqlc.arg(known_seq) AND (m.superseded_seq IS NULL OR m.superseded_seq > sqlc.arg(known_seq))
            AND m.base_sha IS NOT NULL AND m.base_sha IS DISTINCT FROM r.base_sha
    )
ORDER BY (d.completeness = 'complete') DESC, d.known_seq DESC, d.content_hash DESC LIMIT 1;
-- name: InsertAnalysisRun :exec
INSERT INTO normalize.analysis_run (organization_id, receipt_id, known_seq, area_id, component_id, sha, tool_id, record_nos, metadata)
VALUES (sqlc.arg(organization_id), sqlc.arg(receipt_id), sqlc.arg(known_seq), sqlc.arg(area_id), sqlc.narg(component_id),
        sqlc.arg(sha), sqlc.arg(tool_id), sqlc.arg(record_nos)::integer[], sqlc.arg(metadata))
ON CONFLICT (organization_id, receipt_id, tool_id) DO NOTHING;

-- name: ReceiptAnalysisScopes :many
SELECT DISTINCT area_id, component_id, sha
FROM normalize.analysis_run
WHERE organization_id = sqlc.arg(organization_id) AND receipt_id = sqlc.arg(receipt_id)
ORDER BY area_id, component_id, sha;

-- name: ReadAnalysisRuns :many
SELECT a.id, a.receipt_id, a.known_seq, a.tool_id, a.metadata,
       COALESCE((
           SELECT jsonb_agg(jsonb_build_object(
               'id', COALESCE(r.id, rejected.id, '00000000-0000-0000-0000-000000000000'::uuid),
               'reason', CASE
                   WHEN j.outcome = 'conflict' THEN 'report_conflict'
                   WHEN j.outcome = 'rejected' THEN j.reason_code
                   WHEN j.outcome = 'suppressed' THEN 'report_suppressed'
                   WHEN rejected.id IS NOT NULL THEN rejected.reason_code
                   WHEN r.id IS NOT NULL THEN ''
                   WHEN j.record_no IS NOT NULL THEN 'report_not_normalized'
                   ELSE 'report_record_missing'
               END,
               'dependencies', r.dependency_inventory
           ) ORDER BY idx)
           FROM unnest(a.record_nos) WITH ORDINALITY AS refs(no, idx)
           LEFT JOIN ingest.record j ON j.organization_id = a.organization_id
               AND j.receipt_id = a.receipt_id AND j.record_no = refs.no
           LEFT JOIN normalize.report r ON r.organization_id = j.organization_id
               AND r.receipt_id = COALESCE(j.first_receipt_id, j.receipt_id)
               AND r.record_no = COALESCE(j.first_record_no, j.record_no) AND r.known_seq <= sqlc.arg(known_seq)
               AND r.area_id = a.area_id AND r.sha = a.sha AND r.component_id IS NOT DISTINCT FROM a.component_id
           LEFT JOIN normalize.rejected_report rejected ON rejected.organization_id = j.organization_id
               AND rejected.receipt_id = COALESCE(j.first_receipt_id, j.receipt_id)
               AND rejected.record_no = COALESCE(j.first_record_no, j.record_no) AND rejected.known_seq <= sqlc.arg(known_seq)
               AND rejected.area_id = a.area_id AND rejected.sha = a.sha
               AND rejected.component_id IS NOT DISTINCT FROM a.component_id
       ), '[]'::jsonb)::jsonb AS reports
FROM normalize.analysis_run a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.area_id = sqlc.arg(area_id)
  AND a.sha = sqlc.arg(sha) AND a.component_id IS NOT DISTINCT FROM sqlc.narg(component_id)::uuid
  AND a.known_seq <= sqlc.arg(known_seq)
ORDER BY a.known_seq DESC, a.receipt_id, a.tool_id;

-- name: ReadAnalysisDocumentVersions :many
WITH corpus AS (
    SELECT DISTINCT v.document_id, d.path_prefix, r.profile_id
    FROM normalize.report r JOIN normalize.docs_report d
        ON (d.organization_id, d.report_id) = (r.organization_id, r.id)
    JOIN normalize.docs_report_document link ON (link.organization_id, link.report_id) = (d.organization_id, d.report_id)
    JOIN normalize.document_version v ON (v.organization_id, v.id) = (link.organization_id, link.document_version_id)
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.area_id = sqlc.arg(area_id)
      AND r.sha = sqlc.arg(sha) AND r.component_id IS NOT DISTINCT FROM sqlc.narg(component_id)::uuid
      AND r.known_seq <= sqlc.arg(known_seq) AND r.finished_at <= sqlc.arg(as_of)
      AND r.completeness = 'complete' AND r.exit_class IN ('success', 'findings')
)
SELECT DISTINCT ON (v.document_id) v.id, r.id AS report_id, v.path, v.version_ref, r.finished_at AS observed_at
FROM corpus c
JOIN normalize.document_version v ON v.organization_id = sqlc.arg(organization_id) AND v.document_id = c.document_id
JOIN normalize.docs_report_document link ON (link.organization_id, link.document_version_id) = (v.organization_id, v.id)
JOIN normalize.docs_report d ON (d.organization_id, d.report_id) = (link.organization_id, link.report_id)
JOIN normalize.report r ON (r.organization_id, r.id) = (d.organization_id, d.report_id)
WHERE r.area_id = sqlc.arg(area_id) AND r.component_id IS NOT DISTINCT FROM sqlc.narg(component_id)::uuid
  AND d.path_prefix = c.path_prefix AND r.profile_id IS NOT DISTINCT FROM c.profile_id
  AND r.known_seq <= sqlc.arg(known_seq) AND r.finished_at <= sqlc.arg(as_of)
  AND r.completeness = 'complete' AND r.exit_class IN ('success', 'findings')
ORDER BY v.document_id, r.finished_at, v.id, r.id;
