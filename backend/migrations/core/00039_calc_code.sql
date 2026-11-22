-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('C01', 1, 'count', 'docs/product/metrics/code.md#c01', '{"estimators":["production","test"]}'),
    ('C04', 1, 'count', 'docs/product/metrics/code.md#c04', '{"estimators":["p50","p90","max"],"percentiles":"nearest_rank"}'),
    ('C05', 1, 'ratio', 'docs/product/metrics/code.md#c05', '{"threshold":"scan_manifest.function_thresholds.ccn_threshold","operator":">"}'),
    ('C06', 1, 'count', 'docs/product/metrics/code.md#c06', '{"estimators":["p50","p90","max","above_threshold"],"percentiles":"nearest_rank","threshold":"scan_manifest.function_thresholds.nloc_threshold","operator":">"}'),
    ('C08', 1, 'ratio', 'docs/product/metrics/code.md#c08', '{"denominator":"analyzed_physical_lines","ranges":"inclusive_union_by_file"}'),
    ('C09', 1, 'count', 'docs/product/metrics/code.md#c09', '{"group":"connected_clone_pairs"}');

ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt',
        'code_file', 'code_function', 'clone_group', 'code_report'));

ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark', 'code_report', 'profile_threshold'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind IN ('code_file', 'code_function', 'clone_group', 'code_report');
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt'));

DELETE FROM calc.revision_input WHERE input_kind IN ('code_report', 'profile_threshold');
ALTER TABLE calc.revision_input DROP CONSTRAINT revision_input_input_kind_check;
ALTER TABLE calc.revision_input ADD CONSTRAINT revision_input_input_kind_check
    CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark'));

DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('C01','C04','C05','C06','C08','C09');
