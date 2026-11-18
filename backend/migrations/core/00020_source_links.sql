-- +goose Up
CREATE SCHEMA access;
GRANT USAGE ON SCHEMA access TO craftpulse_app, craftpulse_backup;

CREATE TABLE access.source_link (
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('not_linked', 'linked', 'expired', 'revoked')),
    linked_at timestamptz,
    row_version bigint NOT NULL CHECK (row_version > 0),
    PRIMARY KEY (organization_id, user_id, connection_id),
    FOREIGN KEY (organization_id, user_id) REFERENCES org.app_user (organization_id, id),
    FOREIGN KEY (organization_id, connection_id) REFERENCES connect.connection (organization_id, id),
    CHECK (state NOT IN ('linked', 'expired') OR linked_at IS NOT NULL)
);
CREATE INDEX source_link_connection_idx ON access.source_link (organization_id, connection_id);

CREATE TABLE access.source_link_attempt (
    organization_id uuid NOT NULL,
    state_hash bytea NOT NULL CHECK (octet_length(state_hash) = 32),
    user_id uuid NOT NULL,
    session_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    link_version bigint NOT NULL,
    return_path text NOT NULL CHECK (left(return_path, 1) = '/' AND left(return_path, 2) <> '//'),
    redirect_uri text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    PRIMARY KEY (organization_id, state_hash),
    FOREIGN KEY (organization_id, user_id, connection_id)
        REFERENCES access.source_link (organization_id, user_id, connection_id),
    FOREIGN KEY (organization_id, session_id) REFERENCES auth.session (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX source_link_attempt_expiry_idx ON access.source_link_attempt (expires_at);
CREATE INDEX source_link_attempt_session_idx ON access.source_link_attempt (organization_id, session_id);
CREATE INDEX source_link_attempt_link_idx ON access.source_link_attempt (organization_id, user_id, connection_id);

ALTER TABLE access.source_link ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON access.source_link TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON access.source_link FOR SELECT TO craftpulse_backup USING (true);
ALTER TABLE access.source_link_attempt ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON access.source_link_attempt TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON access.source_link_attempt FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA access TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA access TO craftpulse_backup;

-- +goose Down
DROP SCHEMA access CASCADE;
