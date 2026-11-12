package app

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/Avtobot22/craftPulse/backend/config"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	workerctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/worker"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _shutdownTimeout = 10 * time.Second

func RunAPI(cfg config.Config, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(), postgres.MaxConns(cfg.DB.MaxConns))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	gin.SetMode(gin.ReleaseMode)
	reg := newRegistry()
	probes := databaseProbes(pool)
	router := httpctrl.NewRouter(log, reg, probes, handler.New())

	log.Info().Str("commit_sha", health.CommitSHA()).Msg("api started")
	return serve(ctx, log, probes.Ready, map[string]http.Handler{
		cfg.HTTPAddr:    router,
		cfg.MetricsAddr: promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}),
	})
}

func RunWorker(cfg config.Config, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	wd := newWatchdog(_watchdogTimeout)
	pool, err := postgres.New(ctx, cfg.DB.URL, cfg.DB.Password.Value(),
		postgres.MaxConns(cfg.DB.MaxConns), postgres.Tracer(wd))
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	defer pool.Close()

	reg := newRegistry()
	probes := databaseProbes(pool)
	probes.Live = wd.Check

	workers := river.NewWorkers()
	kinds := workerctrl.Register(workers)

	log.Info().Str("commit_sha", health.CommitSHA()).Int("job_kinds", kinds).Msg("worker started")
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return serve(gctx, log, probes.Ready, map[string]http.Handler{
			cfg.HTTPAddr:    health.Handler(probes),
			cfg.MetricsAddr: promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}),
		})
	})
	if kinds > 0 {
		g.Go(func() error {
			return runJobs(gctx, jobsConfig{
				log:         log,
				slog:        logger.Slog(os.Stdout, slog.LevelWarn, "worker", cfg.Secrets()),
				pool:        pool,
				workers:     workers,
				ready:       probes.Ready,
				watchdog:    wd,
				stopTimeout: _jobTimeout,
			})
		})
	}
	return g.Wait()
}

func databaseProbes(pool *pgxpool.Pool) health.Probes {
	ready := &health.Readiness{}
	ready.Register(func(ctx context.Context) error {
		v, err := migrations.Current(ctx, pool)
		if err != nil {
			return err
		}
		if v < migrations.Latest() {
			return fmt.Errorf("app: schema version %d is below %d", v, migrations.Latest())
		}
		return nil
	})
	return health.Probes{
		Ready:         ready,
		SchemaVersion: func(ctx context.Context) (int64, error) { return migrations.Current(ctx, pool) },
	}
}

func newRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

func serve(ctx context.Context, log zerolog.Logger, ready *health.Readiness, handlers map[string]http.Handler) error {
	listeners := make(map[string]net.Listener, len(handlers))
	var lc net.ListenConfig
	for addr := range handlers {
		ln, err := lc.Listen(ctx, "tcp", addr)
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return fmt.Errorf("app: listen %s: %w", addr, err)
		}
		listeners[addr] = ln
	}
	context.AfterFunc(ctx, ready.Drain)

	g, gctx := errgroup.WithContext(ctx)
	for addr, h := range handlers {
		srv := &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
			ErrorLog:          stdlog.New(log, "", 0),
		}
		g.Go(func() error { return run(gctx, srv, listeners[addr]) })
		log.Info().Str("addr", listeners[addr].Addr().String()).Msg("listening")
	}
	if err := g.Wait(); err != nil {
		return err
	}
	log.Info().Msg("stopped")
	return nil
}

func run(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return fmt.Errorf("app: serve %s: %w", ln.Addr(), err)
	case <-ctx.Done():
	}

	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), _shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("app: shutdown %s: %w", ln.Addr(), errors.Join(err, srv.Close()))
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("app: serve %s: %w", ln.Addr(), err)
	}
	return nil
}
