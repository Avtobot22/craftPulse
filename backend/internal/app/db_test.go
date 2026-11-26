package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
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

const _postgresPassword = "postgres-test-secret"

var (
	_container *postgres.PostgresContainer
	_hostPort  string
	_dbAdmin   *pgxpool.Pool
	_dbSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.list").Value.String() != "" {
		os.Exit(m.Run())
	}
	gin.SetMode(gin.TestMode)
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
		postgres.WithPassword(_postgresPassword),
		postgres.WithInitScripts("../../../deploy/postgres/init.sh"),
		testcontainers.WithFiles(files...),
		testcontainers.WithEnv(env),
		// Parallel tests hold pools of their own databases at once. A test database is thrown away, so
		// commits do not wait for the disk.
		testcontainers.WithCmdArgs("-c", "max_connections=500", "-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"),
		postgres.BasicWaitStrategies(),
	)
	if c != nil {
		defer func() { _ = testcontainers.TerminateContainer(c, testcontainers.StopTimeout(0)) }()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	_container = c
	host, err := c.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get postgres host:", err)
		return 1
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "get postgres port:", err)
		return 1
	}
	_hostPort = net.JoinHostPort(host, port.Port())
	admin, err := pg.New(ctx, testDB{name: "postgres"}.url("postgres"), _postgresPassword, pg.MaxConns(8))
	if err != nil {
		fmt.Fprintln(os.Stderr, "create postgres admin pool:", err)
		return 1
	}
	defer admin.Close()
	_dbAdmin = admin
	if err := migrateTemplate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate template:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(_seaweedContainer, testcontainers.StopTimeout(0)) }()
	return m.Run()
}

// _migratedTemplate is the template the migrations ran on once, so a test database cloned from it skips them.
const _migratedTemplate = "craftpulse_migrated"

func migrateTemplate(ctx context.Context) error {
	if err := createDB(ctx, _migratedTemplate, "craftpulse"); err != nil {
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
	if _, err := _dbAdmin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{_migratedTemplate}.Sanitize()+" ALLOW_CONNECTIONS false"); err != nil {
		return err
	}
	if _, err := _dbAdmin.Exec(ctx, `SELECT pg_terminate_backend(pid, 5000)
		FROM pg_stat_activity WHERE datname = $1`, _migratedTemplate); err != nil {
		return err
	}
	var active bool
	err = _dbAdmin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = $1)`, _migratedTemplate).Scan(&active)
	if err != nil {
		return err
	}
	if active {
		return errors.New("could not stop all sessions on template " + _migratedTemplate)
	}
	return nil
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
	require.NoError(t, createDB(t.Context(), name, template))
	_, err := _dbAdmin.Exec(t.Context(), "GRANT CONNECT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" TO craftpulse_app, craftpulse_backup")
	require.NoError(t, err)
	return testDB{name: name}
}

func createDB(ctx context.Context, name, template string) error {
	_, err := _dbAdmin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s OWNER craftpulse_owner",
		pgx.Identifier{name}.Sanitize(), pgx.Identifier{template}.Sanitize()))
	return err
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
