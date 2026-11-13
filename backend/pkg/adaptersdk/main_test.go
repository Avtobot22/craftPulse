package adaptersdk_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_ownerPassword = "adapter-owner-secret"
	_appPassword   = "adapter-app-secret"
)

var (
	_container *postgres.PostgresContainer
	_hostPort  string
	_dbSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.ReleaseMode)
	os.Exit(runWithPostgres(m))
}

func runWithPostgres(m *testing.M) int {
	ctx := context.Background()
	core := map[string]string{"craftpulse_owner": "owner", "craftpulse_app": "app", "craftpulse_backup": "backup"}
	env := map[string]string{
		"ADAPTER_OWNER_PASSWORD_FILE": "/run/secrets/adapter_owner",
		"ADAPTER_PASSWORD_FILE":       "/run/secrets/adapter_app",
	}
	files := make([]testcontainers.ContainerFile, 0, 3+len(core))
	files = append(files,
		testcontainers.ContainerFile{Reader: strings.NewReader(_ownerPassword + "\n"), ContainerFilePath: "/run/secrets/adapter_owner", FileMode: 0o644},
		testcontainers.ContainerFile{Reader: strings.NewReader(_appPassword + "\n"), ContainerFilePath: "/run/secrets/adapter_app", FileMode: 0o644},
		testcontainers.ContainerFile{HostFilePath: "../../../deploy/postgres/adapter-db.sh", ContainerFilePath: "/adapter-db.sh", FileMode: 0o755},
	)
	for role, password := range core {
		path := "/run/secrets/" + role
		files = append(files, testcontainers.ContainerFile{Reader: strings.NewReader(password), ContainerFilePath: path, FileMode: 0o644})
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

// adapterDB is a database made by deploy/postgres/adapter-db.sh, with roles of its own.
type adapterDB struct {
	name string
}

func newAdapterDB(t *testing.T) adapterDB {
	t.Helper()
	name := fmt.Sprintf("sdk%d", _dbSeq.Add(1))
	code, out := exec(t, []string{"sh", "/adapter-db.sh", name})
	require.Zero(t, code, out)
	d := adapterDB{name: name}
	_, err := adaptersdk.Migrate(t.Context(), d.pool(t, d.owner()))
	require.NoError(t, err)
	return d
}

func (d adapterDB) owner() string { return "adapter_" + d.name + "_owner" }

func (d adapterDB) app() string { return "adapter_" + d.name }

func (d adapterDB) pool(t *testing.T, role string) *pgxpool.Pool {
	t.Helper()
	password := _appPassword
	if role == d.owner() {
		password = _ownerPassword
	}
	pool, err := pg.New(t.Context(), fmt.Sprintf("postgres://%s@%s/adapter_%s?sslmode=disable", role, _hostPort, d.name), password)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// dump is pg_dump of the whole database, the copy an operator would take.
func (d adapterDB) dump(t *testing.T) string {
	t.Helper()
	code, out := exec(t, []string{"pg_dump", "-U", "postgres", "-d", "adapter_" + d.name})
	require.Zero(t, code, out)
	return out
}

func exec(t *testing.T, cmd []string) (exitCode int, output string) {
	t.Helper()
	code, r, err := _container.Exec(t.Context(), cmd, tcexec.Multiplexed())
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return code, string(out)
}

func masterKey(t *testing.T, version uint32) crypto.MasterKey {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return crypto.MasterKey{Version: version, Key: key}
}

func keyring(t *testing.T, current crypto.MasterKey, previous ...crypto.MasterKey) *crypto.Keyring {
	t.Helper()
	k, err := crypto.NewKeyring(current, previous...)
	require.NoError(t, err)
	return k
}
