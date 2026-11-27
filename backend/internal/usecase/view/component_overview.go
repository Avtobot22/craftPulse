package view

import (
	"cmp"
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) ComponentOverview(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.ComponentOverview, error) {
	org := s.OrganizationID
	component, err := u.d.Findings.Structure.ConfirmedComponent(ctx, org, id)
	if err != nil {
		return entity.ComponentOverview{}, err
	}
	sel, notes, err := u.findingSelection(ctx, s, scope, "getComponentOverview", []entity.ID{component.RepositoryAreaID}, entity.ID{})
	if err != nil {
		return entity.ComponentOverview{}, err
	}
	if len(sel.Areas) == 0 {
		return entity.ComponentOverview{}, entity.ErrNotFound
	}
	sel.Components = []entity.ID{id}
	rows, err := u.d.Findings.Reader.Findings(ctx, org, sel)
	if err != nil {
		return entity.ComponentOverview{}, err
	}
	profiles, err := u.componentProfiles(ctx, org, scope, &component)
	if err != nil {
		return entity.ComponentOverview{}, err
	}
	var facts []entity.ComponentCodeFact
	if len(profiles) > 0 {
		facts, err = u.d.Findings.Calculator.ComponentCodeFacts(ctx, org, component, profiles)
		if err != nil {
			return entity.ComponentOverview{}, err
		}
	}
	return entity.ComponentOverview{Component: component, Counts: componentFindingCounts(rows), Facts: facts, Notes: notes}, nil
}

func (u *UseCase) componentProfiles(ctx context.Context, org entity.ID, scope entity.Scope, component *entity.Component) ([]entity.ProfileEntry, error) {
	links, err := u.findingProjectAreas(ctx, org)
	if err != nil {
		return nil, err
	}
	projects := scope.Projects()
	var selected []entity.ID
	for _, l := range links {
		if l.To == component.RepositoryAreaID && (scope.Whole() || slices.Contains(projects, l.From)) && !slices.Contains(selected, l.From) {
			selected = append(selected, l.From)
		}
	}
	return u.d.Findings.Reader.ComponentCodeProfiles(ctx, org, component.ID, selected)
}

type findingCountKey struct{ kind, state string }

func componentFindingCounts(rows []entity.FindingDetail) []entity.ComponentFindingCount {
	groups := map[findingCountKey]map[entity.ID]struct{}{}
	add := func(kind, state string, id entity.ID) {
		key := findingCountKey{kind, state}
		if groups[key] == nil {
			groups[key] = map[entity.ID]struct{}{}
		}
		groups[key][id] = struct{}{}
	}
	for i := range rows {
		f := &rows[i]
		switch f.State {
		case entity.FindingOpen, entity.FindingNewlyObserved, entity.FindingResolved:
			add(f.Kind, f.State, f.ID)
		}
		for i := range f.Exceptions {
			v := &f.Exceptions[i]
			if v.Status == entity.ExceptionApproved {
				add(f.Kind, v.Type, v.DecisionID)
			}
		}
	}
	out := make([]entity.ComponentFindingCount, 0, len(groups))
	for key, ids := range groups {
		out = append(out, entity.ComponentFindingCount{Kind: key.kind, State: key.state, Count: len(ids)})
	}
	slices.SortFunc(out, func(a, b entity.ComponentFindingCount) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.State, b.State))
	})
	return out
}
