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

-- name: IdentitiesByFingerprint :many
SELECT id, fingerprint FROM change.finding_identity
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND method_version = sqlc.arg(method_version)
    AND fingerprint = ANY (sqlc.arg(fingerprints)::bytea[]);

-- name: IdentitiesByContent :many
SELECT id, rule_id, content_fingerprint FROM change.finding_identity
WHERE organization_id = sqlc.arg(organization_id) AND area_id = sqlc.arg(area_id) AND method_version = sqlc.arg(method_version)
    AND content_fingerprint = ANY (sqlc.arg(content_fingerprints)::bytea[])
ORDER BY first_seen_at, id;

-- The update of nothing returns the row a concurrent job inserted first.
-- name: InsertFindingIdentity :one
INSERT INTO change.finding_identity (organization_id, area_id, method_version, fingerprint, content_fingerprint, rule_id, finding_kind,
    first_report_id, first_seen_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(area_id), sqlc.arg(method_version), sqlc.arg(fingerprint), sqlc.narg(content_fingerprint),
    sqlc.arg(rule_id), sqlc.arg(finding_kind), sqlc.arg(first_report_id), sqlc.arg(first_seen_at))
ON CONFLICT (organization_id, area_id, method_version, fingerprint) DO UPDATE SET fingerprint = EXCLUDED.fingerprint
RETURNING id;

-- name: AssignedOccurrences :many
SELECT occurrence_id FROM change.occurrence_identity
WHERE organization_id = sqlc.arg(organization_id) AND occurrence_id = ANY (sqlc.arg(occurrence_ids)::uuid[]);

-- name: InsertOccurrenceIdentities :exec
INSERT INTO change.occurrence_identity (organization_id, occurrence_id, finding_identity_id, match_kind, reported_at, known_seq)
SELECT sqlc.arg(organization_id), unnest(sqlc.arg(occurrence_ids)::uuid[]), unnest(sqlc.arg(finding_identity_ids)::uuid[]),
    unnest(sqlc.arg(match_kinds)::text[]), sqlc.arg(reported_at), sqlc.arg(known_seq)
ON CONFLICT (organization_id, occurrence_id) DO NOTHING;

-- name: FindingIdentity :one
SELECT * FROM change.finding_identity WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ReportIdentities :many
SELECT * FROM change.occurrence_identity
WHERE organization_id = sqlc.arg(organization_id) AND occurrence_id = ANY (sqlc.arg(occurrence_ids)::uuid[])
ORDER BY occurrence_id;

-- name: LockCurrentBaseline :one
SELECT * FROM change.baseline
WHERE organization_id = sqlc.arg(organization_id) AND component_id = sqlc.arg(component_id) AND language = sqlc.arg(language)
    AND analysis_kind = sqlc.arg(analysis_kind) AND superseded_at IS NULL
FOR UPDATE;

-- name: SupersedeBaseline :exec
UPDATE change.baseline SET superseded_at = sqlc.arg(superseded_at) WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: InsertBaseline :exec
INSERT INTO change.baseline (organization_id, component_id, language, analysis_kind, profile_version_id, report_id, established_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(component_id), sqlc.arg(language), sqlc.arg(analysis_kind), sqlc.arg(profile_version_id),
    sqlc.arg(report_id), sqlc.arg(established_at));

-- name: CurrentBaselines :many
SELECT * FROM change.baseline
WHERE organization_id = sqlc.arg(organization_id) AND component_id = ANY (sqlc.arg(component_ids)::uuid[]) AND superseded_at IS NULL
ORDER BY component_id, language, analysis_kind;
-- name: PipelineExpectedReports :many
WITH versions AS (
    SELECT DISTINCT ON (v.project_id) v.id
    FROM change.profile_version v
    JOIN org.project_area a ON a.organization_id = v.organization_id AND a.project_id = v.project_id
    WHERE v.organization_id = sqlc.arg(organization_id) AND a.area_id = sqlc.arg(area_id)
        AND (sqlc.arg(whole)::boolean OR a.project_id = ANY(sqlc.arg(project_ids)::uuid[]))
        AND a.superseded_seq IS NULL AND a.valid @> sqlc.arg(run_at)::timestamptz
        AND v.confirmed_at <= sqlc.arg(run_at)::timestamptz
    ORDER BY v.project_id, v.version_no DESC
)
SELECT DISTINCT e.component_id, e.analysis_kind, e.language
FROM versions v
JOIN change.profile_entry e ON e.organization_id = sqlc.arg(organization_id) AND e.profile_version_id = v.id
JOIN org.component c ON c.organization_id = e.organization_id AND c.id = e.component_id
WHERE c.repository_area_id = sqlc.arg(area_id) AND c.status = 'confirmed'
    AND c.confirmed_at <= sqlc.arg(run_at)::timestamptz
    AND (c.superseded_at IS NULL OR c.superseded_at > sqlc.arg(run_at)::timestamptz)
ORDER BY e.analysis_kind, e.component_id, e.language;

-- name: ExceptionFindingKinds :many
SELECT DISTINCT f.finding_kind FROM change.finding_identity f
JOIN change.occurrence_identity i ON i.organization_id = f.organization_id AND i.finding_identity_id = f.id
JOIN normalize.finding_occurrence o ON o.organization_id = i.organization_id AND o.id = i.occurrence_id
JOIN normalize.report r ON r.organization_id = o.organization_id AND r.id = o.report_id
WHERE f.organization_id = sqlc.arg(organization_id) AND f.area_id = sqlc.arg(area_id)
    AND (sqlc.narg(finding_id)::uuid IS NULL OR f.id = sqlc.narg(finding_id))
    AND (sqlc.narg(rule_id)::text IS NULL OR f.rule_id = sqlc.narg(rule_id))
    AND (sqlc.narg(component_id)::uuid IS NULL OR (r.component_id = sqlc.narg(component_id) AND EXISTS (
        SELECT 1 FROM org.component c WHERE c.organization_id = f.organization_id AND c.id = r.component_id
            AND c.repository_area_id = f.area_id AND c.superseded_seq IS NULL AND c.status = 'confirmed')))
    AND (sqlc.narg(path_prefix)::text IS NULL OR o.path = sqlc.narg(path_prefix) OR starts_with(o.path, sqlc.narg(path_prefix) || '/'))
    AND (sqlc.narg(package_purl)::text IS NULL OR o.package_purl = sqlc.narg(package_purl))
ORDER BY f.finding_kind;

-- name: StoreExceptionProjection :exec
INSERT INTO change.exception_projection (organization_id, area_id, as_of, known_seq, active_version_ids)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (organization_id, area_id) DO UPDATE
SET as_of = EXCLUDED.as_of, known_seq = EXCLUDED.known_seq, active_version_ids = EXCLUDED.active_version_ids
WHERE (change.exception_projection.as_of, change.exception_projection.known_seq) <= (EXCLUDED.as_of, EXCLUDED.known_seq);
