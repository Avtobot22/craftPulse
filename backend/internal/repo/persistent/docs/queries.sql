-- name: LockRootWrites :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(organization_id)::uuid::text, 161));

-- name: LatestRootSet :one
SELECT * FROM docs.root_set WHERE organization_id = $1 AND service_id = $2 ORDER BY version_no DESC LIMIT 1;

-- name: RootSetByConfirmation :one
SELECT * FROM docs.root_set WHERE organization_id = $1 AND confirmation_id = $2;

-- name: RootsAt :many
SELECT * FROM docs.root WHERE organization_id = $1 AND service_id = $2
    AND known_seq <= $3 AND (superseded_seq IS NULL OR superseded_seq > $3)
ORDER BY area_id, path_prefix;

-- name: InsertRootSet :exec
INSERT INTO docs.root_set (organization_id, service_id, version_no, known_seq, content_hash, confirmed_by, confirmed_at, confirmation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: SupersedeRoots :exec
UPDATE docs.root SET superseded_seq = $3, superseded_at = $4
WHERE organization_id = $1 AND service_id = $2 AND superseded_seq IS NULL;

-- name: InsertRoot :exec
INSERT INTO docs.root (organization_id, id, service_id, area_id, path_prefix, profile_id, owner_person_id, audience,
    known_seq, confirmed_by, confirmed_at, confirmation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);
