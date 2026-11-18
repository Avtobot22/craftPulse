-- name: SourceLinks :many
SELECT connection_id, state, linked_at, row_version FROM access.source_link
WHERE organization_id = $1 AND user_id = $2 ORDER BY connection_id;

-- name: ReserveSourceLink :one
INSERT INTO access.source_link (organization_id, user_id, connection_id, state, row_version)
VALUES ($1, $2, $3, 'not_linked', 1)
ON CONFLICT (organization_id, user_id, connection_id)
DO UPDATE SET row_version = access.source_link.row_version + 1
RETURNING row_version;

-- name: CreateSourceLinkAttempt :exec
INSERT INTO access.source_link_attempt
(organization_id, state_hash, user_id, session_id, connection_id, link_version, return_path, redirect_uri, created_at, expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: TakeSourceLinkAttempt :one
DELETE FROM access.source_link_attempt
WHERE organization_id = $1 AND user_id = $2 AND session_id = $3 AND state_hash = $4
RETURNING *;

-- name: ConfirmSourceLink :execrows
UPDATE access.source_link SET state = 'linked', linked_at = $5, row_version = row_version + 1
WHERE organization_id = $1 AND user_id = $2 AND connection_id = $3 AND row_version = $4;

-- name: RevokeSourceLink :execrows
INSERT INTO access.source_link (organization_id, user_id, connection_id, state, row_version)
VALUES ($1, $2, $3, 'revoked', 1)
ON CONFLICT (organization_id, user_id, connection_id)
DO UPDATE SET state = 'revoked', row_version = access.source_link.row_version + 1
WHERE access.source_link.state <> 'revoked';

-- name: DeleteSourceLinkAttempts :exec
DELETE FROM access.source_link_attempt WHERE organization_id = $1 AND user_id = $2 AND connection_id = $3;

-- name: ExpireSourceLink :exec
UPDATE access.source_link SET state = 'expired', row_version = row_version + 1
WHERE organization_id = $1 AND user_id = $2 AND connection_id = $3 AND row_version = $4 AND state = 'linked';

-- name: PrepareSourceLinkExchange :execrows
UPDATE access.source_link SET state = 'not_linked'
WHERE organization_id = $1 AND user_id = $2 AND connection_id = $3 AND row_version = $4;
