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
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	ingestrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/ingest"
	orgrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/org"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/access"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newOrgAccess builds the org module and the access decisions on it, which every process checks rights with.
func newOrgAccess(pool *pgxpool.Pool, c clock.Clock) (orgs *org.UseCase, rights *access.UseCase) {
	tx := persistent.NewTransactor(pool)
	auditor := audit.New(auditrepo.NewRepo(pool), tx, c)
	orgs = org.New(org.Deps{
		Orgs: orgrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Auditor: auditor,
		Credentials: auth.NewCredentials(authrepo.NewRepo(pool)), Clock: c, NewID: newID,
	})
	return orgs, access.New(tx, orgs, auditor, c)
}

func newObjectStore(cfg config.ObjectStore) *objstore.Store {
	return objstore.New(objstore.Config{
		Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey.Value(),
	})
}

type ingestMetrics struct {
	rejected *prometheus.CounterVec
}

func (m ingestMetrics) PackageRejected(channel entity.ReceiptChannel, reason string) {
	m.rejected.WithLabelValues(string(channel), reason).Inc()
}

func newIngest(pool *pgxpool.Pool, store repo.ObjectStore, connect ingest.Connect, rights ingest.Authorizer, c clock.Clock,
	reg prometheus.Registerer,
) (*ingest.UseCase, error) {
	rejected := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_ingest_rejected_packages_total",
		Help: "Packages refused as a whole by channel and reason (NFR-015).",
	}, []string{"channel", "reason"})
	if err := reg.Register(rejected); err != nil {
		return nil, fmt.Errorf("app: register ingest metric: %w", err)
	}
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return ingest.New(ingest.Deps{
		Repo: ingestrepo.NewRepo(pool), Objects: store, Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool),
		Connect: connect, Access: rights, Metrics: ingestMetrics{rejected: rejected}, Clock: c, NewID: newID,
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
