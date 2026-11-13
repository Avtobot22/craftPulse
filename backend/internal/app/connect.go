package app

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
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
		Builtin:      make(map[entity.Provider]adapterapi.Builtin, len(cfg.Builtin)),
		Policy:       netguard.Policy{Installation: cfg.InstallationNetworks, Private: cfg.ClientPrivateNetworks},
		SourcePolicy: netguard.Policy{Installation: cfg.InstallationNetworks, Private: cfg.SourcePrivateNetworks},
		Registerer:   reg,
	}
	for provider, url := range cfg.Builtin {
		if !slices.ContainsFunc(connect.Catalog(), func(a entity.AdapterInfo) bool { return string(a.Provider) == provider }) {
			return nil, fmt.Errorf("app: built-in adapter %s has no catalog entry", provider)
		}
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

// newConnect inserts its jobs with a client of its own: api only inserts, worker also takes them.
func newConnect(pool *pgxpool.Pool, keyring *crypto.Keyring, adapters *adapterapi.Caller, c clock.Clock) (*connect.UseCase, error) {
	jobs, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("app: river insert client: %w", err)
	}
	tx := persistent.NewTransactor(pool)
	return connect.New(connect.Deps{
		Repo: connectrepo.NewRepo(pool), Adapters: adapters, Addresses: adapters, Queue: queue.New(jobs), Tx: tx, Keyring: keyring,
		Auditor: audit.New(auditrepo.NewRepo(pool), tx, c), Clock: c, NewID: newID, Builtin: adapters.Builtin,
	}), nil
}
