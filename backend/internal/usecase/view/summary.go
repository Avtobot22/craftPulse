package view

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Summaries reads stored summary revisions and their immutable rows.
type Summaries interface {
	ReadSummary(context.Context, entity.ID, entity.ID, *int, *string) (entity.SummaryRead, error)
	SummarySourceAreas(context.Context, entity.ID, entity.ID) ([]entity.ID, error)
	ReadSummaryItems(context.Context, entity.ID, entity.ID) ([]entity.SummaryItem, error)
}

// SummaryFindingRights authorizes reading security findings in the MR area.
type SummaryFindingRights interface {
	VisibleFindings(context.Context, entity.Subject, entity.Scope, string, []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

// MergeRequestSummary checks current MR access before reading the requested revision.
func (u *UseCase) MergeRequestSummary(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, no *int, head *string) (entity.SummaryRead, error) {
	const operation = "getMergeRequestSummary"
	scope, ends, err := u.codeScope(ctx, s, scope, operation, []entity.ID{id})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	end, ok := ends[id]
	if !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.SummaryRead{}, entity.ErrNotFound
	}
	var out entity.SummaryRead
	var areas []entity.ID
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Summaries.ReadSummary(ctx, s.OrganizationID, id, no, head)
		if err != nil {
			return err
		}
		areas, err = u.d.Summaries.SummarySourceAreas(ctx, s.OrganizationID, out.ID)
		if err != nil {
			return err
		}
		out.Items, err = u.d.Summaries.ReadSummaryItems(ctx, s.OrganizationID, out.ID)
		return err
	})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	if len(areas) > 0 {
		scope, err = u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: operation, Areas: areas})
		if err != nil {
			return entity.SummaryRead{}, err
		}
		out.SourceRestricted = slices.ContainsFunc(areas, func(area entity.ID) bool { return !scope.Area(area) })
		out.Items = slices.DeleteFunc(out.Items, func(item entity.SummaryItem) bool {
			return item.SourceAreaID != nil && !scope.Area(*item.SourceAreaID)
		})
	}
	allowed := slices.Contains(scope.SecurityProjects(), out.ProjectID)
	if allowed {
		target := entity.FindingAccess{AreaID: end.AreaID, Kind: "security"}
		rights, err := u.d.SummaryFindingRights.VisibleFindings(ctx, s, scope, operation, []entity.FindingAccess{target})
		if err != nil {
			return entity.SummaryRead{}, err
		}
		allowed = rights[target]
	}
	out.FilterSecurity(allowed)
	if out.SourceRestricted && !slices.Contains(out.Redactions, "items") {
		out.Redactions = append(out.Redactions, "items")
	}
	return out, nil
}
