-- +goose Up
CREATE SCHEMA outbound;
GRANT USAGE ON SCHEMA outbound TO craftpulse_app, craftpulse_backup;

ALTER TABLE change.summary_revision
    ADD COLUMN reports_ready_at timestamptz,
    ADD COLUMN publication_status text NOT NULL DEFAULT 'not_requested'
        CHECK (publication_status IN ('not_requested','pending','succeeded','failed','unknown','stale')),
    ADD COLUMN publication_external_id text,
    ADD COLUMN publication_error_code text,
    ADD COLUMN published_at timestamptz;

-- Existing queued jobs can already reference revisions built before signal locations were stored.
UPDATE change.summary_item AS i
SET value = jsonb_strip_nulls(jsonb_build_object('rule', o.rule_id, 'path', o.path, 'package', o.package_purl, 'line', o.line_start))
FROM normalize.finding_occurrence AS o
WHERE (i.organization_id, i.occurrence_id) = (o.organization_id, o.id)
  AND i.item_kind = 'new' AND i.required_right = 'none' AND i.value IS NULL;

CREATE TABLE outbound.target (
    organization_id uuid NOT NULL,
    target_kind text NOT NULL CHECK (target_kind IN ('mr_comment','tracker_issue')),
    target_marker text NOT NULL CHECK (length(target_marker) > 0),
    connection_id uuid NOT NULL,
    external_id text,
    last_summary_revision_id uuid,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, target_kind, target_marker),
    FOREIGN KEY (organization_id, connection_id) REFERENCES connect.connection (organization_id, id),
    FOREIGN KEY (organization_id, last_summary_revision_id) REFERENCES change.summary_revision (organization_id, id)
);
CREATE INDEX outbound_target_connection_idx ON outbound.target (organization_id, connection_id);
CREATE INDEX outbound_target_revision_idx ON outbound.target (organization_id, last_summary_revision_id);

CREATE TABLE outbound.action (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    target_kind text NOT NULL,
    target_marker text NOT NULL,
    summary_revision_id uuid,
    confirmation_id uuid,
    status text NOT NULL CHECK (status IN ('intent','succeeded','failed','unknown','stale')),
    attempt_no integer NOT NULL CHECK (attempt_no > 0),
    error_code text,
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, target_kind, target_marker) REFERENCES outbound.target (organization_id, target_kind, target_marker),
    FOREIGN KEY (organization_id, summary_revision_id) REFERENCES change.summary_revision (organization_id, id),
    CHECK (target_kind <> 'tracker_issue' OR confirmation_id IS NOT NULL)
);
CREATE UNIQUE INDEX outbound_action_confirmation_uq ON outbound.action (organization_id, confirmation_id) WHERE confirmation_id IS NOT NULL;
CREATE INDEX outbound_action_target_idx ON outbound.action (organization_id, target_kind, target_marker, created_at);
CREATE INDEX outbound_action_revision_idx ON outbound.action (organization_id, summary_revision_id);

ALTER TABLE outbound.target ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbound.action ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON outbound.target TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY org_isolation ON outbound.action TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON outbound.target FOR SELECT TO craftpulse_backup USING (true);
CREATE POLICY backup_read ON outbound.action FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE ON outbound.target, outbound.action TO craftpulse_app;
GRANT SELECT ON outbound.target, outbound.action TO craftpulse_backup;

-- +goose Down
DROP TABLE outbound.action, outbound.target;
DROP SCHEMA outbound;
ALTER TABLE change.summary_revision
    DROP COLUMN reports_ready_at, DROP COLUMN publication_status, DROP COLUMN publication_external_id,
    DROP COLUMN publication_error_code, DROP COLUMN published_at;
