-- name: InsertEvent :exec
INSERT INTO audit.event (organization_id, occurred_at, actor_kind, actor_id, operation, object_kind, object_id,
    area_id, outcome, reason_code, base_version, new_version, correlation_id, details)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: PageEvents :many
SELECT id, occurred_at, actor_kind, actor_id, operation, object_kind, object_id, outcome, reason_code,
    base_version, new_version, correlation_id
FROM audit.event
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id))
    AND (sqlc.narg(outcome)::text IS NULL OR outcome = sqlc.narg(outcome))
    AND (sqlc.narg(occurred_after)::timestamptz IS NULL OR occurred_at >= sqlc.narg(occurred_after))
    AND (sqlc.narg(occurred_before)::timestamptz IS NULL OR occurred_at < sqlc.narg(occurred_before))
    AND (NOT sqlc.arg(after)::bool OR (occurred_at, id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg(row_limit);
