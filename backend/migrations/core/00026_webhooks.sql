-- +goose Up
CREATE TABLE connect.webhook_route (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    revoked_at timestamptz,
    FOREIGN KEY (organization_id, connection_id) REFERENCES connect.connection (organization_id, id)
);
CREATE INDEX webhook_route_connection_idx ON connect.webhook_route (organization_id, connection_id);
CREATE UNIQUE INDEX webhook_route_active_uq ON connect.webhook_route (organization_id, connection_id) WHERE revoked_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON connect.webhook_route TO craftpulse_app;
GRANT SELECT ON connect.webhook_route TO craftpulse_backup;
INSERT INTO connect.webhook_route (organization_id, connection_id, created_at)
SELECT c.organization_id, c.id, now() FROM connect.connection c
JOIN connect.capability p ON p.organization_id = c.organization_id AND p.connection_id = c.id
WHERE c.status IN ('active', 'access_lost') AND p.capability = 'webhook' AND p.state IN ('verified', 'partial');

CREATE TABLE sync.webhook_delivery (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    connection_id uuid NOT NULL,
    received_at timestamptz NOT NULL,
    rejected_at timestamptz,
    reject_reason text,
    PRIMARY KEY (organization_id, id)
);
CREATE INDEX webhook_delivery_connection_idx ON sync.webhook_delivery (organization_id, connection_id, received_at DESC);
CREATE INDEX webhook_delivery_rejected_idx ON sync.webhook_delivery (organization_id, connection_id, rejected_at DESC) WHERE rejected_at IS NOT NULL;
ALTER TABLE sync.webhook_delivery ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.webhook_delivery TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.webhook_delivery FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON sync.webhook_delivery TO craftpulse_app;
GRANT SELECT ON sync.webhook_delivery TO craftpulse_backup;

-- +goose Down
DROP TABLE sync.webhook_delivery;
DROP TABLE connect.webhook_route;
