-- name: GetLoginRoute :one
SELECT organization_id FROM auth.login_route WHERE slug = $1;

-- name: CreateLoginRoute :exec
INSERT INTO auth.login_route (slug, organization_id) VALUES ($1, $2);

-- name: CreateProviderRegistration :exec
INSERT INTO auth.provider_registration (organization_id, issuer, client_id, client_secret_enc, key_version, status, created_at)
VALUES ($1, $2, $3, $4, $5, 'active', $6);

-- name: GetActiveProviderRegistration :one
SELECT issuer, client_id, client_secret_enc, key_version
FROM auth.provider_registration
WHERE organization_id = $1 AND status = 'active';

-- name: CreateLoginAttempt :exec
INSERT INTO auth.login_attempt (cookie_hash, organization_id, state, nonce, pkce_verifier, issuer, return_path, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: TakeLoginAttempt :one
DELETE FROM auth.login_attempt WHERE cookie_hash = $1
RETURNING organization_id, state, nonce, pkce_verifier, issuer, return_path, created_at, expires_at;

-- name: CreateSession :exec
INSERT INTO auth.session (organization_id, token_hash, user_id, created_at, last_activity_at, expires_at)
VALUES ($1, $2, $3, $4, $4, $5);

-- name: GetSessionByToken :one
SELECT s.organization_id, s.id, s.user_id, s.created_at, s.last_activity_at, s.expires_at,
    (s.ended_at IS NOT NULL)::bool AS ended, r.slug
FROM auth.session s
JOIN auth.login_route r ON r.organization_id = s.organization_id
WHERE s.token_hash = $1;

-- name: TouchSession :exec
UPDATE auth.session SET last_activity_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND ended_at IS NULL
    AND last_activity_at < sqlc.arg(at);

-- name: EndSessionByToken :exec
UPDATE auth.session SET ended_at = sqlc.arg(at)::timestamptz, end_reason = sqlc.arg(reason)::text
WHERE token_hash = sqlc.arg(token_hash) AND ended_at IS NULL;

-- name: EndSession :execrows
UPDATE auth.session SET ended_at = sqlc.arg(at)::timestamptz, end_reason = sqlc.arg(reason)::text
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND ended_at IS NULL;

-- name: ListLiveSessions :many
SELECT id FROM auth.session
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND ended_at IS NULL
    AND expires_at > sqlc.arg(at) AND last_activity_at > sqlc.arg(idle_since)
ORDER BY created_at, id;

-- name: EndSessions :exec
UPDATE auth.session SET ended_at = sqlc.arg(at)::timestamptz, end_reason = sqlc.arg(reason)::text
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY(sqlc.arg(ids)::uuid[]) AND ended_at IS NULL;

-- name: EndUserSessions :execrows
UPDATE auth.session SET ended_at = sqlc.arg(at)::timestamptz, end_reason = sqlc.arg(reason)::text
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND ended_at IS NULL;

-- name: RevokeUserAPIKeys :execrows
UPDATE auth.api_key SET revoked_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND owner_user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: LockUploadProject :one
SELECT id FROM org.project WHERE organization_id=$1 AND id=$2 FOR UPDATE;

-- name: InsertUploadToken :one
INSERT INTO auth.upload_token (organization_id, project_id, token_hash, slot, created_at, created_by)
SELECT $1, $2, $3, s::smallint, $4, $5 FROM generate_series(1,2) s
WHERE NOT EXISTS (SELECT 1 FROM auth.upload_token t WHERE t.organization_id=$1 AND t.project_id=$2 AND t.slot=s AND t.revoked_at IS NULL)
ORDER BY s LIMIT 1
RETURNING id, organization_id, project_id, slot, created_at, revoked_at;

-- name: FindUploadToken :one
SELECT id, organization_id, project_id, slot, created_at, revoked_at FROM auth.upload_token WHERE organization_id=$1 AND id=$2;

-- name: FindUploadTokenHash :one
SELECT id, organization_id, project_id, slot, created_at, revoked_at FROM auth.upload_token WHERE token_hash=$1;

-- name: RevokeUploadToken :execrows
UPDATE auth.upload_token SET revoked_at=$3 WHERE organization_id=$1 AND id=$2 AND revoked_at IS NULL;

-- name: PageUploadTokens :many
SELECT id, organization_id, project_id, slot, created_at, revoked_at FROM auth.upload_token
WHERE organization_id=$1 AND project_id=$2 AND (NOT sqlc.arg(has_cursor)::boolean OR (created_at,id) < (sqlc.arg(after_time)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(page_limit);
