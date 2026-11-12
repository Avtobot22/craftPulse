package auth

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// Authenticate finds the session by the hash of its cookie and checks both limits before any data is read
// (TZ 13.2). The activity time is written at most once a minute.
func (u *UseCase) Authenticate(ctx context.Context, token string) (entity.Subject, error) {
	if len(token) != _tokenLength {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	s, found, err := u.d.Repo.SessionByToken(ctx, hashToken(token))
	if err != nil {
		return entity.Subject{}, err
	}
	now := u.d.Clock.Now()
	if !found || s.Ended || !now.Before(s.ExpiresAt) || !now.Before(s.LastActivityAt.Add(entity.SessionIdleTimeout)) {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	if now.Sub(s.LastActivityAt) >= entity.SessionTouchInterval {
		if err := u.d.Repo.TouchSession(ctx, s.OrganizationID, s.ID, now); err != nil {
			return entity.Subject{}, err
		}
		s.LastActivityAt = now
	}
	idle := s.LastActivityAt.Add(entity.SessionIdleTimeout)
	if idle.After(s.ExpiresAt) {
		idle = s.ExpiresAt
	}
	return entity.Subject{
		OrganizationID:   s.OrganizationID,
		OrganizationSlug: s.OrganizationSlug,
		UserID:           s.UserID,
		SessionID:        s.ID,
		ExpiresAt:        s.ExpiresAt,
		IdleExpiresAt:    idle,
	}, nil
}

// Logout ends only the current session; a session that has already ended gives entity.ErrAuthRequired.
func (u *UseCase) Logout(ctx context.Context, s entity.Subject) error {
	now := u.d.Clock.Now()
	return u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		ended, err := u.d.Repo.EndSession(ctx, s.OrganizationID, s.SessionID, now, entity.SessionLogout)
		if err != nil {
			return err
		}
		if !ended {
			return entity.ErrAuthRequired
		}
		return u.d.Auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: s.OrganizationID,
			OccurredAt:     now,
			ActorKind:      entity.ActorUser,
			ActorID:        s.UserID,
			Operation:      _operationLogout,
			ObjectKind:     "user",
			ObjectID:       s.UserID,
			Outcome:        entity.AuditSucceeded,
			CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		})
	})
}
