package auth

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
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

// Credentials ends sessions and disables API keys of a user for the org module, in the caller's
// transaction (TZ 13.2). It needs only the repository, so org and auth do not depend on each other.
type Credentials struct {
	repo repo.AuthRepo
}

func NewCredentials(r repo.AuthRepo) *Credentials {
	return &Credentials{repo: r}
}

func (c *Credentials) EndUserSessions(ctx context.Context, orgID, userID entity.ID, reason entity.SessionEndReason, at time.Time) (int64, error) {
	return c.repo.EndUserSessions(ctx, orgID, userID, at, reason)
}

func (c *Credentials) RevokeUserAPIKeys(ctx context.Context, orgID, userID entity.ID, at time.Time) (int64, error) {
	return c.repo.RevokeUserAPIKeys(ctx, orgID, userID, at)
}

// _purgeBatch bounds the rows one transaction of a purge deletes.
const _purgeBatch = 1000

// Retention deletes the login attempts and sessions that expired (TZ 12.9). Both tables are outside row-level
// security, so it needs only the repository, like Credentials.
type Retention struct {
	repo  repo.AuthRepo
	tx    repo.Transactor
	clock clock.Clock
}

func NewRetention(r repo.AuthRepo, tx repo.Transactor, c clock.Clock) *Retention {
	return &Retention{repo: r, tx: tx, clock: c}
}

// PurgeExpired deletes in portions, each committed on its own: a login page any visitor opens adds an attempt, and an
// abandoned one is never taken back by its callback.
func (r *Retention) PurgeExpired(ctx context.Context) error {
	now := r.clock.Now()
	for _, purge := range []func(ctx context.Context, before time.Time, limit int) (int64, error){r.repo.PurgeLoginAttempts, r.repo.PurgeSessions} {
		for {
			var n int64
			err := r.tx.InTx(ctx, func(ctx context.Context) error {
				var err error
				n, err = purge(ctx, now, _purgeBatch)
				return err
			})
			if err != nil {
				return err
			}
			if n < _purgeBatch {
				break
			}
		}
	}
	return nil
}
