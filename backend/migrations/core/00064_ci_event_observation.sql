-- +goose Up
ALTER TABLE normalize.ci_run_version
    ADD COLUMN observed_via text CHECK (observed_via IN ('webhook', 'incremental', 'backfill', 'reconcile', 'import')),
    ADD COLUMN start_offset_s integer NOT NULL DEFAULT 0,
    ADD COLUMN finish_offset_s integer NOT NULL DEFAULT 0;
ALTER TABLE normalize.ci_attempt_version
    ADD COLUMN observed_via text CHECK (observed_via IN ('webhook', 'incremental', 'backfill', 'reconcile', 'import')),
    ADD COLUMN start_offset_s integer NOT NULL DEFAULT 0,
    ADD COLUMN finish_offset_s integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE normalize.ci_attempt_version DROP COLUMN finish_offset_s, DROP COLUMN start_offset_s, DROP COLUMN observed_via;
ALTER TABLE normalize.ci_run_version DROP COLUMN finish_offset_s, DROP COLUMN start_offset_s, DROP COLUMN observed_via;
