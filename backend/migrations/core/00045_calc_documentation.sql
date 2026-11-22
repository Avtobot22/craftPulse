-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('D01', 1, 'count', 'docs/product/metrics/documentation.md#d01', '{"estimators":["roots","documents","coverage"],"ratio_estimator":"coverage"}'),
    ('D03', 1, 'ratio', 'docs/product/metrics/documentation.md#d03', '{"estimators":["root_owner","document_owners"],"document_format":"markdown"}'),
    ('D10', 1, 'ratio', 'docs/product/metrics/documentation.md#d10', NULL),
    ('D24', 1, 'ratio', 'docs/product/metrics/documentation.md#d24', '{"count_estimator":"spectral_findings"}'),
    ('D30', 1, 'count', 'docs/product/metrics/documentation.md#d30', '{"age_unit":"seconds"}');

ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report', 'document', 'documentation_root',
        'documentation_finding', 'documentation_report'));

ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold', 'documentation_root', 'documentation_report'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind IN ('document', 'documentation_root', 'documentation_finding', 'documentation_report');
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report'));

DELETE FROM calc.revision_input WHERE input_kind IN ('documentation_root', 'documentation_report');
ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold'));

DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('D01','D03','D10','D24','D30');
