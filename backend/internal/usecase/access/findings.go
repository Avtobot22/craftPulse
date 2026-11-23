package access

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// VisibleFindings checks both the source and the privileges for the finding class and scope.
func (u *UseCase) VisibleFindings(ctx context.Context, s entity.Subject, scope entity.Scope, operation string,
	targets []entity.FindingAccess,
) (map[entity.FindingAccess]bool, error) {
	out := make(map[entity.FindingAccess]bool, len(targets))
	if len(targets) == 0 {
		return out, nil
	}
	owners := map[entity.ID][]entity.ID{}
	if err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		links, err := u.structure.ProjectAreas(ctx, s.OrganizationID, u.clock.Now())
		if err != nil {
			return err
		}
		for _, l := range links {
			owners[l.To] = append(owners[l.To], l.From)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	projects, security := scope.Projects(), scope.SecurityProjects()
	areas := []entity.ID{}
	for _, target := range targets {
		can := func(project entity.ID) bool {
			return (scope.Whole() || slices.Contains(projects, project)) &&
				(!entity.SensitiveFinding(target.Kind) || slices.Contains(security, project))
		}
		ownersOfArea := owners[target.AreaID]
		allowed := slices.ContainsFunc(ownersOfArea, can)
		if len(ownersOfArea) == 0 {
			allowed = scope.Whole() && !entity.SensitiveFinding(target.Kind)
		}
		if target.WholeArea && slices.ContainsFunc(ownersOfArea, func(p entity.ID) bool { return !can(p) }) {
			allowed = false
		}
		if allowed {
			out[target] = true
			if !slices.Contains(areas, target.AreaID) {
				areas = append(areas, target.AreaID)
			}
		}
	}
	if len(areas) == 0 {
		return out, nil
	}
	visible, err := u.Decide(ctx, s, scope, entity.SourceQuery{Operation: operation, Areas: areas})
	if err != nil {
		return nil, err
	}
	for target := range out {
		out[target] = visible.Area(target.AreaID)
	}
	return out, nil
}
