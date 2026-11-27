package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) SaveWikiReconcile(ctx context.Context, org, run entity.ID, seq int64, scopes []entity.WikiReconcileScope) error {
	conn := postgres.Conn(ctx, r.pool)
	for _, scope := range scopes {
		_, err := conn.Exec(ctx, `
INSERT INTO sync.wiki_reconcile_scope (organization_id, run_id, scope_id, known_seq, failed, expected, read_count)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, org, run, scope.ScopeID, seq, scope.Failed, scope.Expected, scope.Read)
		if err != nil {
			return fmt.Errorf("sync: save wiki reconciliation for scope: %w", err)
		}
	}
	return nil
}

func (r *Repo) ReadWikiCollection(ctx context.Context, org entity.ID, scopes []entity.ID, seq int64, asOf time.Time) (entity.WikiCollection, error) {
	var result entity.WikiCollection
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, _wikiCollection, org, scopes, seq, asOf).Scan(
		&result.RunID, &result.ConnectionID, &result.KnownSeq, &result.Failed, &result.Expected, &result.Read, &result.LastSuccess,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return entity.WikiCollection{}, fmt.Errorf("sync: read wiki reconciliation: %w", err)
	}
	return result, nil
}

const _wikiCollection = `
WITH eligible AS (
    SELECT w.run_id, w.expected, w.read_count, w.known_seq, w.failed, rr.finished_at, rr.connection_id
    FROM sync.wiki_reconcile_scope w
    JOIN sync.reconcile_run rr ON (rr.organization_id, rr.id) = (w.organization_id, w.run_id)
    WHERE w.organization_id = $1 AND w.scope_id = ANY($2::uuid[]) AND w.known_seq <= $3 AND rr.finished_at <= $4
)
SELECT run_id, connection_id, known_seq, failed, expected, read_count,
    (SELECT max(e.finished_at) FROM eligible e WHERE NOT e.failed AND e.expected = e.read_count) AS last_success
FROM eligible ORDER BY known_seq DESC, run_id DESC LIMIT 1
`
