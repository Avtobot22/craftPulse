-- +goose Up
ALTER TABLE ingest.receipt
    ADD COLUMN redaction_count bigint NOT NULL DEFAULT 0,
    ADD COLUMN redaction_rules_hash text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE ingest.receipt
    DROP COLUMN redaction_rules_hash,
    DROP COLUMN redaction_count;
