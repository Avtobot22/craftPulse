package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func TestMigrate_AppliesOnce(t *testing.T) {
	db := newDB(t)
	owner := db.pool(t, "craftpulse_owner", pg.RuntimeParam("lock_timeout", "3s"))

	applied := make([]int, 2)
	var wg sync.WaitGroup
	for i := range applied {
		process := db.pool(t, "craftpulse_owner", pg.RuntimeParam("lock_timeout", "3s"))
		wg.Go(func() {
			n, err := migrate(t.Context(), process)
			assert.NoError(t, err)
			applied[i] = n
		})
	}
	wg.Wait()
	again, err := migrate(t.Context(), owner)
	require.NoError(t, err)

	assert.Zero(t, min(applied[0], applied[1]), "the concurrent run waits and finds nothing to apply")
	assert.Positive(t, max(applied[0], applied[1]))
	assert.Zero(t, again)
	assert.Equal(t, int(migrations.Latest()), count(t, t.Context(), owner,
		"SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0"))
	v, err := migrations.Current(t.Context(), owner)
	require.NoError(t, err)
	assert.Equal(t, migrations.Latest(), v)
}

func TestRowLevelSecurity(t *testing.T) {
	db := newDB(t)
	db.migrate(t)
	app := db.pool(t, "craftpulse_app")
	ctx := t.Context()
	orgA, orgB := uuid.New(), uuid.New()
	insert := "INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0)"

	for _, org := range []uuid.UUID{orgA, orgB} {
		require.NoError(t, pg.InOrgTx(ctx, app, org, func(ctx context.Context) error {
			tx, _ := pg.TxFrom(ctx)
			return platform.New(tx).CreateOrgClock(ctx, org)
		}))
	}

	assert.Zero(t, count(t, ctx, app, "SELECT count(*) FROM platform.org_clock"), "no organization set")
	err := pg.InOrgTx(ctx, app, orgA, func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		assert.Equal(t, 1, count(t, ctx, tx, "SELECT count(*) FROM platform.org_clock"))
		tag, err := tx.Exec(ctx, "UPDATE platform.org_clock SET last_seq = 5 WHERE organization_id = $1", orgB)
		require.NoError(t, err)
		assert.Zero(t, tag.RowsAffected(), "rows of B are not visible to A")
		_, err = tx.Exec(ctx, insert, uuid.New())
		return err
	})
	require.Error(t, err, "A cannot write a row of another organization")

	for _, stmt := range []string{"TRUNCATE platform.org_clock", "CREATE TABLE platform.x (id int)"} {
		_, err := app.Exec(ctx, stmt)
		assert.Error(t, err, stmt)
	}
}

func TestPool_DoesNotCarryOrganization(t *testing.T) {
	db := newDB(t)
	db.migrate(t)
	app := db.pool(t, "craftpulse_app", pg.MaxConns(1))
	ctx := t.Context()
	org := uuid.New()

	require.NoError(t, pg.InOrgTx(ctx, app, org, func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		_, err := tx.Exec(ctx, "INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0)", org)
		return err
	}))

	var setting string
	require.NoError(t, app.QueryRow(ctx, "SELECT current_setting('craftpulse.org_id', true)").Scan(&setting))
	assert.Empty(t, setting)
	assert.Zero(t, count(t, ctx, app, "SELECT count(*) FROM platform.org_clock"))
}

func TestBackupRole(t *testing.T) {
	db := newDB(t)
	db.migrate(t)
	app := db.pool(t, "craftpulse_app")
	ctx := t.Context()
	orgs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, org := range orgs {
		require.NoError(t, pg.InOrgTx(ctx, app, org, func(ctx context.Context) error {
			tx, _ := pg.TxFrom(ctx)
			_, err := tx.Exec(ctx, "INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0)", org)
			return err
		}))
	}

	code, out := exec(t, []string{
		"pg_dump", "--enable-row-security", "--data-only", "-h", "localhost",
		"-U", "craftpulse_backup", "-d", db.name, "-t", "platform.org_clock",
	},
		tcexec.WithEnv([]string{"PGPASSWORD=" + _passwords["craftpulse_backup"]}))
	require.Zero(t, code, out)
	for _, org := range orgs {
		assert.Contains(t, out, org.String())
	}

	backup := db.pool(t, "craftpulse_backup")
	_, err := backup.Exec(ctx, "INSERT INTO platform.tenant (organization_id, status, created_at) VALUES ($1, 'active', now())", uuid.New())
	assert.Error(t, err)
}

func TestReadiness(t *testing.T) {
	db := newDB(t)
	app := db.pool(t, "craftpulse_app")
	probes := databaseProbes(app)
	ctx := t.Context()

	assert.False(t, probes.Ready.Ready(ctx), "schema below the expected version")
	db.migrate(t)
	assert.True(t, probes.Ready.Ready(ctx))
	v, err := probes.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.Latest(), v)

	down, err := pg.New(ctx, "postgres://craftpulse_app@127.0.0.1:1/craftpulse?sslmode=disable&connect_timeout=1", "x")
	require.NoError(t, err)
	t.Cleanup(down.Close)
	assert.False(t, databaseProbes(down).Ready.Ready(ctx), "database unavailable")
}

func TestInOrgTx_RollsBackOnError(t *testing.T) {
	db := newDB(t)
	db.migrate(t)
	app := db.pool(t, "craftpulse_app")
	ctx := t.Context()
	org := uuid.New()
	errStop := errors.New("stop")

	err := pg.InOrgTx(ctx, app, org, func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		if _, err := tx.Exec(ctx, "INSERT INTO platform.org_clock (organization_id, last_seq, reproducible_from_seq) VALUES ($1, 0, 0)", org); err != nil {
			return err
		}
		return errStop
	})

	require.ErrorIs(t, err, errStop)
	require.NoError(t, pg.InOrgTx(ctx, app, org, func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		assert.Zero(t, count(t, ctx, tx, "SELECT count(*) FROM platform.org_clock"))
		return nil
	}))
}

func TestDatabaseSettings(t *testing.T) {
	db := newDB(t)
	app := db.pool(t, "craftpulse_app")
	var encoding, provider, locale, tz string
	require.NoError(t, app.QueryRow(t.Context(), `SELECT pg_encoding_to_char(encoding), datlocprovider::text, datlocale, current_setting('TimeZone')
		FROM pg_database WHERE datname = current_database()`).Scan(&encoding, &provider, &locale, &tz))
	assert.Equal(t, []string{"UTF8", "b", "C.UTF-8", "UTC"}, []string{encoding, provider, locale, tz})
	assert.Equal(t, 1, count(t, t.Context(), app, "SELECT count(*) FROM pg_extension WHERE extname = 'btree_gist'"))
}
