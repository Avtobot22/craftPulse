-- +goose Up
ALTER TABLE ingest.ci_result ADD COLUMN head_sha text
    CHECK (head_sha IS NULL OR (pipeline_id IS NOT NULL AND head_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'));

-- +goose Down
ALTER TABLE ingest.ci_result DROP COLUMN head_sha;
