-- +goose Up
-- The branch and the language of the run of a report decide its baseline; a report without them gives none.
ALTER TABLE normalize.report ADD COLUMN ref_name text CHECK (octet_length(ref_name) BETWEEN 1 AND 255);
ALTER TABLE normalize.report ADD COLUMN language text CHECK (octet_length(language) BETWEEN 1 AND 32);
CREATE INDEX report_component_idx ON normalize.report (organization_id, component_id, report_kind, ref_name) WHERE component_id IS NOT NULL;
-- The content fingerprint leaves the path and the lines out: the same content at another place is a moved finding.
ALTER TABLE normalize.finding_occurrence ADD COLUMN content_fingerprint bytea CHECK (octet_length(content_fingerprint) = 32);

-- A canonical finding is the same problem across reports of an area; its fingerprint is the one of the occurrence
-- that first gave it.
CREATE TABLE change.finding_identity (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    area_id uuid NOT NULL,
    method_version integer NOT NULL CHECK (method_version > 0),
    fingerprint bytea NOT NULL CHECK (octet_length(fingerprint) = 32),
    content_fingerprint bytea CHECK (octet_length(content_fingerprint) = 32),
    rule_id text COLLATE "C" NOT NULL CHECK (octet_length(rule_id) BETWEEN 1 AND 255),
    finding_kind text NOT NULL CHECK (finding_kind IN ('code', 'security', 'secret', 'dependency', 'iac', 'docs', 'openapi')),
    first_report_id uuid NOT NULL,
    first_seen_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, area_id, method_version, fingerprint)
);
CREATE INDEX finding_identity_content_idx ON change.finding_identity (organization_id, area_id, method_version, rule_id, content_fingerprint)
    WHERE content_fingerprint IS NOT NULL;

CREATE TABLE change.occurrence_identity (
    organization_id uuid NOT NULL,
    occurrence_id uuid NOT NULL,
    finding_identity_id uuid NOT NULL,
    match_kind text NOT NULL CHECK (match_kind IN ('exact', 'moved', 'uncertain')),
    reported_at timestamptz NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    PRIMARY KEY (organization_id, occurrence_id),
    FOREIGN KEY (organization_id, finding_identity_id) REFERENCES change.finding_identity (organization_id, id)
);
CREATE INDEX occurrence_identity_finding_idx ON change.occurrence_identity (organization_id, finding_identity_id);

-- A baseline is the last complete successful run of the main branch for a component, language and kind of analysis.
CREATE TABLE change.baseline (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    component_id uuid NOT NULL,
    language text NOT NULL CHECK (octet_length(language) <= 32),
    analysis_kind text NOT NULL CHECK (analysis_kind IN ('code', 'sast', 'sca', 'secrets', 'iac', 'image', 'junit', 'coverage', 'docs',
        'openapi', 'sonar')),
    profile_version_id uuid NOT NULL,
    report_id uuid NOT NULL,
    established_at timestamptz NOT NULL,
    superseded_at timestamptz,
    CHECK (superseded_at >= established_at),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, profile_version_id) REFERENCES change.profile_version (organization_id, id)
);
CREATE UNIQUE INDEX baseline_current_idx ON change.baseline (organization_id, component_id, language, analysis_kind) WHERE superseded_at IS NULL;
CREATE INDEX baseline_profile_idx ON change.baseline (organization_id, profile_version_id);

ALTER TABLE change.finding_identity ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON change.finding_identity TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON change.finding_identity FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.finding_identity TO craftpulse_app;
GRANT SELECT ON change.finding_identity TO craftpulse_backup;
ALTER TABLE change.occurrence_identity ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON change.occurrence_identity TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON change.occurrence_identity FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.occurrence_identity TO craftpulse_app;
GRANT SELECT ON change.occurrence_identity TO craftpulse_backup;
ALTER TABLE change.baseline ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON change.baseline TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON change.baseline FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.baseline TO craftpulse_app;
GRANT SELECT ON change.baseline TO craftpulse_backup;

-- +goose Down
DROP TABLE change.baseline, change.occurrence_identity, change.finding_identity;
ALTER TABLE normalize.finding_occurrence DROP COLUMN content_fingerprint;
DROP INDEX normalize.report_component_idx;
ALTER TABLE normalize.report DROP COLUMN language, DROP COLUMN ref_name;
