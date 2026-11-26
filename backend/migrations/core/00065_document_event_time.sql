-- +goose Up
ALTER TABLE normalize.docs_report_document
    ADD COLUMN event_time timestamptz,
    ADD COLUMN event_offset_s integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE normalize.docs_report_document DROP COLUMN event_offset_s, DROP COLUMN event_time;
