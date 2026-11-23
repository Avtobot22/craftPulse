-- name: LockConfirmation :exec
SELECT pg_advisory_xact_lock(hashtextextended('decision.confirmation:' ||
    sqlc.arg(organization_id)::uuid::text || ':' || sqlc.arg(confirmation_id)::uuid::text, 0));

-- name: InsertDecision :one
INSERT INTO decision.decision (organization_id, kind, created_by, created_at)
VALUES ($1, 'exception', $2, $3) RETURNING id;

-- name: DecisionByID :one
SELECT * FROM decision.decision WHERE organization_id = $1 AND id = $2 AND kind = 'exception';

-- name: LockDecision :one
SELECT * FROM decision.decision WHERE organization_id = $1 AND id = $2 AND kind = 'exception' FOR UPDATE;

-- name: SetWake :exec
UPDATE decision.decision SET wake_at = sqlc.narg(wake_at) WHERE organization_id = $1 AND id = $2;

-- name: VersionByConfirmation :one
SELECT * FROM decision.decision_version WHERE organization_id = $1 AND confirmation_id = $2;

-- name: TransitionByConfirmation :one
SELECT t.*, v.decision_id, v.version_no FROM decision.decision_transition t
JOIN decision.decision_version v ON v.organization_id = t.organization_id AND v.id = t.decision_version_id
WHERE t.organization_id = $1 AND t.confirmation_id = $2;

-- name: InsertVersion :one
INSERT INTO decision.decision_version (
    organization_id, decision_id, version_no, base_version_no, decision_type, reason, area_id, component_id,
    finding_identity_id, finding_kind, rule_id, path_prefix, package_purl, effective_from, expires_at,
    approval_required, author_user_id, created_at, known_seq, confirmation_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(decision_id), sqlc.arg(version_no), sqlc.narg(base_version_no),
    sqlc.arg(decision_type), sqlc.arg(reason), sqlc.arg(area_id), sqlc.narg(component_id),
    sqlc.narg(finding_identity_id), sqlc.arg(finding_kind), sqlc.narg(rule_id), sqlc.narg(path_prefix),
    sqlc.narg(package_purl), sqlc.arg(effective_from), sqlc.arg(expires_at), sqlc.arg(approval_required),
    sqlc.arg(author_user_id), sqlc.arg(created_at), sqlc.arg(known_seq), sqlc.arg(confirmation_id))
RETURNING id;

-- name: InsertTransition :exec
INSERT INTO decision.decision_transition (organization_id, decision_version_id, status, actor_user_id, occurred_at,
    known_seq, confirmation_id, comment)
VALUES (sqlc.arg(organization_id), sqlc.arg(decision_version_id), sqlc.arg(status), sqlc.arg(actor_user_id),
    sqlc.arg(occurred_at), sqlc.arg(known_seq), sqlc.narg(confirmation_id), sqlc.arg(comment));

-- name: Versions :many
SELECT * FROM decision.decision_version
WHERE organization_id = sqlc.arg(organization_id) AND decision_id = ANY(sqlc.arg(decision_ids)::uuid[])
    AND known_seq <= sqlc.arg(known_seq)::bigint
ORDER BY decision_id, version_no DESC;

-- name: Transitions :many
SELECT t.*, v.decision_id, v.version_no FROM decision.decision_transition t
JOIN decision.decision_version v ON v.organization_id = t.organization_id AND v.id = t.decision_version_id
WHERE t.organization_id = sqlc.arg(organization_id) AND v.decision_id = ANY(sqlc.arg(decision_ids)::uuid[])
    AND t.known_seq <= sqlc.arg(known_seq)::bigint
ORDER BY t.occurred_at, t.known_seq, t.id;

-- name: AreaDecisionIDs :many
SELECT DISTINCT v.decision_id FROM decision.decision_version v
JOIN decision.decision d ON d.organization_id = v.organization_id AND d.id = v.decision_id AND d.kind = 'exception'
WHERE v.organization_id = sqlc.arg(organization_id) AND v.area_id = sqlc.arg(area_id)
    AND v.known_seq <= sqlc.arg(known_seq)::bigint AND v.effective_from <= sqlc.arg(as_of)
    AND v.expires_at > sqlc.arg(as_of)
ORDER BY v.decision_id;

-- name: PageDecisions :many
WITH latest AS (
    SELECT DISTINCT ON (d.id) d.id, v.component_id, v.finding_identity_id,
        (CASE WHEN sqlc.arg(by_expiry)::bool THEN v.expires_at ELSE d.created_at END)::timestamptz AS sort_at
    FROM decision.decision d
    JOIN decision.decision_version v ON v.organization_id = d.organization_id AND v.decision_id = d.id
    WHERE d.organization_id = sqlc.arg(organization_id) AND d.kind = 'exception' AND v.known_seq <= sqlc.arg(known_seq)::bigint
        AND v.created_at <= sqlc.arg(as_of)::timestamptz
    ORDER BY d.id, v.version_no DESC
)
SELECT id, sort_at FROM latest
WHERE (sqlc.narg(component_id)::uuid IS NULL OR component_id = sqlc.narg(component_id))
    AND (sqlc.narg(finding_id)::uuid IS NULL OR finding_identity_id = sqlc.narg(finding_id))
    AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
        OR (NOT sqlc.arg(descending)::bool AND (sort_at, id) > (sqlc.narg(after_sort_at), sqlc.narg(after_id)::uuid))
        OR (sqlc.arg(descending)::bool AND (sort_at, id) < (sqlc.narg(after_sort_at), sqlc.narg(after_id)::uuid)))
ORDER BY CASE WHEN NOT sqlc.arg(descending)::bool THEN sort_at END,
    CASE WHEN NOT sqlc.arg(descending)::bool THEN id END,
    CASE WHEN sqlc.arg(descending)::bool THEN sort_at END DESC,
    CASE WHEN sqlc.arg(descending)::bool THEN id END DESC
LIMIT sqlc.arg(page_size);
