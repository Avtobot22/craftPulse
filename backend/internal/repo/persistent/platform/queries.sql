-- name: CreateTenant :exec
INSERT INTO platform.tenant (organization_id, status, created_at) VALUES ($1, $2, $3);

-- name: CreateOrgClock :exec
INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0);
