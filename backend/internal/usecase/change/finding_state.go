package change

import (
	"context"

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
		states, initial, err := u.findingStates(ctx, org, c.RepositoryAreaID, c.ID, id, kind)
		state = states[finding]
		if state == "" {
			state = initial
		}
		return err
	})
	return state, err
}

// findingStates compares a report once for all its canonical findings.
func (u *UseCase) findingStates(ctx context.Context, org, area, component, report entity.ID, kind string) (byID map[entity.ID]string, fallback string, err error) {
	scope := entity.TechnicalReportScope{AreaID: area, ComponentID: component, Kind: kind, ReportIDs: []entity.ID{report}}
	observations, err := u.d.Reports.TechnicalReports(ctx, org, scope)
	if err != nil {
		return nil, "", err
	}
	if len(observations) == 0 || !observations[0].Complete() {
		return nil, entity.FindingNewlyObserved, nil
	}
	seen := &observations[0]
	scope.Latest, scope.ReportIDs = true, nil
	scope.Language, scope.ProfileID = &seen.Language, &seen.ProfileID
	scope.ReferenceReportID = &seen.ID
	latest, err := u.d.Reports.TechnicalReports(ctx, org, scope)
	if err != nil {
		return nil, "", err
	}
	if len(latest) == 0 {
		return nil, entity.FindingNewlyObserved, nil
	}
	base, head := seen, &latest[0]
	if base.ID == head.ID {
		if len(latest) == 1 {
			return nil, entity.FindingNewlyObserved, nil
		}
		base = &latest[1]
	}
	q := entity.TechnicalComparisonQuery{ComponentID: scope.ComponentID, Kind: scope.Kind, BaseSHA: base.SHA, HeadSHA: head.SHA}
	result, err := u.compareLoadedReports(ctx, org, q, base, head, true)
	if err != nil {
		return nil, "", err
	}
	states := make(map[entity.ID]string, len(result.Findings))
	for _, f := range result.Findings {
		switch f.Lifecycle {
		case entity.FindingResolved, entity.FindingNewlyObserved:
			states[f.ID] = f.Lifecycle
		default:
			states[f.ID] = entity.FindingOpen
		}
	}
	return states, entity.FindingOpen, nil
}
