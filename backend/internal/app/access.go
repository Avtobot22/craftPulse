package app

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	orgrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/org"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/access"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newOrgAccess builds the org module and the access decisions on it, which every process checks rights with.
// sources is connect: it knows the areas of connections and calls CheckAccess. A process builds them once,
// so its cache of decisions is one; nil metrics count nothing.
func newOrgAccess(pool *pgxpool.Pool, c clock.Clock, sources access.Sources, metrics access.Metrics) (orgs *org.UseCase, rights *access.UseCase) {
	tx := persistent.NewTransactor(pool)
	auditor := audit.New(auditrepo.NewRepo(pool), tx, c)
	orgs = org.New(org.Deps{
		Orgs: orgrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Auditor: auditor,
		Credentials: auth.NewCredentials(authrepo.NewRepo(pool)), Sources: sources, Clock: c, NewID: newID,
	})
	rights = access.New(access.Deps{
		Tx: tx, Profiles: orgs, Structure: orgs, Accounts: orgs, Sources: sources, Auditor: auditor, Metrics: metrics, Clock: c,
	})
	return orgs, rights
}

type accessMetrics struct {
	lookups *prometheus.CounterVec
	failed  *prometheus.CounterVec
}

func newAccessMetrics(reg prometheus.Registerer) (accessMetrics, error) {
	m := accessMetrics{
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_access_cache_lookups_total",
			Help: "Lookups of source access decisions in the cache of the process by result: hit or miss.",
		}, []string{"result"}),
		failed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_access_check_failures_total",
			Help: "Checks of source access that gave no answer in time, by connection (NFR-011).",
		}, []string{"connection_id"}),
	}
	for _, c := range []prometheus.Collector{m.lookups, m.failed} {
		if err := reg.Register(c); err != nil {
			return accessMetrics{}, fmt.Errorf("app: register access metric: %w", err)
		}
	}
	return m, nil
}

func (m accessMetrics) CacheLookup(hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	m.lookups.WithLabelValues(result).Inc()
}

func (m accessMetrics) CheckFailed(connectionID entity.ID) {
	m.failed.WithLabelValues(connectionID.String()).Inc()
}
