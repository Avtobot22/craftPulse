-- +goose Up
CREATE SCHEMA ingest;
GRANT USAGE ON SCHEMA ingest TO craftpulse_app, craftpulse_backup;

CREATE TABLE ingest.receipt (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    channel text NOT NULL CHECK (channel IN ('sync', 'ci', 'import')),
    connection_id uuid,
    scope_id uuid,
    project_id uuid,
    upload_token_id uuid,
    requested_by uuid,
    source_instance_id uuid,
    received_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('quarantined', 'accepted', 'partial', 'rejected', 'duplicate')),
    duplicate_of uuid,
    schema_id text CHECK (octet_length(schema_id) BETWEEN 1 AND 64),
    schema_version text CHECK (octet_length(schema_version) BETWEEN 1 AND 16),
    package_hash bytea CHECK (octet_length(package_hash) = 32),
    object_key text CHECK (octet_length(object_key) BETWEEN 1 AND 1024),
    quarantine_key text CHECK (octet_length(quarantine_key) BETWEEN 1 AND 1024),
    record_count integer NOT NULL CHECK (record_count >= 0),
    accepted_count integer NOT NULL CHECK (accepted_count >= 0),
    rejected_count integer NOT NULL CHECK (rejected_count >= 0),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    repacked_at timestamptz,
    manifest jsonb,
    PRIMARY KEY (organization_id, id),
    CHECK (channel <> 'sync' OR connection_id IS NOT NULL),
    CHECK (channel <> 'ci' OR (project_id IS NOT NULL AND upload_token_id IS NOT NULL)),
    CHECK (channel <> 'import' OR requested_by IS NOT NULL),
    CHECK ((status = 'duplicate') = (duplicate_of IS NOT NULL)),
    CHECK (status NOT IN ('accepted', 'partial') OR (object_key IS NOT NULL AND package_hash IS NOT NULL)),
    CHECK (status <> 'rejected' OR reason_code IS NOT NULL),
    CHECK (accepted_count + rejected_count <= record_count)
);
-- A repeat of the same cleaned package, also after a restore (TZ 14.8).
CREATE INDEX receipt_package_hash_idx ON ingest.receipt (organization_id, package_hash) WHERE package_hash IS NOT NULL;
CREATE INDEX receipt_received_idx ON ingest.receipt (organization_id, received_at);

CREATE TABLE ingest.record (
    organization_id uuid NOT NULL,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    record_class text CHECK (record_class IN ('event', 'object', 'snapshot', 'file')),
    record_kind text CHECK (octet_length(record_kind) BETWEEN 1 AND 64),
    payload_schema text COLLATE "C" NOT NULL CHECK (octet_length(payload_schema) <= 255),
    instance_id uuid,
    native_ns text COLLATE "C" NOT NULL CHECK (octet_length(native_ns) <= 255),
    native_id text COLLATE "C" CHECK (octet_length(native_id) BETWEEN 1 AND 1024),
    source_revision text COLLATE "C" CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea CHECK (octet_length(content_hash) = 32),
    outcome text NOT NULL CHECK (outcome IN ('accepted', 'replay', 'conflict', 'rejected', 'suppressed')),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    first_receipt_id uuid,
    first_record_no integer,
    deleted_at timestamptz,
    PRIMARY KEY (organization_id, receipt_id, record_no),
    FOREIGN KEY (organization_id, receipt_id) REFERENCES ingest.receipt (organization_id, id) ON DELETE CASCADE,
    CHECK (outcome = 'rejected' OR num_nonnulls(record_class, record_kind, instance_id, native_id, source_revision, content_hash) = 6),
    CHECK (outcome <> 'rejected' OR reason_code IS NOT NULL),
    CHECK ((outcome IN ('replay', 'conflict')) = (first_receipt_id IS NOT NULL AND first_record_no IS NOT NULL))
);
-- One accepted record per key: the insert of a record tells a new fact from a replay and a conflict.
CREATE UNIQUE INDEX record_accepted_uq ON ingest.record
    (organization_id, instance_id, record_kind, native_ns, native_id, source_revision, payload_schema) WHERE outcome = 'accepted';
CREATE INDEX record_object_idx ON ingest.record (organization_id, instance_id, record_kind, native_ns, native_id);

CREATE TABLE ingest.conflict (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL,
    existing_receipt_id uuid NOT NULL,
    existing_record_no integer NOT NULL,
    detected_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('open', 'resolved')),
    resolution text CHECK (resolution IN ('source_corrected', 'kept_existing', 'accepted_new')),
    resolved_by uuid,
    resolved_at timestamptz,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, receipt_id, record_no),
    FOREIGN KEY (organization_id, receipt_id, record_no) REFERENCES ingest.record (organization_id, receipt_id, record_no) ON DELETE CASCADE,
    CHECK ((status = 'open') = (resolution IS NULL)),
    CHECK ((resolution IS NULL) = (resolved_at IS NULL))
);
CREATE INDEX conflict_open_idx ON ingest.conflict (organization_id, detected_at) WHERE status = 'open';

ALTER TABLE ingest.receipt ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON ingest.receipt TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON ingest.receipt FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE ingest.record ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON ingest.record TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON ingest.record FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE ingest.conflict ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON ingest.conflict TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON ingest.conflict FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ingest TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA ingest TO craftpulse_backup;

-- +goose Down
DROP SCHEMA ingest CASCADE;
