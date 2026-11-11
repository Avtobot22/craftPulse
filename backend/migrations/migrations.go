package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed core/*.sql
var _core embed.FS

func Core() fs.FS {
	sub, _ := fs.Sub(_core, "core")
	return sub
}

func Latest() int64 {
	files, _ := fs.Glob(Core(), "*.sql")
	var latest int64
	for _, f := range files {
		prefix, _, _ := strings.Cut(f, "_")
		if v, err := strconv.ParseInt(prefix, 10, 64); err == nil && v > latest {
			latest = v
		}
	}
	return latest
}

func Current(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var v int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(max(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&v)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.UndefinedTable {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("migrations: read schema version: %w", err)
	}
	return v, nil
}
