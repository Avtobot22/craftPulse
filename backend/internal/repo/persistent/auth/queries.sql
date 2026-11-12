-- name: GetLoginRoute :one
SELECT organization_id FROM auth.login_route WHERE slug = $1;

-- name: CreateLoginRoute :exec
INSERT INTO auth.login_route (slug, organization_id) VALUES ($1, $2);

-- name: CreateProviderRegistration :exec
INSERT INTO auth.provider_registration (organization_id, issuer, client_id, client_secret_enc, key_version, status, created_at)
VALUES ($1, $2, $3, $4, $5, 'active', $6);
