-- name: GetState :one
SELECT * FROM sync.state WHERE organization_id = $1 AND connection_id = $2 AND data_kind = $3;

-- name: SaveState :exec
INSERT INTO sync.state (organization_id, connection_id, data_kind, phase, cursor, checkpoint_at, failed_cursor,
    last_attempt_at, last_outcome, last_error_code, last_success_at, watermark, history_from, completeness)
VALUES (sqlc.arg(organization_id), sqlc.arg(connection_id), sqlc.arg(data_kind), sqlc.arg(phase), sqlc.narg(cursor),
    sqlc.narg(checkpoint_at), sqlc.narg(failed_cursor), sqlc.narg(last_attempt_at), sqlc.narg(last_outcome),
    sqlc.narg(last_error_code), sqlc.narg(last_success_at), sqlc.narg(watermark), sqlc.narg(history_from), sqlc.arg(completeness))
ON CONFLICT (organization_id, connection_id, data_kind) DO UPDATE SET
    phase = EXCLUDED.phase, cursor = EXCLUDED.cursor, checkpoint_at = EXCLUDED.checkpoint_at,
    failed_cursor = EXCLUDED.failed_cursor, last_attempt_at = EXCLUDED.last_attempt_at, last_outcome = EXCLUDED.last_outcome,
    last_error_code = EXCLUDED.last_error_code, last_success_at = EXCLUDED.last_success_at, watermark = EXCLUDED.watermark,
    history_from = EXCLUDED.history_from, completeness = EXCLUDED.completeness;

-- name: AcquireLease :one
INSERT INTO sync.lease (organization_id, connection_id, holder, lease_until)
VALUES (sqlc.arg(organization_id), sqlc.arg(connection_id), sqlc.arg(holder), sqlc.arg(lease_until))
ON CONFLICT (organization_id, connection_id) DO UPDATE SET holder = EXCLUDED.holder, lease_until = EXCLUDED.lease_until
WHERE sync.lease.lease_until < sqlc.arg(now)::timestamptz OR sync.lease.holder = EXCLUDED.holder
RETURNING holder;

-- name: ReleaseLease :exec
DELETE FROM sync.lease WHERE organization_id = $1 AND connection_id = $2 AND holder = $3;
