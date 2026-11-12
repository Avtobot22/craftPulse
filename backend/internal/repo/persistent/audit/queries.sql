-- name: InsertEvent :exec
INSERT INTO audit.event (organization_id, occurred_at, actor_kind, actor_id, operation, object_kind, object_id,
    area_id, outcome, reason_code, base_version, new_version, correlation_id, details)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);
