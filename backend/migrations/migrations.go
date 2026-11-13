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

var (
	//go:embed core/*.sql
	_core embed.FS
	//go:embed adapter/*.sql
	_adapter embed.FS
)

func Core() fs.FS {
	sub, _ := fs.Sub(_core, "core")
	return sub
}

// Adapter is the schema every adapter database has, from pkg/adaptersdk (TZ 12.5.9).
func Adapter() fs.FS {
	sub, _ := fs.Sub(_adapter, "adapter")
	return sub
}

func Latest() int64 { return latestIn(Core()) }

func AdapterLatest() int64 { return latestIn(Adapter()) }

func latestIn(fsys fs.FS) int64 {
	files, _ := fs.Glob(fsys, "*.sql")
	var v int64
	for _, f := range files {
		prefix, _, _ := strings.Cut(f, "_")
		if n, err := strconv.ParseInt(prefix, 10, 64); err == nil && n > v {
			v = n
		}
	}
	return v
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
