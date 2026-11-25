package auth

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) AuthenticateAPIKey(ctx context.Context, key string) (entity.Subject, error) {
	if !validAPIKey(key) {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	now := u.d.Clock.Now()
	s, err := u.d.Repo.ActiveAPIKey(ctx, hashToken(key), now)
	if err != nil {
		return entity.Subject{}, err
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		s.APIKeyProjects, err = u.d.Repo.APIKeyProjects(ctx, s.OrganizationID, s.APIKeyID, now)
		return err
	})
	return s, err
}

func (u *UseCase) RejectExposedAPIKey(ctx context.Context, key string) error {
	if !validAPIKey(key) {
		return nil
	}
	return u.d.Repo.RevokeExposedAPIKey(ctx, hashToken(key), u.d.Clock.Now())
}

func validAPIKey(key string) bool {
	s, ok := strings.CutPrefix(key, "cpk_")
	if !ok || len(s) != base64.RawURLEncoding.EncodedLen(32) {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == 32
}
