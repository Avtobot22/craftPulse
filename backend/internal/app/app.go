package app

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/Avtobot22/craftPulse/backend/config"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const _shutdownTimeout = 10 * time.Second

func RunAPI(cfg config.Config, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	gin.SetMode(gin.ReleaseMode)
	reg := newRegistry()
	ready := &health.Readiness{}
	router := httpctrl.NewRouter(log, reg, ready, handler.New())

	log.Info().Str("commit_sha", health.CommitSHA()).Msg("api started")
	return serve(ctx, log, ready, map[string]http.Handler{
		cfg.HTTPAddr:    router,
		cfg.MetricsAddr: promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}),
	})
}

func RunWorker(cfg config.Config, log zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := newRegistry()
	ready := &health.Readiness{}

	log.Info().Str("commit_sha", health.CommitSHA()).Msg("worker started")
	return serve(ctx, log, ready, map[string]http.Handler{
		cfg.HTTPAddr:    health.Handler(ready),
		cfg.MetricsAddr: promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}),
	})
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
	for addr := range handlers {
		ln, err := net.Listen("tcp", addr)
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
