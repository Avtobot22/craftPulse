-- name: CreateOrganization :exec
INSERT INTO org.organization (organization_id, name, status, first_admin_issuer, first_admin_subject, structure_version, created_at)
VALUES ($1, $2, $3, $4, $5, 0, $6);

-- name: GetOrganization :one
SELECT organization_id, name, status, first_admin_issuer, first_admin_subject
FROM org.organization WHERE organization_id = $1;

-- name: LockOrganization :one
SELECT organization_id, name, status, first_admin_issuer, first_admin_subject
FROM org.organization WHERE organization_id = $1
FOR UPDATE;

-- name: ActivateOrganization :exec
UPDATE org.organization SET status = 'active', first_admin_user_id = sqlc.arg(user_id)
WHERE organization_id = sqlc.arg(organization_id) AND status = 'setup';

-- name: LockUserByIdentity :one
SELECT id, issuer, subject, person_id, display_name, status
FROM org.app_user
WHERE organization_id = $1 AND issuer = $2 AND subject = $3
FOR UPDATE;

-- name: CreateUser :exec
INSERT INTO org.app_user (organization_id, issuer, subject, display_name, status, created_at)
VALUES ($1, $2, $3, $4, 'active', $5)
ON CONFLICT (organization_id, issuer, subject) DO NOTHING;

-- name: SetUserDisplayName :exec
UPDATE org.app_user SET display_name = $3 WHERE organization_id = $1 AND id = $2;

-- name: GetUser :one
SELECT id, issuer, subject, person_id, display_name, status
FROM org.app_user WHERE organization_id = $1 AND id = $2;

-- name: AssignAdministrator :exec
INSERT INTO org.role_assignment (organization_id, user_id, role, assigned_by, assigned_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), 'administrator', sqlc.arg(user_id), sqlc.arg(at));

-- name: ListActiveRoles :many
SELECT role, team_id, project_id, service_id, assigned_at
FROM org.role_assignment
WHERE organization_id = $1 AND user_id = $2 AND revoked_at IS NULL
ORDER BY assigned_at, id;

-- name: ListMemberTeamIDs :many
SELECT DISTINCT team_id FROM org.team_member
WHERE organization_id = sqlc.arg(organization_id) AND person_id = sqlc.arg(person_id)
    AND superseded_seq IS NULL AND valid @> sqlc.arg(at)::timestamptz;

-- name: ListTeamProjectIDs :many
SELECT DISTINCT project_id FROM org.team_project
WHERE organization_id = sqlc.arg(organization_id) AND team_id = ANY(sqlc.arg(team_ids)::uuid[])
    AND superseded_seq IS NULL AND valid @> sqlc.arg(at)::timestamptz;

-- name: ListActiveTeams :many
SELECT id, name, status FROM org.team
WHERE organization_id = sqlc.arg(organization_id) AND status = 'active'
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
ORDER BY name, id;

-- name: ListActiveProjects :many
SELECT id, name, status FROM org.project
WHERE organization_id = sqlc.arg(organization_id) AND status = 'active'
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
ORDER BY name, id;

-- name: ListActiveServices :many
SELECT id, name, status FROM org.service
WHERE organization_id = sqlc.arg(organization_id) AND status = 'active'
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
ORDER BY name, id;
