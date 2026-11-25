-- +goose Up
UPDATE change.summary_revision
SET publication_status = 'stale',
    publication_error_code = CASE WHEN stale_reason = 'new_head' THEN 'head_changed' END
WHERE publication_status = 'pending' AND status IN ('stale', 'superseded');

-- +goose Down
-- Superseded revisions cannot become eligible for publication again.
SELECT 1;
