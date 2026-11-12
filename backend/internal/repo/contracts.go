package repo

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type JobQueue interface {
	Enqueue(ctx context.Context, job entity.Job, queue string) error
}

// Transactor opens transactions; repositories take the transaction from ctx (TZ 10.3, rule 2).
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
	InOrgTx(ctx context.Context, orgID entity.ID, fn func(ctx context.Context) error) error
}

type TenantRepo interface {
	CreateTenant(ctx context.Context, orgID entity.ID, at time.Time) error
}

type OrgRepo interface {
	CreateOrganization(ctx context.Context, org entity.NewOrganization, at time.Time) error
}

type AuthRepo interface {
	OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error)
	CreateLoginRoute(ctx context.Context, slug string, orgID entity.ID) error
	CreateProviderRegistration(ctx context.Context, reg entity.ProviderRegistration, at time.Time) error
}

type AuditRepo interface {
	InsertEvent(ctx context.Context, e entity.AuditEvent) error
}

type InstallRepo interface {
	CreateProtocol(ctx context.Context, startedAt time.Time, releaseSHA string, manifest []byte) (entity.ID, error)
	AddCheckResult(ctx context.Context, protocolID entity.ID, r entity.CheckResult) error
	FinishProtocol(ctx context.Context, protocolID entity.ID, finishedAt time.Time, outcome entity.ProtocolOutcome) error
	DatabaseFacts(ctx context.Context) (entity.DatabaseFacts, error)
}

// SiteProbe and IdentityProviderProbe return *entity.CheckFailure when the component is not usable.
type SiteProbe interface {
	ProbeSite(ctx context.Context, url string) error
}

type IdentityProviderProbe interface {
	ProbeIssuer(ctx context.Context, issuer string) error
}

type ObjectStoreProbe interface {
	Check(ctx context.Context) error
}
