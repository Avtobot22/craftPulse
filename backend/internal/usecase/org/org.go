package org

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type UseCase struct {
	orgs    repo.OrgRepo
	tenants repo.TenantRepo
	clock   clock.Clock
}

func New(orgs repo.OrgRepo, tenants repo.TenantRepo, c clock.Clock) *UseCase {
	return &UseCase{orgs: orgs, tenants: tenants, clock: c}
}

// CreateOrganization runs inside the caller's organization transaction: the tenant row and the
// organization appear together or not at all (TZ 12.5.1).
func (u *UseCase) CreateOrganization(ctx context.Context, o entity.NewOrganization) error {
	now := u.clock.Now()
	if err := u.tenants.CreateTenant(ctx, o.ID, now); err != nil {
		return err
	}
	return u.orgs.CreateOrganization(ctx, o, now)
}
