-- +goose Up
INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('I01', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i01',
        '{"estimators":["runs","jobs"],"native_status_classes":{"success":"success","failed":"failure","failure":"failure","timeout":"timeout","timed_out":"timeout","canceled":"canceled","cancelled":"canceled","canceling":"canceled","skipped":"skipped","manual":"manual","unstable":"unstable"},"timeout_reasons":["job_execution_timeout","stuck_or_timeout_failure"]}'),
    ('I02', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i02', '{"identity":["run_id","job_key","config_fingerprint"],"first_attempt":1}'),
    ('I03', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i03', '{"identity":["run_id","job_key","config_fingerprint"],"requires":"complete_comparable_attempts","label":"observed_retry_flip"}'),
    ('I04', 1, 'seconds', 'docs/product/metrics/ci-cd.md#i04', '{"estimators":["attempts","seconds","total_seconds"],"duration":"job_execution_seconds","incomplete":"lower_bound"}'),
    ('I05', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i05', '{"providers":["gitlab"],"outcome":"final_job","requires":"successful_complete_run"}'),
    ('I06', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i06', '{"kinds":["runs","jobs"],"classes":["success","failure","timeout","canceled","skipped","manual","unstable","unknown"],"native_status_classes":{"success":"success","failed":"failure","failure":"failure","timeout":"timeout","timed_out":"timeout","canceled":"canceled","cancelled":"canceled","canceling":"canceled","skipped":"skipped","manual":"manual","unstable":"unstable"}}'),
    ('I07', 1, 'count', 'docs/product/metrics/ci-cd.md#i07', '{"group_by":"source_failure_kind","unknown":"separate","classification":"none"}'),
    ('I10', 1, 'ratio', 'docs/product/metrics/ci-cd.md#i10', '{"estimators":["","loaded"],"unknown_total":"loaded_lower_bound","requires":"ci_reconciliation_covering_point"}');

UPDATE calc.definition SET parameters = parameters || '{"cohort":"run_finished_at"}'::jsonb
WHERE metric_id IN ('I01','I02','I03','I04','I05','I06','I07','I10') AND definition_version = 1;

ALTER TABLE calc.contribution ADD COLUMN evidence jsonb;
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request', 'ci_run', 'ci_job', 'ci_attempt'));

-- +goose Down
DELETE FROM calc.contribution WHERE object_kind IN ('ci_run', 'ci_job', 'ci_attempt');
ALTER TABLE calc.contribution DROP CONSTRAINT contribution_object_kind_check;
ALTER TABLE calc.contribution ADD CONSTRAINT contribution_object_kind_check
    CHECK (object_kind IN ('task', 'connection', 'merge_request'));
ALTER TABLE calc.contribution DROP COLUMN evidence;
DELETE FROM calc.definition WHERE metric_id IN ('I01','I02','I03','I04','I05','I06','I07','I10');
