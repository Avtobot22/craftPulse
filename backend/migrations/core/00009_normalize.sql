-- +goose Up
CREATE SCHEMA normalize;
GRANT USAGE ON SCHEMA normalize TO craftpulse_app, craftpulse_backup;

-- Native IDs keep the bounds of ingest.record: every accepted record fits its canonical row.
CREATE TABLE normalize.source_object (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    instance_id uuid NOT NULL,
    object_kind text NOT NULL CHECK (object_kind IN ('tracker_project', 'repository', 'wiki_space', 'ci_project',
        'analysis_project', 'task', 'merge_request', 'ci_run', 'ci_attempt', 'document', 'sprint', 'deployment',
        'artifact', 'incident')),
    native_ns text COLLATE "C" NOT NULL CHECK (octet_length(native_ns) <= 255),
    native_id text COLLATE "C" NOT NULL CHECK (octet_length(native_id) BETWEEN 1 AND 1024),
    identity_quality text NOT NULL CHECK (identity_quality IN ('stable', 'derived', 'uncertain')),
    first_known_seq bigint NOT NULL CHECK (first_known_seq > 0),
    current_area_id uuid,
    restricted boolean NOT NULL,
    deleted_at timestamptz,
    PRIMARY KEY (organization_id, id),
    -- The identity of an external object (BR-001): the key, the project and the status are not part of it.
    UNIQUE (organization_id, instance_id, object_kind, native_ns, native_id)
);

CREATE TABLE normalize.area (
    organization_id uuid NOT NULL,
    area_id uuid NOT NULL,
    area_kind text NOT NULL CHECK (area_kind IN ('tracker_project', 'repository', 'wiki_space', 'ci_project',
        'analysis_project')),
    last_change_seq bigint NOT NULL CHECK (last_change_seq >= 0),
    retained_from timestamptz,
    PRIMARY KEY (organization_id, area_id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.source_object (organization_id, id)
);

ALTER TABLE normalize.source_object ADD FOREIGN KEY (organization_id, current_area_id)
    REFERENCES normalize.area (organization_id, area_id);
CREATE INDEX source_object_area_idx ON normalize.source_object (organization_id, current_area_id)
    WHERE current_area_id IS NOT NULL;

CREATE TABLE normalize.area_change (
    organization_id uuid NOT NULL,
    area_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    earliest_event_time timestamptz,
    fact_kinds text[] NOT NULL,
    recorded_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, area_id, known_seq),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id)
);

CREATE TABLE normalize.task_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    task_id uuid NOT NULL,
    area_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    observed_at timestamptz NOT NULL,
    source_updated_at timestamptz,
    task_key text COLLATE "C" CHECK (char_length(task_key) BETWEEN 1 AND 255),
    title text CHECK (char_length(title) <= 1024),
    type_native_id text COLLATE "C" NOT NULL CHECK (char_length(type_native_id) BETWEEN 1 AND 255),
    status_native_id text COLLATE "C" NOT NULL CHECK (char_length(status_native_id) BETWEEN 1 AND 255),
    resolution_native_id text COLLATE "C" CHECK (char_length(resolution_native_id) BETWEEN 1 AND 255),
    created_src_at timestamptz NOT NULL,
    size_native text CHECK (char_length(size_native) <= 255),
    team_native text CHECK (char_length(team_native) <= 255),
    assignee_account_id uuid,
    parent_task_id uuid,
    restriction_native text COLLATE "C" CHECK (char_length(restriction_native) BETWEEN 1 AND 255),
    raw_fields jsonb,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, task_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    FOREIGN KEY (organization_id, parent_task_id) REFERENCES normalize.source_object (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    -- A repeated normalization of the same input adds nothing; the hash lets a resolved conflict replace a revision.
    UNIQUE (organization_id, task_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX task_version_current_uq ON normalize.task_version (organization_id, task_id) WHERE superseded_seq IS NULL;
-- Tasks of an area for calculation, reports and access, also versions replaced after a slice.
CREATE INDEX task_version_area_idx ON normalize.task_version (organization_id, area_id);
CREATE INDEX task_version_parent_idx ON normalize.task_version (organization_id, parent_task_id) WHERE parent_task_id IS NOT NULL;

CREATE TABLE normalize.task_event (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    task_id uuid NOT NULL,
    area_id uuid NOT NULL,
    event_key text COLLATE "C" NOT NULL CHECK (octet_length(event_key) BETWEEN 1 AND 1024),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    event_kind text NOT NULL CHECK (event_kind IN ('created', 'field_change', 'comment', 'link_change', 'moved')),
    field_native_id text COLLATE "C" CHECK (char_length(field_native_id) BETWEEN 1 AND 255),
    before_value text CHECK (char_length(before_value) <= 1024),
    after_value text CHECK (char_length(after_value) <= 1024),
    before_account_id uuid,
    after_account_id uuid,
    actor_account_id uuid,
    event_time timestamptz NOT NULL,
    event_offset_s integer NOT NULL CHECK (event_offset_s BETWEEN -64800 AND 64800),
    observed_at timestamptz NOT NULL,
    observed_via text NOT NULL CHECK (observed_via IN ('webhook', 'incremental', 'backfill', 'reconcile', 'import')),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, task_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    CHECK (event_kind <> 'field_change' OR field_native_id IS NOT NULL),
    UNIQUE (organization_id, task_id, event_key, source_revision, content_hash)
);
CREATE UNIQUE INDEX task_event_current_uq ON normalize.task_event (organization_id, task_id, event_key) WHERE superseded_seq IS NULL;
-- The timeline of a project at a date (FR-030).
CREATE INDEX task_event_area_time_idx ON normalize.task_event (organization_id, area_id, event_time);

-- Statuses, resolutions, types and fields of an area, as the source names them; process maps them.
CREATE TABLE normalize.dictionary_entry (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    area_id uuid NOT NULL,
    entry_kind text NOT NULL CHECK (entry_kind IN ('task_status', 'task_resolution', 'task_type', 'task_field')),
    native_id text COLLATE "C" NOT NULL CHECK (octet_length(native_id) BETWEEN 1 AND 1024),
    source_revision text COLLATE "C" NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 255),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    attributes jsonb NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    UNIQUE (organization_id, area_id, entry_kind, native_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX dictionary_entry_current_uq ON normalize.dictionary_entry (organization_id, area_id, entry_kind, native_id)
    WHERE superseded_seq IS NULL;

ALTER TABLE normalize.source_object ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.source_object TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.source_object FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE normalize.area ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.area TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.area FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE normalize.area_change ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.area_change TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.area_change FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE normalize.task_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.task_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.task_version FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE normalize.task_event ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.task_event TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.task_event FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE normalize.dictionary_entry ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.dictionary_entry TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.dictionary_entry FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA normalize TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA normalize TO craftpulse_backup;

-- +goose Down
DROP SCHEMA normalize CASCADE;
