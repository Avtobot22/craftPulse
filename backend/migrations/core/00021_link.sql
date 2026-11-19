-- +goose Up
CREATE SCHEMA link;
GRANT USAGE ON SCHEMA link TO craftpulse_app, craftpulse_backup;

-- A version of a link between two objects (BR-010). A key in a branch or a title gives only a candidate; a link
-- is confirmed by an explicit relation of the source or by the decision of a user.
CREATE TABLE link.edge (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    link_type text NOT NULL CHECK (link_type IN ('task_mr', 'mr_run', 'run_report', 'task_parent', 'finding_task',
        'document_service', 'task_task', 'deployment_incident', 'incident_deployment')),
    from_object_id uuid NOT NULL,
    to_object_id uuid NOT NULL,
    method text NOT NULL CHECK (method IN ('source_explicit', 'user_confirmed', 'branch_key', 'title_key',
        'time_window', 'path_match')),
    status text NOT NULL CHECK (status IN ('candidate', 'confirmed', 'rejected')),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    superseded_seq bigint,
    superseded_at timestamptz,
    receipt_id uuid,
    record_no integer,
    evidence jsonb,
    actor_user_id uuid,
    confirmation_id uuid,
    decided_at timestamptz,
    valid tstzrange,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, from_object_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, to_object_id) REFERENCES normalize.source_object (organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id) REFERENCES org.app_user (organization_id, id),
    CHECK (from_object_id <> to_object_id),
    CHECK (status <> 'confirmed' OR method IN ('source_explicit', 'user_confirmed')),
    CHECK (method <> 'user_confirmed' OR actor_user_id IS NOT NULL),
    CHECK ((actor_user_id IS NULL) = (decided_at IS NULL)),
    CHECK ((receipt_id IS NULL) = (record_no IS NULL)),
    CHECK (superseded_seq > known_seq),
    CHECK ((superseded_seq IS NULL) = (superseded_at IS NULL))
);
-- One current link per pair and method: a repeated confirmation does not create a second one.
CREATE UNIQUE INDEX edge_current_idx ON link.edge (organization_id, link_type, from_object_id, to_object_id, method)
    WHERE superseded_seq IS NULL;
CREATE INDEX edge_current_to_idx ON link.edge (organization_id, to_object_id, link_type) WHERE superseded_seq IS NULL;
CREATE UNIQUE INDEX edge_confirmation_idx ON link.edge (organization_id, confirmation_id) WHERE confirmation_id IS NOT NULL;
-- Slices read superseded versions too; these also serve the foreign keys.
CREATE INDEX edge_from_idx ON link.edge (organization_id, from_object_id, known_seq);
CREATE INDEX edge_to_idx ON link.edge (organization_id, to_object_id, known_seq);
CREATE INDEX edge_actor_idx ON link.edge (organization_id, actor_user_id) WHERE actor_user_id IS NOT NULL;

ALTER TABLE link.edge ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON link.edge TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON link.edge FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA link TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA link TO craftpulse_backup;

-- +goose Down
DROP SCHEMA link CASCADE;
