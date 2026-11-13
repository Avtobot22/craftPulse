-- name: SaveReceipt :exec
INSERT INTO ingest.receipt (organization_id, id, channel, connection_id, scope_id, project_id, upload_token_id, requested_by,
    source_instance_id, received_at, status, duplicate_of, schema_id, schema_version, package_hash, object_key, quarantine_key,
    record_count, accepted_count, rejected_count, reason_code, manifest)
VALUES (sqlc.arg(organization_id), sqlc.arg(id), sqlc.arg(channel), sqlc.narg(connection_id), sqlc.narg(scope_id),
    sqlc.narg(project_id), sqlc.narg(upload_token_id), sqlc.narg(requested_by), sqlc.narg(source_instance_id),
    sqlc.arg(received_at), sqlc.arg(status), sqlc.narg(duplicate_of), sqlc.narg(schema_id), sqlc.narg(schema_version),
    sqlc.narg(package_hash), sqlc.narg(object_key), sqlc.narg(quarantine_key), sqlc.arg(record_count),
    sqlc.arg(accepted_count), sqlc.arg(rejected_count), sqlc.narg(reason_code), sqlc.narg(manifest))
ON CONFLICT (organization_id, id) DO UPDATE SET
    source_instance_id = EXCLUDED.source_instance_id, status = EXCLUDED.status, duplicate_of = EXCLUDED.duplicate_of,
    schema_id = EXCLUDED.schema_id, schema_version = EXCLUDED.schema_version, package_hash = EXCLUDED.package_hash,
    object_key = EXCLUDED.object_key, record_count = EXCLUDED.record_count, accepted_count = EXCLUDED.accepted_count,
    rejected_count = EXCLUDED.rejected_count, reason_code = EXCLUDED.reason_code, manifest = EXCLUDED.manifest;

-- name: GetReceipt :one
SELECT * FROM ingest.receipt WHERE organization_id = $1 AND id = $2;

-- name: LockReceipt :one
SELECT * FROM ingest.receipt WHERE organization_id = $1 AND id = $2 FOR UPDATE;

-- name: FirstReceiptOfHash :one
SELECT id FROM ingest.receipt
WHERE organization_id = $1 AND package_hash = $2 AND status IN ('accepted', 'partial')
ORDER BY received_at, id
LIMIT 1;

-- name: LockPackageHash :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(organization_id)::text || encode(sqlc.arg(package_hash)::bytea, 'hex'), 0));

-- name: AcceptRecord :batchone
INSERT INTO ingest.record (organization_id, receipt_id, record_no, record_class, record_kind, payload_schema, instance_id,
    native_ns, native_id, source_revision, content_hash, outcome)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'accepted')
ON CONFLICT (organization_id, instance_id, record_kind, native_ns, native_id, source_revision, payload_schema)
    WHERE outcome = 'accepted' DO NOTHING
RETURNING record_no;

-- name: AcceptedRecord :batchone
SELECT receipt_id, record_no, content_hash FROM ingest.record
WHERE organization_id = $1 AND instance_id = $2 AND record_kind = $3 AND native_ns = $4 AND native_id = $5
    AND source_revision = $6 AND payload_schema = $7 AND outcome = 'accepted';

-- name: InsertRecord :batchexec
INSERT INTO ingest.record (organization_id, receipt_id, record_no, record_class, record_kind, payload_schema, instance_id,
    native_ns, native_id, source_revision, content_hash, outcome, reason_code, first_receipt_id, first_record_no)
VALUES (sqlc.arg(organization_id), sqlc.arg(receipt_id), sqlc.arg(record_no), sqlc.narg(record_class), sqlc.narg(record_kind),
    sqlc.arg(payload_schema), sqlc.narg(instance_id), sqlc.arg(native_ns), sqlc.narg(native_id), sqlc.narg(source_revision),
    sqlc.narg(content_hash), sqlc.arg(outcome), sqlc.narg(reason_code), sqlc.narg(first_receipt_id), sqlc.narg(first_record_no));

-- name: InsertConflict :batchexec
INSERT INTO ingest.conflict (organization_id, receipt_id, record_no, existing_receipt_id, existing_record_no, detected_at, status)
VALUES ($1, $2, $3, $4, $5, $6, 'open');

-- name: ListRecords :many
SELECT * FROM ingest.record WHERE organization_id = $1 AND receipt_id = $2 ORDER BY record_no;
