-- +goose Up
CREATE TABLE sync.activity_check (
    organization_id uuid NOT NULL,
    run_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    object_kind text NOT NULL CHECK (object_kind IN ('task', 'merge_request')),
    native_ns text COLLATE "C" NOT NULL,
    native_id text COLLATE "C" NOT NULL,
    matches boolean NOT NULL,
    PRIMARY KEY (organization_id, run_id, scope_id, object_kind, native_ns, native_id),
    FOREIGN KEY (organization_id, run_id) REFERENCES sync.reconcile_run (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE sync.activity_recovery (
    organization_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    receipt_id uuid NOT NULL,
    object_kind text NOT NULL CHECK (object_kind IN ('task', 'merge_request')),
    native_ns text COLLATE "C" NOT NULL,
    native_id text COLLATE "C" NOT NULL,
    observed_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, scope_id, object_kind, native_ns, native_id, receipt_id)
);

CREATE TABLE sync.activity_pagination (
    organization_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    receipt_id uuid NOT NULL,
    data_kind text NOT NULL CHECK (data_kind IN ('tasks', 'merge_requests')),
    observed_at timestamptz NOT NULL,
    complete boolean NOT NULL,
    PRIMARY KEY (organization_id, scope_id, data_kind, observed_at, receipt_id)
);

CREATE INDEX activity_receipt_scope_idx ON ingest.receipt (organization_id, scope_id, received_at)
    WHERE schema_id IN ('task_events', 'merge_requests') AND status IN ('accepted', 'partial', 'duplicate');

ALTER TABLE sync.activity_check ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.activity_check TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.activity_check FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE sync.activity_recovery ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.activity_recovery TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.activity_recovery FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE sync.activity_pagination ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.activity_pagination TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.activity_pagination FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE ON sync.activity_check TO craftpulse_app;
GRANT SELECT, INSERT ON sync.activity_recovery, sync.activity_pagination TO craftpulse_app;
GRANT SELECT ON sync.activity_check, sync.activity_recovery, sync.activity_pagination TO craftpulse_backup;

-- +goose Down
DROP INDEX ingest.activity_receipt_scope_idx;
DROP TABLE sync.activity_pagination, sync.activity_recovery, sync.activity_check;
