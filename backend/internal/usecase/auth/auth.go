package auth

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Sealer is the envelope encryption of pkg/crypto (TZ 11).
type Sealer interface {
	SealEnvelope(plaintext, aad []byte) (blob []byte, masterVersion uint32, err error)
}

type UseCase struct {
	repo   repo.AuthRepo
	sealer Sealer
	clock  clock.Clock
}

func New(r repo.AuthRepo, s Sealer, c clock.Clock) *UseCase {
	return &UseCase{repo: r, sealer: s, clock: c}
}

func (u *UseCase) OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error) {
	return u.repo.OrganizationBySlug(ctx, slug)
}

// RegisterLogin creates the login route /o/<slug> and the provider registration with an encrypted client secret.
func (u *UseCase) RegisterLogin(ctx context.Context, orgID entity.ID, slug string, p entity.OIDCProvider) error {
	blob, version, err := u.sealer.SealEnvelope([]byte(p.ClientSecret.Value()), ClientSecretAAD(orgID))
	if err != nil {
		return err
	}
	if err := u.repo.CreateLoginRoute(ctx, slug, orgID); err != nil {
		return err
	}
	return u.repo.CreateProviderRegistration(ctx, entity.ProviderRegistration{
		OrganizationID:  orgID,
		Issuer:          p.Issuer,
		ClientID:        p.ClientID,
		ClientSecretEnc: blob,
		KeyVersion:      version,
	}, u.clock.Now())
}

// ClientSecretAAD binds the ciphertext to its organization, so a row copied to another organization does not open.
func ClientSecretAAD(orgID entity.ID) []byte {
	return append([]byte("auth.provider_registration.client_secret:"), orgID[:]...)
}
