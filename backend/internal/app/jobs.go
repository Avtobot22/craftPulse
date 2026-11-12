package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_jobTimeout      = 60 * time.Second
	_watchdogTimeout = 3 * time.Minute
	_readyPoll       = 2 * time.Second
	_cancelTimeout   = 5 * time.Second
)

var errStalled = errors.New("app: job fetch loop stalled")

type jobsConfig struct {
	log         zerolog.Logger
	slog        *slog.Logger
	pool        *pgxpool.Pool
	workers     *river.Workers
	ready       *health.Readiness
	watchdog    *watchdog
	stopTimeout time.Duration
}

func runJobs(ctx context.Context, c jobsConfig) error {
	if !waitReady(ctx, c.ready) {
		return nil
	}
	client, err := river.NewClient(riverpgxv5.New(c.pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			entity.QueueInteractive: {MaxWorkers: 2},
			entity.QueueBulk:        {MaxWorkers: 2},
		},
		Workers:    c.workers,
		JobTimeout: _jobTimeout,
		Logger:     c.slog,
	})
	if err != nil {
		return fmt.Errorf("app: river client: %w", err)
	}
	c.watchdog.arm()
	if err := client.Start(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("app: start river: %w", err)
	}
	c.log.Info().Msg("taking jobs")

	stalled := make(chan error, 1)
	go func() { stalled <- c.watchdog.run(ctx) }()

	select {
	case <-ctx.Done():
	case err := <-stalled:
		if err != nil {
			cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), _cancelTimeout)
			defer cancel()
			_ = client.StopAndCancel(cancelCtx)
			return err
		}
	}
	c.watchdog.disarm()

	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.stopTimeout)
	defer cancel()
	if err := client.Stop(stopCtx); err == nil {
		return nil
	}
	c.log.Warn().Msg("jobs did not finish in time, cancelling")
	cancelCtx, cancelStop := context.WithTimeout(context.WithoutCancel(ctx), _cancelTimeout)
	defer cancelStop()
	if err := client.StopAndCancel(cancelCtx); err != nil {
		return fmt.Errorf("app: stop river: %w", err)
	}
	return nil
}

func waitReady(ctx context.Context, ready *health.Readiness) bool {
	t := time.NewTicker(_readyPoll)
	defer t.Stop()
	for !ready.Ready(ctx) {
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
		}
	}
	return true
}

// River's fetch query starts even when the database fails, so an outage is not taken for a hang.
type watchdog struct {
	timeout time.Duration
	armed   atomic.Bool
	last    atomic.Int64
}

func newWatchdog(timeout time.Duration) *watchdog {
	return &watchdog{timeout: timeout}
}

func (w *watchdog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "name: JobGetAvailable") {
		w.last.Store(time.Now().UnixNano())
	}
	return ctx
}

func (*watchdog) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (w *watchdog) arm() {
	w.last.Store(time.Now().UnixNano())
	w.armed.Store(true)
}

func (w *watchdog) disarm() { w.armed.Store(false) }

func (w *watchdog) Check(context.Context) error {
	if w.armed.Load() && time.Since(time.Unix(0, w.last.Load())) > w.timeout {
		return errStalled
	}
	return nil
}

func (w *watchdog) run(ctx context.Context) error {
	t := time.NewTicker(w.timeout / 4)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := w.Check(ctx); err != nil {
				return err
			}
		}
	}
}
