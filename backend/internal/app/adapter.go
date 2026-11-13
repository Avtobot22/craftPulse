package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// _adapterShutdownTimeout lets a call that waits up to 30 s for its source finish after SIGTERM.
const _adapterShutdownTimeout = 35 * time.Second

// NewSource builds the source of an adapter around the only client it may use to reach it.
type NewSource func(client *adaptersdk.SourceClient) (adaptersdk.Source, error)

// AdapterMain is the main of an adapter binary: `migrate` applies the adapter schema and rewraps data
// keys under the owner role, anything else serves the contract.
func AdapterMain(service string, newSource NewSource) {
	boot := logger.Default(service)
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	environ := env.ToMap(os.Environ())
	cfg, err := config.LoadAdapter(environ)
	var srv config.AdapterServer
	if command != "migrate" {
		var srvErr error
		srv, srvErr = config.LoadAdapterServer(environ)
		err = errors.Join(err, srvErr)
	}
	if err != nil {
		boot.Fatal().Err(err).Msg("config")
	}
	log, err := logger.New(os.Stdout, cfg.LogLevel, service, append(cfg.Secrets(), srv.Secrets()...))
	if err != nil {
		boot.Fatal().Err(err).Msg("logger")
	}
	if command == "migrate" {
		err = RunAdapterMigrate(cfg, log)
	} else {
		err = RunAdapter(cfg, srv, newSource, log)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("adapter stopped with error")
	}
}

func RunAdapter(cfg config.Adapter, srv config.AdapterServer, newSource NewSource, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serveAdapter(ctx, cfg, srv, newSource, log, _adapterShutdownTimeout)
}

func serveAdapter(ctx context.Context, cfg config.Adapter, srv config.AdapterServer, newSource NewSource, log zerolog.Logger, shutdown time.Duration) error {
	keys, err := cfg.MasterKeys.Decode()
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	keyring, err := newKeyring(keys)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	serverTLS, err := adapterTLS(srv)
	if err != nil {
		return err
	}
	roots, err := sourceRoots(srv.SourceCAFile)
	if err != nil {
		return err
	}
	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(), postgres.MaxConns(cfg.DB.MaxConns))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		Policy:  netguard.Policy{Installation: srv.InstallationNetworks, Private: srv.SourcePrivateNetworks},
		RootCAs: roots,
	})
	source, err := newSource(client)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	store := adaptersdk.NewStore(pool, keyring)
	probes := adapterProbes(pool, store)

	gin.SetMode(gin.ReleaseMode)
	handler, err := adaptersdk.NewHandler(adaptersdk.Options{ //nolint:contextcheck // handlers use the context of each call
		Source:      source,
		Store:       store,
		ServiceKeys: []string{srv.ServiceKey.Value(), srv.PreviousServiceKey.Value()},
		Log:         log,
		Probes:      probes,
		SpoolDir:    srv.SpoolDir,
	})
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}

	log.Info().Str("commit_sha", health.CommitSHA()).Str("contract_version", adaptersdk.CurrentContractVersion).Msg("adapter started")
	return serveEndpoints(ctx, log, probes.Ready, shutdown, []endpoint{{addr: srv.HTTPAddr, handler: handler, tls: serverTLS}})
}

// adapterProbes: ready while its own database has the schema and the master key is loaded (TZ 14.6).
func adapterProbes(pool *pgxpool.Pool, store *adaptersdk.Store) health.Probes {
	ready := &health.Readiness{}
	ready.Register(func(ctx context.Context) error { return adaptersdk.SchemaReady(ctx, pool) })
	ready.Register(store.Ready)
	return health.Probes{
		Ready:         ready,
		SchemaVersion: func(ctx context.Context) (int64, error) { return migrations.Current(ctx, pool) },
	}
}

func adapterTLS(srv config.AdapterServer) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(srv.TLSCertFile, srv.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("app: load adapter certificate: %w", err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}, nil
}

// sourceRoots trusts the system authorities and, if given, the authority named at installation.
func sourceRoots(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("app: read source CA: %w", err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("app: source CA file has no PEM certificate")
	}
	return pool, nil
}

func RunAdapterMigrate(cfg config.Adapter, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	keys, err := cfg.MasterKeys.Decode()
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	keyring, err := newKeyring(keys)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(),
		postgres.MaxConns(3), postgres.RuntimeParam("lock_timeout", "3s"))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	applied, rewrapped, err := migrateAdapter(ctx, pool, adaptersdk.NewStore(pool, keyring))
	if err != nil {
		return err
	}
	version, err := migrations.Current(ctx, pool)
	if err != nil {
		return err
	}
	log.Info().Int("applied", applied).Int("rewrapped_keys", rewrapped).Int64("schema_version", version).Msg("migrations done")
	return nil
}

func migrateAdapter(ctx context.Context, pool *pgxpool.Pool, store *adaptersdk.Store) (applied, rewrapped int, err error) {
	if applied, err = adaptersdk.Migrate(ctx, pool); err != nil {
		return 0, 0, fmt.Errorf("app: %w", err)
	}
	if rewrapped, err = store.RewrapKeys(ctx); err != nil {
		return applied, 0, fmt.Errorf("app: %w", err)
	}
	return applied, rewrapped, nil
}
