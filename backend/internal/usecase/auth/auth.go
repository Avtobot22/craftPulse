package auth

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Keyring is the envelope encryption of pkg/crypto (TZ 11).
type Keyring interface {
	SealEnvelope(plaintext, aad []byte) (blob []byte, masterVersion uint32, err error)
	OpenEnvelope(blob []byte, masterVersion uint32, aad []byte) ([]byte, error)
}

// Users is the org module: it owns users, roles and the first administrator rule.
type Users interface {
	SignIn(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) (entity.SignIn, error)
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
	RecordSeparately(ctx context.Context, e entity.AuditEvent) error
}

// Deps of the module; installation uses only Repo, Keyring and Clock.
type Authorizer interface {
	Authorize(context.Context, entity.Subject, entity.AccessRequest) (entity.Scope, error)
}

type Deps struct {
	Tokens   repo.UploadTokenRepo
	Access   Authorizer
	Repo     repo.AuthRepo
	Tx       repo.Transactor
	Provider repo.IdentityProvider
	Keyring  Keyring
	Users    Users
	Auditor  Auditor
	Clock    clock.Clock
	// SiteURL is the origin of the installation; the provider returns the browser to SiteURL/o/<slug>/callback.
	SiteURL string
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

func (u *UseCase) OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error) {
	return u.d.Repo.OrganizationBySlug(ctx, slug)
}

// RegisterLogin creates the login route /o/<slug> and the provider registration with an encrypted client secret.
func (u *UseCase) RegisterLogin(ctx context.Context, orgID entity.ID, slug string, p entity.OIDCProvider) error {
	blob, version, err := u.d.Keyring.SealEnvelope([]byte(p.ClientSecret.Value()), ClientSecretAAD(orgID))
	if err != nil {
		return err
	}
	if err := u.d.Repo.CreateLoginRoute(ctx, slug, orgID); err != nil {
		return err
	}
	return u.d.Repo.CreateProviderRegistration(ctx, entity.ProviderRegistration{
		OrganizationID:  orgID,
		Issuer:          p.Issuer,
		ClientID:        p.ClientID,
		ClientSecretEnc: blob,
		KeyVersion:      version,
	}, u.d.Clock.Now())
}

// ClientSecretAAD binds the ciphertext to its organization, so a row copied to another organization does not open.
func ClientSecretAAD(orgID entity.ID) []byte {
	return append([]byte("auth.provider_registration.client_secret:"), orgID[:]...)
}
