-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('X03', 1, 'ratio', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["warning", "new_code", "new_security", "docs_violation", "code_exception", "security_exception"], "state_at": "point_end", "counted_object": "task"}'),
    ('X04', 1, 'ratio', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["caveat", "new_code", "new_security", "docs_violation", "missing_report", "fail_to_pass", "allowed_failure", "code_exception", "security_exception"], "state_at": "point_end", "counted_object": "ci_run"}'),
    ('X05', 1, 'ratio', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["confirmed_mr", "current_head_ci", "complete_reports", "checked_code", "checked_security", "checked_tests", "checked_coverage", "checked_docs"], "state_at": "point_end", "counted_object": "task"}'),
    ('X06', 1, 'count', 'docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ',
        '{"estimators": ["code:new", "code:persisting", "code:resolved", "code:accepted", "code:unverified", "sast:new", "sast:persisting", "sast:resolved", "sast:accepted", "sast:unverified", "sca:new", "sca:persisting", "sca:resolved", "sca:accepted", "sca:unverified", "secrets:new", "secrets:persisting", "secrets:resolved", "secrets:accepted", "secrets:unverified", "iac:new", "iac:persisting", "iac:resolved", "iac:accepted", "iac:unverified", "image:new", "image:persisting", "image:resolved", "image:accepted", "image:unverified", "docs:new", "docs:persisting", "docs:resolved", "docs:accepted", "docs:unverified"], "state_at": "point_end", "counted_object": "merge_request"}');

-- +goose Down
DELETE FROM calc.definition WHERE metric_id IN ('X03', 'X04', 'X05', 'X06') AND definition_version = 1;
