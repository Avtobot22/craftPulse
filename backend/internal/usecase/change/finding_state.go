package change

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CurrentFindingState reads the canonical lifecycle in the latest completed report of the same report stream.
func (u *UseCase) CurrentFindingState(ctx context.Context, org, component, finding entity.ID, kind string) (string, error) {
	var state string
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		c, err := u.d.Components.ConfirmedComponent(ctx, org, component)
		if err != nil {
			return err
		}
		id, err := u.d.Repo.LastFindingReport(ctx, org, component, finding, kind)
		if err != nil {
			return err
		}
		scope := entity.TechnicalReportScope{AreaID: c.RepositoryAreaID, ComponentID: c.ID, Kind: kind, ReportIDs: []entity.ID{id}}
		observations, err := u.d.Reports.TechnicalReports(ctx, org, scope)
		if err != nil {
			return err
		}
		if len(observations) == 0 {
			return entity.ErrNotFound
		}
		seen := &observations[0]
		scope.Latest, scope.ReportIDs = true, nil
		scope.Language, scope.ProfileID = &seen.Language, &seen.ProfileID
		scope.ReferenceReportID = &seen.ID
		latest, err := u.d.Reports.TechnicalReports(ctx, org, scope)
		if err != nil {
			return err
		}
		state, err = u.stateFromReports(ctx, org, &scope, seen, latest, finding)
		return err
	})
	return state, err
}

func (u *UseCase) stateFromReports(ctx context.Context, org entity.ID, scope *entity.TechnicalReportScope, seen *entity.TechnicalReport,
	latest []entity.TechnicalReport, finding entity.ID,
) (string, error) {
	if len(latest) == 0 || !seen.Complete() {
		return entity.FindingNewlyObserved, nil
	}
	base, head := seen, &latest[0]
	if base.ID == head.ID {
		if len(latest) == 1 {
			return entity.FindingNewlyObserved, nil
		}
		base = &latest[1]
	}
	q := entity.TechnicalComparisonQuery{ComponentID: scope.ComponentID, Kind: scope.Kind, BaseSHA: base.SHA, HeadSHA: head.SHA}
	result, err := u.compareLoadedReports(ctx, org, q, base, head, true)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(result.Findings, func(f entity.TechnicalFinding) bool { return f.ID == finding })
	if i < 0 {
		return entity.FindingOpen, nil
	}
	switch result.Findings[i].Lifecycle {
	case entity.FindingResolved:
		return entity.FindingResolved, nil
	case entity.FindingNewlyObserved:
		return entity.FindingNewlyObserved, nil
	default:
		return entity.FindingOpen, nil
	}
}
