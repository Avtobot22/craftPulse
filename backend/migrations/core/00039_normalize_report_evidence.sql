-- +goose Up
ALTER TABLE normalize.test_execution ADD COLUMN suite text;

-- A rejected record has provenance, but never becomes a normalized report.
CREATE TABLE normalize.rejected_report (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    area_id uuid NOT NULL,
    component_id uuid,
    report_kind text NOT NULL,
    language text NOT NULL,
    sha text CHECK (sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    run_id uuid,
    attempt_id uuid,
    reason_code text NOT NULL,
    PRIMARY KEY (organization_id, receipt_id, record_no),
    FOREIGN KEY (organization_id, receipt_id, record_no) REFERENCES ingest.record (organization_id, receipt_id, record_no) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    FOREIGN KEY (organization_id, run_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, attempt_id) REFERENCES normalize.source_object (organization_id, id)
);
CREATE INDEX rejected_report_area_sha_idx ON normalize.rejected_report (organization_id, area_id, sha);
ALTER TABLE normalize.rejected_report ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.rejected_report TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.rejected_report FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.rejected_report TO craftpulse_app;
GRANT SELECT ON normalize.rejected_report TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.rejected_report;
ALTER TABLE normalize.test_execution DROP COLUMN suite;
