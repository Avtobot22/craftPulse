-- +goose Up
CREATE TABLE calc.saved_view (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    owner_user_id uuid NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    series_id uuid NOT NULL,
    filters jsonb NOT NULL CHECK (jsonb_typeof(filters) = 'object'),
    pinned_revision_id uuid,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, owner_user_id) REFERENCES org.app_user (organization_id, id),
    FOREIGN KEY (organization_id, series_id) REFERENCES calc.series (organization_id, id),
    FOREIGN KEY (organization_id, pinned_revision_id) REFERENCES calc.revision (organization_id, id)
);
CREATE INDEX saved_view_owner_idx ON calc.saved_view (organization_id, owner_user_id, created_at DESC, id DESC);
CREATE INDEX saved_view_series_idx ON calc.saved_view (organization_id, series_id);
CREATE INDEX saved_view_revision_idx ON calc.saved_view (organization_id, pinned_revision_id) WHERE pinned_revision_id IS NOT NULL;

ALTER TABLE calc.saved_view ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.saved_view TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.saved_view FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON calc.saved_view TO craftpulse_app;
GRANT SELECT ON calc.saved_view TO craftpulse_backup;

-- +goose Down
DELETE FROM calc.revision_pin WHERE holder_kind = 'saved_view';
DROP TABLE calc.saved_view;
