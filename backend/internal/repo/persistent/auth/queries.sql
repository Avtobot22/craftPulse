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
