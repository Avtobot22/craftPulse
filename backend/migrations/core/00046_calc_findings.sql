-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('C11', 1, 'count', 'docs/product/metrics/code.md#c11', NULL),
    ('C12', 1, 'count', 'docs/product/metrics/code.md#c12', NULL),
    ('C13', 1, 'count', 'docs/product/metrics/code.md#c13', NULL),
    ('C26', 1, 'count', 'docs/product/metrics/code.md#c26', NULL),
    ('S10', 1, 'count', 'docs/product/metrics/security.md#s10', NULL),
    ('S11', 1, 'count', 'docs/product/metrics/security.md#s11', NULL),
    ('S35', 1, 'count', 'docs/product/metrics/security.md#s35', NULL),
    ('S38', 1, 'count', 'docs/product/metrics/security.md#s38', NULL),
    ('I08', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i08', NULL),
    ('I09', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i09', NULL);

ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'document', 'documentation_root',
        'documentation_finding', 'documentation_report', 'finding', 'exception', 'finding_report',
        'expected_check', 'expected_report'));

ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'documentation_root', 'documentation_report',
        'finding_report', 'finding_comparison', 'expected_checks', 'expected_reports'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind IN ('finding', 'exception', 'finding_report', 'expected_check', 'expected_report');
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'document', 'documentation_root',
        'documentation_finding', 'documentation_report'));

DELETE FROM calc.revision_input WHERE input_kind IN ('finding_report', 'finding_comparison', 'expected_checks', 'expected_reports');
ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'documentation_root', 'documentation_report'));

DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('C11','C12','C13','C26','S10','S11','S35','S38','I08','I09');
