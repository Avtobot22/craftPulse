-- +goose Up
-- The confirmed link of an account of a source to a person and the kind of the participant (TZ 12.5.1):
-- a match of a login or an email is only a proposal until an administrator confirms it.
CREATE TABLE org.account_assignment (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    source_account_id uuid NOT NULL,
    person_id uuid,
    actor_kind text NOT NULL CHECK (actor_kind IN ('human', 'bot', 'system', 'unknown')),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, source_account_id) REFERENCES org.source_account (organization_id, id),
    FOREIGN KEY (organization_id, person_id) REFERENCES org.person (organization_id, id),
    CHECK (actor_kind <> 'human' OR person_id IS NOT NULL),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    -- A repeat of a confirmation finds the version it saved.
    UNIQUE (organization_id, confirmation_id)
);
-- One current link per account.
CREATE UNIQUE INDEX account_assignment_current_uq ON org.account_assignment (organization_id, source_account_id)
    WHERE superseded_seq IS NULL;
-- The partial index does not cover superseded versions, so the foreign key has its own.
CREATE INDEX account_assignment_account_idx ON org.account_assignment (organization_id, source_account_id);
-- The accounts of a user for CheckAccess; also the index of the foreign key, so it covers superseded versions.
CREATE INDEX account_assignment_person_idx ON org.account_assignment (organization_id, person_id) WHERE person_id IS NOT NULL;

ALTER TABLE org.account_assignment ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.account_assignment TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.account_assignment FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON org.account_assignment TO craftpulse_app;
GRANT SELECT ON org.account_assignment TO craftpulse_backup;

-- +goose Down
DROP TABLE org.account_assignment;
