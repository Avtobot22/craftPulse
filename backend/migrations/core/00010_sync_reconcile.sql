-- +goose Up
-- Objects to read again: a key added while it is read gets a later added_at and is not deleted (TZ 12.5.3).
CREATE TABLE sync.dirty_key (
    organization_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    key_hash bytea NOT NULL CHECK (octet_length(key_hash) = 32),
    object_kind text NOT NULL CHECK (octet_length(object_kind) BETWEEN 1 AND 64),
    native_ref text COLLATE "C" NOT NULL CHECK (octet_length(native_ref) BETWEEN 1 AND 4096),
    added_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, connection_id, key_hash)
);

CREATE TABLE sync.reconcile_run (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    connection_id uuid NOT NULL,
    data_kind text NOT NULL CHECK (data_kind IN ('tasks', 'merge_requests', 'ci', 'reports', 'documents', 'accounts', 'permissions')),
    as_of timestamptz NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    expected_count integer CHECK (expected_count >= 0),
    observed_count integer CHECK (observed_count >= 0),
    missing_count integer CHECK (missing_count >= 0),
    changed_count integer CHECK (changed_count >= 0),
    outcome text NOT NULL CHECK (outcome IN ('running', 'complete', 'incomplete', 'failed')),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    PRIMARY KEY (organization_id, id),
    CHECK ((outcome = 'running') = (finished_at IS NULL)),
    CHECK (outcome <> 'failed' OR reason_code IS NOT NULL)
);
-- The last reconciliation in the diagnostics (FR-020).
CREATE INDEX reconcile_run_last_idx ON sync.reconcile_run (organization_id, connection_id, data_kind, started_at DESC);
-- One reconciliation of a data kind runs at a time.
CREATE UNIQUE INDEX reconcile_run_running_uq ON sync.reconcile_run (organization_id, connection_id, data_kind) WHERE outcome = 'running';

ALTER TABLE sync.dirty_key ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.dirty_key TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.dirty_key FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE sync.reconcile_run ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.reconcile_run TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.reconcile_run FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON sync.dirty_key, sync.reconcile_run TO craftpulse_app;
GRANT SELECT ON sync.dirty_key, sync.reconcile_run TO craftpulse_backup;

-- +goose Down
DROP TABLE sync.reconcile_run;
DROP TABLE sync.dirty_key;
