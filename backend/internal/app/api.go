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
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/install"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

func newAPI(pool *pgxpool.Pool, keyring *crypto.Keyring, idp repo.IdentityProvider, c clock.Clock, siteURL string, reg prometheus.Registerer,
	connections usecase.Connections,
) v1.Deps {
	tx := persistent.NewTransactor(pool)
	sessions := authrepo.NewRepo(pool)
	auditor := audit.New(auditrepo.NewRepo(pool), tx, c)
	orgs, decisions := newOrgAccess(pool, c)
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
			Protocols: install.New(install.Deps{Repo: installrepo.NewRepo(pool)}), Registerer: reg, Connections: connections,
		}),
		UploadTokens: logins,
		Logins:       logins,
		Access:       decisions,
		SiteOrigin:   siteURL,
	}
}
