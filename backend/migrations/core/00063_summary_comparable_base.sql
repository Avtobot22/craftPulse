-- +goose Up
-- A known source SHA is not evidence of a comparable base. Use only facts selected
-- when the revision was built, including the stored diff size and exact report inputs.
UPDATE change.summary_revision AS s
SET base_sha = NULL
WHERE s.base_sha IS NOT NULL AND NOT (
    EXISTS (SELECT 1 FROM change.summary_item i
        WHERE (i.organization_id, i.summary_revision_id) = (s.organization_id, s.id)
          AND i.item_kind = 'coverage' AND i.value #>> '{Size,State}' IN ('ASSESSED', 'PROVISIONAL'))
    OR EXISTS (
       WITH reports AS (
           SELECT r.* FROM change.summary_input i
           JOIN normalize.report r ON (r.organization_id, r.id) = (i.organization_id, i.input_id)
           WHERE (i.organization_id, i.summary_revision_id) = (s.organization_id, s.id)
             AND i.input_kind = 'report' AND r.sha IN (s.base_sha, s.head_sha)
             AND r.report_kind NOT IN ('junit', 'coverage')
             AND r.completeness = 'complete' AND r.exit_class IN ('success', 'findings')
             AND r.producer IS NOT NULL AND r.rules_hash IS NOT NULL
             AND r.config_hash IS NOT NULL AND r.scope_hash IS NOT NULL
             AND (r.report_kind NOT IN ('sca', 'image') OR r.db_version IS NOT NULL)
             AND NOT EXISTS (SELECT 1 FROM normalize.ci_attempt_version a
                 WHERE (a.organization_id, a.attempt_id) = (r.organization_id, r.attempt_id)
                   AND a.known_seq <= s.knowledge_seq
                   AND (a.superseded_seq IS NULL OR a.superseded_seq > s.knowledge_seq)
                   AND a.outcome = 'canceled')
       )
       SELECT 1 FROM reports b JOIN reports h ON b.area_id = h.area_id
         AND b.component_id = h.component_id AND b.report_kind = h.report_kind
         AND b.report_format = h.report_format AND b.language IS NOT DISTINCT FROM h.language
         AND b.mode = h.mode AND b.producer = h.producer AND b.producer_version = h.producer_version
         AND b.profile_id IS NOT DISTINCT FROM h.profile_id AND b.rules_hash = h.rules_hash
         AND b.config_hash = h.config_hash AND b.scope_hash = h.scope_hash
         AND b.db_version IS NOT DISTINCT FROM h.db_version
       WHERE b.sha = s.base_sha AND h.sha = s.head_sha
    )
);

-- +goose Down
-- The corrected projection is compatible with earlier versions and needs no reversal.
SELECT 1;
