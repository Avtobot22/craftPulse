\set ON_ERROR_STOP on
BEGIN READ ONLY;
SELECT set_config('craftpulse.org_id', :'organization', true) AS acceptance_org \gset

WITH objects AS (
    SELECT o.id, o.organization_id, o.native_id
    FROM normalize.source_object AS o
    JOIN connect.source_instance AS s
        ON (s.organization_id, s.id) = (o.organization_id, o.instance_id)
    WHERE s.organization_id = :'organization'::uuid AND o.native_ns = :'project'
        AND s.provider = 'universal' AND s.instance_key = :'instance'
        AND o.object_kind = 'task'
),
versions AS (
    SELECT o.native_id, v.source_updated_at, v.known_seq,
        rtrim(rtrim(to_char(v.source_updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US'), '0'), '.') || 'Z' AS at,
        v.task_key AS key, v.title, v.type_native_id AS type,
        v.status_native_id AS status, v.superseded_seq IS NULL AS current
    FROM objects AS o
    JOIN normalize.task_version AS v
        ON (v.organization_id, v.task_id) = (o.organization_id, o.id)
),
events AS (
    SELECT o.native_id, e.event_key, e.event_kind AS kind, e.field_native_id AS field,
        rtrim(rtrim(to_char(e.event_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US'), '0'), '.') || 'Z' AS at,
        e.before_value AS before, e.after_value AS after
    FROM objects AS o
    JOIN normalize.task_event AS e
        ON (e.organization_id, e.task_id) = (o.organization_id, o.id)
),
as_of_state AS (
    SELECT DISTINCT ON (native_id) native_id, key, title, type, status
    FROM versions
    WHERE source_updated_at <= :'as_of'::timestamptz
    ORDER BY native_id, source_updated_at DESC, known_seq DESC
)
SELECT json_build_object(
    'entity_ids', coalesce((SELECT json_object_agg(native_id, id) FROM objects), '{}'),
    'objects', coalesce((SELECT json_agg(native_id ORDER BY native_id) FROM objects), '[]'),
    'versions', coalesce((
        SELECT json_agg(to_jsonb(versions) - ARRAY['source_updated_at', 'known_seq']
            ORDER BY native_id, source_updated_at, known_seq)
        FROM versions
    ), '[]'),
    'events', coalesce((SELECT json_agg(events ORDER BY native_id, event_key) FROM events), '[]'),
    'as_of', coalesce((SELECT json_agg(as_of_state ORDER BY native_id) FROM as_of_state), '[]')
);
COMMIT;
