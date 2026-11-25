-- +goose Up
ALTER TABLE outbound.target ADD COLUMN pending_create_action_id uuid;

-- Earlier attempts did not record whether the request could create a comment.
WITH latest AS (
    SELECT DISTINCT ON (organization_id, target_kind, target_marker)
        organization_id, target_kind, target_marker, id, status
    FROM outbound.action
    WHERE target_kind = 'mr_comment' AND status IN ('intent', 'unknown', 'succeeded')
    ORDER BY organization_id, target_kind, target_marker, attempt_no DESC
)
UPDATE outbound.target AS t SET pending_create_action_id = a.id
FROM latest AS a
WHERE (t.organization_id, t.target_kind, t.target_marker) = (a.organization_id, a.target_kind, a.target_marker)
    AND a.status IN ('intent', 'unknown');

-- +goose Down
ALTER TABLE outbound.target DROP COLUMN pending_create_action_id;
