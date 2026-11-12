package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/cli"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	installrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/install"
	orgrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/org"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/probe"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/install"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_secretMarker = "MARKER-oidc-client-secret"
	_testSHA      = "0123456789abcdef0123456789abcdef01234567"
)

var _now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

type storeProbe struct{ err error }

func (s storeProbe) Check(context.Context) error { return s.err }

type failingAuditor struct{}

func (failingAuditor) Record(context.Context, entity.AuditEvent) error {
	return errors.New("audit unavailable")
}

type installEnv struct {
	db      testDB
	app     *pgxpool.Pool
	owner   *pgxpool.Pool
	keyring *crypto.Keyring
	idp     *httptest.Server
	site    *httptest.Server
	uc      *install.UseCase
}

func newInstallEnv(t *testing.T) *installEnv {
	t.Helper()
	db := newDB(t)
	db.migrate(t)
	keyring, err := crypto.NewKeyring(crypto.MasterKey{Version: 2, Key: bytes.Repeat([]byte{7}, 32)})
	require.NoError(t, err)
	e := &installEnv{db: db, app: db.pool(t, "craftpulse_app"), owner: db.pool(t, "craftpulse_owner"), keyring: keyring}

	e.idp = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := e.idp.URL
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": issuer, "authorization_endpoint": issuer + "/oauth/authorize",
			"token_endpoint": issuer + "/oauth/token", "jwks_uri": issuer + "/oauth/discovery/keys",
		})
	}))
	t.Cleanup(e.idp.Close)
	e.site = httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(e.site.Close)
	e.uc = newInstallation(e.app, keyring, storeProbe{}, probe.New(e.idp.Client()), clock.NewManual(_now))
	return e
}

func (e *installEnv) params() entity.InstallParams {
	orgs := make([]entity.NewOrganization, 0, 2)
	for i, slug := range []string{"demo", "second-org"} {
		orgs = append(orgs, entity.NewOrganization{
			Slug: slug,
			Name: "Organization " + slug,
			Provider: entity.OIDCProvider{
				Issuer: e.idp.URL, ClientID: slug + "-client", ClientSecret: entity.Secret(_secretMarker + "-" + slug),
			},
			FirstAdminSubject: []string{"17", "42"}[i],
		})
	}
	return entity.InstallParams{
		ReleaseSHA:    _testSHA,
		Images:        map[string]string{"craftpulse-server": "sha256:aa"},
		SiteURL:       e.site.URL,
		Organizations: orgs,
		RunnerNetwork: []string{"10.20.0.0/16"},
	}
}

func (e *installEnv) count(t *testing.T, sql string) int {
	t.Helper()
	return count(t, t.Context(), e.owner, sql)
}

func TestInstall_CreatesOrganizationsOnce(t *testing.T) {
	e := newInstallEnv(t)
	ctx := t.Context()
	params := e.params()

	first, err := e.uc.Apply(ctx, params)
	require.NoError(t, err)
	params.Organizations[0].FirstAdminSubject = "999"
	second, err := e.uc.Apply(ctx, params)
	require.NoError(t, err)

	require.Len(t, first, 2)
	for i := range first {
		assert.True(t, first[i].Created)
		assert.False(t, second[i].Created, "a repeated run does not create the organization again")
		assert.Equal(t, first[i].ID, second[i].ID)
	}
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM org.organization WHERE status = 'setup' AND structure_version = 0"))
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM platform.tenant"))
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM platform.org_clock"))
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM auth.provider_registration WHERE status = 'active' AND key_version = 2"))
	assert.Equal(t, 2, e.count(t, "SELECT count(*) FROM audit.event WHERE operation = 'organization.create' AND actor_kind = 'system'"))
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM org.organization WHERE first_admin_subject = '17'"),
		"the first administrator is not changed by a repeated run")

	var orgID uuid.UUID
	require.NoError(t, e.owner.QueryRow(ctx, "SELECT organization_id FROM auth.login_route WHERE slug = 'demo'").Scan(&orgID))
	assert.Equal(t, uuid.UUID(first[0].ID), orgID)
	var blob []byte
	var version int32
	require.NoError(t, e.owner.QueryRow(ctx, "SELECT client_secret_enc, key_version FROM auth.provider_registration WHERE organization_id = $1",
		orgID).Scan(&blob, &version))
	assert.NotContains(t, string(blob), _secretMarker)
	secret, err := e.keyring.OpenEnvelope(blob, uint32(version), auth.ClientSecretAAD(entity.ID(orgID)))
	require.NoError(t, err)
	assert.Equal(t, _secretMarker+"-demo", string(secret))
	_, err = e.keyring.OpenEnvelope(blob, uint32(version), auth.ClientSecretAAD(first[1].ID))
	assert.Error(t, err, "the secret does not open under another organization")
}

func TestInstall_OrganizationTablesAreIsolated(t *testing.T) {
	e := newInstallEnv(t)
	ctx := t.Context()
	applied, err := e.uc.Apply(ctx, e.params())
	require.NoError(t, err)
	a := uuid.UUID(applied[0].ID)

	assert.Zero(t, count(t, ctx, e.app, "SELECT count(*) FROM org.organization"), "no organization set")
	require.NoError(t, pg.InOrgTx(ctx, e.app, a, func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		for _, table := range []string{"org.organization", "auth.provider_registration", "audit.event"} {
			assert.Equal(t, 1, count(t, ctx, tx, "SELECT count(*) FROM "+table), table)
		}
		tag, err := tx.Exec(ctx, "UPDATE org.organization SET name = 'x' WHERE organization_id = $1", uuid.UUID(applied[1].ID))
		require.NoError(t, err)
		assert.Zero(t, tag.RowsAffected(), "rows of B are not visible to A")
		return nil
	}))
	assert.Equal(t, 2, count(t, ctx, e.app, "SELECT count(*) FROM auth.login_route"), "login routes are found before the organization is known")
}

func TestInstall_AuditFailureRollsBackOrganization(t *testing.T) {
	e := newInstallEnv(t)
	c := clock.NewManual(_now)
	tx := persistent.NewTransactor(e.app)
	uc := install.New(install.Deps{
		Tx:            tx,
		Repo:          installrepo.NewRepo(e.app),
		Organizations: org.New(orgrepo.NewRepo(e.app), platform.NewRepo(e.app), c),
		Logins:        auth.New(authrepo.NewRepo(e.app), e.keyring, c),
		Auditor:       failingAuditor{},
		Clock:         c,
		NewID:         newID,
	})

	_, err := uc.Apply(t.Context(), e.params())

	require.Error(t, err)
	for _, table := range []string{"org.organization", "platform.tenant", "platform.org_clock", "auth.login_route", "auth.provider_registration"} {
		assert.Zero(t, e.count(t, "SELECT count(*) FROM "+table), table)
	}
}

func TestInstall_CheckWritesProtocol(t *testing.T) {
	e := newInstallEnv(t)
	ctx := logger.WithCorrelation(t.Context(), zerolog.Nop())
	var out bytes.Buffer
	log := zerolog.New(&out)

	require.NoError(t, cli.Install(ctx, e.uc, e.params(), log))

	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM install.protocol WHERE outcome = 'passed' AND finished_at IS NOT NULL AND release_sha = '"+_testSHA+"'"))
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM install.protocol WHERE manifest -> 'images' ->> 'craftpulse-server' = 'sha256:aa'"))
	outcomes := map[string]string{}
	rows, err := e.owner.Query(ctx, "SELECT check_kind, outcome FROM install.check_result")
	require.NoError(t, err)
	for rows.Next() {
		var kind, outcome string
		require.NoError(t, rows.Scan(&kind, &outcome))
		outcomes[kind] = outcome
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{
		"site": "passed", "identity_provider": "passed", "database": "passed", "object_store": "passed", "runner_network": "passed",
		"adapter": "skipped", "adapter_store": "skipped", "update_source": "skipped",
	}, outcomes)
	assert.NotContains(t, out.String(), _secretMarker)
}

func TestInstall_CheckReportsUnavailableProvider(t *testing.T) {
	e := newInstallEnv(t)
	ctx := t.Context()
	params := e.params()
	_, err := e.uc.Apply(ctx, params)
	require.NoError(t, err)
	e.idp.Close()
	var out bytes.Buffer

	err = cli.Install(ctx, e.uc, params, zerolog.New(&out))

	require.ErrorIs(t, err, cli.ErrCheckFailed)
	var outcome, reason, detail string
	require.NoError(t, e.owner.QueryRow(ctx, `SELECT outcome, reason_code, detail FROM install.check_result WHERE check_kind = 'identity_provider'`).
		Scan(&outcome, &reason, &detail))
	assert.Equal(t, "failed", outcome)
	assert.Equal(t, entity.ReasonAddress, reason)
	assert.Contains(t, detail, "demo: ")
	assert.Contains(t, detail, "second-org: ")
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM install.protocol WHERE outcome = 'failed'"))
	assert.NotContains(t, out.String(), _secretMarker)
	assert.Contains(t, out.String(), `"reason":"address"`)
}

func TestInstall_SecretOnlyEncrypted(t *testing.T) {
	e := newInstallEnv(t)
	require.NoError(t, cli.Install(t.Context(), e.uc, e.params(), zerolog.Nop()))

	code, dump := exec(t, []string{
		"pg_dump", "--enable-row-security", "--data-only", "-h", "localhost",
		"-U", "craftpulse_backup", "-d", e.db.name,
	}, tcexec.WithEnv([]string{"PGPASSWORD=" + _passwords["craftpulse_backup"]}))
	require.Zero(t, code, dump)
	assert.Contains(t, dump, "organization.create", "the dump includes the audit journal")
	assert.False(t, strings.Contains(dump, _secretMarker), "the client secret appears only encrypted")
}

func TestAudit_Rules(t *testing.T) {
	e := newInstallEnv(t)
	ctx := t.Context()
	applied, err := e.uc.Apply(ctx, e.params())
	require.NoError(t, err)
	orgID := applied[0].ID
	tx := persistent.NewTransactor(e.app)
	uc := audit.New(auditrepo.NewRepo(e.app), tx, clock.NewManual(_now))
	denied := entity.AuditEvent{
		OrganizationID: orgID, ActorKind: entity.ActorUser, ActorID: orgID, Operation: "task.read",
		Outcome: entity.AuditDenied, ReasonCode: "outside_scope", CorrelationID: orgID,
	}
	errRollback := errors.New("request failed")

	err = tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		require.NoError(t, uc.RecordSeparately(ctx, denied))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	assert.Equal(t, 1, e.count(t, "SELECT count(*) FROM audit.event WHERE outcome = 'denied'"), "a denial survives the rollback of its request")

	for _, details := range []map[string]any{{"nonce": "n"}, {"session_id": "s"}, {"note": strings.Repeat("x", 5000)}} {
		assert.Error(t, uc.RecordSeparately(ctx, entity.AuditEvent{
			OrganizationID: orgID, ActorKind: entity.ActorSystem,
			Operation: "x", Outcome: entity.AuditSucceeded, CorrelationID: orgID, Details: details,
		}))
	}

	err = pg.InOrgTx(ctx, e.app, uuid.UUID(orgID), func(ctx context.Context) error {
		tx, _ := pg.TxFrom(ctx)
		_, err := tx.Exec(ctx, "UPDATE audit.event SET outcome = 'succeeded'")
		return err
	})
	assert.Error(t, err, "the application cannot rewrite the journal")
}
