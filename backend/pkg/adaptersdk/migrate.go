package adaptersdk

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Avtobot22/craftPulse/backend/migrations"
)

const _migrateLock = 4710201

// Migrate applies the adapter schema under the owner role, once per rollout (TZ 14.4); a concurrent run
// waits for the lock and finds nothing to apply.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("adaptersdk: acquire connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", _migrateLock); err != nil {
		return 0, fmt.Errorf("adaptersdk: migration lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", _migrateLock) }()

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Adapter())
	if err != nil {
		return 0, fmt.Errorf("adaptersdk: goose: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("adaptersdk: apply migrations: %w", err)
	}
	return len(results), nil
}

// SchemaReady fails until the adapter database has the schema this library needs, for /readyz.
func SchemaReady(ctx context.Context, pool *pgxpool.Pool) error {
	v, err := migrations.Current(ctx, pool)
	if err != nil {
		return err
	}
	if v < migrations.AdapterLatest() {
		return fmt.Errorf("adaptersdk: schema version %d is below %d", v, migrations.AdapterLatest())
	}
	return nil
}
