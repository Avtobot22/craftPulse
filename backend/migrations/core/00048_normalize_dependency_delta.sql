-- +goose Up
ALTER TABLE normalize.report ADD COLUMN dependency_delta text
    CHECK (dependency_delta IN ('computed', 'no_base', 'unavailable'));

ALTER TABLE normalize.rejected_report
    ADD COLUMN report_format text,
    ADD COLUMN producer text,
    ADD COLUMN mode text,
    ADD COLUMN profile_id text,
    ADD COLUMN scope_hash bytea,
    ADD COLUMN secret_scope text,
    ADD COLUMN history_from_sha text,
    ADD COLUMN history_to_sha text;

-- +goose Down
ALTER TABLE normalize.rejected_report
    DROP COLUMN report_format,
    DROP COLUMN producer,
    DROP COLUMN mode,
    DROP COLUMN profile_id,
    DROP COLUMN scope_hash,
    DROP COLUMN secret_scope,
    DROP COLUMN history_from_sha,
    DROP COLUMN history_to_sha;

ALTER TABLE normalize.report DROP COLUMN dependency_delta;
