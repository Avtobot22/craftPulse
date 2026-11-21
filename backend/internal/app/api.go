package app

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	v1 "github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	docsrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/docs"
	installrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/install"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/access"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/docs"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/install"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

// apiModules are the modules the api process builds before its handlers.
type apiModules struct {
	orgs               *org.UseCase
	access             *access.UseCase
	connections        usecase.Connections
	webhooks           usecase.Webhooks
	intake             usecase.Intake
	sync               usecase.SyncStatus
	operations         usecase.Operations
	reports            usecase.TeamReports
	code               usecase.CodeDetails
	process            usecase.Process
	previews           usecase.MappingPreviews
	targets            usecase.Targets
	documentationAreas docs.Areas
	links              usecase.Links
	// catalog is the catalog of check profiles of the release.
	catalog entity.CheckCatalog
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
	documentation := docs.New(docs.Deps{
		Repo: docsrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx,
		Access: decisions, References: orgs, Areas: m.documentationAreas, Auditor: auditor, Clock: c, NewID: newID,
	})
	checks := change.New(change.Deps{
		Repo: changerepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Components: orgs, Roots: documentation, Auditor: auditor,
		Catalog: m.catalog, Clock: c, NewID: newID,
	})
	return v1.Deps{
		Handler: handler.New(handler.Deps{
			SourceLinks: decisions, SiteURL: siteURL, Webhooks: m.webhooks, Documentation: documentation, Checks: checks,
			Links:        m.links,
			UploadTokens: logins, Logins: logins, Access: decisions, Structure: orgs, Users: orgs, Audit: auditor,
			Protocols: install.New(install.Deps{Repo: installrepo.NewRepo(pool)}), Registerer: reg, Connections: m.connections, Intake: m.intake,
			Sync: m.sync, Operations: m.operations, Reports: m.reports, Code: m.code, Accounts: orgs, Process: m.process, Previews: m.previews, Targets: m.targets,
		}),
		UploadTokens: logins,
		Logins:       logins,
		Access:       decisions,
		SiteOrigin:   siteURL,
	}
}
