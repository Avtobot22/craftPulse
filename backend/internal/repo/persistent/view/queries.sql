-- name: CreateOperation :one
INSERT INTO view.operation (organization_id, id, requested_by, kind, resource_kind, resource_id, action, parameters, request_hash, status,
    created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'queued', $10, $10)
ON CONFLICT (organization_id, requested_by, kind, resource_kind, resource_id) WHERE status IN ('queued', 'running') DO NOTHING
RETURNING id;

-- name: ActiveOperation :one
SELECT * FROM view.operation
WHERE organization_id = $1 AND requested_by = $2 AND kind = $3 AND resource_kind = $4 AND resource_id = $5
    AND status IN ('queued', 'running')
FOR UPDATE;

-- name: GetOperation :one
SELECT * FROM view.operation WHERE organization_id = $1 AND id = $2;

-- name: UpdateOperation :exec
UPDATE view.operation SET status = sqlc.arg(status), updated_at = sqlc.arg(updated_at), completed_at = sqlc.narg(completed_at),
    result_uri = sqlc.narg(result_uri), error_code = sqlc.narg(error_code), correlation_id = sqlc.narg(correlation_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('queued', 'running');
