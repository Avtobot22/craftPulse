-- +goose Up
ALTER TABLE normalize.report ADD COLUMN dependency_delta text
    CHECK (dependency_delta IN ('computed', 'no_base', 'unavailable'));

-- +goose Down
ALTER TABLE normalize.report DROP COLUMN dependency_delta;
