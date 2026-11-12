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

-- name: ListVisibleTeams :many
SELECT id, name, status FROM org.team
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR (status = 'active' AND id = ANY(sqlc.arg(ids)::uuid[])))
ORDER BY name, id;

-- name: ListVisibleProjects :many
SELECT id, name, status FROM org.project
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR (status = 'active' AND id = ANY(sqlc.arg(ids)::uuid[])))
ORDER BY name, id;

-- name: ListVisibleServices :many
SELECT id, name, status FROM org.service
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR (status = 'active' AND id = ANY(sqlc.arg(ids)::uuid[])))
ORDER BY name, id;

-- name: LockStructureVersion :one
SELECT structure_version FROM org.organization WHERE organization_id = $1 FOR UPDATE;

-- name: GetStructureVersion :one
SELECT structure_version FROM org.organization WHERE organization_id = $1;

-- name: SetStructureVersion :exec
UPDATE org.organization SET structure_version = $2 WHERE organization_id = $1;

-- name: ListPersonIDs :many
SELECT id FROM org.person WHERE organization_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListAllTeams :many
SELECT id, name, status FROM org.team WHERE organization_id = $1 ORDER BY name, id;

-- name: CreateTeam :exec
INSERT INTO org.team (organization_id, id, name, status, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UpdateTeam :exec
UPDATE org.team SET name = $3, status = $4 WHERE organization_id = $1 AND id = $2;

-- name: PageTeamsAsc :many
SELECT id, name, status FROM org.team
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
    AND (NOT sqlc.arg(after)::bool OR (name, id) > (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY name, id
LIMIT sqlc.arg(row_limit);

-- name: PageTeamsDesc :many
SELECT id, name, status FROM org.team
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
    AND (NOT sqlc.arg(after)::bool OR (name, id) < (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY name DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListAllProjects :many
SELECT id, name, status FROM org.project WHERE organization_id = $1 ORDER BY name, id;

-- name: CreateProject :exec
INSERT INTO org.project (organization_id, id, name, status, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UpdateProject :exec
UPDATE org.project SET name = $3, status = $4 WHERE organization_id = $1 AND id = $2;

-- name: PageProjectsAsc :many
SELECT p.id, p.name, p.status FROM org.project p
WHERE p.organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR p.id = ANY(sqlc.arg(ids)::uuid[]))
    AND (sqlc.narg(team_id)::uuid IS NULL OR p.id IN (
        SELECT tp.project_id FROM org.team_project tp
        WHERE tp.organization_id = sqlc.arg(organization_id) AND tp.team_id = sqlc.narg(team_id)
            AND tp.superseded_seq IS NULL AND tp.valid @> sqlc.arg(at)::timestamptz))
    AND (NOT sqlc.arg(after)::bool OR (p.name, p.id) > (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY p.name, p.id
LIMIT sqlc.arg(row_limit);

-- name: PageProjectsDesc :many
SELECT p.id, p.name, p.status FROM org.project p
WHERE p.organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR p.id = ANY(sqlc.arg(ids)::uuid[]))
    AND (sqlc.narg(team_id)::uuid IS NULL OR p.id IN (
        SELECT tp.project_id FROM org.team_project tp
        WHERE tp.organization_id = sqlc.arg(organization_id) AND tp.team_id = sqlc.narg(team_id)
            AND tp.superseded_seq IS NULL AND tp.valid @> sqlc.arg(at)::timestamptz))
    AND (NOT sqlc.arg(after)::bool OR (p.name, p.id) < (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY p.name DESC, p.id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListAllServices :many
SELECT id, name, status FROM org.service WHERE organization_id = $1 ORDER BY name, id;

-- name: CreateService :exec
INSERT INTO org.service (organization_id, id, name, status, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UpdateService :exec
UPDATE org.service SET name = $3, status = $4 WHERE organization_id = $1 AND id = $2;

-- name: PageServicesAsc :many
SELECT id, name, status FROM org.service
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
    AND (NOT sqlc.arg(after)::bool OR (name, id) > (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY name, id
LIMIT sqlc.arg(row_limit);

-- name: PageServicesDesc :many
SELECT id, name, status FROM org.service
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.arg(whole)::bool OR id = ANY(sqlc.arg(ids)::uuid[]))
    AND (NOT sqlc.arg(after)::bool OR (name, id) < (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY name DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListCurrentTeamProjects :many
SELECT id, team_id, project_id AS to_id, lower(valid)::timestamptz AS valid_from,
    coalesce(upper(valid), lower(valid))::timestamptz AS valid_to, upper_inf(valid)::bool AS open
FROM org.team_project
WHERE organization_id = sqlc.arg(organization_id) AND superseded_seq IS NULL AND valid @> sqlc.arg(at)::timestamptz
ORDER BY team_id, project_id;

-- name: SupersedeTeamProject :exec
UPDATE org.team_project SET superseded_seq = sqlc.arg(seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND superseded_seq IS NULL;

-- name: InsertTeamProject :exec
INSERT INTO org.team_project (organization_id, team_id, project_id, valid, known_seq)
VALUES (sqlc.arg(organization_id), sqlc.arg(team_id), sqlc.arg(to_id),
    tstzrange(sqlc.arg(valid_from)::timestamptz, sqlc.narg(valid_to)::timestamptz, '[)'), sqlc.arg(seq));

-- name: ListCurrentTeamMembers :many
SELECT id, team_id, person_id AS to_id, lower(valid)::timestamptz AS valid_from,
    coalesce(upper(valid), lower(valid))::timestamptz AS valid_to, upper_inf(valid)::bool AS open
FROM org.team_member
WHERE organization_id = sqlc.arg(organization_id) AND superseded_seq IS NULL AND valid @> sqlc.arg(at)::timestamptz
ORDER BY team_id, person_id;

-- name: SupersedeTeamMember :exec
UPDATE org.team_member SET superseded_seq = sqlc.arg(seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND superseded_seq IS NULL;

-- name: InsertTeamMember :exec
INSERT INTO org.team_member (organization_id, team_id, person_id, valid, known_seq)
VALUES (sqlc.arg(organization_id), sqlc.arg(team_id), sqlc.arg(to_id),
    tstzrange(sqlc.arg(valid_from)::timestamptz, sqlc.narg(valid_to)::timestamptz, '[)'), sqlc.arg(seq));
