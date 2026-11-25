-- +goose Up
-- Before 00055, each revision could enqueue a job without a River uniqueness key.
-- Keep one live job per MR. It reads the latest revision regardless of its arguments.
-- On a fresh database the application migrations run before River creates its tables.
-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('public.river_job') IS NOT NULL THEN
        LOCK TABLE public.river_job IN SHARE ROW EXCLUSIVE MODE;
        WITH ranked AS (
            SELECT id, row_number() OVER (
                PARTITION BY args->>'organization_id', args->>'mr_id'
                ORDER BY (state = 'running') DESC, (unique_key IS NOT NULL) DESC, id DESC
            ) AS ordinal
            FROM public.river_job
            WHERE kind = 'outbound.publish'
                AND state IN ('available', 'pending', 'running', 'retryable', 'scheduled')
        )
        UPDATE public.river_job AS job
        SET state = 'cancelled', finalized_at = now()
        FROM ranked
        WHERE job.id = ranked.id AND ranked.ordinal > 1;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
