package app

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

// newAdapterCaller trusts built-in adapters only through the internal CA, and adapters of clients
// through the system roots plus the CA given at installation.
func newAdapterCaller(cfg config.Adapters, reg prometheus.Registerer) (*adapterapi.Caller, error) {
	c := adapterapi.Config{
		Builtin:    make(map[entity.Provider]adapterapi.Builtin, len(cfg.Builtin)),
		Policy:     netguard.Policy{Installation: cfg.InstallationNetworks, Private: cfg.ClientPrivateNetworks},
		Registerer: reg,
	}
	for provider, url := range cfg.Builtin {
		c.Builtin[entity.Provider(provider)] = adapterapi.Builtin{URL: url, Key: entity.Secret(cfg.ServiceKeys[provider].Value())}
	}
	if cfg.InternalCAFile != "" {
		pool := x509.NewCertPool()
		if err := appendPEM(pool, cfg.InternalCAFile); err != nil {
			return nil, err
		}
		c.InternalCA = pool
	}
	if cfg.ClientCAFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("app: system roots: %w", err)
		}
		if err := appendPEM(pool, cfg.ClientCAFile); err != nil {
			return nil, err
		}
		c.ClientRoots = pool
	}
	return adapterapi.NewCaller(c)
}

func appendPEM(pool *x509.CertPool, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("app: read CA: %w", err)
	}
	if !pool.AppendCertsFromPEM(b) {
		return errors.New("app: " + path + " has no PEM certificate")
	}
	return nil
}

func newConnect(pool *pgxpool.Pool, keyring *crypto.Keyring, adapters repo.AdapterClient, c clock.Clock) *connect.UseCase {
	tx := persistent.NewTransactor(pool)
	return connect.New(connect.Deps{
		Repo: connectrepo.NewRepo(pool), Adapters: adapters, Tx: tx, Keyring: keyring,
		Auditor: audit.New(auditrepo.NewRepo(pool), tx, c), Clock: c, NewID: newID,
	})
}
