-- +goose Up
CREATE SCHEMA view;
GRANT USAGE ON SCHEMA view TO craftpulse_app, craftpulse_backup;

-- The public state of a run a user asked for; the job of the queue behind it is never shown (TZ 12.5.8).
CREATE TABLE view.operation (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    requested_by uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('sync_run', 'series_refresh', 'documentation_check')),
    resource_kind text NOT NULL CHECK (octet_length(resource_kind) BETWEEN 1 AND 64),
    resource_id uuid NOT NULL,
    action text NOT NULL CHECK (octet_length(action) BETWEEN 1 AND 64),
    parameters jsonb NOT NULL,
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    completed_at timestamptz,
    result_uri text CHECK (octet_length(result_uri) BETWEEN 1 AND 4096),
    error_code text CHECK (octet_length(error_code) BETWEEN 1 AND 64),
    correlation_id uuid,
    PRIMARY KEY (organization_id, id),
    CHECK ((status IN ('succeeded', 'failed')) = (completed_at IS NOT NULL)),
    CHECK ((status = 'succeeded') = (result_uri IS NOT NULL)),
    CHECK ((status = 'failed') = (error_code IS NOT NULL)),
    CHECK (correlation_id IS NULL OR status = 'failed')
);
-- A repeated request of an active run finds its descriptor.
CREATE UNIQUE INDEX operation_active_uq ON view.operation (organization_id, requested_by, kind, resource_kind, resource_id)
    WHERE status IN ('queued', 'running');
CREATE INDEX operation_completed_idx ON view.operation (organization_id, completed_at);
CREATE INDEX operation_requested_idx ON view.operation (organization_id, requested_by, created_at);

ALTER TABLE view.operation ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON view.operation TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON view.operation FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON view.operation TO craftpulse_app;
GRANT SELECT ON view.operation TO craftpulse_backup;

-- +goose Down
DROP SCHEMA view CASCADE;
