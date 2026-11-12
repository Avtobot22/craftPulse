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
	orgrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/org"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/access"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

func newAPI(pool *pgxpool.Pool, keyring *crypto.Keyring, idp repo.IdentityProvider, c clock.Clock, siteURL string, reg prometheus.Registerer) v1.Deps {
	tx := persistent.NewTransactor(pool)
	orgs := org.New(orgrepo.NewRepo(pool), platform.NewRepo(pool), c)
	auditor := audit.New(auditrepo.NewRepo(pool), tx, c)
	logins := auth.New(auth.Deps{
		Repo:     authrepo.NewRepo(pool),
		Tx:       tx,
		Provider: idp,
		Keyring:  keyring,
		Users:    orgs,
		Auditor:  auditor,
		Clock:    c,
		SiteURL:  siteURL,
	})
	decisions := access.New(tx, orgs, auditor, c)
	return v1.Deps{
		Handler:    handler.New(handler.Deps{Logins: logins, Access: decisions, Registerer: reg}),
		Logins:     logins,
		Access:     decisions,
		SiteOrigin: siteURL,
	}
}
