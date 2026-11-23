-- +goose Up
CREATE SCHEMA decision;
GRANT USAGE ON SCHEMA decision TO craftpulse_app, craftpulse_backup;

CREATE TABLE decision.decision (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    kind text NOT NULL CHECK (kind IN ('exception', 'bypass')),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    wake_at timestamptz,
    PRIMARY KEY (organization_id, id)
);
CREATE INDEX decision_created_idx ON decision.decision (organization_id, created_at, id);

CREATE TABLE decision.decision_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    decision_id uuid NOT NULL,
    version_no integer NOT NULL CHECK (version_no > 0),
    base_version_no integer,
    decision_type text NOT NULL CHECK (decision_type IN ('false_positive', 'accepted_risk', 'not_applicable', 'bypass')),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 4000),
    area_id uuid NOT NULL,
    component_id uuid,
    finding_identity_id uuid,
    finding_kind text NOT NULL CHECK (finding_kind IN ('code', 'security', 'secret', 'dependency', 'iac', 'docs', 'openapi')),
    rule_id text COLLATE "C" CHECK (octet_length(rule_id) BETWEEN 1 AND 255),
    path_prefix text COLLATE "C" CHECK (octet_length(path_prefix) BETWEEN 1 AND 1024),
    package_purl text CHECK (octet_length(package_purl) BETWEEN 1 AND 1024),
    effective_from timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    approval_required boolean NOT NULL,
    external_authority text,
    external_ref text,
    author_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, decision_id, version_no),
    UNIQUE (organization_id, confirmation_id),
    FOREIGN KEY (organization_id, decision_id) REFERENCES decision.decision (organization_id, id),
    CHECK (base_version_no IS NOT DISTINCT FROM CASE WHEN version_no > 1 THEN version_no - 1 END),
    CHECK (expires_at > effective_from),
    CHECK (finding_identity_id IS NOT NULL OR rule_id IS NOT NULL)
);
CREATE INDEX decision_version_scope_idx ON decision.decision_version (organization_id, area_id, finding_identity_id, known_seq);
CREATE INDEX decision_version_expiry_idx ON decision.decision_version (organization_id, expires_at, decision_id);

CREATE TABLE decision.decision_transition (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    decision_version_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
    actor_user_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmation_id uuid,
    comment text NOT NULL DEFAULT '' CHECK (length(comment) <= 2000),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, confirmation_id),
    FOREIGN KEY (organization_id, decision_version_id) REFERENCES decision.decision_version (organization_id, id)
);
CREATE INDEX decision_transition_history_idx ON decision.decision_transition
    (organization_id, decision_version_id, occurred_at, known_seq);

CREATE TABLE change.exception_projection (
    organization_id uuid NOT NULL,
    area_id uuid NOT NULL,
    as_of timestamptz NOT NULL,
    known_seq bigint NOT NULL,
    active_version_ids uuid[] NOT NULL,
    PRIMARY KEY (organization_id, area_id)
);

-- +goose StatementBegin
DO $body$
DECLARE rel text;
BEGIN
    FOREACH rel IN ARRAY ARRAY['decision.decision', 'decision.decision_version', 'decision.decision_transition', 'change.exception_projection']
    LOOP
        EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', rel);
        EXECUTE format('CREATE POLICY org_isolation ON %s TO craftpulse_app USING
            (organization_id = NULLIF(current_setting(''craftpulse.org_id'', true), '''')::uuid)
            WITH CHECK (organization_id = NULLIF(current_setting(''craftpulse.org_id'', true), '''')::uuid)', rel);
        EXECUTE format('CREATE POLICY backup_read ON %s FOR SELECT TO craftpulse_backup USING (true)', rel);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE ON %s TO craftpulse_app', rel);
        EXECUTE format('GRANT SELECT ON %s TO craftpulse_backup', rel);
    END LOOP;
END
$body$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE change.exception_projection;
DROP TABLE decision.decision_transition, decision.decision_version, decision.decision;
DROP SCHEMA decision;
