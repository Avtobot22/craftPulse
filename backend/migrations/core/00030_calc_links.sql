-- +goose Up
-- Definitions of the coverage of the links of tasks and MRs (TZ 12.5.8): only confirmed links count, the
-- code-required types come from code_required_type rules of the mapping of the team.
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('A18', 1, 'ratio', 'docs/product/metrics/activity.md#a18', '{"population": "merged_in_period", "links": "confirmed"}'),
    ('A20', 1, 'ratio', 'docs/product/metrics/activity.md#a20',
        '{"population": "in_work_at_end_or_completed", "types": "code_required_type", "mr": "open_or_merged", "links": "confirmed"}'),
    ('T28', 1, 'ratio', 'docs/product/metrics/tasks.md#t28',
        '{"population": "in_work_at_end_or_completed", "types": "code_required_type", "mr": "merged_by_end", "links": "confirmed"}');

-- +goose Down
DELETE FROM calc.definition WHERE metric_id IN ('A18', 'A20', 'T28');
