-- +goose Up
CREATE TABLE normalize.ci_run_version (
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
    run_id uuid NOT NULL,
    area_id uuid NOT NULL,
    mr_id uuid,
    source_updated_at timestamptz,
    ref_name text NOT NULL CHECK (octet_length(ref_name) <= 1024),
    sha text NOT NULL CHECK (sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    parent_run_id uuid,
    definition_version text,
    native_status text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('running', 'success', 'failed', 'canceled', 'skipped', 'unknown')),
    started_at timestamptz,
    finished_at timestamptz,
    attempt_history text NOT NULL CHECK (attempt_history IN ('complete', 'truncated')),
    CHECK (finished_at >= started_at),
    CHECK (parent_run_id <> run_id),
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, run_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, mr_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, parent_run_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    UNIQUE (organization_id, run_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX ci_run_version_current_uq ON normalize.ci_run_version (organization_id, run_id) WHERE superseded_seq IS NULL;
CREATE INDEX ci_run_version_mr_idx ON normalize.ci_run_version (organization_id, mr_id) WHERE mr_id IS NOT NULL;
CREATE INDEX ci_run_version_parent_idx ON normalize.ci_run_version (organization_id, parent_run_id) WHERE parent_run_id IS NOT NULL;
CREATE INDEX ci_run_version_area_time_idx ON normalize.ci_run_version (organization_id, area_id, started_at);
ALTER TABLE normalize.ci_run_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.ci_run_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.ci_run_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.ci_run_version TO craftpulse_app;
GRANT SELECT ON normalize.ci_run_version TO craftpulse_backup;

CREATE TABLE normalize.ci_attempt_version (
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
    attempt_id uuid NOT NULL,
    run_id uuid NOT NULL,
    job_key text COLLATE "C" NOT NULL CHECK (octet_length(job_key) BETWEEN 1 AND 1024),
    config_fingerprint bytea NOT NULL CHECK (octet_length(config_fingerprint) = 32),
    attempt_no integer NOT NULL CHECK (attempt_no >= 1),
    native_status text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('running', 'success', 'failed', 'canceled', 'skipped', 'unknown')),
    allow_failure boolean NOT NULL,
    failure_kind text,
    runner_class text,
    started_at timestamptz,
    finished_at timestamptz,
    duration_s bigint CHECK (duration_s >= 0),
    CHECK (finished_at >= started_at),
    PRIMARY KEY (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    FOREIGN KEY (organization_id, attempt_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, run_id) REFERENCES normalize.source_object (organization_id, id),
    UNIQUE (organization_id, attempt_id, source_revision, content_hash)
);
CREATE UNIQUE INDEX ci_attempt_version_current_uq ON normalize.ci_attempt_version (organization_id, attempt_id) WHERE superseded_seq IS NULL;
CREATE INDEX ci_attempt_version_run_idx ON normalize.ci_attempt_version (organization_id, run_id);
ALTER TABLE normalize.ci_attempt_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.ci_attempt_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.ci_attempt_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.ci_attempt_version TO craftpulse_app;
GRANT SELECT ON normalize.ci_attempt_version TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.ci_attempt_version, normalize.ci_run_version;
