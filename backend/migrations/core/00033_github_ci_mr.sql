-- +goose Up
-- A pull request of GitHub goes back to draft; the transition is an event like ready.
ALTER TABLE normalize.mr_event DROP CONSTRAINT mr_event_event_kind_check;
ALTER TABLE normalize.mr_event ADD CONSTRAINT mr_event_event_kind_check CHECK (event_kind IN ('ready', 'draft', 'reviewer_assigned',
    'reviewer_removed', 'review', 'comment', 'approval', 'head_pushed', 'merged', 'closed', 'reopened'));

-- Only GitLab CI tells an allowed failure of a job; a run of another CI has no such semantics and I05 leaves it out.
ALTER TABLE normalize.ci_run_version ADD COLUMN allow_failure_semantics boolean NOT NULL DEFAULT true;
UPDATE normalize.ci_run_version v SET allow_failure_semantics = false
FROM normalize.source_object o JOIN connect.source_instance i ON i.organization_id = o.organization_id AND i.id = o.instance_id
WHERE o.organization_id = v.organization_id AND o.id = v.run_id AND i.provider <> 'gitlab';
ALTER TABLE normalize.ci_run_version ALTER COLUMN allow_failure_semantics DROP DEFAULT;

-- +goose Down
ALTER TABLE normalize.ci_run_version DROP COLUMN allow_failure_semantics;
DELETE FROM normalize.mr_event WHERE event_kind = 'draft';
ALTER TABLE normalize.mr_event DROP CONSTRAINT mr_event_event_kind_check;
ALTER TABLE normalize.mr_event ADD CONSTRAINT mr_event_event_kind_check CHECK (event_kind IN ('ready', 'reviewer_assigned',
    'reviewer_removed', 'review', 'comment', 'approval', 'head_pushed', 'merged', 'closed', 'reopened'));
