-- +goose Up
CREATE SCHEMA calc;
GRANT USAGE ON SCHEMA calc TO craftpulse_app, craftpulse_backup;

-- A definition of a metric of the catalog, the same for the whole installation (TZ 12.5.8): rows come with the
-- release migration and the application does not change them.
CREATE TABLE calc.definition (
    metric_id text COLLATE "C" NOT NULL CHECK (octet_length(metric_id) BETWEEN 1 AND 255),
    definition_version integer NOT NULL CHECK (definition_version > 0),
    unit text NOT NULL CHECK (unit IN ('count', 'seconds', 'ratio')),
    catalog_ref text NOT NULL CHECK (octet_length(catalog_ref) BETWEEN 1 AND 255),
    parameters jsonb,
    PRIMARY KEY (metric_id, definition_version)
);

INSERT INTO calc.definition (metric_id, definition_version, unit, catalog_ref, parameters) VALUES
    ('T04', 1, 'count', 'docs/product/metrics/tasks.md#t04', NULL),
    ('T05', 1, 'count', 'docs/product/metrics/tasks.md#t05', '{"cohort": "first_accepted_completion"}'),
    ('T07', 1, 'count', 'docs/product/metrics/tasks.md#t07', NULL),
    ('T08', 1, 'seconds', 'docs/product/metrics/tasks.md#t08',
        '{"cohort": "first_accepted_completion", "estimators": ["p50", "p85"], "percentile": "nearest_rank", "low_sample_below": 20}'),
    ('T12', 1, 'count', 'docs/product/metrics/tasks.md#t12', NULL),
    ('T13', 1, 'ratio', 'docs/product/metrics/tasks.md#t13', NULL),
    ('T14', 1, 'seconds', 'docs/product/metrics/tasks.md#t14',
        '{"estimators": ["p50", "p85"], "percentile": "nearest_rank", "low_sample_below": 20}'),
    ('T15', 1, 'ratio', 'docs/product/metrics/tasks.md#t15', NULL),
    ('T18', 1, 'count', 'docs/product/metrics/tasks.md#t18', NULL);

GRANT SELECT ON calc.definition TO craftpulse_app, craftpulse_backup;

-- +goose Down
DROP SCHEMA calc CASCADE;
