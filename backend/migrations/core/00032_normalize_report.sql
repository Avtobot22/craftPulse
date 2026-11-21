-- +goose Up
-- A report is immutable: a repeated report is a new row with its own findings (TZ 12.5.5). NULL producer and
-- scope_hash are unknown: a report file of an adapter comes without a scan manifest.
CREATE TABLE normalize.report (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    area_id uuid NOT NULL,
    component_id uuid,
    report_kind text NOT NULL CHECK (report_kind IN ('code', 'sast', 'sca', 'secrets', 'iac', 'image', 'junit', 'coverage', 'docs', 'openapi', 'sonar')),
    mode text NOT NULL CHECK (mode IN ('import', 'builtin')),
    producer text CHECK (octet_length(producer) BETWEEN 1 AND 128),
    producer_version text CHECK (octet_length(producer_version) BETWEEN 1 AND 64),
    report_format text NOT NULL CHECK (octet_length(report_format) BETWEEN 1 AND 64),
    schema_version text NOT NULL CHECK (octet_length(schema_version) BETWEEN 1 AND 64),
    parser_version text NOT NULL CHECK (octet_length(parser_version) BETWEEN 1 AND 64),
    profile_id text CHECK (octet_length(profile_id) BETWEEN 1 AND 128),
    rules_hash bytea CHECK (octet_length(rules_hash) = 32),
    config_hash bytea CHECK (octet_length(config_hash) = 32),
    db_version text CHECK (octet_length(db_version) BETWEEN 1 AND 128),
    db_published_at timestamptz,
    scope_hash bytea CHECK (octet_length(scope_hash) = 32),
    sha text CHECK (sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    artifact_digest text CHECK (octet_length(artifact_digest) BETWEEN 1 AND 255),
    run_id uuid,
    attempt_id uuid,
    started_at timestamptz,
    finished_at timestamptz,
    exit_class text NOT NULL CHECK (exit_class IN ('success', 'findings', 'partial', 'failed', 'timeout', 'unknown')),
    completeness text NOT NULL CHECK (completeness IN ('complete', 'partial', 'unknown')),
    scanned_count integer CHECK (scanned_count >= 0),
    skipped_count integer CHECK (skipped_count >= 0),
    skipped_targets jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(skipped_targets) = 'array'),
    history_from_sha text CHECK (history_from_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    history_to_sha text CHECK (history_to_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    history_truncated boolean,
    CHECK (sha IS NOT NULL OR artifact_digest IS NOT NULL),
    CHECK (finished_at >= started_at),
    CHECK ((producer IS NULL) = (producer_version IS NULL)),
    CHECK ((history_from_sha IS NULL) = (history_to_sha IS NULL) AND (history_to_sha IS NULL) = (history_truncated IS NULL)),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    FOREIGN KEY (organization_id, run_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, attempt_id) REFERENCES normalize.source_object (organization_id, id),
    -- A record of a package gives one report, so normalizing the package again adds nothing.
    UNIQUE (organization_id, receipt_id, record_no)
);
CREATE INDEX report_area_sha_idx ON normalize.report (organization_id, area_id, sha, report_kind);
CREATE INDEX report_attempt_idx ON normalize.report (organization_id, attempt_id) WHERE attempt_id IS NOT NULL;
CREATE INDEX report_run_idx ON normalize.report (organization_id, run_id) WHERE run_id IS NOT NULL;
ALTER TABLE normalize.report ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.report TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.report FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.report TO craftpulse_app;
GRANT SELECT ON normalize.report TO craftpulse_backup;

CREATE TABLE normalize.finding_occurrence (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    report_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    native_id text CHECK (octet_length(native_id) BETWEEN 1 AND 255),
    rule_id text COLLATE "C" NOT NULL CHECK (octet_length(rule_id) BETWEEN 1 AND 255),
    finding_kind text NOT NULL CHECK (finding_kind IN ('code', 'security', 'secret', 'dependency', 'iac', 'docs', 'openapi')),
    path text COLLATE "C" CHECK (octet_length(path) BETWEEN 1 AND 4096),
    line_start integer CHECK (line_start >= 1),
    line_end integer,
    package_purl text CHECK (octet_length(package_purl) <= 1024),
    native_severity text CHECK (octet_length(native_severity) <= 64),
    native_confidence text CHECK (octet_length(native_confidence) <= 64),
    mapped_severity text,
    severity_mapping_version integer,
    fingerprint bytea NOT NULL CHECK (octet_length(fingerprint) = 32),
    evidence jsonb CHECK (jsonb_typeof(evidence) = 'object'),
    CHECK (line_end >= line_start),
    CHECK ((mapped_severity IS NULL) = (severity_mapping_version IS NULL)),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.report (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX finding_occurrence_report_idx ON normalize.finding_occurrence (organization_id, report_id);
ALTER TABLE normalize.finding_occurrence ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.finding_occurrence TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.finding_occurrence FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.finding_occurrence TO craftpulse_app;
GRANT SELECT ON normalize.finding_occurrence TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.finding_occurrence, normalize.report;
