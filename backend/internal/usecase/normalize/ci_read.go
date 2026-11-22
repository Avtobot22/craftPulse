package normalize

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// RunAt reads a run at the slice seq with its child runs and the attempts of all of them: a job of a child
// pipeline is an attempt of its own run and is counted once.
func (u *UseCase) RunAt(ctx context.Context, scope entity.Scope, orgID, runID entity.ID, seq int64) (entity.CIRun, error) {
	var out entity.CIRun
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		runs, err := u.d.Repo.ReadCIRunVersions(ctx, orgID, scope.Areas(), scope.Objects(), []entity.ID{runID}, seq)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			return entity.ErrNotFound
		}
		out.Run = runs[0]
		ids := []entity.ID{runID}
		for parents := ids; len(parents) > 0; {
			children, err := u.d.Repo.FindCIRunIDs(ctx, orgID, scope.Areas(), scope.Objects(), entity.RunsQuery{Parents: parents}, seq)
			if err != nil {
				return err
			}
			parents = slices.DeleteFunc(children, func(id entity.ID) bool { return slices.Contains(ids, id) })
			ids = append(ids, parents...)
		}
		if len(ids) > 1 {
			if out.Children, err = u.d.Repo.ReadCIRunVersions(ctx, orgID, scope.Areas(), scope.Objects(), ids[1:], seq); err != nil {
				return err
			}
		}
		out.History = entity.AttemptHistoryComplete
		unknown := func(r entity.CIRunVersion) bool {
			return r.AttemptHistory != entity.AttemptHistoryComplete && r.AttemptHistory != entity.AttemptHistoryTruncated
		}
		if unknown(out.Run) || slices.ContainsFunc(out.Children, unknown) {
			out.History = entity.AttemptHistoryUnknown
		}
		truncated := func(r entity.CIRunVersion) bool { return r.AttemptHistory == entity.AttemptHistoryTruncated }
		if truncated(out.Run) || slices.ContainsFunc(out.Children, truncated) {
			out.History = entity.AttemptHistoryTruncated
		}
		out.Attempts, err = u.d.Repo.ReadCIAttemptVersions(ctx, orgID, scope.Areas(), scope.Objects(), ids, seq)
		return err
	})
	if err != nil {
		return entity.CIRun{}, err
	}
	return out, nil
}

// AttemptsAt reads the known attempts of a run and its child runs with the completeness of their history.
func (u *UseCase) AttemptsAt(ctx context.Context, scope entity.Scope, orgID, runID entity.ID, seq int64) (entity.CIAttempts, error) {
	run, err := u.RunAt(ctx, scope, orgID, runID, seq)
	if err != nil {
		return entity.CIAttempts{}, err
	}
	return run.CIAttempts, nil
}

// MRRunsAt reads the runs linked to an MR at the slice seq.
func (u *UseCase) MRRunsAt(ctx context.Context, scope entity.Scope, orgID, mrID entity.ID, seq int64) ([]entity.CIRunVersion, error) {
	return u.findRuns(ctx, scope, orgID, scope.Areas(), entity.RunsQuery{MRID: &mrID}, seq)
}

// AreaRunsAt reads the runs of an area started in [from, before) at the slice seq.
func (u *UseCase) AreaRunsAt(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, from, before time.Time, seq int64) ([]entity.CIRunVersion, error) {
	if !scope.Area(areaID) {
		return nil, nil
	}
	return u.findRuns(ctx, scope, orgID, []entity.ID{areaID}, entity.RunsQuery{From: &from, Before: &before}, seq)
}

func (u *UseCase) findRuns(ctx context.Context, scope entity.Scope, orgID entity.ID, areas []entity.ID, q entity.RunsQuery, seq int64) ([]entity.CIRunVersion, error) {
	var out []entity.CIRunVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		ids, err := u.d.Repo.FindCIRunIDs(ctx, orgID, areas, scope.Objects(), q, seq)
		if err != nil || len(ids) == 0 {
			return err
		}
		out, err = u.d.Repo.ReadCIRunVersions(ctx, orgID, areas, scope.Objects(), ids, seq)
		return err
	})
	return out, err
}
