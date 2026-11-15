-- +goose Up
-- A version of the target duration of a group of tasks of a team by type and size (FR-019). The basis of a
-- historical p85 keeps its revision, value, n and window, so it outlives the revision deleted by its term.
CREATE TABLE process.target_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    team_id uuid NOT NULL,
    task_type text NOT NULL CHECK (char_length(task_type) BETWEEN 1 AND 200),
    size_label text NOT NULL CHECK (char_length(size_label) BETWEEN 1 AND 200),
    version_no integer NOT NULL CHECK (version_no > 0),
    base_version_no integer,
    duration_s bigint NOT NULL CHECK (duration_s > 0),
    basis text NOT NULL CHECK (basis IN ('manual', 'historical_p85')),
    basis_revision_id uuid,
    basis_value_s bigint CHECK (basis_value_s >= 0),
    basis_n integer CHECK (basis_n > 0),
    basis_window tstzrange CHECK (NOT isempty(basis_window) AND lower_inc(basis_window) AND NOT upper_inc(basis_window)),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    CHECK ((version_no = 1 AND base_version_no IS NULL) OR base_version_no = version_no - 1),
    CHECK ((basis = 'historical_p85') = (basis_revision_id IS NOT NULL)),
    CHECK ((basis_revision_id IS NULL) = (basis_value_s IS NULL) AND (basis_value_s IS NULL) = (basis_n IS NULL)
        AND (basis_n IS NULL) = (basis_window IS NULL)),
    -- A stale form conflicts on the number it would take; the last version of the group is in force.
    UNIQUE (organization_id, team_id, task_type, size_label, version_no),
    -- A repeated confirmation finds the version it saved (FR-007).
    UNIQUE (organization_id, confirmation_id)
);
-- The list of versions of a team newest first and the versions in force at a knowledge number.
CREATE INDEX target_version_seq_idx ON process.target_version (organization_id, team_id, known_seq DESC);

ALTER TABLE process.target_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON process.target_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON process.target_version FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON process.target_version TO craftpulse_app;
GRANT SELECT ON process.target_version TO craftpulse_backup;

-- +goose Down
DROP TABLE process.target_version;
