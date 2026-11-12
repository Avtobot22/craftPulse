package app

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/cli"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/objstore"
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
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _probeTimeout = 10 * time.Second

var _releaseSHA = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// RunInstall is `api install`: it creates the organizations of the parameters and writes the
// installation protocol (UC-01, FR-011). A failed check makes the command fail.
func RunInstall(cfg config.Config, in config.Install, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = logger.WithCorrelation(ctx, log)

	sha := health.CommitSHA()
	if !_releaseSHA.MatchString(sha) {
		return fmt.Errorf("app: binary has no release SHA (%q), build it with make build", sha)
	}
	keys, err := cfg.MasterKeys.Decode()
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	keyring, err := newKeyring(keys)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(), postgres.MaxConns(2))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	httpClient := &http.Client{Timeout: _probeTimeout}
	uc := newInstallation(pool, keyring, objstore.New(objstore.Config{
		Endpoint:        cfg.ObjectStore.Endpoint,
		Region:          cfg.ObjectStore.Region,
		Bucket:          cfg.ObjectStore.Bucket,
		AccessKeyID:     cfg.ObjectStore.AccessKeyID,
		SecretAccessKey: cfg.ObjectStore.SecretAccessKey.Value(),
	}), probe.New(httpClient), clock.System{})
	return cli.Install(ctx, uc, installParams(in, sha), log)
}

type installProbes interface {
	repo.SiteProbe
	repo.IdentityProviderProbe
}

func newInstallation(pool *pgxpool.Pool, keyring *crypto.Keyring, store repo.ObjectStoreProbe, probes installProbes, c clock.Clock) *install.UseCase {
	tx := persistent.NewTransactor(pool)
	return install.New(install.Deps{
		Tx:            tx,
		Repo:          installrepo.NewRepo(pool),
		Organizations: org.New(orgrepo.NewRepo(pool), platform.NewRepo(pool), c),
		Logins:        auth.New(authrepo.NewRepo(pool), keyring, c),
		Auditor:       audit.New(auditrepo.NewRepo(pool), tx, c),
		Site:          probes,
		Identity:      probes,
		ObjectStore:   store,
		Clock:         c,
		NewID:         newID,
	})
}

func newKeyring(keys []config.MasterKey) (*crypto.Keyring, error) {
	mk := make([]crypto.MasterKey, len(keys))
	for i, k := range keys {
		mk[i] = crypto.MasterKey{Version: k.Version, Key: k.Key}
	}
	return crypto.NewKeyring(mk[0], mk[1:]...)
}

func newID() (entity.ID, error) {
	id, err := uuid.NewV7()
	return entity.ID(id), err
}

func installParams(in config.Install, releaseSHA string) entity.InstallParams {
	p := entity.InstallParams{
		ReleaseSHA:    releaseSHA,
		Images:        in.Images,
		SiteURL:       in.SiteURL,
		RunnerNetwork: in.RunnerNetwork,
	}
	for _, o := range in.Organizations {
		p.Organizations = append(p.Organizations, entity.NewOrganization{
			Slug: o.Slug,
			Name: o.Name,
			Provider: entity.OIDCProvider{
				Issuer:       o.Issuer,
				ClientID:     o.ClientID,
				ClientSecret: entity.Secret(o.ClientSecret.Value()),
			},
			FirstAdminSubject: o.AdminSubject,
		})
	}
	return p
}
