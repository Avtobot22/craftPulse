package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Option func(*pgxpool.Config)

func MaxConns(n int32) Option {
	return func(c *pgxpool.Config) { c.MaxConns = n }
}

func RuntimeParam(name, value string) Option {
	return func(c *pgxpool.Config) { c.ConnConfig.RuntimeParams[name] = value }
}

func Tracer(t pgx.QueryTracer) Option {
	return func(c *pgxpool.Config) { c.ConnConfig.Tracer = t }
}

func New(ctx context.Context, url, password string, opts ...Option) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("postgres: invalid connection URL")
	}
	cfg.ConnConfig.Password = password
	for _, opt := range opts {
		opt(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	return pool, nil
}

type txKey struct{}

func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// DBTX is the query surface shared by the pool and a transaction, the same as in sqlc code.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Conn returns the transaction of ctx, or the pool when the call runs outside one.
func Conn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := TxFrom(ctx); ok {
		return tx
	}
	return pool
}

func InOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID, fn func(ctx context.Context) error) error {
	return withTransaction(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('craftpulse.org_id', $1, true)", orgID.String()); err != nil {
			return fmt.Errorf("postgres: set organization: %w", err)
		}
		return fn(ctx)
	})
}

// InTx serves tables outside row-level security (TZ 12.2), read before the organization is known.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	return withTransaction(ctx, pool, func(ctx context.Context, _ pgx.Tx) error { return fn(ctx) })
}

func withTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(context.WithValue(ctx, txKey{}, tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}
