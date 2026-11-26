package app

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/objstore"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	ingestrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/ingest"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func newObjectStore(cfg config.ObjectStore) *objstore.Store {
	return objstore.New(objstore.Config{
		Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey.Value(),
	})
}

type ingestMetrics struct {
	rejected *prometheus.CounterVec
	resolved *prometheus.CounterVec
	ci       *prometheus.CounterVec
	redacted *prometheus.CounterVec
}

func (m ingestMetrics) PackageResolved(channel entity.ReceiptChannel, status entity.ReceiptStatus, reason string) {
	m.resolved.WithLabelValues(string(channel), string(status)).Inc()
	if status == entity.ReceiptRejected {
		m.rejected.WithLabelValues(string(channel), reason).Inc()
	}
}

func (m ingestMetrics) CIPackage(outcome, reason string) {
	m.ci.WithLabelValues(outcome, reason).Inc()
}

func (m ingestMetrics) SecretsRedacted(channel entity.ReceiptChannel, count int64) {
	if m.redacted != nil {
		m.redacted.WithLabelValues(string(channel)).Add(float64(count))
	}
}

func newIngest(pool *pgxpool.Pool, store repo.ObjectStore, connect ingest.Connect, rights ingest.Authorizer, projects ingest.Projects, c clock.Clock,
	reg prometheus.Registerer,
) (*ingest.UseCase, error) {
	rejected := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_ingest_rejected_packages_total",
		Help: "Packages refused as a whole by channel and reason (NFR-015).",
	}, []string{"channel", "reason"})
	if err := reg.Register(rejected); err != nil {
		return nil, fmt.Errorf("app: register ingest metric: %w", err)
	}
	resolved := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_ingest_resolved_packages_total",
		Help: "Packages with a terminal intake outcome by channel and status.",
	}, []string{"channel", "status"})
	for _, channel := range []entity.ReceiptChannel{entity.ChannelSync, entity.ChannelCI, entity.ChannelImport} {
		for _, status := range []entity.ReceiptStatus{entity.ReceiptAccepted, entity.ReceiptPartial, entity.ReceiptRejected, entity.ReceiptDuplicate} {
			resolved.WithLabelValues(string(channel), string(status))
		}
	}
	if err := reg.Register(resolved); err != nil {
		return nil, fmt.Errorf("app: register terminal ingest metric: %w", err)
	}
	ci := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "craftpulse_ingest_ci_packages_total", Help: "CI packages by outcome and reason."}, []string{"outcome", "reason"})
	if err := reg.Register(ci); err != nil {
		return nil, fmt.Errorf("app: register CI ingest metric: %w", err)
	}
	redacted := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "craftpulse_ingest_secret_replacements_total", Help: "Secret replacements in journaled package-processing attempts by channel."}, []string{"channel"})
	if err := reg.Register(redacted); err != nil {
		return nil, fmt.Errorf("app: register redaction metric: %w", err)
	}
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return ingest.New(ingest.Deps{
		Repo: ingestrepo.NewRepo(pool), Objects: store, Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool),
		Connect: connect, Access: rights, Projects: projects, Metrics: ingestMetrics{rejected: rejected, resolved: resolved, ci: ci, redacted: redacted}, Clock: c, NewID: newID,
		Auditor: audit.New(auditrepo.NewRepo(pool), persistent.NewTransactor(pool), c),
	}), nil
}

// riverInsertClient inserts jobs of a module; the worker process takes them with its own client.
func riverInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	jobs, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("app: river insert client: %w", err)
	}
	return jobs, nil
}
