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
SELECT id, issuer, subject, person_id, display_name, status, created_at
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
SELECT id, issuer, subject, person_id, display_name, status, created_at
FROM org.app_user WHERE organization_id = $1 AND id = $2;

-- name: LockUser :one
SELECT id, issuer, subject, person_id, display_name, status, created_at
FROM org.app_user WHERE organization_id = $1 AND id = $2
FOR UPDATE;

-- name: SetUserStatus :exec
UPDATE org.app_user SET status = sqlc.arg(status), blocked_at = sqlc.narg(blocked_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: AssignAdministrator :exec
INSERT INTO org.role_assignment (organization_id, user_id, role, assigned_by, assigned_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), 'administrator', sqlc.arg(user_id), sqlc.arg(at));

-- name: ListActiveRoles :many
SELECT id, role, team_id, project_id, service_id, assigned_at
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

-- name: ListCurrentProjectAreas :many
SELECT id, project_id, area_id AS to_id, lower(valid)::timestamptz AS valid_from,
    coalesce(upper(valid), lower(valid))::timestamptz AS valid_to, upper_inf(valid)::bool AS open
FROM org.project_area
WHERE organization_id = sqlc.arg(organization_id) AND superseded_seq IS NULL AND valid @> sqlc.arg(at)::timestamptz
ORDER BY project_id, area_id;

-- name: SupersedeProjectArea :exec
UPDATE org.project_area SET superseded_seq = sqlc.arg(seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND superseded_seq IS NULL;

-- name: InsertProjectArea :exec
INSERT INTO org.project_area (organization_id, project_id, area_id, valid, known_seq)
VALUES (sqlc.arg(organization_id), sqlc.arg(project_id), sqlc.arg(to_id),
    tstzrange(sqlc.arg(valid_from)::timestamptz, sqlc.narg(valid_to)::timestamptz, '[)'), sqlc.arg(seq));

-- name: RoleSetVersion :one
SELECT (count(*) + count(revoked_at))::bigint FROM org.role_assignment WHERE organization_id = $1 AND user_id = $2;

-- name: RevokeRole :exec
UPDATE org.role_assignment SET revoked_at = sqlc.arg(at)::timestamptz, revoked_by = sqlc.arg(revoked_by)::uuid
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: AssignRole :exec
INSERT INTO org.role_assignment (organization_id, user_id, role, team_id, project_id, service_id, assigned_by, assigned_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListTeamIDs :many
SELECT id FROM org.team WHERE organization_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListProjectIDs :many
SELECT id FROM org.project WHERE organization_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListServiceIDs :many
SELECT id FROM org.service WHERE organization_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PageUsersByNameAsc :many
SELECT id, issuer, subject, person_id, display_name, status, created_at FROM org.app_user
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
    AND (NOT sqlc.arg(after)::bool OR (coalesce(display_name, ''), id) > (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY coalesce(display_name, ''), id
LIMIT sqlc.arg(row_limit);

-- name: PageUsersByNameDesc :many
SELECT id, issuer, subject, person_id, display_name, status, created_at FROM org.app_user
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
    AND (NOT sqlc.arg(after)::bool OR (coalesce(display_name, ''), id) < (sqlc.arg(after_name)::text, sqlc.arg(after_id)::uuid))
ORDER BY coalesce(display_name, '') DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: PageUsersByCreatedAsc :many
SELECT id, issuer, subject, person_id, display_name, status, created_at FROM org.app_user
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
    AND (NOT sqlc.arg(after)::bool OR (created_at, id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: PageUsersByCreatedDesc :many
SELECT id, issuer, subject, person_id, display_name, status, created_at FROM org.app_user
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
    AND (NOT sqlc.arg(after)::bool OR (created_at, id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: DescribeSourceAccounts :exec
INSERT INTO org.source_account (organization_id, instance_id, native_account_id, login, display_name, email, proposed_kind, first_seen_at)
SELECT sqlc.arg(organization_id)::uuid, sqlc.arg(instance_id)::uuid, (sqlc.arg(native_ids)::text[])[i],
    NULLIF((sqlc.arg(logins)::text[])[i], ''), NULLIF((sqlc.arg(display_names)::text[])[i], ''),
    NULLIF((sqlc.arg(emails)::text[])[i], ''), (sqlc.arg(kinds)::text[])[i], sqlc.arg(seen_at)::timestamptz
FROM generate_subscripts(sqlc.arg(native_ids)::text[], 1) AS i
ORDER BY 3
ON CONFLICT (organization_id, instance_id, native_account_id) DO UPDATE
SET login = EXCLUDED.login, display_name = EXCLUDED.display_name, email = EXCLUDED.email, proposed_kind = EXCLUDED.proposed_kind
WHERE org.source_account.anonymized_at IS NULL;

-- name: ReferSourceAccounts :exec
INSERT INTO org.source_account (organization_id, instance_id, native_account_id, proposed_kind, first_seen_at)
SELECT sqlc.arg(organization_id)::uuid, sqlc.arg(instance_id)::uuid, t.native_id, 'unknown', sqlc.arg(seen_at)::timestamptz
FROM unnest(sqlc.arg(native_ids)::text[]) AS t (native_id)
ORDER BY t.native_id
ON CONFLICT (organization_id, instance_id, native_account_id) DO NOTHING;

-- name: SourceAccountIDs :many
SELECT id, native_account_id FROM org.source_account
WHERE organization_id = sqlc.arg(organization_id) AND instance_id = sqlc.arg(instance_id)
    AND native_account_id = ANY (sqlc.arg(native_ids)::text[]);

-- name: PagePersons :many
SELECT id, display_name, kind FROM org.person
WHERE organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
    AND (NOT sqlc.arg(after)::bool OR id > sqlc.arg(after_id)::uuid)
ORDER BY id
LIMIT sqlc.arg(row_limit);

-- name: InsertPerson :one
INSERT INTO org.person (organization_id, display_name, kind, created_at)
VALUES (sqlc.arg(organization_id), sqlc.narg(display_name), sqlc.arg(kind), sqlc.arg(created_at))
RETURNING id;

-- name: PageSourceAccounts :many
SELECT a.id, a.instance_id, a.native_account_id, a.login, a.display_name, a.email, a.proposed_kind,
    x.id AS assignment_id, x.person_id, x.actor_kind, x.known_seq, x.confirmed_by, x.confirmed_at, x.confirmation_id
FROM org.source_account a
LEFT JOIN org.account_assignment x
    ON x.organization_id = a.organization_id AND x.source_account_id = a.id AND x.superseded_seq IS NULL
WHERE a.organization_id = sqlc.arg(organization_id)
    AND (sqlc.narg(instance_id)::uuid IS NULL OR a.instance_id = sqlc.narg(instance_id))
    AND (sqlc.narg(assigned)::bool IS NULL OR (x.id IS NOT NULL) = sqlc.narg(assigned))
    AND (NOT sqlc.arg(after)::bool OR a.id > sqlc.arg(after_id)::uuid)
ORDER BY a.id
LIMIT sqlc.arg(row_limit);

-- name: AccountProposals :many
SELECT DISTINCT a.id AS source_account_id, x.person_id::uuid AS person_id,
    CASE WHEN a.login IS NOT NULL AND lower(o.login) = lower(a.login) THEN 'login_match' ELSE 'email_match' END::text AS basis
FROM org.source_account a
JOIN org.source_account o ON o.organization_id = a.organization_id AND o.id <> a.id
    AND ((a.login IS NOT NULL AND lower(o.login) = lower(a.login)) OR (a.email IS NOT NULL AND lower(o.email) = lower(a.email)))
JOIN org.account_assignment x ON x.organization_id = o.organization_id AND x.source_account_id = o.id
    AND x.superseded_seq IS NULL AND x.person_id IS NOT NULL
WHERE a.organization_id = sqlc.arg(organization_id) AND a.id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY 1, 2, 3;

-- name: SourceAccountsByID :many
SELECT id, instance_id, native_account_id, login, display_name, email, proposed_kind FROM org.source_account
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]);

-- name: LockCurrentAssignments :many
SELECT * FROM org.account_assignment
WHERE organization_id = sqlc.arg(organization_id) AND source_account_id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL
ORDER BY id
FOR UPDATE;

-- name: AssignmentsByConfirmation :many
SELECT * FROM org.account_assignment
WHERE organization_id = sqlc.arg(organization_id) AND confirmation_id = ANY (sqlc.arg(ids)::uuid[]);

-- name: SupersedeAssignments :exec
UPDATE org.account_assignment SET superseded_seq = sqlc.arg(known_seq)::bigint, superseded_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY (sqlc.arg(ids)::uuid[]) AND superseded_seq IS NULL;

-- name: InsertAssignment :one
INSERT INTO org.account_assignment (organization_id, source_account_id, person_id, actor_kind, known_seq, confirmed_by, confirmed_at,
    confirmation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: AssignmentAt :one
SELECT * FROM org.account_assignment
WHERE organization_id = sqlc.arg(organization_id) AND source_account_id = sqlc.arg(source_account_id)
    AND known_seq <= sqlc.arg(known_seq) AND (superseded_seq IS NULL OR superseded_seq > sqlc.arg(known_seq));

-- name: UserSourceAccount :one
SELECT a.id, a.native_account_id FROM org.app_user u
JOIN org.account_assignment x ON x.organization_id = u.organization_id AND x.person_id = u.person_id
    AND x.superseded_seq IS NULL AND x.actor_kind = 'human'
JOIN org.source_account a ON a.organization_id = x.organization_id AND a.id = x.source_account_id
WHERE u.organization_id = sqlc.arg(organization_id) AND u.id = sqlc.arg(user_id) AND a.instance_id = sqlc.arg(instance_id)
ORDER BY x.confirmed_at DESC, x.id DESC
LIMIT 1;
