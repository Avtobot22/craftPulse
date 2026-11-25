package org

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) ProjectSources(ctx context.Context, orgID, projectID entity.ID, kind entity.ScopeKind) ([]entity.AreaSource, error) {
	var ids []entity.ID
	err := u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		links, err := u.orgs.CurrentLinks(ctx, orgID, entity.LinkProjectArea, u.clock.Now())
		if err != nil {
			return err
		}
		for _, link := range links {
			if link.From == projectID {
				ids = append(ids, link.To)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sources, err := u.sources.AreaSources(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(sources, func(s entity.AreaSource) bool { return s.Kind != kind }), nil
}
