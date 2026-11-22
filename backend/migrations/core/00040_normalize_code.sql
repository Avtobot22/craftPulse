-- +goose Up
CREATE TABLE normalize.code_report_fact (
    organization_id uuid NOT NULL,
    report_id uuid NOT NULL,
    facts jsonb NOT NULL CHECK (jsonb_typeof(facts) = 'object'),
    PRIMARY KEY (organization_id, report_id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.report (organization_id, id) ON DELETE CASCADE
);
ALTER TABLE normalize.code_report_fact ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.code_report_fact TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.code_report_fact FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.code_report_fact TO craftpulse_app;
GRANT SELECT ON normalize.code_report_fact TO craftpulse_backup;

CREATE INDEX code_report_scope_idx ON normalize.report
    (organization_id, area_id, component_id, sha, report_format, producer, profile_id, known_seq DESC)
    WHERE report_format IN ('lizard_csv', 'jscpd_json');

-- +goose Down
DROP INDEX normalize.code_report_scope_idx;
DROP TABLE normalize.code_report_fact;
