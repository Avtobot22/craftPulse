-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('X01', 1, 'ratio', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["no_reviewer", "no_first_response"], "state_at": "point_end", "counted_object": "task"}'),
    ('X02', 1, 'ratio', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["failed", "passed_after_retry", "missing_report"], "state_at": "point_end", "counted_object": "task"}');

-- +goose Down
DELETE FROM calc.definition WHERE metric_id IN ('X01', 'X02');
