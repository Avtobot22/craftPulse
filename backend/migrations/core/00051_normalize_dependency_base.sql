-- +goose Up
ALTER TABLE normalize.report ADD COLUMN dependency_base_sha text
    CHECK (dependency_base_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$');

-- +goose Down
ALTER TABLE normalize.report DROP COLUMN dependency_base_sha;
