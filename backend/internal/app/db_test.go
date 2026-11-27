package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
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

var _adapterPasswords = map[string]string{"owner": "adapter-owner-secret", "app": "adapter-app-secret"}

var (
	_container *postgres.PostgresContainer
	_hostPort  string
	_dbSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.list").Value.String() != "" {
		os.Exit(m.Run())
	}
	gin.EnableJsonDecoderDisallowUnknownFields()
	os.Exit(runWithPostgres(m))
}

func runWithPostgres(m *testing.M) int {
	ctx := context.Background()
	env := map[string]string{
		"ADAPTER_OWNER_PASSWORD_FILE": "/run/secrets/adapter_owner",
		"ADAPTER_PASSWORD_FILE":       "/run/secrets/adapter_app",
	}
	files := make([]testcontainers.ContainerFile, 0, 3+len(_passwords))
	files = append(files,
		testcontainers.ContainerFile{Reader: strings.NewReader(_adapterPasswords["owner"]), ContainerFilePath: "/run/secrets/adapter_owner", FileMode: 0o644},
		testcontainers.ContainerFile{Reader: strings.NewReader(_adapterPasswords["app"]), ContainerFilePath: "/run/secrets/adapter_app", FileMode: 0o644},
		testcontainers.ContainerFile{HostFilePath: "../../../deploy/postgres/adapter-db.sh", ContainerFilePath: "/adapter-db.sh", FileMode: 0o755},
	)
	for role, password := range _passwords {
		path := "/run/secrets/" + role
		files = append(files, testcontainers.ContainerFile{Reader: strings.NewReader(password + "\n"), ContainerFilePath: path, FileMode: 0o644})
		env[strings.ToUpper(role)+"_PASSWORD_FILE"] = path
	}
	c, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithInitScripts("../../../deploy/postgres/init.sh"),
		testcontainers.WithFiles(files...),
		testcontainers.WithEnv(env),
		// Parallel tests hold pools of their own databases at once. A test database is thrown away, so
		// commits do not wait for the disk.
		testcontainers.WithCmdArgs("-c", "max_connections=500", "-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"),
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
	if err := migrateTemplate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate template:", err)
		return 1
	}
	return m.Run()
}

// _migratedTemplate is the template the migrations ran on once, so a test database cloned from it skips them.
const _migratedTemplate = "craftpulse_migrated"

func migrateTemplate(ctx context.Context) error {
	if err := psqlRun(ctx, "postgres", fmt.Sprintf("CREATE DATABASE %s TEMPLATE craftpulse OWNER craftpulse_owner", _migratedTemplate)); err != nil {
		return err
	}
	pool, err := pg.New(ctx, testDB{name: _migratedTemplate}.url("craftpulse_owner"), _passwords["craftpulse_owner"], pg.RuntimeParam("lock_timeout", "3s"))
	if err != nil {
		return err
	}
	_, err = migrate(ctx, pool)
	pool.Close()
	if err != nil {
		return err
	}
	// A database with a session cannot be a template: the closed pool must not leave one behind.
	if err := psqlRun(ctx, "postgres", fmt.Sprintf("ALTER DATABASE %s ALLOW_CONNECTIONS false", _migratedTemplate)); err != nil {
		return err
	}
	return psqlRun(ctx, "postgres", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s'", _migratedTemplate))
}

type testDB struct {
	name string
}

// newDB is a database without migrations, for the tests of migrating itself.
func newDB(t *testing.T) testDB {
	t.Helper()
	return cloneDB(t, "craftpulse")
}

// newMigratedDB is a database with every migration applied, cloned from the template migrated once.
func newMigratedDB(t *testing.T) testDB {
	t.Helper()
	return cloneDB(t, _migratedTemplate)
}

func cloneDB(t *testing.T, template string) testDB {
	t.Helper()
	name := fmt.Sprintf("craftpulse_t%d", _dbSeq.Add(1))
	psql(t, "postgres", fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s OWNER craftpulse_owner", name, template))
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

// psqlRun is psql outside a test, for TestMain.
func psqlRun(ctx context.Context, db, sql string) error {
	code, r, err := _container.Exec(ctx, []string{"psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", db, "-c", sql}, tcexec.Multiplexed())
	if err != nil {
		return err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("psql exited with %d: %s", code, out)
	}
	return nil
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
