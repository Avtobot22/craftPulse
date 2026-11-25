package normalize

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) TaskViewFacts(ctx context.Context, org, area, task entity.ID, seq int64, allowed []entity.ID) ([]entity.TaskVersion, []entity.TaskEvent, error) {
	q := r.q(ctx)
	versions, err := q.ReadTaskViewVersion(ctx, ReadTaskViewVersionParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), TaskID: uuid.UUID(task), KnownSeq: seq, Allowed: ids(allowed),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("normalize: task detail version: %w", err)
	}
	if len(versions) == 0 {
		return nil, nil, nil
	}
	events, err := q.ReadTaskViewEvents(ctx, ReadTaskViewEventsParams{
		OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), TaskID: uuid.UUID(task), KnownSeq: seq, Allowed: ids(allowed),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("normalize: task detail events: %w", err)
	}
	return taskVersions(versions), taskEvents(events), nil
}
