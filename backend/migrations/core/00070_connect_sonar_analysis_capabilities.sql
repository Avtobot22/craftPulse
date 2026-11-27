-- +goose Up
ALTER TABLE connect.capability
    DROP CONSTRAINT capability_capability_check,
    ADD CONSTRAINT capability_capability_check CHECK (capability IN (
        'read_current', 'read_status_history', 'read_field_history', 'read_resolution_history', 'read_sprint_history',
        'read_links', 'read_permissions', 'read_deletions', 'write_mr_comment', 'write_tracker_issue', 'webhook',
        'read_branch_analysis', 'read_mr_analysis'
    ));

-- +goose Down
-- Remove the new capability from completed probes before they can be used to activate a connection.
UPDATE connect.probe
SET capabilities = jsonb_path_query_array(
    capabilities, '$[*] ? (@.capability != "read_branch_analysis")'
)
WHERE capabilities @> '[{"capability":"read_branch_analysis"}]';
DELETE FROM connect.capability WHERE capability = 'read_branch_analysis';

ALTER TABLE connect.capability
    DROP CONSTRAINT capability_capability_check,
    ADD CONSTRAINT capability_capability_check CHECK (capability IN (
        'read_current', 'read_status_history', 'read_field_history', 'read_resolution_history', 'read_sprint_history',
        'read_links', 'read_permissions', 'read_deletions', 'write_mr_comment', 'write_tracker_issue', 'webhook', 'read_mr_analysis'
    ));
