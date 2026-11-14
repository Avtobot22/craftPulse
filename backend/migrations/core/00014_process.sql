-- +goose Up
CREATE SCHEMA process;
GRANT USAGE ON SCHEMA process TO craftpulse_app, craftpulse_backup;

-- A version of the working calendar of a team (TZ 12.5.6); workdays is a mask with Monday in bit 0.
CREATE TABLE process.calendar_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    team_id uuid NOT NULL,
    version_no integer NOT NULL CHECK (version_no > 0),
    base_version_no integer,
    timezone text NOT NULL CHECK (octet_length(timezone) BETWEEN 1 AND 64),
    workdays smallint NOT NULL CHECK (workdays BETWEEN 0 AND 127),
    work_start time,
    work_end time,
    holidays date[] NOT NULL CHECK (cardinality(holidays) <= 400),
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    CHECK ((version_no = 1 AND base_version_no IS NULL) OR base_version_no = version_no - 1),
    CHECK ((work_start IS NULL) = (work_end IS NULL)),
    -- A stale form conflicts on the number it would take; the last version of the team.
    UNIQUE (organization_id, team_id, version_no),
    -- A repeated confirmation finds the version it saved (FR-007).
    UNIQUE (organization_id, confirmation_id)
);

-- A version of the mapping of statuses, resolutions and fields of a team (DR-004).
CREATE TABLE process.mapping_version (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT uuidv7(),
    team_id uuid NOT NULL,
    version_no integer NOT NULL CHECK (version_no > 0),
    base_version_no integer,
    content_hash bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    timezone text NOT NULL CHECK (octet_length(timezone) BETWEEN 1 AND 64),
    calendar_version_id uuid,
    known_seq bigint NOT NULL CHECK (known_seq > 0),
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    confirmation_id uuid NOT NULL,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, calendar_version_id) REFERENCES process.calendar_version (organization_id, id),
    CHECK ((version_no = 1 AND base_version_no IS NULL) OR base_version_no = version_no - 1),
    UNIQUE (organization_id, team_id, version_no),
    UNIQUE (organization_id, confirmation_id)
);
CREATE INDEX mapping_version_calendar_idx ON process.mapping_version (organization_id, calendar_version_id)
    WHERE calendar_version_id IS NOT NULL;

-- A rule of a mapping version; a value without a rule stays unknown and gives no start or end (FR-017 A3).
CREATE TABLE process.mapping_rule (
    organization_id uuid NOT NULL,
    mapping_version_id uuid NOT NULL,
    rule_kind text NOT NULL CHECK (rule_kind IN ('status', 'resolution', 'issue_type', 'code_required_type', 'size_field',
        'size_value', 'team_field', 'team_value', 'blocked_flag', 'acceptance_gate', 'epic_field', 'business_epic_value')),
    instance_id uuid NOT NULL,
    native_ref text COLLATE "C" NOT NULL CHECK (octet_length(native_ref) BETWEEN 1 AND 255),
    native_value text COLLATE "C" NOT NULL CHECK (octet_length(native_value) <= 255),
    meaning text NOT NULL CHECK (char_length(meaning) <= 200),
    rank smallint,
    PRIMARY KEY (organization_id, mapping_version_id, rule_kind, instance_id, native_ref, native_value),
    FOREIGN KEY (organization_id, mapping_version_id) REFERENCES process.mapping_version (organization_id, id) ON DELETE CASCADE,
    CHECK (rule_kind <> 'status' OR meaning IN ('intake', 'active', 'wait', 'blocked', 'done', 'cancelled', 'duplicate', 'ignored')),
    CHECK (rule_kind <> 'resolution' OR meaning IN ('accepted', 'cancelled', 'duplicate', 'ignored')),
    CHECK (rule_kind <> 'code_required_type' OR (meaning = 'code_required' AND native_value = '')),
    CHECK (rule_kind <> 'blocked_flag' OR (meaning = 'blocked' AND native_value <> '')),
    CHECK (rule_kind <> 'acceptance_gate' OR meaning = 'acceptance'),
    CHECK (rule_kind <> 'epic_field' OR meaning = 'parent_epic'),
    CHECK (rule_kind <> 'business_epic_value' OR meaning = 'business_epic'),
    CHECK (rule_kind <> 'size_value' OR rank IS NOT NULL)
);

ALTER TABLE process.calendar_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON process.calendar_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON process.calendar_version FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE process.mapping_version ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON process.mapping_version TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON process.mapping_version FOR SELECT TO craftpulse_backup USING (true);

ALTER TABLE process.mapping_rule ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation ON process.mapping_rule TO craftpulse_app
    USING (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid)
    WITH CHECK (organization_id = NULLIF(current_setting('craftpulse.org_id', true), '')::uuid);
CREATE POLICY backup_read ON process.mapping_rule FOR SELECT TO craftpulse_backup USING (true);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA process TO craftpulse_app;
GRANT SELECT ON ALL TABLES IN SCHEMA process TO craftpulse_backup;

-- +goose Down
DROP SCHEMA process CASCADE;
