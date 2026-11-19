package calc

import (
	"context"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_contributionPage = 200
	// _gcBatch is a portion of the collection of parts, one transaction each, as retention.apply (TZ 10.6).
	_gcBatch = 500
	// _gcAge keeps a part a day: a part saved by a chain is referred to by the time it commits.
	_gcAge = 24 * time.Hour
)

// Revision reads a revision of a series, the current one when revisionID is nil. The values are what the formula
// gave; view narrows them to what the reader may see (TZ 10.4, 12.7).
func (u *UseCase) Revision(ctx context.Context, orgID, seriesID entity.ID, revisionID *entity.ID) (entity.CalcRevision, error) {
	var out entity.CalcRevision
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		id := revisionID
		if id == nil {
			s, err := u.d.Repo.Series(ctx, orgID, seriesID)
			if err != nil {
				return err
			}
			if id = s.CurrentRevisionID; id == nil {
				return entity.ErrNotFound
			}
		}
		var err error
		if out, err = u.d.Repo.Revision(ctx, orgID, *id); err != nil {
			return err
		}
		if out.SeriesID != seriesID {
			return entity.ErrNotFound
		}
		return nil
	})
	return out, err
}

// RevisionPoints reads the points of every metric of a revision by metric, estimator and start.
func (u *UseCase) RevisionPoints(ctx context.Context, orgID, revisionID entity.ID) ([]entity.SeriesPoint, error) {
	var out []entity.SeriesPoint
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.RevisionPoints(ctx, orgID, revisionID)
		return err
	})
	return out, err
}

// PointContributions pages the cohort of a point by object and item.
func (u *UseCase) PointContributions(ctx context.Context, orgID, revisionID entity.ID, q entity.ContributionQuery) (entity.ContributionPage, error) {
	if q.Limit <= 0 {
		q.Limit = _contributionPage
	}
	var page entity.ContributionPage
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		items, err := u.d.Repo.PointContributions(ctx, orgID, revisionID, q)
		if err != nil {
			return err
		}
		if len(items) > q.Limit {
			items = items[:q.Limit]
			last := items[len(items)-1]
			page.Next = &entity.ContributionKey{ObjectID: last.ObjectID, ItemNo: last.ItemNo}
		}
		page.Items = items
		return nil
	})
	return page, err
}

// TaskIntervals reads the workflow intervals of a task in a revision.
func (u *UseCase) TaskIntervals(ctx context.Context, orgID, revisionID, taskID entity.ID) ([]entity.WorkflowInterval, error) {
	var out []entity.WorkflowInterval
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TaskIntervals(ctx, orgID, revisionID, taskID)
		return err
	})
	return out, err
}

// RevisionInputs reads the versions of the inputs a revision was computed with, for the passport of a metric.
func (u *UseCase) RevisionInputs(ctx context.Context, orgID, revisionID entity.ID) ([]entity.RevisionInput, error) {
	var out []entity.RevisionInput
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.RevisionInputs(ctx, orgID, revisionID)
		return err
	})
	return out, err
}

// MarkBasisDeleted marks the revisions an object deleted on request contributed to; their values stay (TZ 12.7).
func (u *UseCase) MarkBasisDeleted(ctx context.Context, orgID, objectID entity.ID) (int64, error) {
	var n int64
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		n, err = u.d.Repo.MarkBasisDeleted(ctx, orgID, objectID)
		return err
	})
	return n, err
}

// MarkBasisExpired marks the revisions whose slice is older than the oldest reproducible knowledge number.
func (u *UseCase) MarkBasisExpired(ctx context.Context, orgID entity.ID) (int64, error) {
	var n int64
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		_, from, err := u.d.Tenants.Clock(ctx, orgID)
		if err != nil {
			return err
		}
		n, err = u.d.Repo.MarkBasisExpired(ctx, orgID, from)
		return err
	})
	return n, err
}

// MarkDefinitionRetired marks the revisions computed with a definition the registry no longer has.
func (u *UseCase) MarkDefinitionRetired(ctx context.Context, orgID entity.ID) (int64, error) {
	current := make([]string, len(_definitions))
	for i := range _definitions {
		current[i] = _definitions[i].ID + "@" + strconv.Itoa(_definitions[i].Version)
	}
	var n int64
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		n, err = u.d.Repo.MarkDefinitionRetired(ctx, orgID, current)
		return err
	})
	return n, err
}

// TickGC queues the collection of parts of every active organization.
func (u *UseCase) TickGC(ctx context.Context) error {
	var orgs []entity.ID
	err := u.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		orgs, err = u.d.Tenants.ActiveTenants(ctx)
		return err
	})
	if err != nil {
		return err
	}
	for _, org := range orgs {
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			return u.d.Queue.Enqueue(ctx, entity.CalcGCJob{OrganizationID: org}, entity.QueueBulk)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RunGC is the job calc.gc: one portion of parts no live revision refers to, older than a day, goes in one
// transaction with the job of the next portion.
func (u *UseCase) RunGC(ctx context.Context, job entity.CalcGCJob, complete Complete) error {
	org := job.OrganizationID
	before := u.d.Clock.Now().Add(-_gcAge)
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		ids, err := u.d.Repo.UnusedParts(ctx, org, job.AfterPartID, before, u.d.GCBatch)
		if err != nil {
			return err
		}
		if err := u.d.Repo.DeleteParts(ctx, org, ids); err != nil {
			return err
		}
		if err := complete(ctx); err != nil {
			return err
		}
		if len(ids) < u.d.GCBatch {
			return nil
		}
		return u.d.Queue.Enqueue(ctx, entity.CalcGCJob{OrganizationID: org, AfterPartID: ids[len(ids)-1]}, entity.QueueBulk)
	})
}

// RevisionOf reads a revision by its ID; a revision of another set of metrics is entity.ErrNotFound.
func (u *UseCase) RevisionOf(ctx context.Context, orgID, revisionID entity.ID) (entity.CalcRevision, error) {
	var out entity.CalcRevision
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.Revision(ctx, orgID, revisionID)
		return err
	})
	return out, err
}

// RequestTeamRevision queues the recount of the series of a team, for a series that has no revision yet: its
// first chain starts after the delay that gathers the facts (TZ 10.6).
func (u *UseCase) RequestTeamRevision(ctx context.Context, orgID, teamID, corr entity.ID) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		return u.d.Queue.Enqueue(ctx, entity.CalcRecountJob{OrganizationID: orgID, TeamID: teamID, CorrelationID: corr}, entity.QueueBulk)
	})
}

// MetricDefinitions is the registry of the definitions in force as the API shows it.
func (*UseCase) MetricDefinitions() []entity.MetricDefinition {
	defs := Definitions()
	out := make([]entity.MetricDefinition, len(defs))
	for i := range defs {
		d := &defs[i]
		out[i] = entity.MetricDefinition{ID: d.ID, Version: d.Version, Name: d.Name, Unit: d.Unit, CatalogRef: d.CatalogRef, Parameters: d.Parameters}
	}
	return out
}
