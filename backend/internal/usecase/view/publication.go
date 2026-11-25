package view

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// SummaryPublications reads the stored outcome from change.
type SummaryPublications interface {
	Publication(context.Context, entity.ID, entity.ID, entity.ID) (entity.Publication, error)
}

// SummaryPublication returns the external publication outcome after checking source visibility.
func (u *UseCase) SummaryPublication(ctx context.Context, s entity.Subject, scope entity.Scope, mr, revision entity.ID) (entity.Publication, error) {
	_, visible, err := u.codeScope(ctx, s, scope, "getMergeRequestSummary", []entity.ID{mr})
	if err != nil {
		return entity.Publication{}, err
	}
	if end, ok := visible[mr]; !ok || end.Kind != entity.ObjectMergeRequest {
		return entity.Publication{}, entity.ErrNotFound
	}
	return u.d.Publications.Publication(ctx, s.OrganizationID, mr, revision)
}
