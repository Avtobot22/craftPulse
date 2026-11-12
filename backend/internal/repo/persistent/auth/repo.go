package auth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).GetLoginRoute(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, false, nil
	}
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("auth: get login route: %w", err)
	}
	return entity.ID(id), true, nil
}

func (r *Repo) CreateLoginRoute(ctx context.Context, slug string, orgID entity.ID) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateLoginRoute(ctx, CreateLoginRouteParams{Slug: slug, OrganizationID: uuid.UUID(orgID)})
	if err != nil {
		return fmt.Errorf("auth: create login route: %w", err)
	}
	return nil
}

func (r *Repo) CreateProviderRegistration(ctx context.Context, reg entity.ProviderRegistration, at time.Time) error {
	if reg.KeyVersion > math.MaxInt32 {
		return fmt.Errorf("auth: master key version %d does not fit the key_version column", reg.KeyVersion)
	}
	err := New(postgres.Conn(ctx, r.pool)).CreateProviderRegistration(ctx, CreateProviderRegistrationParams{
		OrganizationID:  uuid.UUID(reg.OrganizationID),
		Issuer:          reg.Issuer,
		ClientID:        reg.ClientID,
		ClientSecretEnc: reg.ClientSecretEnc,
		KeyVersion:      int32(reg.KeyVersion), //nolint:gosec // bounded by the check above
		CreatedAt:       at,
	})
	if err != nil {
		return fmt.Errorf("auth: create provider registration: %w", err)
	}
	return nil
}

func (r *Repo) ActiveProviderRegistration(ctx context.Context, orgID entity.ID) (entity.ProviderRegistration, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetActiveProviderRegistration(ctx, uuid.UUID(orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ProviderRegistration{}, false, nil
	}
	if err != nil {
		return entity.ProviderRegistration{}, false, fmt.Errorf("auth: get provider registration: %w", err)
	}
	if row.KeyVersion < 0 {
		return entity.ProviderRegistration{}, false, fmt.Errorf("auth: provider registration has key version %d", row.KeyVersion)
	}
	return entity.ProviderRegistration{
		OrganizationID:  orgID,
		Issuer:          row.Issuer,
		ClientID:        row.ClientID,
		ClientSecretEnc: row.ClientSecretEnc,
		KeyVersion:      uint32(row.KeyVersion),
	}, true, nil
}

func (r *Repo) CreateLoginAttempt(ctx context.Context, a entity.LoginAttempt) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateLoginAttempt(ctx, CreateLoginAttemptParams{
		CookieHash:     a.CookieHash,
		OrganizationID: uuid.UUID(a.OrganizationID),
		State:          a.State,
		Nonce:          a.Nonce,
		PkceVerifier:   a.PKCEVerifier,
		Issuer:         a.Issuer,
		ReturnPath:     a.ReturnPath,
		CreatedAt:      a.CreatedAt,
		ExpiresAt:      a.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("auth: create login attempt: %w", err)
	}
	return nil
}

// TakeLoginAttempt deletes the attempt as it reads it, so one provider response is accepted once.
func (r *Repo) TakeLoginAttempt(ctx context.Context, cookieHash []byte) (entity.LoginAttempt, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).TakeLoginAttempt(ctx, cookieHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.LoginAttempt{}, false, nil
	}
	if err != nil {
		return entity.LoginAttempt{}, false, fmt.Errorf("auth: take login attempt: %w", err)
	}
	return entity.LoginAttempt{
		CookieHash:     cookieHash,
		OrganizationID: entity.ID(row.OrganizationID),
		State:          row.State,
		Nonce:          row.Nonce,
		PKCEVerifier:   row.PkceVerifier,
		Issuer:         row.Issuer,
		ReturnPath:     row.ReturnPath,
		CreatedAt:      row.CreatedAt,
		ExpiresAt:      row.ExpiresAt,
	}, true, nil
}

func (r *Repo) CreateSession(ctx context.Context, s entity.NewSession) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateSession(ctx, CreateSessionParams{
		OrganizationID: uuid.UUID(s.OrganizationID),
		TokenHash:      s.TokenHash,
		UserID:         uuid.UUID(s.UserID),
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

func (r *Repo) SessionByToken(ctx context.Context, tokenHash []byte) (entity.Session, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetSessionByToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Session{}, false, nil
	}
	if err != nil {
		return entity.Session{}, false, fmt.Errorf("auth: get session: %w", err)
	}
	return entity.Session{
		ID:               entity.ID(row.ID),
		OrganizationID:   entity.ID(row.OrganizationID),
		OrganizationSlug: row.Slug,
		UserID:           entity.ID(row.UserID),
		CreatedAt:        row.CreatedAt,
		LastActivityAt:   row.LastActivityAt,
		ExpiresAt:        row.ExpiresAt,
		Ended:            row.Ended,
	}, true, nil
}

func (r *Repo) TouchSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).TouchSession(ctx, TouchSessionParams{
		At: at, OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(sessionID),
	})
	if err != nil {
		return fmt.Errorf("auth: touch session: %w", err)
	}
	return nil
}

func (r *Repo) EndSessionByToken(ctx context.Context, tokenHash []byte, at time.Time, reason entity.SessionEndReason) error {
	err := New(postgres.Conn(ctx, r.pool)).EndSessionByToken(ctx, EndSessionByTokenParams{
		At: at, Reason: string(reason), TokenHash: tokenHash,
	})
	if err != nil {
		return fmt.Errorf("auth: end session: %w", err)
	}
	return nil
}

// EndSession reports false when the session had already ended.
func (r *Repo) EndSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time, reason entity.SessionEndReason) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).EndSession(ctx, EndSessionParams{
		At: at, Reason: string(reason), OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(sessionID),
	})
	if err != nil {
		return false, fmt.Errorf("auth: end session: %w", err)
	}
	return n > 0, nil
}

// LiveSessions returns the sessions that still work, oldest first.
func (r *Repo) LiveSessions(ctx context.Context, orgID, userID entity.ID, at, idleSince time.Time) ([]entity.ID, error) {
	ids, err := New(postgres.Conn(ctx, r.pool)).ListLiveSessions(ctx, ListLiveSessionsParams{
		OrganizationID: uuid.UUID(orgID), UserID: uuid.UUID(userID), At: at, IdleSince: idleSince,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	out := make([]entity.ID, len(ids))
	for i, id := range ids {
		out[i] = entity.ID(id)
	}
	return out, nil
}

func (r *Repo) EndSessions(ctx context.Context, orgID entity.ID, ids []entity.ID, at time.Time, reason entity.SessionEndReason) error {
	uuids := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		uuids[i] = uuid.UUID(id)
	}
	err := New(postgres.Conn(ctx, r.pool)).EndSessions(ctx, EndSessionsParams{
		At: at, Reason: string(reason), OrganizationID: uuid.UUID(orgID), Ids: uuids,
	})
	if err != nil {
		return fmt.Errorf("auth: end sessions: %w", err)
	}
	return nil
}
