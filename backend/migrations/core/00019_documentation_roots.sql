-- +goose Up
CREATE SCHEMA docs;
GRANT USAGE ON SCHEMA docs TO craftpulse_app, craftpulse_backup;

-- The set keeps confirmations of empty replacements as well as populated ones.
CREATE TABLE docs.root_set (
    organization_id uuid NOT NULL,
    service_id uuid NOT NULL,
    version_no bigint NOT NULL CHECK (version_no > 0),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, service_id, version_no),
    UNIQUE (organization_id, confirmation_id),
    FOREIGN KEY (organization_id, service_id) REFERENCES org.service (organization_id, id),
    FOREIGN KEY (organization_id, confirmed_by) REFERENCES org.app_user (organization_id, id)
);

CREATE TABLE docs.root (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    service_id uuid NOT NULL,
    area_id uuid NOT NULL,
    path_prefix text COLLATE "C" NOT NULL CHECK (octet_length(path_prefix) <= 1024),
    profile_id text NOT NULL CHECK (profile_id = 'git-markdown-openapi'),
    owner_person_id uuid,
    audience text NOT NULL CHECK (audience IN ('team', 'organization', 'external')),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, service_id) REFERENCES org.service (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    FOREIGN KEY (organization_id, owner_person_id) REFERENCES org.person (organization_id, id),
    FOREIGN KEY (organization_id, confirmed_by) REFERENCES org.app_user (organization_id, id),
    FOREIGN KEY (organization_id, confirmation_id) REFERENCES docs.root_set (organization_id, confirmation_id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL))
);
CREATE UNIQUE INDEX root_current_path_idx ON docs.root (organization_id, service_id, area_id, path_prefix)
    WHERE superseded_seq IS NULL;
CREATE INDEX root_current_area_idx ON docs.root (organization_id, area_id) WHERE superseded_seq IS NULL;
CREATE INDEX root_history_idx ON docs.root (organization_id, service_id, known_seq);
CREATE INDEX root_owner_idx ON docs.root (organization_id, owner_person_id) WHERE owner_person_id IS NOT NULL;
CREATE INDEX root_confirmation_idx ON docs.root (organization_id, confirmation_id);
CREATE INDEX root_set_author_idx ON docs.root_set (organization_id, confirmed_by);
CREATE INDEX root_author_idx ON docs.root (organization_id, confirmed_by);

-- Every successful key is bound to its result, including a confirmation without changes.
CREATE TABLE docs.root_confirmation (
    organization_id uuid NOT NULL,
    confirmation_id uuid NOT NULL,
    service_id uuid NOT NULL,
    version_no bigint NOT NULL,
    actor_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, confirmation_id),
    FOREIGN KEY (organization_id, service_id, version_no) REFERENCES docs.root_set (organization_id, service_id, version_no),
    FOREIGN KEY (organization_id, actor_user_id) REFERENCES org.app_user (organization_id, id)
);
CREATE INDEX root_confirmation_set_idx ON docs.root_confirmation (organization_id, service_id, version_no);
CREATE INDEX root_confirmation_actor_idx ON docs.root_confirmation (organization_id, actor_user_id);
ALTER TABLE docs.root_confirmation ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON docs.root_confirmation TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON docs.root_confirmation FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE docs.root_set ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON docs.root_set TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON docs.root_set FOR SELECT TO craftpulse_backup USING (true);
ALTER TABLE docs.root ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON docs.root TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON docs.root FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA docs TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA docs TO craftpulse_backup;

-- +goose Down
DROP SCHEMA docs CASCADE;
