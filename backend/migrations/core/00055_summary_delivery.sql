-- +goose Up
ALTER TABLE change.summary_revision
    ADD COLUMN publication_status text NOT NULL DEFAULT 'not_requested'
        CHECK (publication_status IN ('not_requested', 'pending', 'succeeded', 'failed', 'unknown', 'stale')),
    ADD COLUMN publication_external_id text,
    ADD COLUMN publication_error_code text,
    ADD COLUMN published_at timestamptz;

-- Before this migration, every saved non-running revision queued publication in the same transaction.
UPDATE change.summary_revision SET publication_status = 'pending' WHERE pipeline_state <> 'running';

-- Restore the display snapshot from the exact run and attempt versions of existing revisions.
WITH snapshots AS (
    SELECT i.organization_id, i.summary_revision_id, i.item_no,
           jsonb_build_object('PipelineID', run.run_id, 'ParentPipelineID', run.parent_run_id,
               'Outcome', run.outcome, 'Attempts', stats.attempts, 'RetriedJobs', stats.retried_jobs) AS display
    FROM change.summary_item i
    JOIN normalize.ci_run_version run
      ON run.organization_id = i.organization_id AND run.id = (i.value->'run_versions'->>0)::uuid
    CROSS JOIN LATERAL (
        SELECT COALESCE(sum(attempts), 0) AS attempts, count(*) FILTER (WHERE last_attempt > 1) AS retried_jobs
        FROM (
            SELECT count(*) AS attempts, max(a.attempt_no) AS last_attempt
            FROM jsonb_array_elements_text(i.value->'attempt_versions') AS ids(version_id)
            JOIN normalize.ci_attempt_version a ON a.organization_id = i.organization_id AND a.id = ids.version_id::uuid
            GROUP BY a.run_id, a.job_key, a.config_fingerprint
        ) jobs
    ) stats
    WHERE i.item_kind = 'ci_attempts' AND NOT (i.value ? 'display')
)
UPDATE change.summary_item i SET value = i.value || jsonb_build_object('display', s.display)
FROM snapshots s
WHERE (i.organization_id, i.summary_revision_id, i.item_no) = (s.organization_id, s.summary_revision_id, s.item_no);

ALTER TABLE change.summary_item ADD COLUMN mode text CHECK (mode IN ('import', 'builtin'));

CREATE TABLE ingest.ci_result (
    organization_id uuid NOT NULL,
    receipt_id uuid NOT NULL,
    pending boolean NOT NULL,
    pipeline_id uuid,
    mr_id uuid,
    PRIMARY KEY (organization_id, receipt_id),
    FOREIGN KEY (organization_id, receipt_id) REFERENCES ingest.receipt (organization_id, id) ON DELETE CASCADE,
    CHECK (mr_id IS NULL OR pipeline_id IS NOT NULL)
);
ALTER TABLE ingest.ci_result ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON ingest.ci_result TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON ingest.ci_result FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON ingest.ci_result TO craftpulse_app;
GRANT SELECT ON ingest.ci_result TO craftpulse_backup;

-- +goose Down
DROP TABLE ingest.ci_result;
UPDATE change.summary_item SET value = value - 'display' - 'public_reports' WHERE item_kind = 'ci_attempts';
ALTER TABLE change.summary_item DROP COLUMN mode;
ALTER TABLE change.summary_revision DROP COLUMN published_at, DROP COLUMN publication_error_code,
    DROP COLUMN publication_external_id, DROP COLUMN publication_status;
