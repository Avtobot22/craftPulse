-- +goose Up
ALTER TABLE normalize.report
    ADD COLUMN dependency_inventory jsonb CHECK (jsonb_typeof(dependency_inventory) = 'object');

CREATE TABLE normalize.analysis_run (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    receipt_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    area_id uuid NOT NULL,
    component_id uuid,
    sha text NOT NULL CHECK (sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    tool_id text COLLATE "C" NOT NULL CHECK (tool_id <> ''),
    record_nos integer[] NOT NULL,
    metadata jsonb NOT NULL CHECK (jsonb_typeof(metadata) = 'object'),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, receipt_id, tool_id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id),
    FOREIGN KEY (organization_id, receipt_id) REFERENCES ingest.receipt (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX analysis_run_scope_idx ON normalize.analysis_run (organization_id, area_id, sha, component_id, known_seq);
ALTER TABLE normalize.analysis_run ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.analysis_run TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.analysis_run FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.analysis_run TO craftpulse_app;
GRANT SELECT ON normalize.analysis_run TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.analysis_run;
ALTER TABLE normalize.report DROP COLUMN dependency_inventory;
