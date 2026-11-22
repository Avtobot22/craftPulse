-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref) VALUES
    ('S09', 1, 'count', 'docs/product/metrics/security.md#s09'),
    ('S12', 1, 'count', 'docs/product/metrics/security.md#s12'),
    ('S13', 1, 'count', 'docs/product/metrics/security.md#s13'),
    ('S14', 1, 'count', 'docs/product/metrics/security.md#s14'),
    ('S15', 1, 'ratio', 'docs/product/metrics/security.md#s15'),
    ('S16', 1, 'count', 'docs/product/metrics/security.md#s16'),
    ('S18', 1, 'count', 'docs/product/metrics/security.md#s18'),
    ('S19', 1, 'count', 'docs/product/metrics/security.md#s19');

ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'security_finding', 'affected_package', 'security_advisory'));

ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'security_report'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind IN ('security_finding', 'affected_package', 'security_advisory');
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report'));

DELETE FROM calc.revision_input WHERE input_kind = 'security_report';
ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold'));

DELETE FROM calc.definition
WHERE definition_version = 1 AND metric_id IN ('S09', 'S12', 'S13', 'S14', 'S15', 'S16', 'S18', 'S19');
