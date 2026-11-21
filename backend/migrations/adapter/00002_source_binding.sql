-- +goose Up
ALTER TABLE adapter.credential ADD COLUMN source_binding jsonb;
ALTER TABLE adapter.credential ADD CHECK (source_binding IS NULL OR purpose = 'collect');

-- +goose Down
ALTER TABLE adapter.credential DROP COLUMN source_binding;
