package calc

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) TimelinePublications(ctx context.Context, org entity.ID, teams []entity.ID, from, observedBefore time.Time) ([]entity.TimelinePublication, error) {
	if len(teams) == 0 {
		return nil, nil
	}
	var out []entity.TimelinePublication
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TimelinePublications(ctx, org, teams, from, observedBefore)
		return err
	})
	return out, err
}

func (u *UseCase) TimelineRevision(ctx context.Context, org, id entity.ID, teams []entity.ID) (entity.CalcRevision, error) {
	if len(teams) == 0 {
		return entity.CalcRevision{}, entity.ErrNotFound
	}
	var revision entity.CalcRevision
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		revision, err = u.d.Repo.Revision(ctx, org, id)
		if err != nil {
			return err
		}
		if revision.PublishedAt == nil {
			return entity.ErrNotFound
		}
		series, err := u.d.Repo.Series(ctx, org, revision.SeriesID)
		if err != nil {
			return err
		}
		team, err := scopeTeam(series.Scope)
		if err != nil || !slices.Contains(teams, team) {
			return entity.ErrNotFound
		}
		return nil
	})
	return revision, err
}
