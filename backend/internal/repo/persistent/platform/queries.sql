-- name: CreateTenant :exec
INSERT INTO platform.tenant (organization_id, status, created_at) VALUES ($1, $2, $3);

-- name: CreateOrgClock :exec
INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0);

-- name: NextSeq :one
UPDATE platform.org_clock SET last_seq = last_seq + 1 WHERE organization_id = $1 RETURNING last_seq;

-- name: ActiveTenants :many
SELECT organization_id FROM platform.tenant WHERE status = 'active' ORDER BY organization_id;
