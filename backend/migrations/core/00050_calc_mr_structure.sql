-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('C15', 1, 'count', 'docs/product/metrics/code.md#c15', '{}'),
    ('C16', 1, 'count', 'docs/product/metrics/code.md#c16', '{}'),
    ('C17', 1, 'count', 'docs/product/metrics/code.md#c17', '{}'),
    ('C28', 1, 'ratio', 'docs/product/metrics/code.md#c28', '{"alias":"I27","coverage_kind":"line"}');

-- +goose Down
DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('C15', 'C16', 'C17', 'C28');
