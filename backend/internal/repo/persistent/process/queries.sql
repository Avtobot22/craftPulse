-- name: TimezoneExists :one
SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_timezone_names WHERE name = sqlc.arg(name)::text);

-- name: LastSettingsSeq :one
SELECT GREATEST(
    (SELECT COALESCE(max(known_seq), 0) FROM process.mapping_version m
        WHERE m.organization_id = sqlc.arg(organization_id) AND m.team_id = sqlc.arg(team_id)),
    (SELECT COALESCE(max(known_seq), 0) FROM process.calendar_version c
        WHERE c.organization_id = sqlc.arg(organization_id) AND c.team_id = sqlc.arg(team_id))
)::bigint;

-- name: LatestMappingVersionNo :one
SELECT COALESCE(max(version_no), 0)::integer FROM process.mapping_version
WHERE organization_id = sqlc.arg(organization_id) AND team_id = sqlc.arg(team_id);

-- name: MappingVersionAt :one
SELECT sqlc.embed(v), c.version_no AS calendar_version_no FROM process.mapping_version v
LEFT JOIN process.calendar_version c ON c.organization_id = v.organization_id AND c.id = v.calendar_version_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.team_id = sqlc.arg(team_id) AND v.known_seq <= sqlc.arg(known_seq)
ORDER BY v.version_no DESC
LIMIT 1;

-- name: MappingVersionByConfirmation :one
SELECT sqlc.embed(v), c.version_no AS calendar_version_no FROM process.mapping_version v
LEFT JOIN process.calendar_version c ON c.organization_id = v.organization_id AND c.id = v.calendar_version_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.confirmation_id = sqlc.arg(confirmation_id);

-- name: PageMappingVersions :many
SELECT sqlc.embed(v), c.version_no AS calendar_version_no FROM process.mapping_version v
LEFT JOIN process.calendar_version c ON c.organization_id = v.organization_id AND c.id = v.calendar_version_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.team_id = sqlc.arg(team_id)
    AND (sqlc.narg(before)::integer IS NULL OR v.version_no < sqlc.narg(before)::integer)
ORDER BY v.version_no DESC
LIMIT sqlc.arg(row_limit);

-- name: MappingRules :many
SELECT * FROM process.mapping_rule
WHERE organization_id = sqlc.arg(organization_id) AND mapping_version_id = ANY (sqlc.arg(version_ids)::uuid[])
ORDER BY mapping_version_id, rule_kind, instance_id, native_ref, native_value;

-- name: InsertMappingVersion :exec
INSERT INTO process.mapping_version (organization_id, id, team_id, version_no, base_version_no, content_hash, timezone,
    calendar_version_id, known_seq, confirmed_by, confirmed_at, confirmation_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(team_id), sqlc.arg(version_no), sqlc.narg(base_version_no),
    sqlc.arg(content_hash), sqlc.arg(timezone), sqlc.narg(calendar_version_id), sqlc.arg(known_seq), sqlc.arg(confirmed_by),
    sqlc.arg(confirmed_at), sqlc.arg(confirmation_id));

-- name: InsertMappingRule :batchexec
INSERT INTO process.mapping_rule (organization_id, mapping_version_id, rule_kind, instance_id, native_ref, native_value, meaning, rank)
VALUES (sqlc.arg(organization_id), sqlc.arg(mapping_version_id), sqlc.arg(rule_kind), sqlc.arg(instance_id), sqlc.arg(native_ref),
    sqlc.arg(native_value), sqlc.arg(meaning), sqlc.narg(rank));

-- name: LatestCalendarVersionNo :one
SELECT COALESCE(max(version_no), 0)::integer FROM process.calendar_version
WHERE organization_id = sqlc.arg(organization_id) AND team_id = sqlc.arg(team_id);

-- name: CalendarVersionAt :one
SELECT * FROM process.calendar_version
WHERE organization_id = sqlc.arg(organization_id) AND team_id = sqlc.arg(team_id) AND known_seq <= sqlc.arg(known_seq)
ORDER BY version_no DESC
LIMIT 1;

-- name: CalendarVersionByNo :one
SELECT * FROM process.calendar_version
WHERE organization_id = sqlc.arg(organization_id) AND team_id = sqlc.arg(team_id) AND version_no = sqlc.arg(version_no);

-- name: CalendarVersionByConfirmation :one
SELECT * FROM process.calendar_version
WHERE organization_id = sqlc.arg(organization_id) AND confirmation_id = sqlc.arg(confirmation_id);

-- name: PageCalendarVersions :many
SELECT * FROM process.calendar_version
WHERE organization_id = sqlc.arg(organization_id) AND team_id = sqlc.arg(team_id)
    AND (sqlc.narg(before)::integer IS NULL OR version_no < sqlc.narg(before)::integer)
ORDER BY version_no DESC
LIMIT sqlc.arg(row_limit);

-- name: InsertCalendarVersion :exec
INSERT INTO process.calendar_version (organization_id, id, team_id, version_no, base_version_no, timezone, workdays, work_start,
    work_end, holidays, known_seq, confirmed_by, confirmed_at, confirmation_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(team_id), sqlc.arg(version_no), sqlc.narg(base_version_no),
    sqlc.arg(timezone), sqlc.arg(workdays), sqlc.narg(work_start), sqlc.narg(work_end), sqlc.arg(holidays), sqlc.arg(known_seq),
    sqlc.arg(confirmed_by), sqlc.arg(confirmed_at), sqlc.arg(confirmation_id));
