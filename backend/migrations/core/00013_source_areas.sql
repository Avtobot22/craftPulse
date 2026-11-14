-- +goose Up
-- The source areas of a project in time (TZ 12.5.1): tracker projects, repositories and wiki spaces. The area
-- is an object of normalize, so it is referenced without a foreign key across modules.
CREATE TABLE org.project_area (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    project_id uuid NOT NULL,
    area_id uuid NOT NULL,
    valid tstzrange NOT NULL CHECK (NOT lower_inf(valid) AND lower_inc(valid) AND NOT upper_inc(valid)),
    known_seq bigint NOT NULL,
    superseded_seq bigint,
    superseded_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES org.project (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    EXCLUDE USING gist (organization_id WITH =, project_id WITH =, area_id WITH =, valid WITH &&)
        WHERE (superseded_seq IS NULL)
);
-- The partial exclusion does not cover superseded versions, so the foreign key has its own index.
CREATE INDEX project_area_project_idx ON org.project_area (organization_id, project_id);
-- Which projects an area belongs to when access is decided.
CREATE INDEX project_area_area_idx ON org.project_area (organization_id, area_id) WHERE superseded_seq IS NULL;

ALTER TABLE org.project_area ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.project_area TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.project_area FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON org.project_area TO craftpulse_app;
GRANT SELECT ON org.project_area TO craftpulse_backup;

-- access finds the connection of an area on every read, listAreas pages the areas of connections.
CREATE INDEX connection_scope_area_idx ON connect.connection_scope (organization_id, area_id) WHERE area_id IS NOT NULL;

-- +goose Down
DROP INDEX connect.connection_scope_area_idx;
DROP TABLE org.project_area;
