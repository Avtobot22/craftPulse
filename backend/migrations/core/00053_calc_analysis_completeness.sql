-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref) VALUES
    ('C02', 1, 'ratio', 'docs/product/metrics/code.md#c02'),
    ('C03', 1, 'ratio', 'docs/product/metrics/code.md#c03'),
    ('C35', 1, 'seconds', 'docs/product/metrics/code.md#c35'),
    ('C36', 1, 'seconds', 'docs/product/metrics/code.md#c36'),
    ('S01', 1, 'seconds', 'docs/product/metrics/security.md#s01'),
    ('S02', 1, 'ratio', 'docs/product/metrics/security.md#s02'),
    ('S03', 1, 'ratio', 'docs/product/metrics/security.md#s03'),
    ('S04', 1, 'count', 'docs/product/metrics/security.md#s04'),
    ('S05', 1, 'seconds', 'docs/product/metrics/security.md#s05'),
    ('S07', 1, 'ratio', 'docs/product/metrics/security.md#s07'),
    ('S08', 1, 'seconds', 'docs/product/metrics/security.md#s08'),
    ('D27', 1, 'ratio', 'docs/product/metrics/documentation.md#d27');

ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'document', 'documentation_root',
        'documentation_finding', 'documentation_report', 'finding', 'exception', 'finding_report',
        'expected_check', 'expected_report', 'security_finding', 'affected_package', 'security_advisory', 'analysis_run'));

ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'documentation_root', 'documentation_report',
        'finding_report', 'finding_comparison', 'expected_checks', 'expected_reports', 'security_report', 'analysis_run'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind = 'analysis_run';
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'document', 'documentation_root',
        'documentation_finding', 'documentation_report', 'finding', 'exception', 'finding_report',
        'expected_check', 'expected_report', 'security_finding', 'affected_package', 'security_advisory'));

DELETE FROM calc.revision_input WHERE input_kind = 'analysis_run';
ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'documentation_root', 'documentation_report',
        'finding_report', 'finding_comparison', 'expected_checks', 'expected_reports', 'security_report'));

DELETE FROM calc.definition
WHERE definition_version = 1 AND metric_id IN ('C02', 'C03', 'C35', 'C36', 'S01', 'S02', 'S03', 'S04', 'S05', 'S07', 'S08', 'D27');
