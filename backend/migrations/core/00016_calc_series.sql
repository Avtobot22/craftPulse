-- +goose Up
-- A series: an area, a set of metrics and a step (TZ 12.5.8); scope is canonical JSON, scope_hash its SHA-256.
CREATE TABLE calc.series (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    scope_hash bytea NOT NULL CHECK (octet_length(scope_hash) = 32),
    scope jsonb NOT NULL,
    metric_set text COLLATE "C" NOT NULL CHECK (octet_length(metric_set) BETWEEN 1 AND 255),
    step text NOT NULL CHECK (step IN ('day', 'week', 'month')),
    current_revision_id uuid,
    consumed_seq bigint NOT NULL CHECK (consumed_seq >= 0),
    PRIMARY KEY (organization_id, id),
    -- One area always finds one series of a set and a step.
    UNIQUE (organization_id, scope_hash, metric_set, step)
);

-- A revision of a series (DR-011); it does not change after publication.
CREATE TABLE calc.revision (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    series_id uuid NOT NULL,
    revision_no integer NOT NULL CHECK (revision_no > 0),
    status text NOT NULL CHECK (status IN ('building', 'published', 'superseded', 'failed')),
    progress_at timestamptz NOT NULL,
    window_range tstzrange NOT NULL CHECK (NOT isempty(window_range) AND lower_inc(window_range) AND NOT upper_inc(window_range)),
    as_of timestamptz NOT NULL,
    knowledge_seq bigint NOT NULL CHECK (knowledge_seq >= 0),
    timezone text NOT NULL CHECK (octet_length(timezone) BETWEEN 1 AND 64),
    input_hash bytea NOT NULL CHECK (octet_length(input_hash) = 32),
    loading boolean NOT NULL,
    started_at timestamptz NOT NULL,
    published_at timestamptz,
    superseded_at timestamptz,
    irreproducible_reason text CHECK (irreproducible_reason IN ('basis_deleted', 'basis_expired', 'definition_retired')),
    previous_revision_id uuid,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, series_id) REFERENCES calc.series (organization_id, id),
    FOREIGN KEY (organization_id, previous_revision_id) REFERENCES calc.revision (organization_id, id)
        ON DELETE SET NULL (previous_revision_id),
    CHECK ((status IN ('published', 'superseded')) = (published_at IS NOT NULL)),
    CHECK ((status = 'superseded') = (superseded_at IS NOT NULL)),
    -- The history of a series and the index of its FK.
    UNIQUE (organization_id, series_id, revision_no)
);
-- One chain of calculation per series (TZ 10.6).
CREATE UNIQUE INDEX revision_building_uq ON calc.revision (organization_id, series_id) WHERE status = 'building';
CREATE INDEX revision_superseded_idx ON calc.revision (organization_id, superseded_at) WHERE status = 'superseded';
CREATE INDEX revision_previous_idx ON calc.revision (organization_id, previous_revision_id) WHERE previous_revision_id IS NOT NULL;

ALTER TABLE calc.series ADD FOREIGN KEY (organization_id, current_revision_id) REFERENCES calc.revision (organization_id, id);
CREATE INDEX series_current_revision_idx ON calc.series (organization_id, current_revision_id) WHERE current_revision_id IS NOT NULL;

-- The versions of the inputs a revision was computed with.
CREATE TABLE calc.revision_input (
    organization_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    input_kind text NOT NULL CHECK (input_kind IN ('definition', 'mapping', 'calendar', 'target', 'decision', 'structure', 'accounts',
        'links', 'source_watermark')),
    input_id text COLLATE "C" NOT NULL CHECK (octet_length(input_id) BETWEEN 1 AND 255),
    version text CHECK (octet_length(version) BETWEEN 1 AND 255),
    watermark timestamptz,
    completeness text CHECK (octet_length(completeness) BETWEEN 1 AND 64),
    PRIMARY KEY (organization_id, revision_id, input_kind, input_id),
    FOREIGN KEY (organization_id, revision_id) REFERENCES calc.revision (organization_id, id) ON DELETE CASCADE
);
-- Whether stored revisions hold a version of a setting before it is deleted by its term.
CREATE INDEX revision_input_kind_idx ON calc.revision_input (organization_id, input_kind, input_id);

-- A part of a series, reused by the hash of what it read (TZ 10.6, 12.7).
CREATE TABLE calc.part (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    input_hash bytea NOT NULL CHECK (octet_length(input_hash) = 32),
    metric_set text COLLATE "C" NOT NULL CHECK (octet_length(metric_set) BETWEEN 1 AND 255),
    part_range tstzrange NOT NULL CHECK (NOT isempty(part_range) AND lower_inc(part_range) AND NOT upper_inc(part_range)),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, input_hash)
);

CREATE TABLE calc.revision_part (
    organization_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    part_no integer NOT NULL CHECK (part_no > 0),
    part_id uuid NOT NULL,
    PRIMARY KEY (organization_id, revision_id, part_no),
    FOREIGN KEY (organization_id, revision_id) REFERENCES calc.revision (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, part_id) REFERENCES calc.part (organization_id, id)
);
-- The collection of unused parts and the index of the FK.
CREATE INDEX revision_part_part_idx ON calc.revision_part (organization_id, part_id);

-- A point of a series; estimator tells the series of one metric apart, such as p50 and p85 of T08, and is
-- empty for a metric with one series.
CREATE TABLE calc.point (
    organization_id uuid NOT NULL,
    part_id uuid NOT NULL,
    metric_id text COLLATE "C" NOT NULL CHECK (octet_length(metric_id) BETWEEN 1 AND 255),
    estimator text COLLATE "C" NOT NULL CHECK (octet_length(estimator) <= 32),
    point_start timestamptz NOT NULL,
    point_end timestamptz NOT NULL,
    value numeric,
    unit text NOT NULL CHECK (unit IN ('count', 'seconds', 'ratio')),
    numerator numeric,
    denominator numeric CHECK (denominator >= 0),
    n integer CHECK (n >= 0),
    state text NOT NULL CHECK (state IN ('ASSESSED', 'PROVISIONAL', 'BOUNDED', 'UNKNOWN', 'NOT_EVALUATED', 'NOT_APPLICABLE')),
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    bound_kind text CHECK (bound_kind IN ('lower', 'upper')),
    bound_value numeric,
    excluded_count integer NOT NULL CHECK (excluded_count >= 0),
    missing_count integer NOT NULL CHECK (missing_count >= 0),
    low_sample boolean NOT NULL,
    PRIMARY KEY (organization_id, part_id, metric_id, estimator, point_start),
    FOREIGN KEY (organization_id, part_id) REFERENCES calc.part (organization_id, id) ON DELETE CASCADE,
    CHECK (point_end > point_start),
    CHECK ((value IS NOT NULL) = (state IN ('ASSESSED', 'PROVISIONAL'))),
    CHECK ((bound_kind IS NOT NULL) = (state = 'BOUNDED') AND (bound_kind IS NULL) = (bound_value IS NULL)),
    CHECK (state <> 'ASSESSED' OR denominator IS NULL OR denominator > 0)
);

-- A contribution of an object to a point: the cohort of the drill-down (FR-021).
CREATE TABLE calc.contribution (
    organization_id uuid NOT NULL,
    part_id uuid NOT NULL,
    metric_id text COLLATE "C" NOT NULL,
    estimator text COLLATE "C" NOT NULL,
    point_start timestamptz NOT NULL,
    object_id uuid NOT NULL,
    item_no smallint NOT NULL CHECK (item_no > 0),
    object_kind text NOT NULL CHECK (object_kind IN ('task')),
    role text NOT NULL CHECK (role IN ('numerator', 'denominator', 'excluded', 'missing')),
    value numeric,
    reason_code text CHECK (octet_length(reason_code) BETWEEN 1 AND 64),
    PRIMARY KEY (organization_id, part_id, metric_id, estimator, point_start, object_id, item_no),
    FOREIGN KEY (organization_id, part_id, metric_id, estimator, point_start)
        REFERENCES calc.point (organization_id, part_id, metric_id, estimator, point_start) ON DELETE CASCADE
);
-- The parts an object entered, to mark revisions when it is deleted (FR-010).
CREATE INDEX contribution_object_idx ON calc.contribution (organization_id, object_id);

-- The workflow intervals of the tasks of a part (DR-007).
CREATE TABLE calc.interval (
    organization_id uuid NOT NULL,
    part_id uuid NOT NULL,
    task_id uuid NOT NULL,
    episode_no integer NOT NULL CHECK (episode_no > 0),
    interval_no integer NOT NULL CHECK (interval_no > 0),
    class text NOT NULL CHECK (class IN ('active', 'wait', 'blocked')),
    during tstzrange NOT NULL CHECK (NOT isempty(during) AND lower_inc(during) AND NOT upper_inc(during)),
    source_event_ids uuid[] NOT NULL,
    PRIMARY KEY (organization_id, part_id, task_id, episode_no, interval_no),
    FOREIGN KEY (organization_id, part_id) REFERENCES calc.part (organization_id, id) ON DELETE CASCADE
);

-- The attributes a report cuts a revision by: the project, the type and the size at start of a task of a part.
CREATE TABLE calc.task_slice (
    organization_id uuid NOT NULL,
    part_id uuid NOT NULL,
    task_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_type text CHECK (char_length(task_type) BETWEEN 1 AND 200),
    size_label text CHECK (char_length(size_label) BETWEEN 1 AND 200),
    PRIMARY KEY (organization_id, part_id, task_id),
    FOREIGN KEY (organization_id, part_id) REFERENCES calc.part (organization_id, id) ON DELETE CASCADE
);

-- Who holds a revision from deletion.
CREATE TABLE calc.revision_pin (
    organization_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    holder_kind text NOT NULL CHECK (holder_kind IN ('saved_view', 'export', 'mr_summary')),
    holder_id uuid NOT NULL,
    PRIMARY KEY (organization_id, revision_id, holder_kind, holder_id),
    FOREIGN KEY (organization_id, revision_id) REFERENCES calc.revision (organization_id, id)
);

ALTER TABLE calc.series ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.series TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.series FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.revision ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.revision TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.revision FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.revision_input ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.revision_input TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.revision_input FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.part ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.part TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.part FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.revision_part ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.revision_part TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.revision_part FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.point ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.point TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.point FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.contribution ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.contribution TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.contribution FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.interval ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.interval TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.interval FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.task_slice ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.task_slice TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.task_slice FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE calc.revision_pin ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON calc.revision_pin TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON calc.revision_pin FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON calc.series, calc.revision, calc.revision_input, calc.part, calc.revision_part,
    calc.point, calc.contribution, calc.interval, calc.task_slice, calc.revision_pin TO craftpulse_app;

GRANT SELECT ON calc.series, calc.revision, calc.revision_input, calc.part, calc.revision_part, calc.point, calc.contribution,
    calc.interval, calc.task_slice, calc.revision_pin TO craftpulse_backup;

-- +goose Down
DROP TABLE calc.revision_pin, calc.task_slice, calc.interval, calc.contribution, calc.point, calc.revision_part, calc.part,
    calc.revision_input;
ALTER TABLE calc.series DROP COLUMN current_revision_id;
DROP TABLE calc.revision, calc.series;
