-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('I19', 1, 'count', 'docs/product/metrics/ci-cd.md#i19', '{"group_by":["suite","config_fingerprint","attempt_id"]}'),
    ('I25', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i25', '{"coverage_kind":"line"}'),
    ('I26', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i26', '{"coverage_kind":"branch"}');

-- +goose Down
DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('I19','I25','I26');
