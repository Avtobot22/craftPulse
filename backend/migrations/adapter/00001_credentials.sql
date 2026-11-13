-- +goose Up
-- Schema adapter, its grants to the process role and craftpulse_backup are created with the database by
-- deploy/postgres/adapter-db.sh, because the role name differs per adapter.

CREATE TABLE adapter.org_key (
    organization_id uuid PRIMARY KEY,
    data_key_enc bytea NOT NULL,
    master_key_version integer NOT NULL CHECK (master_key_version > 0)
);

CREATE TABLE adapter.credential (
    organization_id uuid NOT NULL REFERENCES adapter.org_key (organization_id),
    connection_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('collect', 'write_bot', 'webhook_secret', 'oauth_client')),
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    previous_ciphertext bytea,
    previous_nonce bytea CHECK (octet_length(previous_nonce) = 12),
    replaced_at timestamptz NOT NULL,
    PRIMARY KEY (connection_id, purpose),
    CHECK ((previous_ciphertext IS NULL) = (previous_nonce IS NULL))
);

-- The refresh token has its own nonce: one nonce under one data key must never seal two values.
CREATE TABLE adapter.user_token (
    organization_id uuid NOT NULL REFERENCES adapter.org_key (organization_id),
    connection_id uuid NOT NULL,
    user_id uuid NOT NULL,
    account_native_id text,
    access_ciphertext bytea NOT NULL,
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    refresh_ciphertext bytea,
    refresh_nonce bytea CHECK (octet_length(refresh_nonce) = 12),
    expires_at timestamptz,
    replaced_at timestamptz NOT NULL,
    PRIMARY KEY (connection_id, user_id),
    CHECK ((refresh_ciphertext IS NULL) = (refresh_nonce IS NULL))
);

ALTER TABLE adapter.org_key ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON adapter.org_key
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON adapter.org_key FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE adapter.credential ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON adapter.credential
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON adapter.credential FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE adapter.user_token ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON adapter.user_token
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON adapter.user_token FOR SELECT TO craftpulse_backup USING (true);

-- +goose Down
DROP TABLE adapter.user_token;
DROP TABLE adapter.credential;
DROP TABLE adapter.org_key;
