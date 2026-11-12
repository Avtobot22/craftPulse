-- name: CreateProtocol :one
INSERT INTO install.protocol (started_at, release_sha, manifest, outcome)
VALUES ($1, $2, $3, 'running')
RETURNING id;

-- name: InsertCheckResult :exec
INSERT INTO install.check_result (protocol_id, check_kind, outcome, reason_code, detail)
VALUES ($1, $2, $3, $4, $5);

-- name: FinishProtocol :exec
UPDATE install.protocol SET finished_at = $2, outcome = $3 WHERE id = $1;
