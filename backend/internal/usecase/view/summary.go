package view

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Summaries reads stored summary revisions and their immutable rows.
type Summaries interface {
	ReadSummary(context.Context, entity.ID, entity.ID, *int, *string) (entity.SummaryRead, error)
	ReadSummaryItems(context.Context, entity.ID, entity.ID) ([]entity.SummaryItem, error)
}

// SummaryFindingRights authorizes reading security findings in the MR area.
type SummaryFindingRights interface {
	VisibleFindings(context.Context, entity.Subject, entity.Scope, string, []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

// MergeRequestSummary checks current MR access before reading the requested revision.
func (u *UseCase) MergeRequestSummary(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, no *int, head *string) (entity.SummaryRead, error) {
	scope, ends, err := u.codeScope(ctx, s, scope, "getMergeRequestSummary", []entity.ID{id})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	end, ok := ends[id]
	if !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.SummaryRead{}, entity.ErrNotFound
	}
	var out entity.SummaryRead
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Summaries.ReadSummary(ctx, s.OrganizationID, id, no, head)
		if err != nil {
			return err
		}
		out.Items, err = u.d.Summaries.ReadSummaryItems(ctx, s.OrganizationID, out.ID)
		return err
	})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	allowed := slices.Contains(scope.SecurityProjects(), out.ProjectID)
	if allowed {
		target := entity.FindingAccess{AreaID: end.AreaID, Kind: "security"}
		rights, err := u.d.SummaryFindingRights.VisibleFindings(ctx, s, scope, "getMergeRequestSummary", []entity.FindingAccess{target})
		if err != nil {
			return entity.SummaryRead{}, err
		}
		allowed = rights[target]
	}
	out.FilterSecurity(allowed)
	return out, nil
}
