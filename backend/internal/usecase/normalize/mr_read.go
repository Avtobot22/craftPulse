package normalize

import (
	"context"
	"math"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) Repository(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID) (entity.RepositoryVersion, error) {
	if !scope.Area(areaID) {
		return entity.RepositoryVersion{}, entity.ErrNotFound
	}
	var out entity.RepositoryVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadRepositoryVersion(ctx, orgID, areaID)
		return err
	})
	return out, err
}

func (u *UseCase) CurrentMR(ctx context.Context, scope entity.Scope, orgID, mrID entity.ID) (entity.MRVersion, error) {
	return u.MRAt(ctx, scope, orgID, mrID, math.MaxInt64)
}

func (u *UseCase) MRAt(ctx context.Context, scope entity.Scope, orgID, mrID entity.ID, seq int64) (entity.MRVersion, error) {
	var out []entity.MRVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadMRVersions(ctx, orgID, scope.Areas(), scope.Objects(), []entity.ID{mrID}, seq)
		return err
	})
	if err != nil {
		return entity.MRVersion{}, err
	}
	if len(out) == 0 {
		return entity.MRVersion{}, entity.ErrNotFound
	}
	return out[0], nil
}

func (u *UseCase) MRsAt(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) ([]entity.MRVersion, error) {
	if !scope.Area(areaID) {
		return nil, nil
	}
	return u.findMRs(ctx, scope, orgID, []entity.ID{areaID}, entity.SourceKey{}, "", seq)
}

func (u *UseCase) MRByNative(ctx context.Context, scope entity.Scope, orgID, instanceID entity.ID, nativeID string) (entity.MRVersion, error) {
	if nativeID == "" {
		return entity.MRVersion{}, entity.ErrNotFound
	}
	out, err := u.findMRs(ctx, scope, orgID, scope.Areas(), entity.SourceKey{InstanceID: instanceID, NativeID: nativeID}, "", math.MaxInt64)
	if err != nil {
		return entity.MRVersion{}, err
	}
	if len(out) == 0 {
		return entity.MRVersion{}, entity.ErrNotFound
	}
	return out[0], nil
}

func (u *UseCase) MRsByHeadSHA(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, sha string, seq int64) ([]entity.MRVersion, error) {
	if !scope.Area(areaID) || sha == "" {
		return nil, nil
	}
	return u.findMRs(ctx, scope, orgID, []entity.ID{areaID}, entity.SourceKey{}, sha, seq)
}

func (u *UseCase) findMRs(ctx context.Context, scope entity.Scope, orgID entity.ID, areas []entity.ID, key entity.SourceKey, sha string, seq int64) ([]entity.MRVersion, error) {
	var out []entity.MRVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		ids, err := u.d.Repo.FindMRIDs(ctx, orgID, areas, scope.Objects(), key, sha, seq)
		if err != nil {
			return err
		}
		out, err = u.d.Repo.ReadMRVersions(ctx, orgID, areas, scope.Objects(), ids, seq)
		return err
	})
	return out, err
}

func (u *UseCase) MRRevisionsAt(ctx context.Context, scope entity.Scope, orgID, mrID entity.ID, seq int64) ([]entity.MRRevision, error) {
	var out []entity.MRRevision
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadMRRevisions(ctx, orgID, scope.Areas(), scope.Objects(), []entity.ID{mrID}, seq)
		return err
	})
	return out, err
}

func (u *UseCase) MREventsAt(ctx context.Context, scope entity.Scope, orgID, mrID entity.ID, seq int64) ([]entity.MREvent, error) {
	var out []entity.MREvent
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadMREvents(ctx, orgID, scope.Areas(), scope.Objects(), []entity.ID{mrID}, seq)
		return err
	})
	if err != nil {
		return nil, err
	}
	kinds := map[entity.ID]entity.PersonKind{}
	for i := range out {
		e := &out[i]
		if e.ActorAccountID == nil {
			continue
		}
		id := *e.ActorAccountID
		kind, ok := kinds[id]
		if !ok {
			assignment, found, err := u.d.Accounts.AssignmentAt(ctx, orgID, id, seq)
			if err != nil {
				return nil, err
			}
			kind = entity.PersonUnknown
			if found {
				kind = assignment.ActorKind
			}
			kinds[id] = kind
		}
		e.ActorKind = kind
	}
	return out, nil
}

// ReviewFacts reads, for calc, the MRs of an area at the slice seq with their events, the events of its tasks and
// the confirmed kinds of the accounts acting in them at seq; an account without a confirmed assignment is unknown.
func (u *UseCase) ReviewFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.ReviewFacts, error) {
	return u.readReviewFacts(ctx, orgID, areaID, seq, nil, true)
}

// MRReviewFactsAt reads only visible MR facts, without loading task events for the review queue.
func (u *UseCase) MRReviewFactsAt(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) (entity.ReviewFacts, error) {
	if !scope.Area(areaID) {
		return entity.ReviewFacts{}, entity.ErrNotFound
	}
	return u.readReviewFacts(ctx, orgID, areaID, seq, scope.Objects(), false)
}

func (u *UseCase) readReviewFacts(ctx context.Context, orgID, areaID entity.ID, seq int64, allowed []entity.ID, tasks bool) (entity.ReviewFacts, error) {
	out := entity.ReviewFacts{AreaID: areaID, ActorKinds: map[entity.ID]entity.PersonKind{}}
	area := []entity.ID{areaID}
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		if tasks {
			restricted, err := u.d.Repo.RestrictedObjects(ctx, orgID, area)
			if err != nil {
				return err
			}
			allowed = make([]entity.ID, len(restricted))
			for i := range restricted {
				allowed[i] = restricted[i].ID
			}
		}
		ids, err := u.d.Repo.FindMRIDs(ctx, orgID, area, allowed, entity.SourceKey{}, "", seq)
		if err != nil {
			return err
		}
		if out.MRs, err = u.d.Repo.ReadMRVersions(ctx, orgID, area, allowed, ids, seq); err != nil {
			return err
		}
		if out.MREvents, err = u.d.Repo.ReadMREvents(ctx, orgID, area, allowed, ids, seq); err != nil || !tasks {
			return err
		}
		if len(ids) > 0 {
			if out.MRRevisions, err = u.d.Repo.ReadMRRevisions(ctx, orgID, area, allowed, ids, seq); err != nil {
				return err
			}
		}
		out.TaskEvents, err = u.d.Repo.TaskEventsAt(ctx, orgID, areaID, seq, allowed)
		return err
	})
	if err != nil {
		return entity.ReviewFacts{}, err
	}
	if err := u.reviewActors(ctx, orgID, seq, &out, tasks); err != nil {
		return entity.ReviewFacts{}, err
	}
	return out, nil
}

func (u *UseCase) reviewActors(ctx context.Context, orgID entity.ID, seq int64, out *entity.ReviewFacts, tasks bool) error {
	actors := make([]*entity.ID, 0, len(out.MRs)+len(out.MREvents)+len(out.TaskEvents))
	if !tasks {
		for i := range out.MRs {
			actors = append(actors, out.MRs[i].AuthorAccountID)
		}
	}
	for i := range out.MREvents {
		e := &out.MREvents[i]
		out.ReviewsGiven = out.ReviewsGiven || e.Kind == "review" || e.Kind == "approval"
		actors = append(actors, e.ActorAccountID)
	}
	for i := range out.TaskEvents {
		actors = append(actors, out.TaskEvents[i].ActorAccountID)
	}
	for _, id := range actors {
		if id == nil {
			continue
		}
		if _, ok := out.ActorKinds[*id]; ok {
			continue
		}
		assignment, found, err := u.d.Accounts.AssignmentAt(ctx, orgID, *id, seq)
		if err != nil {
			return err
		}
		out.ActorKinds[*id] = entity.PersonUnknown
		if found {
			out.ActorKinds[*id] = assignment.ActorKind
		}
	}
	return nil
}
