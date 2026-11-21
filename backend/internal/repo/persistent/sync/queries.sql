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

-- name: ListStates :many
SELECT * FROM sync.state WHERE organization_id = sqlc.arg(organization_id)
    AND (NOT sqlc.arg(one_connection)::bool OR connection_id = sqlc.arg(connection_id)::uuid)
ORDER BY connection_id, data_kind;

-- name: LeaseHeld :one
SELECT EXISTS (SELECT 1 FROM sync.lease WHERE organization_id = $1 AND connection_id = $2 AND lease_until >= sqlc.arg(now)::timestamptz);

-- name: StartReconcile :one
INSERT INTO sync.reconcile_run (organization_id, connection_id, data_kind, as_of, started_at, outcome)
VALUES (sqlc.arg(organization_id), sqlc.arg(connection_id), sqlc.arg(data_kind), sqlc.arg(at), sqlc.arg(at), 'running')
ON CONFLICT (organization_id, connection_id, data_kind) WHERE outcome = 'running' DO NOTHING
RETURNING id;

-- name: RunningReconcile :one
SELECT * FROM sync.reconcile_run
WHERE organization_id = $1 AND connection_id = $2 AND data_kind = $3 AND outcome = 'running';

-- name: GetReconcile :one
SELECT * FROM sync.reconcile_run WHERE organization_id = $1 AND id = $2;

-- name: FinishReconcile :exec
UPDATE sync.reconcile_run SET as_of = sqlc.arg(as_of), started_at = sqlc.arg(started_at), finished_at = sqlc.arg(finished_at)::timestamptz, expected_count = sqlc.narg(expected_count),
    observed_count = sqlc.narg(observed_count), missing_count = sqlc.narg(missing_count), changed_count = sqlc.narg(changed_count),
    outcome = sqlc.arg(outcome), reason_code = sqlc.narg(reason_code)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND outcome = 'running';

-- name: LastReconcile :one
SELECT * FROM sync.reconcile_run WHERE organization_id = sqlc.arg(organization_id) AND connection_id = sqlc.arg(connection_id)
    AND (NOT sqlc.arg(one_kind)::bool OR data_kind = sqlc.arg(data_kind)::text)
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: AddDirtyKey :batchexec
INSERT INTO sync.dirty_key (organization_id, connection_id, key_hash, object_kind, native_ref, added_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (organization_id, connection_id, key_hash) DO UPDATE SET added_at = EXCLUDED.added_at;

-- name: DirtyKeys :many
SELECT key_hash, object_kind, native_ref, added_at FROM sync.dirty_key
WHERE organization_id = $1 AND connection_id = $2
ORDER BY key_hash
LIMIT sqlc.arg(row_limit);

-- name: DeleteDirtyKeys :execrows
DELETE FROM sync.dirty_key WHERE organization_id = sqlc.arg(organization_id) AND connection_id = sqlc.arg(connection_id)
    AND key_hash = ANY (sqlc.arg(key_hashes)::bytea[]) AND added_at <= sqlc.arg(read_started_at);

-- name: CountDirtyKeys :one
SELECT count(*) FROM sync.dirty_key WHERE organization_id = $1 AND connection_id = $2;

-- name: ReceiveWebhook :one
INSERT INTO sync.webhook_delivery (organization_id, connection_id, received_at) VALUES ($1, $2, $3) RETURNING id;

-- name: RejectWebhook :execrows
UPDATE sync.webhook_delivery SET rejected_at = $3, reject_reason = $4
WHERE organization_id = $1 AND id = $2 AND rejected_at IS NULL;

-- name: WebhookStatus :one
SELECT received_at AS last_received_at,
    (SELECT count(*) FROM sync.webhook_delivery r WHERE r.organization_id = d.organization_id AND r.connection_id = d.connection_id
        AND r.rejected_at >= sqlc.arg(since)::timestamptz) AS rejected_count
FROM sync.webhook_delivery d WHERE d.organization_id = $1 AND d.connection_id = $2
ORDER BY received_at DESC, id DESC LIMIT 1;

-- name: LastWebhookRejection :one
SELECT rejected_at, reject_reason FROM sync.webhook_delivery WHERE organization_id = $1 AND connection_id = $2 AND rejected_at IS NOT NULL
ORDER BY rejected_at DESC, id DESC LIMIT 1;

-- name: SaveActivityCheck :batchexec
INSERT INTO sync.activity_check (organization_id, run_id, scope_id, object_kind, native_ns, native_id, matches)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (organization_id, run_id, scope_id, object_kind, native_ns, native_id)
DO UPDATE SET matches = sync.activity_check.matches AND excluded.matches;

-- name: SaveActivityPagination :exec
INSERT INTO sync.activity_pagination (organization_id, scope_id, receipt_id, data_kind, observed_at, complete)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: SaveActivityRecovery :exec
INSERT INTO sync.activity_recovery (organization_id, scope_id, receipt_id, object_kind, native_ns, native_id, observed_at, page_receipt_ids)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ActivityReconciliations :many
SELECT r.* FROM sync.reconcile_run r
WHERE r.organization_id = $1 AND r.connection_id = ANY(sqlc.arg(connection_ids)::uuid[])
    AND r.started_at <= sqlc.arg(cutoff)::timestamptz
    AND (COALESCE(finished_at, sqlc.arg(cutoff)::timestamptz) >= sqlc.arg(window_from)::timestamptz OR id IN (
        SELECT prior.id FROM (
            SELECT DISTINCT ON (connection_id, data_kind, (outcome = 'complete')) id
            FROM sync.reconcile_run
            WHERE organization_id = $1 AND connection_id = ANY(sqlc.arg(connection_ids)::uuid[])
                AND finished_at < sqlc.arg(window_from)::timestamptz
            ORDER BY connection_id, data_kind, (outcome = 'complete'), started_at DESC, id DESC
        ) prior
    ))
ORDER BY started_at, id;

-- name: ActivityChecks :many
WITH runs AS (
    SELECT DISTINCT ON (connection_id, data_kind) id, connection_id, data_kind, finished_at
    FROM sync.reconcile_run
    WHERE organization_id = sqlc.arg(organization_id) AND connection_id = ANY(sqlc.arg(connection_ids)::uuid[])
        AND data_kind IN ('tasks', 'merge_requests') AND outcome IN ('complete', 'incomplete')
        AND finished_at <= sqlc.arg(cutoff)::timestamptz
    ORDER BY connection_id, data_kind, started_at DESC, id DESC
)
SELECT ck.run_id, ck.scope_id, ck.object_kind, ck.native_ns, ck.native_id, ck.matches,
    r.finished_at, COALESCE(paged.receipt_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS pagination_receipt
FROM sync.activity_check ck
JOIN runs r ON r.id = ck.run_id
LEFT JOIN LATERAL (
    SELECT p.receipt_id, p.complete FROM sync.activity_pagination p
    WHERE p.organization_id = ck.organization_id AND p.scope_id = ck.scope_id AND p.data_kind = r.data_kind
        AND p.observed_at <= r.finished_at
    ORDER BY p.observed_at DESC, p.receipt_id DESC LIMIT 1
) paged ON paged.complete
WHERE ck.organization_id = sqlc.arg(organization_id) AND ck.scope_id = ANY(sqlc.arg(scope_ids)::uuid[]);

-- name: ActivityRecoveries :many
SELECT DISTINCT ON (scope_id, object_kind, native_ns, native_id)
    scope_id, object_kind, native_ns, native_id, receipt_id, observed_at, page_receipt_ids
FROM sync.activity_recovery
WHERE organization_id = $1 AND scope_id = ANY(sqlc.arg(scope_ids)::uuid[]) AND observed_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY scope_id, object_kind, native_ns, native_id, observed_at DESC, receipt_id DESC;
