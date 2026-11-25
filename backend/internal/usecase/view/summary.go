package view

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// Summaries reads stored summary revisions and their immutable rows.
type Summaries interface {
	ReadSummary(context.Context, entity.ID, entity.ID, *int) (entity.SummaryRead, error)
	ReadSummaryItems(context.Context, entity.ID, entity.ID) ([]entity.SummaryItem, error)
}

// MergeRequestSummary checks current MR access before reading the requested revision.
func (u *UseCase) MergeRequestSummary(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, no *int) (entity.SummaryRead, error) {
	scope, ends, err := u.codeScope(ctx, s, scope, "getMergeRequestSummary", []entity.ID{id})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	end, ok := ends[id]
	if !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.SummaryRead{}, entity.ErrNotFound
	}
	if no != nil && *no < 1 {
		return entity.SummaryRead{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/revision_no", Code: entity.FieldOutOfRange}}}
	}
	var out entity.SummaryRead
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Summaries.ReadSummary(ctx, s.OrganizationID, id, no)
		if err != nil {
			return err
		}
		out.Items, err = u.d.Summaries.ReadSummaryItems(ctx, s.OrganizationID, out.ID)
		return err
	})
	if err != nil {
		return entity.SummaryRead{}, err
	}
	out.FilterSecurity(slices.Contains(scope.SecurityProjects(), out.ProjectID))
	return out, nil
}
