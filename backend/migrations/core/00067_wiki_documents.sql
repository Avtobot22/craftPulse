-- +goose Up
ALTER TABLE connect.source_instance DROP CONSTRAINT source_instance_provider_check;
ALTER TABLE connect.source_instance ADD CONSTRAINT source_instance_provider_check
    CHECK (provider IN ('jira_cloud', 'gitlab', 'github', 'jenkins', 'sonarqube',
        'confluence_cloud', 'confluence_dc', 'git', 'universal', 'client'));

ALTER TABLE docs.root DROP CONSTRAINT root_profile_id_check;
ALTER TABLE docs.root ADD CONSTRAINT root_profile_id_check
    CHECK (profile_id IN ('git-markdown-openapi', 'wiki-inventory'));

ALTER TABLE normalize.document_version
    DROP CONSTRAINT document_version_version_ref_check,
    DROP CONSTRAINT document_version_doc_format_check,
    ALTER COLUMN content_hash DROP NOT NULL,
    ADD COLUMN page_native_id text COLLATE "C",
    ADD COLUMN restriction_native text,
    ADD COLUMN source_url text,
    ADD COLUMN source_updated_at timestamptz,
    ADD CONSTRAINT document_version_format_check CHECK (
        (doc_format IN ('markdown', 'openapi') AND version_ref ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'
            AND page_native_id IS NULL AND content_hash IS NOT NULL)
        OR (doc_format = 'confluence' AND version_ref ~ '^[1-9][0-9]{0,18}$' AND page_native_id IS NOT NULL)
    );
CREATE INDEX document_version_wiki_idx ON normalize.document_version (organization_id, area_id, known_seq)
    WHERE doc_format = 'confluence';

-- Permissions, ancestry and body availability can change without a new page version.
CREATE TABLE normalize.wiki_page_observation (
    organization_id uuid NOT NULL,
    version_id uuid NOT NULL,
    area_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    ancestor_ids text[] NOT NULL,
    restriction_native text,
    acl_state text NOT NULL CHECK (acl_state IN ('known', 'unknown')),
    source_url text,
    content_hash bytea CHECK (octet_length(content_hash) = 32),
    deleted boolean NOT NULL,
    observed_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, version_id, known_seq),
    FOREIGN KEY (organization_id, version_id) REFERENCES normalize.document_version (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, area_id) REFERENCES normalize.area (organization_id, area_id)
);
CREATE INDEX wiki_page_observation_area_idx ON normalize.wiki_page_observation (organization_id, area_id, known_seq);
ALTER TABLE normalize.wiki_page_observation ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON normalize.wiki_page_observation TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON normalize.wiki_page_observation FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON normalize.wiki_page_observation TO craftpulse_app;
GRANT SELECT ON normalize.wiki_page_observation TO craftpulse_backup;

-- +goose Down
DROP TABLE normalize.wiki_page_observation;
DROP INDEX normalize.document_version_wiki_idx;
DELETE FROM normalize.document_version WHERE doc_format = 'confluence';
ALTER TABLE normalize.document_version
    DROP CONSTRAINT document_version_format_check,
    DROP COLUMN page_native_id, DROP COLUMN restriction_native, DROP COLUMN source_url, DROP COLUMN source_updated_at,
    ALTER COLUMN content_hash SET NOT NULL,
    ADD CONSTRAINT document_version_version_ref_check CHECK (version_ref ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    ADD CONSTRAINT document_version_doc_format_check CHECK (doc_format IN ('markdown', 'openapi'));
ALTER TABLE docs.root DROP CONSTRAINT root_profile_id_check;
ALTER TABLE docs.root ADD CONSTRAINT root_profile_id_check CHECK (profile_id = 'git-markdown-openapi');
ALTER TABLE connect.source_instance DROP CONSTRAINT source_instance_provider_check;
ALTER TABLE connect.source_instance ADD CONSTRAINT source_instance_provider_check
    CHECK (provider IN ('jira_cloud', 'gitlab', 'github', 'jenkins', 'sonarqube', 'confluence_cloud', 'git', 'universal', 'client'));
