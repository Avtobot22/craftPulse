-- +goose Up
REVOKE INSERT, UPDATE, DELETE ON goose_db_version FROM craftpulse_app;

CREATE SCHEMA platform;
GRANT USAGE ON SCHEMA platform TO craftpulse_app, craftpulse_backup;

CREATE TABLE platform.tenant (
    organization_id uuid PRIMARY KEY,
    status text NOT NULL CHECK (status IN ('active', 'blocked', 'deleting')),
    created_at timestamptz NOT NULL
);
GRANT SELECT, INSERT, UPDATE, DELETE ON platform.tenant TO craftpulse_app;
GRANT SELECT ON platform.tenant TO craftpulse_backup;

CREATE TABLE platform.org_clock (
    organization_id uuid PRIMARY KEY,
    last_seq bigint NOT NULL CHECK (last_seq >= 0),
    reproducible_from_seq bigint NOT NULL,
    CHECK (reproducible_from_seq <= last_seq)
) WITH (fillfactor = 50, autovacuum_vacuum_scale_factor = 0.02);
ALTER TABLE platform.org_clock ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON platform.org_clock TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON platform.org_clock FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON platform.org_clock TO craftpulse_app;
GRANT SELECT ON platform.org_clock TO craftpulse_backup;

-- +goose Down
DROP SCHEMA platform CASCADE;
