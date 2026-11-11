package app

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _migrateLock = 4710200

func RunMigrate(cfg config.Config, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(),
		postgres.MaxConns(3), postgres.RuntimeParam("lock_timeout", "3s"))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	applied, err := migrate(ctx, pool)
	if err != nil {
		return err
	}
	version, err := migrations.Current(ctx, pool)
	if err != nil {
		return err
	}
	log.Info().Int("applied", applied).Int64("schema_version", version).Msg("migrations done")
	return nil
}

// One advisory lock covers goose and River, so a concurrent run waits and finds nothing to apply.
func migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("app: acquire connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", _migrateLock); err != nil {
		return 0, fmt.Errorf("app: migration lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", _migrateLock) }()

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Core())
	if err != nil {
		return 0, fmt.Errorf("app: goose: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("app: apply migrations: %w", err)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return 0, fmt.Errorf("app: river migrator: %w", err)
	}
	river, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return 0, fmt.Errorf("app: apply river migrations: %w", err)
	}
	return len(results) + len(river.Versions), nil
}
