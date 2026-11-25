package normalize

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// VisibleTaskFacts reads the selected task only, after the caller has checked its current source access.
func (u *UseCase) VisibleTaskFacts(ctx context.Context, scope entity.Scope, org entity.ID, end entity.LinkEnd, seq int64) (entity.AreaFacts, error) {
	out := entity.AreaFacts{AreaID: end.AreaID, InstanceID: end.InstanceID}
	if !scope.Area(end.AreaID) {
		return out, entity.ErrNotFound
	}
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out.Tasks, out.Events, err = u.d.Repo.TaskViewFacts(ctx, org, end.AreaID, end.ID, seq, scope.Objects())
		return err
	})
	return out, err
}

// TaskMRFacts reads the linked MR, its history and its revisions at the card's knowledge number.
func (u *UseCase) TaskMRFacts(ctx context.Context, scope entity.Scope, org entity.ID, end entity.LinkEnd, seq int64) (entity.ReviewFacts, error) {
	var out entity.ReviewFacts
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out = entity.ReviewFacts{AreaID: end.AreaID, ActorKinds: map[entity.ID]entity.PersonKind{}}
		areas, ids, allowed := []entity.ID{end.AreaID}, []entity.ID{end.ID}, scope.Objects()
		if !scope.Area(end.AreaID) {
			return entity.ErrNotFound
		}
		out.MRs, err = u.d.Repo.ReadMRVersions(ctx, org, areas, allowed, ids, seq)
		if err != nil {
			return err
		}
		if len(out.MRs) == 0 {
			return entity.ErrNotFound
		}
		out.MREvents, err = u.d.Repo.ReadMREvents(ctx, org, areas, allowed, ids, seq)
		if err != nil {
			return err
		}
		out.MRRevisions, err = u.d.Repo.ReadMRRevisions(ctx, org, areas, allowed, ids, seq)
		return err
	})
	if err != nil {
		return entity.ReviewFacts{}, err
	}
	err = u.reviewActors(ctx, org, seq, &out, false)
	return out, err
}
