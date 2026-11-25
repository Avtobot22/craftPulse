-- +goose Up
ALTER TABLE normalize.mr_event DROP CONSTRAINT mr_event_review_state_check;
ALTER TABLE normalize.mr_event ADD CONSTRAINT mr_event_review_state_check
    CHECK (review_state IN ('approved', 'changes_requested', 'commented', 'dismissed'));

-- +goose Down
UPDATE normalize.mr_event SET review_state = NULL WHERE review_state = 'dismissed';
ALTER TABLE normalize.mr_event DROP CONSTRAINT mr_event_review_state_check;
ALTER TABLE normalize.mr_event ADD CONSTRAINT mr_event_review_state_check
    CHECK (review_state IN ('approved', 'changes_requested', 'commented'));
