-- +goose Up
CREATE SCHEMA org;
CREATE SCHEMA auth;
CREATE SCHEMA audit;
CREATE SCHEMA install;
GRANT USAGE ON SCHEMA org, auth, audit, install TO craftpulse_app, craftpulse_backup;

CREATE TABLE org.organization (
    organization_id uuid PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('setup', 'active', 'blocked', 'deleting')),
    first_admin_issuer text NOT NULL CHECK (octet_length(first_admin_issuer) BETWEEN 1 AND 512),
    first_admin_subject text COLLATE "C" NOT NULL CHECK (octet_length(first_admin_subject) BETWEEN 1 AND 255),
    first_admin_user_id uuid,
    structure_version bigint NOT NULL CHECK (structure_version >= 0),
    created_at timestamptz NOT NULL
);

CREATE TABLE org.team (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id)
);

CREATE TABLE org.project (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id)
);

CREATE TABLE org.service (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id)
);

CREATE TABLE org.person (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    display_name text,
    kind text NOT NULL CHECK (kind IN ('human', 'bot', 'system', 'unknown')),
    anonymized_at timestamptz,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    CHECK (anonymized_at IS NULL OR display_name IS NULL)
);

CREATE TABLE org.app_user (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    issuer text NOT NULL CHECK (octet_length(issuer) BETWEEN 1 AND 512),
    subject text COLLATE "C" NOT NULL CHECK (octet_length(subject) BETWEEN 1 AND 255),
    person_id uuid,
    display_name text,
    status text NOT NULL CHECK (status IN ('active', 'blocked')),
    created_at timestamptz NOT NULL,
    blocked_at timestamptz,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, issuer, subject),
    FOREIGN KEY (organization_id, person_id) REFERENCES org.person (organization_id, id),
    CHECK ((status = 'blocked') = (blocked_at IS NOT NULL))
);
CREATE INDEX app_user_person_idx ON org.app_user (organization_id, person_id) WHERE person_id IS NOT NULL;

CREATE TABLE org.role_assignment (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('administrator', 'dev_lead', 'project_manager', 'tech_lead', 'developer',
        'process_owner', 'service_owner', 'security_access', 'wiki_access')),
    team_id uuid,
    project_id uuid,
    service_id uuid,
    assigned_by uuid NOT NULL,
    assigned_at timestamptz NOT NULL,
    revoked_by uuid,
    revoked_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, user_id) REFERENCES org.app_user (organization_id, id),
    FOREIGN KEY (organization_id, team_id) REFERENCES org.team (organization_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES org.project (organization_id, id),
    FOREIGN KEY (organization_id, service_id) REFERENCES org.service (organization_id, id),
    CHECK (num_nonnulls(team_id, project_id, service_id) <= 1),
    CHECK (role <> 'administrator' OR num_nonnulls(team_id, project_id, service_id) = 0),
    CHECK (role <> 'process_owner' OR team_id IS NOT NULL),
    CHECK (role <> 'service_owner' OR service_id IS NOT NULL)
);
CREATE UNIQUE INDEX role_assignment_active_uq ON org.role_assignment
    (organization_id, user_id, role, team_id, project_id, service_id) NULLS NOT DISTINCT WHERE revoked_at IS NULL;
-- The partial unique index does not cover revoked rows, so the user FK needs its own index (TZ 12.2).
CREATE INDEX role_assignment_user_idx ON org.role_assignment (organization_id, user_id);
CREATE INDEX role_assignment_team_idx ON org.role_assignment (organization_id, team_id) WHERE team_id IS NOT NULL;
CREATE INDEX role_assignment_project_idx ON org.role_assignment (organization_id, project_id) WHERE project_id IS NOT NULL;
CREATE INDEX role_assignment_service_idx ON org.role_assignment (organization_id, service_id) WHERE service_id IS NOT NULL;

CREATE TABLE org.team_member (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    team_id uuid NOT NULL,
    person_id uuid NOT NULL,
    valid tstzrange NOT NULL CHECK (NOT lower_inf(valid) AND lower_inc(valid) AND NOT upper_inc(valid)),
    known_seq bigint NOT NULL,
    superseded_seq bigint,
    superseded_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, team_id) REFERENCES org.team (organization_id, id),
    FOREIGN KEY (organization_id, person_id) REFERENCES org.person (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    EXCLUDE USING gist (organization_id WITH =, team_id WITH =, person_id WITH =, valid WITH &&)
        WHERE (superseded_seq IS NULL)
);
CREATE INDEX team_member_team_idx ON org.team_member (organization_id, team_id);
CREATE INDEX team_member_person_idx ON org.team_member (organization_id, person_id);

CREATE TABLE org.team_project (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    team_id uuid NOT NULL,
    project_id uuid NOT NULL,
    valid tstzrange NOT NULL CHECK (NOT lower_inf(valid) AND lower_inc(valid) AND NOT upper_inc(valid)),
    known_seq bigint NOT NULL,
    superseded_seq bigint,
    superseded_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, team_id) REFERENCES org.team (organization_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES org.project (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    EXCLUDE USING gist (organization_id WITH =, team_id WITH =, project_id WITH =, valid WITH &&)
        WHERE (superseded_seq IS NULL)
);
CREATE INDEX team_project_team_idx ON org.team_project (organization_id, team_id);
CREATE INDEX team_project_project_idx ON org.team_project (organization_id, project_id);

CREATE TABLE auth.login_route (
    slug text COLLATE "C" PRIMARY KEY CHECK (slug ~ '^[a-z0-9-]{2,40}$'),
    organization_id uuid NOT NULL UNIQUE
);

CREATE TABLE auth.provider_registration (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    issuer text NOT NULL CHECK (octet_length(issuer) BETWEEN 1 AND 512),
    client_id text NOT NULL CHECK (octet_length(client_id) BETWEEN 1 AND 255),
    client_secret_enc bytea NOT NULL,
    key_version integer NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id)
);
CREATE UNIQUE INDEX provider_registration_active_uq ON auth.provider_registration (organization_id) WHERE status = 'active';

CREATE TABLE auth.login_attempt (
    cookie_hash bytea PRIMARY KEY CHECK (octet_length(cookie_hash) = 32),
    organization_id uuid NOT NULL,
    state text NOT NULL,
    nonce text NOT NULL,
    pkce_verifier text NOT NULL,
    issuer text NOT NULL,
    return_path text NOT NULL CHECK (return_path ~ '^/([^/]|$)'),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX login_attempt_expires_idx ON auth.login_attempt (expires_at);

CREATE TABLE auth.session (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    last_activity_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    ended_at timestamptz,
    end_reason text CHECK (end_reason IN ('logout', 'admin', 'blocked', 'limit', 'replaced')),
    PRIMARY KEY (organization_id, id),
    CHECK ((ended_at IS NULL) = (end_reason IS NULL)),
    CHECK (expires_at > created_at)
) WITH (fillfactor = 90);
CREATE INDEX session_user_active_idx ON auth.session (organization_id, user_id, created_at) WHERE ended_at IS NULL;
CREATE INDEX session_expires_idx ON auth.session (expires_at);

CREATE TABLE auth.api_key (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    owner_user_id uuid NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    PRIMARY KEY (organization_id, id),
    CHECK (expires_at > created_at)
);
CREATE INDEX api_key_owner_active_idx ON auth.api_key (organization_id, owner_user_id) WHERE revoked_at IS NULL;

CREATE TABLE auth.api_key_grant (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    api_key_id uuid NOT NULL,
    team_id uuid,
    project_id uuid,
    service_id uuid,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, api_key_id) REFERENCES auth.api_key (organization_id, id) ON DELETE CASCADE,
    CHECK (num_nonnulls(team_id, project_id, service_id) = 1)
);
CREATE UNIQUE INDEX api_key_grant_uq ON auth.api_key_grant
    (organization_id, api_key_id, team_id, project_id, service_id) NULLS NOT DISTINCT;

CREATE TABLE auth.upload_token (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    project_id uuid NOT NULL,
    slot smallint NOT NULL CHECK (slot IN (1, 2)),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    revoked_at timestamptz,
    PRIMARY KEY (organization_id, id)
);
CREATE UNIQUE INDEX upload_token_slot_uq ON auth.upload_token (organization_id, project_id, slot) WHERE revoked_at IS NULL;

CREATE TABLE audit.event (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    occurred_at timestamptz NOT NULL,
    actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'api_key', 'upload_token', 'system', 'source')),
    actor_id uuid,
    operation text NOT NULL CHECK (octet_length(operation) BETWEEN 1 AND 255),
    object_kind text,
    object_id uuid,
    area_id uuid,
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'denied', 'failed')),
    reason_code text,
    base_version text,
    new_version text,
    correlation_id uuid NOT NULL,
    details jsonb CHECK (pg_column_size(details) <= 4096),
    PRIMARY KEY (organization_id, id)
);
CREATE INDEX event_occurred_idx ON audit.event (organization_id, occurred_at);
CREATE INDEX event_actor_idx ON audit.event (organization_id, actor_id, occurred_at) WHERE actor_id IS NOT NULL;

CREATE TABLE audit.deletion (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    requested_at timestamptz NOT NULL,
    requested_by uuid,
    basis text NOT NULL CHECK (basis IN ('user_request', 'connection_deleted', 'organization_deleted', 'retention')),
    scope_kind text NOT NULL CHECK (scope_kind IN ('object', 'area', 'connection', 'class', 'organization')),
    object_kind text,
    object_id uuid,
    retention_class text,
    cutoff_at timestamptz,
    status text NOT NULL CHECK (status IN ('pending', 'recorded', 'applied')),
    backup_object_key text,
    recorded_at timestamptz,
    applied_at timestamptz,
    PRIMARY KEY (organization_id, id),
    CHECK (basis <> 'user_request' OR status = 'pending' OR backup_object_key IS NOT NULL)
);
CREATE INDEX deletion_open_idx ON audit.deletion (organization_id, status, requested_at) WHERE status <> 'applied';

CREATE TABLE install.protocol (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    release_sha text COLLATE "C" NOT NULL CHECK (release_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    manifest jsonb NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('running', 'passed', 'failed'))
);
CREATE INDEX protocol_started_idx ON install.protocol (started_at);

CREATE TABLE install.check_result (
    protocol_id uuid NOT NULL REFERENCES install.protocol (id) ON DELETE CASCADE,
    check_kind text NOT NULL CHECK (check_kind IN ('site', 'adapter', 'adapter_store', 'identity_provider',
        'update_source', 'database', 'object_store', 'runner_network')),
    outcome text NOT NULL CHECK (outcome IN ('passed', 'failed', 'skipped')),
    reason_code text,
    detail text,
    PRIMARY KEY (protocol_id, check_kind)
);

ALTER TABLE org.organization ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.organization TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.organization FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.team ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.team TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.team FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.project ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.project TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.project FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.service ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.service TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.service FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.person ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.person TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.person FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.app_user ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.app_user TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.app_user FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.role_assignment ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.role_assignment TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.role_assignment FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.team_member ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.team_member TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.team_member FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE org.team_project ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.team_project TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.team_project FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE auth.provider_registration ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON auth.provider_registration TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON auth.provider_registration FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE auth.api_key_grant ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON auth.api_key_grant TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON auth.api_key_grant FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE audit.event ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON audit.event TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON audit.event FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE audit.deletion ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON audit.deletion TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON audit.deletion FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA org, auth, install TO craftpulse_app;
GRANT SELECT, INSERT, DELETE ON ALL TABLES IN SCHEMA audit TO craftpulse_app;
GRANT UPDATE ON audit.deletion TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA org, auth, audit, install TO craftpulse_backup;

-- +goose Down
DROP SCHEMA install, audit, auth, org CASCADE;
