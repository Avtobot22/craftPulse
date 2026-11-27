-- +goose Up
ALTER TABLE normalize.report
    ADD COLUMN repository_id uuid,
    ADD COLUMN sonar_analysis_id text COLLATE "C" CHECK (octet_length(sonar_analysis_id) BETWEEN 1 AND 255),
    ADD COLUMN sonar_measures jsonb CHECK (jsonb_typeof(sonar_measures) = 'object'),
    ADD FOREIGN KEY (organization_id, repository_id) REFERENCES normalize.area (organization_id, area_id),
    ADD CHECK (sonar_analysis_id IS NULL OR (report_kind = 'sonar' AND sha IS NOT NULL));

CREATE UNIQUE INDEX report_sonar_analysis_uq ON normalize.report (organization_id, area_id, sonar_analysis_id)
    WHERE sonar_analysis_id IS NOT NULL;
CREATE INDEX report_repository_sha_idx ON normalize.report (organization_id, repository_id, sha)
    WHERE repository_id IS NOT NULL;

ALTER TABLE connect.capability DROP CONSTRAINT capability_capability_check;
ALTER TABLE connect.capability ADD CHECK (capability IN ('read_current', 'read_status_history', 'read_field_history',
    'read_resolution_history', 'read_sprint_history', 'read_links', 'read_permissions', 'read_deletions',
    'write_mr_comment', 'write_tracker_issue', 'webhook', 'read_mr_analysis'));

INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref)
VALUES ('C03', 2, 'ratio', 'docs/product/metrics/code.md#c03');

-- +goose Down
DELETE FROM calc.definition WHERE metric_id = 'C03' AND definition_version = 2;

DELETE FROM connect.capability WHERE capability = 'read_mr_analysis';
ALTER TABLE connect.capability DROP CONSTRAINT capability_capability_check;
ALTER TABLE connect.capability ADD CHECK (capability IN ('read_current', 'read_status_history', 'read_field_history',
    'read_resolution_history', 'read_sprint_history', 'read_links', 'read_permissions', 'read_deletions',
    'write_mr_comment', 'write_tracker_issue', 'webhook'));

ALTER TABLE normalize.report DROP COLUMN repository_id, DROP COLUMN sonar_analysis_id, DROP COLUMN sonar_measures;
