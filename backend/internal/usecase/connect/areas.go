package connect

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// AreaSources are the scopes of connections that collected the areas, for access and for links of projects.
func (u *UseCase) AreaSources(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaSource, error) {
	if len(areaIDs) == 0 {
		return nil, nil
	}
	var out []entity.AreaSource
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.AreaSources(ctx, orgID, areaIDs)
		return err
	})
	return out, err
}

// ListAreas pages the source areas of active connections by ID; an area appears after its first collection.
func (u *UseCase) ListAreas(ctx context.Context, s entity.Subject, f entity.SourceAreaFilter) (entity.SourceAreaPage, error) {
	var page entity.SourceAreaPage
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageSourceAreas(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > f.Limit {
			items = items[:f.Limit]
			next := items[len(items)-1].ID
			page.Next = &next
		}
		page.Items = items
		return nil
	})
	return page, err
}
