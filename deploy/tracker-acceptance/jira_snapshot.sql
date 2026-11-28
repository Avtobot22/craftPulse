\set ON_ERROR_STOP on
BEGIN READ ONLY;
SELECT set_config('craftpulse.org_id', :'organization', true) AS acceptance_org \gset

WITH objects AS (
    SELECT o.id, o.organization_id, o.native_id, v.task_key, v.title,
        v.status_native_id, v.restriction_native, v.created_src_at
    FROM normalize.source_object AS o
    JOIN normalize.task_version AS v
        ON (v.organization_id, v.task_id) = (o.organization_id, o.id)
    JOIN normalize.source_object AS area
        ON (area.organization_id, area.id) = (v.organization_id, v.area_id)
    WHERE area.native_id = :'project' AND area.object_kind = 'tracker_project'
        AND o.organization_id = :'organization'::uuid AND o.instance_id = :'instance'::uuid
        AND area.instance_id = o.instance_id
        AND o.object_kind = 'task' AND v.superseded_seq IS NULL
)
SELECT coalesce(json_agg(json_build_object(
    'id', o.id, 'native_id', o.native_id, 'key', o.task_key, 'summary', o.title,
    'status', o.status_native_id, 'security_id', o.restriction_native, 'created', o.created_src_at,
    'stored_as_of', (
        SELECT json_build_object('status', v.status_native_id, 'summary', v.title)
        FROM normalize.task_version AS v
        WHERE (v.organization_id, v.task_id) = (o.organization_id, o.id)
            AND v.source_updated_at <= :'as_of'::timestamptz
        ORDER BY v.source_updated_at DESC, v.known_seq DESC
        LIMIT 1
    ),
    'history', coalesce((
        SELECT json_agg(json_build_object(
            'id', e.event_key, 'at', e.event_time, 'field', e.field_native_id, 'kind', e.event_kind,
            'before', e.before_value, 'after', e.after_value,
            'before_account', before_actor.native_account_id, 'after_account', after_actor.native_account_id,
            'actor', actor.native_account_id
        ) ORDER BY e.event_time, e.event_key)
        FROM normalize.task_event AS e
        LEFT JOIN org.source_account AS before_actor
            ON (before_actor.organization_id, before_actor.id) = (e.organization_id, e.before_account_id)
        LEFT JOIN org.source_account AS after_actor
            ON (after_actor.organization_id, after_actor.id) = (e.organization_id, e.after_account_id)
        LEFT JOIN org.source_account AS actor
            ON (actor.organization_id, actor.id) = (e.organization_id, e.actor_account_id)
        WHERE (e.organization_id, e.task_id) = (o.organization_id, o.id)
    ), '[]')
) ORDER BY o.native_id), '[]')
FROM objects AS o;
COMMIT;
