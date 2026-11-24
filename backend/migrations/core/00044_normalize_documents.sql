-- +goose Up
ALTER TABLE normalize.report
    ADD COLUMN docs_run_id text,
    ADD COLUMN docs_file_hash bytea CHECK (octet_length(docs_file_hash) = 32);

CREATE INDEX report_docs_file_idx ON normalize.report (organization_id, area_id, sha, report_kind, docs_run_id, docs_file_hash)
    WHERE report_format = 'sarif' AND docs_file_hash IS NOT NULL;
CREATE INDEX finding_docs_fingerprint_idx ON normalize.finding_occurrence (organization_id, fingerprint, report_id)
    WHERE finding_kind IN ('docs', 'openapi');

CREATE TABLE normalize.document_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    document_id uuid NOT NULL,
    area_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    version_ref text COLLATE "C" NOT NULL CHECK (version_ref ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    doc_format text NOT NULL CHECK (doc_format IN ('markdown', 'openapi')),
    path text COLLATE "C" NOT NULL CHECK (octet_length(path) BETWEEN 1 AND 4096),
    title text,
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    owner text,
    acl_state text NOT NULL CHECK (acl_state IN ('known', 'unknown')),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, document_id, version_ref),
    FOREIGN KEY (organization_id, document_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id)
);
CREATE INDEX document_version_commit_idx ON normalize.document_version (organization_id, area_id, version_ref);
ALTER TABLE normalize.document_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.document_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.document_version FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.document_version TO craftpulse_app;
GRANT SELECT ON normalize.document_version TO craftpulse_backup;

-- The inventory and its findings are separate reports of the same check.
CREATE TABLE normalize.docs_report (
    organization_id uuid NOT NULL,
    report_id uuid NOT NULL,
    root_id uuid,
    path_prefix text COLLATE "C" NOT NULL,
    owner_rule text,
    checks text[] NOT NULL,
    links jsonb CHECK (jsonb_typeof(links) = 'object'),
    sarif_hash bytea CHECK (octet_length(sarif_hash) = 32),
    PRIMARY KEY (organization_id, report_id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.report (organization_id, id) ON DELETE CASCADE
);
ALTER TABLE normalize.docs_report ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.docs_report TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.docs_report FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.docs_report TO craftpulse_app;
GRANT SELECT ON normalize.docs_report TO craftpulse_backup;

CREATE TABLE normalize.docs_report_document (
    organization_id uuid NOT NULL,
    report_id uuid NOT NULL,
    document_version_id uuid NOT NULL,
    spec_valid boolean,
    PRIMARY KEY (organization_id, report_id, document_version_id),
    FOREIGN KEY (organization_id, report_id) REFERENCES normalize.docs_report (organization_id, report_id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, document_version_id) REFERENCES normalize.document_version (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX docs_report_document_version_idx ON normalize.docs_report_document (organization_id, document_version_id);
ALTER TABLE normalize.docs_report_document ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.docs_report_document TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.docs_report_document FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.docs_report_document TO craftpulse_app;
GRANT SELECT ON normalize.docs_report_document TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.docs_report_document, normalize.docs_report, normalize.document_version;
DROP INDEX normalize.finding_docs_fingerprint_idx;
DROP INDEX normalize.report_docs_file_idx;
ALTER TABLE normalize.report DROP COLUMN docs_file_hash, DROP COLUMN docs_run_id;
