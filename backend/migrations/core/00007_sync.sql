-- +goose Up
CREATE SCHEMA sync;
GRANT USAGE ON SCHEMA sync TO craftpulse_app, craftpulse_backup;

CREATE TABLE sync.state (
    organization_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    data_kind text NOT NULL CHECK (data_kind IN ('tasks', 'merge_requests', 'ci', 'reports', 'documents', 'accounts', 'permissions')),
    phase text NOT NULL CHECK (phase IN ('backfill', 'incremental', 'paused', 'stopped')),
    cursor text CHECK (octet_length(cursor) <= 4096),
    checkpoint_at timestamptz,
    failed_cursor text CHECK (octet_length(failed_cursor) <= 4096),
    last_attempt_at timestamptz,
    last_outcome text CHECK (last_outcome IN ('succeeded', 'failed')),
    last_error_code text CHECK (octet_length(last_error_code) BETWEEN 1 AND 64),
    last_success_at timestamptz,
    watermark timestamptz,
    history_from timestamptz,
    completeness text NOT NULL CHECK (completeness IN ('complete', 'partial', 'unknown')),
    PRIMARY KEY (organization_id, connection_id, data_kind),
    CHECK (watermark IS NULL OR last_success_at IS NOT NULL)
) WITH (fillfactor = 80);

-- One active attempt of collection per connection (NFR-006); a lease of a dead process runs out by itself.
CREATE TABLE sync.lease (
    organization_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    holder text NOT NULL CHECK (octet_length(holder) BETWEEN 1 AND 255),
    lease_until timestamptz NOT NULL,
    PRIMARY KEY (organization_id, connection_id)
);

ALTER TABLE sync.state ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.state TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.state FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE sync.lease ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.lease TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.lease FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA sync TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA sync TO craftpulse_backup;

-- +goose Down
DROP SCHEMA sync CASCADE;
