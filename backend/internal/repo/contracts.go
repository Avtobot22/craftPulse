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

// OrgRepo reads and writes tables under row-level security: every call runs in an organization transaction.
type OrgRepo interface {
	CreateOrganization(ctx context.Context, org entity.NewOrganization, at time.Time) error
	Organization(ctx context.Context, orgID entity.ID) (entity.Organization, error)
	LockOrganization(ctx context.Context, orgID entity.ID) (entity.Organization, error)
	ActivateOrganization(ctx context.Context, orgID, userID entity.ID) error
	LockUserByIdentity(ctx context.Context, orgID entity.ID, issuer, subject string) (entity.User, bool, error)
	CreateUser(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) error
	SetUserDisplayName(ctx context.Context, orgID, userID entity.ID, name string) error
	User(ctx context.Context, orgID, userID entity.ID) (entity.User, error)
	AssignAdministrator(ctx context.Context, orgID, userID entity.ID, at time.Time) error
	ActiveRoles(ctx context.Context, orgID, userID entity.ID) ([]entity.RoleAssignment, error)
	MemberTeamIDs(ctx context.Context, orgID, personID entity.ID, at time.Time) ([]entity.ID, error)
	TeamProjectIDs(ctx context.Context, orgID entity.ID, teamIDs []entity.ID, at time.Time) ([]entity.ID, error)
	ActiveAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, whole bool, ids []entity.ID) ([]entity.Area, error)
}

// AuthRepo serves login routes, login attempts and sessions outside row-level security and provider
// registrations under it.
type AuthRepo interface {
	OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error)
	CreateLoginRoute(ctx context.Context, slug string, orgID entity.ID) error
	CreateProviderRegistration(ctx context.Context, reg entity.ProviderRegistration, at time.Time) error
	ActiveProviderRegistration(ctx context.Context, orgID entity.ID) (entity.ProviderRegistration, bool, error)
	CreateLoginAttempt(ctx context.Context, a entity.LoginAttempt) error
	TakeLoginAttempt(ctx context.Context, cookieHash []byte) (entity.LoginAttempt, bool, error)
	CreateSession(ctx context.Context, s entity.NewSession) error
	SessionByToken(ctx context.Context, tokenHash []byte) (entity.Session, bool, error)
	TouchSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time) error
	EndSessionByToken(ctx context.Context, tokenHash []byte, at time.Time, reason entity.SessionEndReason) error
	EndSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time, reason entity.SessionEndReason) (bool, error)
	LiveSessions(ctx context.Context, orgID, userID entity.ID, at, idleSince time.Time) ([]entity.ID, error)
	EndSessions(ctx context.Context, orgID entity.ID, ids []entity.ID, at time.Time, reason entity.SessionEndReason) error
}

// IdentityProvider is the OpenID Connect client; errors are *entity.LoginError with the failure class.
type IdentityProvider interface {
	AuthCodeURL(ctx context.Context, p entity.ProviderConfig, state, nonce, verifier string) (string, error)
	Exchange(ctx context.Context, p entity.ProviderConfig, code, verifier, nonce string) (entity.Identity, error)
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
