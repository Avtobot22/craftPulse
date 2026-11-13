-- +goose Up
CREATE TABLE connect.probe (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    connection_id uuid NOT NULL,
    configuration_version integer NOT NULL CHECK (configuration_version > 0),
    status text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    instance_key text COLLATE "C" CHECK (octet_length(instance_key) BETWEEN 1 AND 512),
    edition text CHECK (octet_length(edition) BETWEEN 1 AND 64),
    source_version text CHECK (octet_length(source_version) BETWEEN 1 AND 64),
    contract_version text CHECK (octet_length(contract_version) BETWEEN 1 AND 16),
    access_mode text CHECK (access_mode IN ('service_account', 'user_token', 'synced')),
    capabilities jsonb NOT NULL CHECK (jsonb_typeof(capabilities) = 'array'),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, connection_id, id),
    FOREIGN KEY (organization_id, connection_id) REFERENCES connect.connection (organization_id, id),
    CHECK ((finished_at IS NOT NULL) = (status IN ('succeeded', 'failed'))),
    CHECK (status <> 'failed' OR reason_code IS NOT NULL),
    CHECK (status <> 'succeeded' OR (instance_key IS NOT NULL AND contract_version IS NOT NULL))
);
-- One probe runs per connection; a repeated start finds it.
CREATE UNIQUE INDEX probe_running_uq ON connect.probe (organization_id, connection_id) WHERE status IN ('pending', 'running');
CREATE INDEX probe_latest_idx ON connect.probe (organization_id, connection_id, started_at DESC, id DESC);
CREATE INDEX probe_finished_idx ON connect.probe (organization_id, finished_at) WHERE finished_at IS NOT NULL;

ALTER TABLE connect.connection
    ADD COLUMN credentials_set_at timestamptz,
    ADD COLUMN sync_paused boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT connection_activated_probe_fkey FOREIGN KEY (organization_id, id, activated_probe_id)
        REFERENCES connect.probe (organization_id, connection_id, id);
ALTER TABLE connect.connection ALTER COLUMN sync_paused DROP DEFAULT;
CREATE INDEX connection_page_idx ON connect.connection (organization_id, created_at DESC, id DESC);

-- Activation moves a connection to the instance its probe named; its scopes follow it.
ALTER TABLE connect.connection_scope
    DROP CONSTRAINT connection_scope_organization_id_connection_id_instance_id_fkey,
    ADD CONSTRAINT connection_scope_connection_fkey FOREIGN KEY (organization_id, connection_id, instance_id)
        REFERENCES connect.connection (organization_id, id, instance_id) ON UPDATE CASCADE;

ALTER TABLE connect.probe ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON connect.probe TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON connect.probe FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON connect.probe TO craftpulse_app;
GRANT SELECT ON connect.probe TO craftpulse_backup;

-- +goose Down
ALTER TABLE connect.connection_scope
    DROP CONSTRAINT connection_scope_connection_fkey,
    ADD CONSTRAINT connection_scope_organization_id_connection_id_instance_id_fkey
        FOREIGN KEY (organization_id, connection_id, instance_id) REFERENCES connect.connection (organization_id, id, instance_id);
DROP INDEX connect.connection_page_idx;
ALTER TABLE connect.connection
    DROP CONSTRAINT connection_activated_probe_fkey,
    DROP COLUMN credentials_set_at,
    DROP COLUMN sync_paused;
DROP TABLE connect.probe;
