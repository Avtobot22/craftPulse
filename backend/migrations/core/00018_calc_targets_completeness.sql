-- +goose Up
-- Definitions of the targets, reopens and completeness of tasks (TZ 12.5.8). T23 keeps its follow-up window H as
-- a parameter of version 1 (BR-008); another window is a new version.
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('T10', 1, 'ratio', 'docs/product/metrics/tasks.md#t10', '{"cohort": "first_accepted_completion", "target": "type_size_version_at_knowledge"}'),
    ('T11', 1, 'count', 'docs/product/metrics/tasks.md#t11', '{"target": "type_size_version_at_knowledge"}'),
    ('T23', 1, 'ratio', 'docs/product/metrics/tasks.md#t23', '{"cohort": "first_accepted_completion", "follow_up_days": 30}'),
    ('T38', 1, 'ratio', 'docs/product/metrics/tasks.md#t38', '{"estimators": ["size", "type", "team", "start"]}'),
    ('T39', 1, 'ratio', 'docs/product/metrics/tasks.md#t39', NULL),
    ('T40', 1, 'seconds', 'docs/product/metrics/tasks.md#t40', '{"estimators": ["lag", "last_success_age"]}'),
    ('T41', 1, 'ratio', 'docs/product/metrics/tasks.md#t41', '{"estimators": ["tasks", "transitions"]}'),
    ('T42', 1, 'count', 'docs/product/metrics/tasks.md#t42', '{"estimators": ["missing", "changed"]}');

-- T40 and T42 describe the collection of a connection, which a point of them names as its object.
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check CHECK (object_kind IN ('task', 'connection'));

-- +goose Down
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check CHECK (object_kind IN ('task'));
DELETE FROM calc.definition WHERE metric_id IN ('T10', 'T11', 'T23', 'T38', 'T39', 'T40', 'T41', 'T42');
