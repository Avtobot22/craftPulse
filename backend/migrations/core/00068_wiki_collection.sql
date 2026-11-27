-- +goose Up
CREATE TABLE sync.wiki_reconcile_scope (
    organization_id uuid NOT NULL,
    run_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    failed boolean NOT NULL,
    expected integer NOT NULL CHECK (expected >= 0),
    read_count integer NOT NULL CHECK (read_count BETWEEN 0 AND expected),
    PRIMARY KEY (organization_id, run_id, scope_id),
    FOREIGN KEY (organization_id, run_id) REFERENCES sync.reconcile_run (organization_id, id)
);
CREATE INDEX wiki_reconcile_scope_latest_idx ON sync.wiki_reconcile_scope (organization_id, scope_id, known_seq DESC);
ALTER TABLE sync.wiki_reconcile_scope ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON sync.wiki_reconcile_scope TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON sync.wiki_reconcile_scope FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON sync.wiki_reconcile_scope TO craftpulse_app;
GRANT SELECT ON sync.wiki_reconcile_scope TO craftpulse_backup;

INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters)
VALUES ('D27', 2, 'ratio', 'docs/product/metrics/documentation.md#d27', '{"scope":"wiki_space"}');

-- +goose Down
DELETE FROM calc.definition WHERE metric_id = 'D27' AND definition_version = 2;
DROP TABLE sync.wiki_reconcile_scope;
