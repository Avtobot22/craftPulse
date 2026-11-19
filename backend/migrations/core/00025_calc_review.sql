-- +goose Up
-- Definitions of the review of MRs and of the history of events (TZ 12.5.8).
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('A05', 1, 'count', 'docs/product/metrics/activity.md#a05', '{"state_at": "point_end"}'),
    ('A06', 1, 'seconds', 'docs/product/metrics/activity.md#a06',
        '{"estimators": ["p50", "p85"], "percentile": "nearest_rank", "low_sample_below": 20, "from": "ready_or_open"}'),
    ('A07', 1, 'seconds', 'docs/product/metrics/activity.md#a07',
        '{"cohort": "first_response", "estimators": ["p50", "p85"], "percentile": "nearest_rank", "low_sample_below": 20}'),
    ('A10', 1, 'ratio', 'docs/product/metrics/activity.md#a10', '{"excluded": "reviewer_field_unavailable"}'),
    ('A22', 1, 'count', 'docs/product/metrics/activity.md#a22', '{"estimators": ["human", "bot", "system", "unknown"]}');

-- The review metrics count MRs, and the events of A22 name their task or MR.
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check CHECK (object_kind IN ('task', 'connection', 'merge_request'));

-- +goose Down
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check CHECK (object_kind IN ('task', 'connection'));
DELETE FROM calc.definition WHERE metric_id IN ('A05', 'A06', 'A07', 'A10', 'A22');
