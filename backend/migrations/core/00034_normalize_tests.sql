-- +goose Up
-- A test case is the identity of a test in its area: the hash of its suite, class and name.
CREATE TABLE normalize.test_case (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    area_id uuid NOT NULL,
    case_key text COLLATE "C" NOT NULL CHECK (octet_length(case_key) BETWEEN 1 AND 2048),
    case_key_hash bytea NOT NULL CHECK (octet_length(case_key_hash) = 32),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    UNIQUE (organization_id, area_id, case_key_hash)
);
ALTER TABLE normalize.test_case ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.test_case TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.test_case FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.test_case TO craftpulse_app;
GRANT SELECT ON normalize.test_case TO craftpulse_backup;

-- An execution is a case of a report in the configuration of its attempt (DR-006); like the report it is immutable.
CREATE TABLE normalize.test_execution (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    report_id uuid NOT NULL,
    test_case_id uuid NOT NULL,
    attempt_id uuid,
    config_fingerprint bytea NOT NULL CHECK (octet_length(config_fingerprint) = 32),
    ordinal smallint NOT NULL CHECK (ordinal >= 1),
    outcome text NOT NULL CHECK (outcome IN ('passed', 'failed', 'error', 'skipped')),
    duration_ms bigint CHECK (duration_ms >= 0),
    run_started_at timestamptz NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.report (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, test_case_id) REFERENCES normalize.test_case (organization_id, id),
    FOREIGN KEY (organization_id, attempt_id) REFERENCES normalize.source_object (organization_id, id),
    UNIQUE (organization_id, report_id, test_case_id, config_fingerprint, ordinal)
);
CREATE INDEX test_execution_case_idx ON normalize.test_execution (organization_id, test_case_id, run_started_at);
CREATE INDEX test_execution_attempt_idx ON normalize.test_execution (organization_id, attempt_id) WHERE attempt_id IS NOT NULL;
ALTER TABLE normalize.test_execution ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.test_execution TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.test_execution FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.test_execution TO craftpulse_app;
GRANT SELECT ON normalize.test_execution TO craftpulse_backup;

-- Coverage of a file by one kind (DR-008); the map of its lines lies in the object store by line_map_key.
CREATE TABLE normalize.coverage_fact (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    report_id uuid NOT NULL,
    path text COLLATE "C" NOT NULL CHECK (octet_length(path) BETWEEN 1 AND 4096),
    path_hash bytea NOT NULL CHECK (octet_length(path_hash) = 32),
    kind text NOT NULL CHECK (kind IN ('line', 'branch', 'block')),
    covered integer NOT NULL,
    executable integer NOT NULL,
    line_map_key text CHECK (octet_length(line_map_key) BETWEEN 1 AND 1024),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    CHECK (0 <= covered AND covered <= executable),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.report (organization_id, id) ON DELETE CASCADE,
    UNIQUE (organization_id, report_id, path_hash, kind)
);
ALTER TABLE normalize.coverage_fact ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.coverage_fact TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.coverage_fact FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.coverage_fact TO craftpulse_app;
GRANT SELECT ON normalize.coverage_fact TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.coverage_fact, normalize.test_execution, normalize.test_case;
