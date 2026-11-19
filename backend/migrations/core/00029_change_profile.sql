-- +goose Up
CREATE SCHEMA change;
GRANT USAGE ON SCHEMA change TO craftpulse_app, craftpulse_backup;

-- A confirmed version of the matrix of checks of a project (FR-035); content_hash tells a repeat of the same
-- matrix, which creates no version.
CREATE TABLE change.profile_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_no integer NOT NULL CHECK (version_no > 0),
    base_version_no integer,
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES org.project (organization_id, id),
    CHECK ((version_no = 1 AND base_version_no IS NULL) OR base_version_no = version_no - 1),
    UNIQUE (organization_id, project_id, version_no),
    UNIQUE (organization_id, confirmation_id)
);

-- A row of the matrix: one per component, language and analysis kind; the language is empty for checks
-- without one.
CREATE TABLE change.profile_entry (
    organization_id uuid NOT NULL,
    profile_version_id uuid NOT NULL,
    component_id uuid NOT NULL,
    language text NOT NULL CHECK (octet_length(language) <= 32),
    analysis_kind text NOT NULL CHECK (analysis_kind IN ('code', 'sast', 'sca', 'secrets', 'iac', 'image', 'junit', 'coverage', 'docs',
        'openapi', 'sonar')),
    mode text NOT NULL CHECK (mode IN ('import', 'builtin')),
    profile_id text NOT NULL CHECK (octet_length(profile_id) BETWEEN 1 AND 64),
    tool_versions jsonb NOT NULL,
    rules_hash bytea CHECK (octet_length(rules_hash) = 32),
    PRIMARY KEY (organization_id, profile_version_id, component_id, language, analysis_kind),
    FOREIGN KEY (organization_id, profile_version_id) REFERENCES change.profile_version (organization_id, id) ON DELETE CASCADE
);

ALTER TABLE change.profile_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON change.profile_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON change.profile_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.profile_version TO craftpulse_app;
GRANT SELECT ON change.profile_version TO craftpulse_backup;

ALTER TABLE change.profile_entry ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON change.profile_entry TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON change.profile_entry FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.profile_entry TO craftpulse_app;
GRANT SELECT ON change.profile_entry TO craftpulse_backup;

-- +goose Down
DROP SCHEMA change CASCADE;
