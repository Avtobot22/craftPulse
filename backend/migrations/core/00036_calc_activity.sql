-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('A23', 1, 'seconds', 'docs/product/metrics/activity.md#a23',
        '{"cohort":"observed_at","percentiles":[50,95],"group_by":"observed_via"}'),
    ('A24', 1, 'ratio', 'docs/product/metrics/activity.md#a24',
        '{"estimators":["","history_from"],"denominator":"checked_objects","history_from_unit":"unix_seconds"}'),
    ('A27', 1, 'seconds', 'docs/product/metrics/activity.md#a27',
        '{"estimators":["last_success_at","watermark","last_reconcile_at"],"group_by":["connection","data_kind"],"timestamp_unit":"unix_seconds"}'),
    ('A28', 1, 'ratio', 'docs/product/metrics/activity.md#a28',
        '{"estimators":["","human","bot","system","unknown"],"kind_source":"confirmed_account_assignment_at_knowledge_seq"}'),
    ('A30', 1, 'count', 'docs/product/metrics/activity.md#a30',
        '{"estimators":["confirmed","proposed","rejected","ambiguous","unambiguous","ambiguous_share"],"candidate_identity":["task","merge_request"]}');

-- +goose Down
DELETE FROM calc.definition WHERE definition_version = 1 AND metric_id IN ('A23','A24','A27','A28','A30');
