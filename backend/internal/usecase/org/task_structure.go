package org

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// TaskStructure reads the links needed to resolve a task's project and team at the same time cut.
func (u *UseCase) TaskStructure(ctx context.Context, orgID entity.ID, at time.Time) (areas, teams []entity.Link, err error) {
	err = u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		areas, err = u.orgs.CurrentLinks(ctx, orgID, entity.LinkProjectArea, at)
		if err != nil {
			return err
		}
		teams, err = u.orgs.CurrentLinks(ctx, orgID, entity.LinkTeamProject, at)
		return err
	})
	return areas, teams, err
}
