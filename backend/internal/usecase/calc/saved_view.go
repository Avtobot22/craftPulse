package calc

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// ReportSeries reads a stored flow series; saving a view never creates another series.
func (u *UseCase) ReportSeries(ctx context.Context, orgID, seriesID entity.ID) (entity.CalcSeries, error) {
	var out entity.CalcSeries
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.Series(ctx, orgID, seriesID)
		return err
	})
	if err != nil {
		return entity.CalcSeries{}, err
	}
	if out.MetricSet != entity.MetricSetFlow {
		return entity.CalcSeries{}, entity.ErrNotFound
	}
	return out, nil
}

// CreateSavedView writes the descriptor and the revision holder atomically, after view checked access.
func (u *UseCase) CreateSavedView(ctx context.Context, v entity.SavedView) error {
	return u.d.Tx.InOrgTx(ctx, v.OrganizationID, func(ctx context.Context) error {
		if _, err := u.d.Repo.Series(ctx, v.OrganizationID, v.SeriesID); err != nil {
			return err
		}
		return u.d.Repo.CreateSavedView(ctx, v)
	})
}

func (u *UseCase) SavedView(ctx context.Context, orgID, id entity.ID) (entity.SavedView, error) {
	var out entity.SavedView
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.SavedView(ctx, orgID, id)
		return err
	})
	return out, err
}

func (u *UseCase) ListSavedViews(ctx context.Context, orgID, ownerID entity.ID, q entity.SavedViewQuery) (entity.SavedViewPage, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return entity.SavedViewPage{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "limit", Code: entity.FieldOutOfRange}}}
	}
	var out entity.SavedViewPage
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageSavedViews(ctx, orgID, ownerID, q)
		if err != nil {
			return err
		}
		if len(items) > q.Limit {
			items = items[:q.Limit]
			last := items[len(items)-1]
			out.Next = &entity.SavedViewKey{CreatedAt: last.CreatedAt, ID: last.ID}
		}
		out.Items = items
		return nil
	})
	return out, err
}

func (u *UseCase) DeleteSavedView(ctx context.Context, orgID, ownerID, id entity.ID) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		return u.d.Repo.DeleteSavedView(ctx, orgID, ownerID, id)
	})
}

// PurgeRevisions applies the installation's SUPERSEDED_REVISIONS cutoff, one bounded transaction at a
// time. Pins of all holder kinds retain their revision; calc.gc later collects the unreferenced parts.
func (u *UseCase) PurgeRevisions(ctx context.Context, orgID entity.ID, before time.Time) error {
	for {
		var n int64
		err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
			var err error
			n, err = u.d.Repo.PurgeRevisions(ctx, orgID, before, u.d.Clock.Now(), _gcBatch)
			return err
		})
		if err != nil || n < _gcBatch {
			return err
		}
	}
}
