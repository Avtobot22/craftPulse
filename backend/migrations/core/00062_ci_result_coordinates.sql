-- +goose Up
ALTER TABLE ingest.ci_result
    ADD COLUMN area_id uuid,
    ADD COLUMN mr_native_id text,
    ADD CHECK (area_id IS NULL OR pipeline_id IS NOT NULL),
    ADD CHECK (mr_native_id IS NULL OR area_id IS NOT NULL);

-- +goose Down
ALTER TABLE ingest.ci_result DROP COLUMN mr_native_id, DROP COLUMN area_id;
