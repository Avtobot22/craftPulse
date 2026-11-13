package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) CreateUploadToken(ctx context.Context, orgID, projectID, createdBy entity.ID, hash []byte, at time.Time) (entity.UploadToken, error) {
	q := New(postgres.Conn(ctx, r.pool))
	if _, err := q.LockUploadProject(ctx, LockUploadProjectParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(projectID)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.UploadToken{}, entity.ErrNotFound
		}
		return entity.UploadToken{}, fmt.Errorf("auth: lock upload project: %w", err)
	}
	row, err := q.InsertUploadToken(ctx, InsertUploadTokenParams{OrganizationID: uuid.UUID(orgID), ProjectID: uuid.UUID(projectID), CreatedBy: uuid.UUID(createdBy), TokenHash: hash, CreatedAt: at})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.UploadToken{}, entity.ErrTokenSlotsFull
	}
	if err != nil {
		return entity.UploadToken{}, fmt.Errorf("auth: create upload token: %w", err)
	}
	return uploadToken(FindUploadTokenRow(row)), nil
}

func (r *Repo) UploadToken(ctx context.Context, orgID, id entity.ID) (entity.UploadToken, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).FindUploadToken(ctx, FindUploadTokenParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.UploadToken{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.UploadToken{}, fmt.Errorf("auth: find upload token: %w", err)
	}
	return uploadToken(row), nil
}

func (r *Repo) UploadTokenByHash(ctx context.Context, hash []byte) (entity.UploadToken, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).FindUploadTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.UploadToken{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.UploadToken{}, fmt.Errorf("auth: authenticate upload token: %w", err)
	}
	return uploadToken(FindUploadTokenRow(row)), nil
}

func (r *Repo) RevokeUploadToken(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).RevokeUploadToken(ctx, RevokeUploadTokenParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), RevokedAt: pgtype.Timestamptz{Time: at, Valid: true}})
	if err != nil {
		return false, fmt.Errorf("auth: revoke upload token: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) PageUploadTokens(ctx context.Context, orgID, projectID entity.ID, limit int, after *entity.AuditKey) ([]entity.UploadToken, error) {
	if limit < 1 || limit > 201 {
		return nil, errors.New("auth: invalid upload page limit")
	}
	p := PageUploadTokensParams{OrganizationID: uuid.UUID(orgID), ProjectID: uuid.UUID(projectID), PageLimit: int32(limit)}
	if after != nil {
		p.HasCursor = true
		p.AfterTime = after.OccurredAt
		p.AfterID = uuid.UUID(after.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageUploadTokens(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("auth: page upload tokens: %w", err)
	}
	out := make([]entity.UploadToken, 0, len(rows))
	for _, row := range rows {
		out = append(out, uploadToken(FindUploadTokenRow(row)))
	}
	return out, nil
}

func uploadToken(row FindUploadTokenRow) entity.UploadToken {
	out := entity.UploadToken{ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), ProjectID: entity.ID(row.ProjectID), Slot: int(row.Slot), CreatedAt: row.CreatedAt}
	if row.RevokedAt.Valid {
		out.RevokedAt = &row.RevokedAt.Time
	}
	return out
}
