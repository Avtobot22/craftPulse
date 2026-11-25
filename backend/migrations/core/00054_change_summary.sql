-- +goose Up
CREATE TABLE change.summary_revision (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    mr_id uuid NOT NULL,
    revision_no integer NOT NULL CHECK (revision_no > 0),
    head_sha text NOT NULL CHECK (head_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    base_sha text CHECK (base_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    profile_version_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('building', 'ready', 'stale', 'superseded')),
    pipeline_state text NOT NULL CHECK (pipeline_state IN ('running', 'complete', 'reports_missing')),
    input_hash bytea NOT NULL CHECK (octet_length(input_hash) = 32),
    knowledge_seq bigint NOT NULL CHECK (knowledge_seq > 0),
    built_at timestamptz,
    stale_reason text CHECK (stale_reason = 'new_head'),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, mr_id, revision_no),
    FOREIGN KEY (organization_id, profile_version_id) REFERENCES change.profile_version (organization_id, id)
);
CREATE UNIQUE INDEX summary_revision_ready_uq ON change.summary_revision (organization_id, mr_id) WHERE status = 'ready';
CREATE INDEX summary_revision_profile_idx ON change.summary_revision (organization_id, profile_version_id);
CREATE INDEX summary_revision_good_idx ON change.summary_revision (organization_id, mr_id, revision_no DESC)
    WHERE pipeline_state = 'complete' AND built_at IS NOT NULL;

CREATE TABLE change.summary_input (
    organization_id uuid NOT NULL,
    summary_revision_id uuid NOT NULL,
    input_kind text NOT NULL CHECK (input_kind IN ('report', 'calc_revision', 'decision_version', 'baseline')),
    input_id uuid NOT NULL,
    -- A calculation made for this summary has its own immutable input ID and stores the definition
    -- versions and scoped values here. Inputs from other modules carry their existing IDs.
    calculation jsonb CHECK (input_kind = 'calc_revision' OR calculation IS NULL),
    PRIMARY KEY (organization_id, summary_revision_id, input_kind, input_id),
    FOREIGN KEY (organization_id, summary_revision_id) REFERENCES change.summary_revision (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE change.summary_item (
    organization_id uuid NOT NULL,
    summary_revision_id uuid NOT NULL,
    item_no integer NOT NULL CHECK (item_no > 0),
    item_kind text NOT NULL CHECK (item_kind IN ('new', 'resolved', 'newly_observed', 'unchanged', 'accepted',
        'not_evaluated', 'ci_attempts', 'coverage', 'tests')),
    finding_identity_id uuid,
    occurrence_id uuid,
    required_right text NOT NULL CHECK (required_right IN ('none', 'security')),
    rank smallint CHECK (rank BETWEEN 1 AND 5),
    component_id uuid,
    analysis_kind text,
    reason_code text CHECK (reason_code IN ('scan_missing', 'scan_failed', 'import_failed', 'scan_incomplete')),
    severity text,
    value jsonb,
    PRIMARY KEY (organization_id, summary_revision_id, item_no),
    UNIQUE (organization_id, summary_revision_id, rank),
    FOREIGN KEY (organization_id, summary_revision_id) REFERENCES change.summary_revision (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, finding_identity_id) REFERENCES change.finding_identity (organization_id, id)
);
CREATE INDEX summary_item_finding_idx ON change.summary_item (organization_id, finding_identity_id) WHERE finding_identity_id IS NOT NULL;

ALTER TABLE change.summary_revision ENABLE ROW LEVEL SECURITY;
ALTER TABLE change.summary_input ENABLE ROW LEVEL SECURITY;
ALTER TABLE change.summary_item ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation ON change.summary_revision TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY org_isolation ON change.summary_input TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY org_isolation ON change.summary_item TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);

CREATE POLICY backup_read ON change.summary_revision FOR SELECT TO craftpulse_backup USING (true);
CREATE POLICY backup_read ON change.summary_input FOR SELECT TO craftpulse_backup USING (true);
CREATE POLICY backup_read ON change.summary_item FOR SELECT TO craftpulse_backup USING (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON change.summary_revision, change.summary_input, change.summary_item TO craftpulse_app;
GRANT SELECT ON change.summary_revision, change.summary_input, change.summary_item TO craftpulse_backup;

-- +goose Down
DROP TABLE change.summary_item, change.summary_input, change.summary_revision;
