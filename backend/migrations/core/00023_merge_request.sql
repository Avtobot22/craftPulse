-- +goose Up
CREATE TABLE normalize.repository_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    observed_at timestamptz NOT NULL,
    repository_id uuid NOT NULL,
    source_updated_at timestamptz,
    path text NOT NULL,
    name text NOT NULL,
    default_branch text,
    visibility text NOT NULL CHECK (visibility IN ('private', 'internal', 'public', 'unknown')),
    web_url text NOT NULL,
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, repository_id) REFERENCES normalize.area (organization_id, area_id),
    UNIQUE (organization_id, repository_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX repository_version_current_uq ON normalize.repository_version (organization_id, repository_id) WHERE superseded_seq IS NULL;
ALTER TABLE normalize.repository_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.repository_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.repository_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.repository_version TO craftpulse_app;
GRANT SELECT ON normalize.repository_version TO craftpulse_backup;

CREATE TABLE normalize.mr_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    observed_at timestamptz NOT NULL,
    mr_id uuid NOT NULL,
    area_id uuid NOT NULL,
    source_updated_at timestamptz,
    iid bigint CHECK (iid > 0),
    title text,
    state text NOT NULL CHECK (state IN ('open', 'merged', 'closed')),
    draft boolean NOT NULL,
    author_account_id uuid,
    source_branch text NOT NULL,
    target_branch text NOT NULL,
    head_sha text NOT NULL CHECK (head_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    base_sha text,
    merge_base_sha text,
    reviewer_field boolean NOT NULL,
    reviewer_account_ids uuid[] NOT NULL,
    opened_at timestamptz NOT NULL,
    ready_at timestamptz,
    merged_at timestamptz,
    closed_at timestamptz,
    CHECK (state <> 'merged' OR merged_at IS NOT NULL),
    CHECK (reviewer_field OR cardinality(reviewer_account_ids) = 0),
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, mr_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    UNIQUE (organization_id, mr_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX mr_version_current_uq ON normalize.mr_version (organization_id, mr_id) WHERE superseded_seq IS NULL;
ALTER TABLE normalize.mr_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.mr_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.mr_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.mr_version TO craftpulse_app;
GRANT SELECT ON normalize.mr_version TO craftpulse_backup;

CREATE TABLE normalize.mr_event (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    observed_at timestamptz NOT NULL,
    mr_id uuid NOT NULL,
    event_key text COLLATE "C" NOT NULL CHECK (octet_length(event_key) BETWEEN 1 AND 1024),
    event_kind text NOT NULL CHECK (event_kind IN ('ready','reviewer_assigned','reviewer_removed','review','comment','approval','head_pushed','merged','closed','reopened')),
    actor_account_id uuid,
    target_account_id uuid,
    head_sha text,
    review_state text CHECK (review_state IN ('approved','changes_requested','commented')),
    event_time timestamptz NOT NULL,
    event_offset_s integer NOT NULL CHECK (event_offset_s BETWEEN -64800 AND 64800),
    observed_via text NOT NULL CHECK (observed_via IN ('webhook','incremental','backfill','reconcile','import')),
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, mr_id) REFERENCES normalize.source_object (organization_id, id),
    UNIQUE (organization_id, mr_id, event_key, source_revision, content_hash)
);
CREATE UNIQUE INDEX mr_event_current_uq ON normalize.mr_event (organization_id, mr_id, event_key) WHERE superseded_seq IS NULL;
ALTER TABLE normalize.mr_event ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.mr_event TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.mr_event FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.mr_event TO craftpulse_app;
GRANT SELECT ON normalize.mr_event TO craftpulse_backup;

CREATE TABLE normalize.mr_revision (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    observed_at timestamptz NOT NULL,
    mr_id uuid NOT NULL,
    revision_key text COLLATE "C" NOT NULL CHECK (octet_length(revision_key) BETWEEN 1 AND 1024),
    head_sha text NOT NULL CHECK (head_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    base_sha text,
    start_sha text,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, mr_id) REFERENCES normalize.source_object (organization_id, id),
    UNIQUE (organization_id, mr_id, revision_key, source_revision, content_hash)
);
CREATE UNIQUE INDEX mr_revision_current_uq ON normalize.mr_revision (organization_id, mr_id, revision_key) WHERE superseded_seq IS NULL;
ALTER TABLE normalize.mr_revision ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.mr_revision TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.mr_revision FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.mr_revision TO craftpulse_app;
GRANT SELECT ON normalize.mr_revision TO craftpulse_backup;

CREATE INDEX mr_version_open_area_idx ON normalize.mr_version (organization_id, area_id) WHERE superseded_seq IS NULL AND state = 'open';
CREATE INDEX mr_version_area_time_idx ON normalize.mr_version (organization_id, area_id, opened_at);
CREATE INDEX mr_version_head_idx ON normalize.mr_version (organization_id, area_id, head_sha);
CREATE INDEX mr_event_time_idx ON normalize.mr_event (organization_id, mr_id, event_time);
CREATE INDEX mr_revision_head_idx ON normalize.mr_revision (organization_id, head_sha, mr_id);

-- +goose Down
DROP TABLE normalize.mr_revision, normalize.mr_event, normalize.mr_version, normalize.repository_version;
