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
    key_version, contract_version, access_mode, status, row_version, credentials_set_at, sync_paused, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', 1, $11, false, $12, $13);

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

-- name: LockConnection :one
SELECT c.*, i.provider
FROM connect.connection c
JOIN connect.source_instance i ON i.organization_id = c.organization_id AND i.id = c.instance_id
WHERE c.organization_id = $1 AND c.id = $2
FOR UPDATE OF c;

-- name: PageConnections :many
SELECT c.*, i.provider
FROM connect.connection c
JOIN connect.source_instance i ON i.organization_id = c.organization_id AND i.id = c.instance_id
WHERE c.organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(status)::text = '' OR c.status = sqlc.arg(status)::text)
    AND (NOT sqlc.arg(has_cursor)::bool OR (c.created_at, c.id) < (sqlc.arg(after_time)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(page_limit);

-- name: SaveConnection :exec
UPDATE connect.connection SET
    instance_id = sqlc.arg(instance_id), name = sqlc.arg(name), adapter_url = sqlc.narg(adapter_url),
    adapter_key_enc = sqlc.narg(adapter_key_enc), key_version = sqlc.narg(key_version),
    contract_version = sqlc.arg(contract_version), access_mode = sqlc.arg(access_mode), status = sqlc.arg(status),
    access_lost_at = sqlc.narg(access_lost_at), credentials_set_at = sqlc.narg(credentials_set_at),
    sync_paused = sqlc.arg(sync_paused), row_version = sqlc.arg(row_version),
    activated_probe_id = sqlc.narg(activated_probe_id), activated_row_version = sqlc.narg(activated_row_version),
    deleted_at = sqlc.narg(deleted_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ScopesOf :many
SELECT * FROM connect.connection_scope
WHERE organization_id = sqlc.arg(organization_id) AND connection_id = ANY(sqlc.arg(connection_ids)::uuid[])
ORDER BY connection_id, added_at, id;

-- name: RemoveScope :exec
UPDATE connect.connection_scope SET status = 'removed', removed_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active';

-- name: RemoveConnectionScopes :exec
UPDATE connect.connection_scope SET status = 'removed', removed_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id) AND connection_id = sqlc.arg(connection_id) AND status = 'active';

-- name: SetScopeHistory :exec
UPDATE connect.connection_scope SET history_from = sqlc.narg(history_from)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: CapabilitiesOf :many
SELECT connection_id, capability, state, reason_code, checked_at, source_version FROM connect.capability
WHERE organization_id = sqlc.arg(organization_id) AND connection_id = ANY(sqlc.arg(connection_ids)::uuid[])
ORDER BY connection_id, capability;

-- name: DeleteCapabilities :exec
DELETE FROM connect.capability WHERE organization_id = $1 AND connection_id = $2;

-- name: InsertCapability :exec
INSERT INTO connect.capability (organization_id, connection_id, capability, state, reason_code, checked_at, source_version)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: SetInstanceBaseURL :exec
UPDATE connect.source_instance SET base_url = sqlc.arg(base_url)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND base_url IS NULL;

-- name: VerifyInstance :exec
UPDATE connect.source_instance SET base_url = sqlc.arg(base_url), edition = sqlc.narg(edition),
    source_version = sqlc.narg(source_version), verified_at = sqlc.arg(verified_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: RunningProbe :one
SELECT * FROM connect.probe
WHERE organization_id = $1 AND connection_id = $2 AND status IN ('pending', 'running');

-- name: InsertProbe :one
INSERT INTO connect.probe (organization_id, connection_id, configuration_version, status, started_at, capabilities)
VALUES ($1, $2, $3, 'pending', $4, '[]')
RETURNING *;

-- name: GetProbe :one
SELECT * FROM connect.probe WHERE organization_id = $1 AND connection_id = $2 AND id = $3;

-- name: LatestProbe :one
SELECT * FROM connect.probe WHERE organization_id = $1 AND connection_id = $2
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: StartProbeRun :execrows
UPDATE connect.probe SET status = 'running'
WHERE organization_id = $1 AND id = $2 AND status IN ('pending', 'running');

-- name: FinishProbe :exec
UPDATE connect.probe SET status = sqlc.arg(status), finished_at = sqlc.arg(finished_at),
    instance_key = sqlc.narg(instance_key), edition = sqlc.narg(edition), source_version = sqlc.narg(source_version),
    contract_version = sqlc.narg(contract_version), access_mode = sqlc.narg(access_mode),
    capabilities = sqlc.arg(capabilities), reason_code = sqlc.narg(reason_code)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('pending', 'running');
