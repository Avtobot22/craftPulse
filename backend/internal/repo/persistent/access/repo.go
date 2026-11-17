package access

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) SourceLinks(ctx context.Context, org, user entity.ID) ([]entity.SourceLink, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).SourceLinks(ctx, SourceLinksParams{OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user)})
	if err != nil {
		return nil, fmt.Errorf("access: source links: %w", err)
	}
	out := make([]entity.SourceLink, 0, len(rows))
	for _, row := range rows {
		link := entity.SourceLink{ConnectionID: entity.ID(row.ConnectionID), State: row.State, Version: row.RowVersion}
		if row.LinkedAt.Valid {
			link.LinkedAt = &row.LinkedAt.Time
		}
		out = append(out, link)
	}
	return out, nil
}

func (r *Repo) ReserveSourceLink(ctx context.Context, org, user, conn entity.ID) (int64, error) {
	v, err := New(postgres.Conn(ctx, r.pool)).ReserveSourceLink(ctx, ReserveSourceLinkParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), ConnectionID: uuid.UUID(conn),
	})
	if err != nil {
		return 0, fmt.Errorf("access: reserve source link: %w", err)
	}
	return v, nil
}

func (r *Repo) CreateSourceLinkAttempt(ctx context.Context, org entity.ID, a entity.SourceLinkAttempt) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateSourceLinkAttempt(ctx, CreateSourceLinkAttemptParams{
		OrganizationID: uuid.UUID(org), StateHash: a.StateHash, UserID: uuid.UUID(a.UserID), SessionID: uuid.UUID(a.SessionID),
		ConnectionID: uuid.UUID(a.ConnectionID), LinkVersion: a.Version, ReturnPath: a.ReturnPath, RedirectUri: a.RedirectURI,
		CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("access: create source link attempt: %w", err)
	}
	return nil
}

func (r *Repo) TakeSourceLinkAttempt(ctx context.Context, org, user, session entity.ID, stateHash []byte) (entity.SourceLinkAttempt, bool, error) {
	a, err := New(postgres.Conn(ctx, r.pool)).TakeSourceLinkAttempt(ctx, TakeSourceLinkAttemptParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), SessionID: uuid.UUID(session), StateHash: stateHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.SourceLinkAttempt{}, false, nil
	}
	if err != nil {
		return entity.SourceLinkAttempt{}, false, fmt.Errorf("access: take source link attempt: %w", err)
	}
	return entity.SourceLinkAttempt{
		StateHash: a.StateHash, UserID: user, SessionID: session, ConnectionID: entity.ID(a.ConnectionID),
		Version: a.LinkVersion, ReturnPath: a.ReturnPath, RedirectURI: a.RedirectUri, CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt,
	}, true, nil
}

func (r *Repo) ConfirmSourceLink(ctx context.Context, org, user, conn entity.ID, version int64, at time.Time) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).ConfirmSourceLink(ctx, ConfirmSourceLinkParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), ConnectionID: uuid.UUID(conn), RowVersion: version,
		LinkedAt: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("access: confirm source link: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) RevokeSourceLink(ctx context.Context, org, user, conn entity.ID) (bool, error) {
	q := New(postgres.Conn(ctx, r.pool))
	n, err := q.RevokeSourceLink(ctx, RevokeSourceLinkParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), ConnectionID: uuid.UUID(conn),
	})
	if err != nil {
		return false, fmt.Errorf("access: revoke source link: %w", err)
	}
	err = q.DeleteSourceLinkAttempts(ctx, DeleteSourceLinkAttemptsParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), ConnectionID: uuid.UUID(conn),
	})
	if err != nil {
		return false, fmt.Errorf("access: remove source link attempts: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) ExpireSourceLink(ctx context.Context, org, user, conn entity.ID, version int64) error {
	err := New(postgres.Conn(ctx, r.pool)).ExpireSourceLink(ctx, ExpireSourceLinkParams{
		OrganizationID: uuid.UUID(org), UserID: uuid.UUID(user), ConnectionID: uuid.UUID(conn), RowVersion: version,
	})
	if err != nil {
		return fmt.Errorf("access: expire source link: %w", err)
	}
	return nil
}
