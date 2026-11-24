-- +goose Up
CREATE TABLE normalize.mr_diff (
    organization_id uuid NOT NULL,
    mr_revision_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    receipt_id uuid NOT NULL,
    record_no integer NOT NULL CHECK (record_no > 0),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    completeness text NOT NULL CHECK (completeness IN ('complete', 'partial')),
    files jsonb NOT NULL CHECK (jsonb_typeof(files) = 'array'),
    PRIMARY KEY (organization_id, mr_revision_id, content_hash),
    FOREIGN KEY (organization_id, mr_revision_id) REFERENCES normalize.mr_revision (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX mr_diff_snapshot_idx ON normalize.mr_diff (organization_id, mr_revision_id, known_seq DESC);
CREATE INDEX mr_revision_head_lookup_idx ON normalize.mr_revision (organization_id, mr_id, head_sha, created_at DESC, known_seq DESC, id DESC);

ALTER TABLE normalize.mr_diff ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.mr_diff TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.mr_diff FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, DELETE ON normalize.mr_diff TO craftpulse_app;
GRANT SELECT ON normalize.mr_diff TO craftpulse_backup;

-- +goose Down
DROP INDEX normalize.mr_revision_head_lookup_idx;
DROP TABLE normalize.mr_diff;
