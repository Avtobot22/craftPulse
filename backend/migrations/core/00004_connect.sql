-- +goose Up
CREATE SCHEMA connect;
GRANT USAGE ON SCHEMA connect TO craftpulse_app, craftpulse_backup;

CREATE TABLE connect.source_instance (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    provider text NOT NULL CHECK (provider IN ('jira_cloud', 'gitlab', 'github', 'jenkins', 'sonarqube',
        'confluence_cloud', 'git', 'universal', 'client')),
    instance_key text COLLATE "C" NOT NULL CHECK (octet_length(instance_key) BETWEEN 1 AND 512),
    base_url text CHECK (octet_length(base_url) BETWEEN 1 AND 2048),
    edition text CHECK (octet_length(edition) BETWEEN 1 AND 64),
    source_version text CHECK (octet_length(source_version) BETWEEN 1 AND 64),
    verified_at timestamptz,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, provider, instance_key)
);

CREATE TABLE connect.connection (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    instance_id uuid NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    adapter_kind text NOT NULL CHECK (adapter_kind IN ('builtin', 'client')),
    adapter_url text CHECK (octet_length(adapter_url) BETWEEN 1 AND 2048),
    adapter_key_enc bytea,
    key_version integer CHECK (key_version > 0),
    contract_version text NOT NULL CHECK (octet_length(contract_version) BETWEEN 1 AND 16),
    access_mode text NOT NULL CHECK (access_mode IN ('service_account', 'user_token', 'synced')),
    status text NOT NULL CHECK (status IN ('draft', 'active', 'access_lost', 'deleted')),
    paused_until timestamptz,
    access_lost_at timestamptz,
    row_version integer NOT NULL CHECK (row_version > 0),
    activated_probe_id uuid,
    activated_row_version integer,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    deleted_at timestamptz,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, id, instance_id),
    FOREIGN KEY (organization_id, instance_id) REFERENCES connect.source_instance (organization_id, id),
    CHECK (CASE adapter_kind
        WHEN 'client' THEN num_nonnulls(adapter_url, adapter_key_enc, key_version) = 3
        ELSE num_nulls(adapter_url, adapter_key_enc, key_version) = 3 END),
    CHECK ((status = 'deleted') = (deleted_at IS NOT NULL)),
    CHECK ((status = 'access_lost') = (access_lost_at IS NOT NULL)),
    CHECK (num_nulls(activated_probe_id, activated_row_version) IN (0, 2)),
    CHECK (activated_row_version <= row_version)
);
CREATE INDEX connection_instance_idx ON connect.connection (organization_id, instance_id);

CREATE TABLE connect.connection_scope (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    connection_id uuid NOT NULL,
    instance_id uuid NOT NULL,
    scope_kind text NOT NULL CHECK (scope_kind IN ('tracker_project', 'repository', 'wiki_space', 'ci_project',
        'analysis_project')),
    native_scope_id text COLLATE "C" NOT NULL CHECK (octet_length(native_scope_id) BETWEEN 1 AND 255),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 255),
    history_from timestamptz,
    repository_scope_id uuid,
    area_id uuid,
    status text NOT NULL CHECK (status IN ('active', 'removed')),
    added_at timestamptz NOT NULL,
    removed_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, connection_id, instance_id)
        REFERENCES connect.connection (organization_id, id, instance_id),
    CHECK ((status = 'removed') = (removed_at IS NOT NULL))
);
-- One active connection collects a scope of an instance (TZ 12.4).
CREATE UNIQUE INDEX connection_scope_active_uq ON connect.connection_scope
    (organization_id, instance_id, scope_kind, native_scope_id) WHERE status = 'active';
CREATE INDEX connection_scope_connection_idx ON connect.connection_scope (organization_id, connection_id, instance_id);

CREATE TABLE connect.capability (
    organization_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    capability text NOT NULL CHECK (capability IN ('read_current', 'read_status_history', 'read_field_history',
        'read_resolution_history', 'read_sprint_history', 'read_links', 'read_permissions', 'read_deletions',
        'write_mr_comment', 'write_tracker_issue', 'webhook')),
    state text NOT NULL CHECK (state IN ('verified', 'partial', 'missing')),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    checked_at timestamptz NOT NULL,
    source_version text CHECK (octet_length(source_version) BETWEEN 1 AND 64),
    PRIMARY KEY (organization_id, connection_id, capability),
    FOREIGN KEY (organization_id, connection_id) REFERENCES connect.connection (organization_id, id)
);

ALTER TABLE connect.source_instance ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON connect.source_instance TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON connect.source_instance FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE connect.connection ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON connect.connection TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON connect.connection FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE connect.connection_scope ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON connect.connection_scope TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON connect.connection_scope FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE connect.capability ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON connect.capability TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON connect.capability FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA connect TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA connect TO craftpulse_backup;

-- +goose Down
DROP SCHEMA connect CASCADE;
