package org

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

func (r *Repo) CreateOrganization(ctx context.Context, o entity.NewOrganization, at time.Time) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateOrganization(ctx, CreateOrganizationParams{
		OrganizationID:    uuid.UUID(o.ID),
		Name:              o.Name,
		Status:            string(entity.OrgStatusSetup),
		FirstAdminIssuer:  o.Provider.Issuer,
		FirstAdminSubject: o.FirstAdminSubject,
		CreatedAt:         at,
	})
	if err != nil {
		return fmt.Errorf("org: create organization: %w", err)
	}
	return nil
}
