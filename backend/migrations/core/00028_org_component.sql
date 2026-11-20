-- +goose Up
-- A component is a path of a repository inside a service (TZ 12.5.1). Its ID stays across versions: a version is
-- the row of its knowledge number, so the matrix of checks and reports keep pointing to the same component.
CREATE TABLE org.component (
    organization_id uuid NOT NULL,
    id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    service_id uuid NOT NULL,
    repository_area_id uuid NOT NULL,
    path_prefix text NOT NULL CHECK (octet_length(path_prefix) <= 1024),
    status text NOT NULL CHECK (status IN ('proposed', 'confirmed', 'rejected')),
    origin text NOT NULL CHECK (origin IN ('manifest', 'manual')),
    confirmed_by uuid,
    confirmed_at timestamptz,
    PRIMARY KEY (organization_id, id, known_seq),
    FOREIGN KEY (organization_id, service_id) REFERENCES org.service (organization_id, id),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL)),
    CHECK (status <> 'confirmed' OR (confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL))
);
-- One component in force per path.
CREATE UNIQUE INDEX component_path_uq ON org.component (organization_id, service_id, repository_area_id, path_prefix)
    WHERE superseded_seq IS NULL;
CREATE UNIQUE INDEX component_current_uq ON org.component (organization_id, id) WHERE superseded_seq IS NULL;
CREATE INDEX component_service_idx ON org.component (organization_id, service_id);
-- Which components cover the repository of a report or of a project.
CREATE INDEX component_area_idx ON org.component (organization_id, repository_area_id)
    WHERE superseded_seq IS NULL AND status = 'confirmed';

ALTER TABLE org.component ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON org.component TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON org.component FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON org.component TO craftpulse_app;
GRANT SELECT ON org.component TO craftpulse_backup;

-- +goose Down
DROP TABLE org.component;
