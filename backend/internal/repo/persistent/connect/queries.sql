-- name: InsertInstance :exec
INSERT INTO connect.source_instance (organization_id, provider, instance_key)
VALUES ($1, $2, $3)
ON CONFLICT (organization_id, provider, instance_key) DO NOTHING;

-- name: FindInstanceByKey :one
SELECT * FROM connect.source_instance
WHERE organization_id = $1 AND provider = $2 AND instance_key = $3;

-- name: GetInstance :one
SELECT * FROM connect.source_instance WHERE organization_id = $1 AND id = $2;

-- name: InsertConnection :exec
INSERT INTO connect.connection (organization_id, id, instance_id, name, adapter_kind, adapter_url, adapter_key_enc,
    key_version, contract_version, access_mode, status, row_version, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', 1, $11, $12);

-- name: GetConnection :one
SELECT c.*, i.provider
FROM connect.connection c
JOIN connect.source_instance i ON i.organization_id = c.organization_id AND i.id = c.instance_id
WHERE c.organization_id = $1 AND c.id = $2;

-- name: PauseConnection :exec
UPDATE connect.connection SET paused_until = GREATEST(paused_until, sqlc.arg(until)::timestamptz)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: MarkAccessLost :execrows
UPDATE connect.connection SET status = 'access_lost', access_lost_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active';

-- name: InsertScope :one
INSERT INTO connect.connection_scope (organization_id, connection_id, instance_id, scope_kind, native_scope_id,
    display_name, history_from, repository_scope_id, status, added_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9)
RETURNING id;

-- name: ListScopes :many
SELECT * FROM connect.connection_scope
WHERE organization_id = $1 AND connection_id = $2
ORDER BY added_at, id;

-- name: SetScopeArea :execrows
UPDATE connect.connection_scope SET area_id = sqlc.arg(area_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
    AND (area_id IS NULL OR area_id = sqlc.arg(area_id));

-- name: ListCapabilities :many
SELECT capability, state, reason_code, checked_at, source_version FROM connect.capability
WHERE organization_id = $1 AND connection_id = $2
ORDER BY capability;
