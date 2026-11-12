-- name: CreateOrganization :exec
INSERT INTO org.organization (organization_id, name, status, first_admin_issuer, first_admin_subject, structure_version, created_at)
VALUES ($1, $2, $3, $4, $5, 0, $6);
