package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var _passwords = map[string]string{
	"craftpulse_owner":  "owner-secret",
	"craftpulse_app":    "app-secret",
	"craftpulse_backup": "backup-secret",
}

var (
	_container *postgres.PostgresContainer
	_hostPort  string
	_dbSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	os.Exit(runWithPostgres(m))
}

func runWithPostgres(m *testing.M) int {
	ctx := context.Background()
	env := map[string]string{}
	files := make([]testcontainers.ContainerFile, 0, len(_passwords))
	for role, password := range _passwords {
		path := "/run/secrets/" + role
		files = append(files, testcontainers.ContainerFile{Reader: strings.NewReader(password + "\n"), ContainerFilePath: path, FileMode: 0o644})
		env[strings.ToUpper(role)+"_PASSWORD_FILE"] = path
	}
	c, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithInitScripts("../../../deploy/postgres/init.sh"),
		testcontainers.WithFiles(files...),
		testcontainers.WithEnv(env),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(c) }()
	_container = c
	host, err := c.Host(ctx)
	if err != nil {
		return 1
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return 1
	}
	_hostPort = host + ":" + port.Port()
	return m.Run()
}

type testDB struct {
	name string
}

func newDB(t *testing.T) testDB {
	t.Helper()
	name := fmt.Sprintf("craftpulse_t%d", _dbSeq.Add(1))
	psql(t, "postgres", fmt.Sprintf("CREATE DATABASE %s TEMPLATE craftpulse OWNER craftpulse_owner", name))
	psql(t, "postgres", fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO craftpulse_app, craftpulse_backup", name))
	return testDB{name: name}
}

func (d testDB) url(role string) string {
	return fmt.Sprintf("postgres://%s@%s/%s?sslmode=disable", role, _hostPort, d.name)
}

func (d testDB) pool(t *testing.T, role string, opts ...pg.Option) *pgxpool.Pool {
	t.Helper()
	pool, err := pg.New(t.Context(), d.url(role), _passwords[role], opts...)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func (d testDB) migrate(t *testing.T) {
	t.Helper()
	_, err := migrate(t.Context(), d.pool(t, "craftpulse_owner", pg.RuntimeParam("lock_timeout", "3s")))
	require.NoError(t, err)
}

func psql(t *testing.T, db, sql string) {
	t.Helper()
	code, out := exec(t, []string{"psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", db, "-c", sql})
	require.Zero(t, code, out)
}

func exec(t *testing.T, cmd []string, opts ...tcexec.ProcessOption) (exitCode int, output string) {
	t.Helper()
	code, r, err := _container.Exec(t.Context(), cmd, append(opts, tcexec.Multiplexed())...)
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return code, string(out)
}

func count(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string,
) int {
	t.Helper()
	var n int
	require.NoError(t, q.QueryRow(ctx, sql).Scan(&n))
	return n
}
