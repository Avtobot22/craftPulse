-- +goose Up
CREATE TABLE org.source_account (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    instance_id uuid NOT NULL,
    native_account_id text COLLATE "C" NOT NULL CHECK (octet_length(native_account_id) BETWEEN 1 AND 255),
    login text CHECK (char_length(login) BETWEEN 1 AND 255),
    display_name text CHECK (char_length(display_name) BETWEEN 1 AND 255),
    email text CHECK (char_length(email) BETWEEN 1 AND 320),
    proposed_kind text NOT NULL CHECK (proposed_kind IN ('human', 'bot', 'system', 'unknown')),
    anonymized_at timestamptz,
    first_seen_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    -- An account of an instance is found by its native ID when events are normalized; a name joins nothing.
    UNIQUE (organization_id, instance_id, native_account_id),
    CHECK (anonymized_at IS NULL OR num_nulls(login, display_name, email) = 3)
);

ALTER TABLE org.source_account ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.source_account TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.source_account FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON org.source_account TO craftpulse_app;
GRANT SELECT ON org.source_account TO craftpulse_backup;

-- +goose Down
DROP TABLE org.source_account;
