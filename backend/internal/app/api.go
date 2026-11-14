package app

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	v1 "github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	installrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/install"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/access"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/install"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

// apiModules are the modules the api process builds before its handlers.
type apiModules struct {
	orgs        *org.UseCase
	access      *access.UseCase
	connections usecase.Connections
	intake      usecase.Intake
	sync        usecase.SyncStatus
	operations  usecase.Operations
}

func newAPI(pool *pgxpool.Pool, keyring *crypto.Keyring, idp repo.IdentityProvider, c clock.Clock, siteURL string, reg prometheus.Registerer,
	m apiModules,
) v1.Deps {
	tx := persistent.NewTransactor(pool)
	sessions := authrepo.NewRepo(pool)
	auditor := audit.New(auditrepo.NewRepo(pool), tx, c)
	orgs, decisions := m.orgs, m.access
	logins := auth.New(auth.Deps{
		Tokens: sessions, Access: decisions,
		Repo:     sessions,
		Tx:       tx,
		Provider: idp,
		Keyring:  keyring,
		Users:    orgs,
		Auditor:  auditor,
		Clock:    c,
		SiteURL:  siteURL,
	})
	return v1.Deps{
		Handler: handler.New(handler.Deps{
			UploadTokens: logins, Logins: logins, Access: decisions, Structure: orgs, Users: orgs, Audit: auditor,
			Protocols: install.New(install.Deps{Repo: installrepo.NewRepo(pool)}), Registerer: reg, Connections: m.connections, Intake: m.intake,
			Sync: m.sync, Operations: m.operations, Accounts: orgs,
		}),
		UploadTokens: logins,
		Logins:       logins,
		Access:       decisions,
		SiteOrigin:   siteURL,
	}
}
