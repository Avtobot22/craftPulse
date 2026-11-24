package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _uniqueViolation = "23505"

func (r *Repo) CurrentComponents(ctx context.Context, orgID entity.ID) ([]entity.Component, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).CurrentComponents(ctx, uuid.UUID(orgID))
	if err != nil {
		return nil, fmt.Errorf("org: current components: %w", err)
	}
	return convert(rows, component), nil
}

func (r *Repo) ConfirmedComponent(ctx context.Context, orgID, id entity.ID) (entity.Component, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).ConfirmedComponent(ctx, ConfirmedComponentParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Component{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Component{}, fmt.Errorf("org: confirmed component: %w", err)
	}
	return component(row), nil
}

func (r *Repo) ProjectComponents(ctx context.Context, orgID, projectID entity.ID, at time.Time) ([]entity.Component, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ProjectComponents(ctx, ProjectComponentsParams{
		OrganizationID: uuid.UUID(orgID), ProjectID: uuid.UUID(projectID), At: at,
	})
	if err != nil {
		return nil, fmt.Errorf("org: project components: %w", err)
	}
	return convert(rows, component), nil
}

func (r *Repo) SupersedeComponent(ctx context.Context, orgID, id entity.ID, seq int64, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).SupersedeComponent(ctx, SupersedeComponentParams{
		Seq: seq, At: at, OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id),
	})
	if err != nil {
		return fmt.Errorf("org: supersede component: %w", err)
	}
	return nil
}

// InsertComponent adds the version of a component known since seq; a path a component in force already takes
// gives entity.ErrComponentPath.
func (r *Repo) InsertComponent(ctx context.Context, orgID entity.ID, c entity.Component, seq int64) error {
	err := New(postgres.Conn(ctx, r.pool)).InsertComponent(ctx, InsertComponentParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(c.ID), KnownSeq: seq, ServiceID: uuid.UUID(c.ServiceID),
		RepositoryAreaID: uuid.UUID(c.RepositoryAreaID), PathPrefix: c.PathPrefix, Status: c.Status, Origin: c.Origin,
		ConfirmedBy: optionalID(c.ConfirmedBy), ConfirmedAt: tstz(c.ConfirmedAt),
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == _uniqueViolation && pgErr.ConstraintName == "component_path_uq" {
		return entity.ErrComponentPath
	}
	if err != nil {
		return fmt.Errorf("org: insert component: %w", err)
	}
	return nil
}

func component(row OrgComponent) entity.Component {
	c := entity.Component{
		ID: entity.ID(row.ID), ServiceID: entity.ID(row.ServiceID), RepositoryAreaID: entity.ID(row.RepositoryAreaID),
		PathPrefix: row.PathPrefix, Status: row.Status, Origin: row.Origin,
	}
	if row.ConfirmedBy.Valid {
		id := entity.ID(row.ConfirmedBy.UUID)
		c.ConfirmedBy = &id
	}
	if row.ConfirmedAt.Valid {
		t := row.ConfirmedAt.Time
		c.ConfirmedAt = &t
	}
	return c
}

func tstz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func optionalID(id *entity.ID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return nullID(*id)
}
