-- name: LockEdgeWrites :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(organization_id)::uuid::text, 171));

-- name: Edge :one
SELECT * FROM link.edge WHERE organization_id = $1 AND id = $2;

-- name: CurrentEdge :one
SELECT * FROM link.edge
WHERE organization_id = $1 AND link_type = $2 AND from_object_id = $3 AND to_object_id = $4 AND method = $5
    AND superseded_seq IS NULL;

-- name: CurrentConfirmedEdge :one
SELECT * FROM link.edge
WHERE organization_id = $1 AND link_type = $2 AND from_object_id = $3 AND to_object_id = $4
    AND status = 'confirmed' AND superseded_seq IS NULL
ORDER BY method = 'source_explicit' DESC, known_seq
LIMIT 1;

-- name: EdgeByConfirmation :one
SELECT * FROM link.edge WHERE organization_id = $1 AND confirmation_id = $2;

-- name: EdgesFromAt :many
SELECT * FROM link.edge
WHERE organization_id = sqlc.arg(organization_id) AND from_object_id = sqlc.arg(object_id)
    AND known_seq <= sqlc.arg(seq) AND (superseded_seq IS NULL OR superseded_seq > sqlc.arg(seq))
ORDER BY link_type, to_object_id, method;

-- name: EdgesToAt :many
SELECT * FROM link.edge
WHERE organization_id = sqlc.arg(organization_id) AND to_object_id = sqlc.arg(object_id)
    AND known_seq <= sqlc.arg(seq) AND (superseded_seq IS NULL OR superseded_seq > sqlc.arg(seq))
ORDER BY link_type, from_object_id, method;

-- name: InsertEdge :exec
INSERT INTO link.edge (organization_id, id, link_type, from_object_id, to_object_id, method, status, known_seq,
    receipt_id, record_no, evidence, actor_user_id, confirmation_id, decided_at, valid)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: SupersedeEdge :execrows
UPDATE link.edge SET superseded_seq = $3, superseded_at = $4
WHERE organization_id = $1 AND id = $2 AND superseded_seq IS NULL;
