-- name: LatestProfileVersion :one
SELECT * FROM change.profile_version WHERE organization_id = sqlc.arg(organization_id) AND project_id = sqlc.arg(project_id)
ORDER BY version_no DESC LIMIT 1;

-- name: ProfileVersionByConfirmation :one
SELECT * FROM change.profile_version WHERE organization_id = sqlc.arg(organization_id) AND confirmation_id = sqlc.arg(confirmation_id);

-- name: ProfileEntries :many
SELECT * FROM change.profile_entry WHERE organization_id = sqlc.arg(organization_id) AND profile_version_id = sqlc.arg(profile_version_id)
ORDER BY component_id, language, analysis_kind;

-- name: InsertProfileVersion :exec
INSERT INTO change.profile_version (organization_id, id, project_id, version_no, base_version_no, content_hash, known_seq, confirmed_by, confirmed_at,
    confirmation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: InsertProfileEntries :batchexec
INSERT INTO change.profile_entry (organization_id, profile_version_id, component_id, language, analysis_kind, mode, profile_id, tool_versions,
    rules_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
