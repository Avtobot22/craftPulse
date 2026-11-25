package view

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// SummaryPublications reads the saved revision and outcome from change.
type SummaryPublications interface {
	ReadSummary(context.Context, entity.ID, entity.ID, *int) (entity.SummaryRevision, error)
}

// SummaryFindingRights authorizes reading security findings in the MR area.
type SummaryFindingRights interface {
	VisibleFindings(context.Context, entity.Subject, entity.Scope, string, []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

// MergeRequestSummary returns the requested revision for the current audience.
func (u *UseCase) MergeRequestSummary(ctx context.Context, s entity.Subject, scope entity.Scope, mr entity.ID, no *int) (entity.SummaryRevision, bool, error) {
	const op = "getMergeRequestSummary"
	scope, visible, err := u.codeScope(ctx, s, scope, op, []entity.ID{mr})
	if err != nil {
		return entity.SummaryRevision{}, false, err
	}
	end, ok := visible[mr]
	if !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.SummaryRevision{}, false, entity.ErrNotFound
	}
	v, err := u.d.Publications.ReadSummary(ctx, s.OrganizationID, mr, no)
	if err != nil {
		return entity.SummaryRevision{}, false, err
	}
	hidden := slices.ContainsFunc(v.Items, func(item entity.SummaryItem) bool { return item.RequiredRight == "security" })
	if hidden {
		target := entity.FindingAccess{AreaID: end.AreaID, Kind: "security"}
		allowed, err := u.d.SummaryFindingRights.VisibleFindings(ctx, s, scope, op, []entity.FindingAccess{target})
		if err != nil {
			return entity.SummaryRevision{}, false, err
		}
		hidden = !allowed[target]
		if hidden {
			v.Items = slices.DeleteFunc(v.Items, func(item entity.SummaryItem) bool { return item.RequiredRight == "security" })
			entity.RankSummary(v.Items)
		}
	}
	return v, hidden, nil
}
