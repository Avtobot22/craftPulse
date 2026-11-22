package view

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type ReviewCalculator interface {
	MRReviewsAt(context.Context, entity.Scope, entity.ID, entity.ID, int64, time.Time) ([]entity.MRReview, error)
}

type ProjectAreas interface {
	ProjectAreas(context.Context, entity.ID, time.Time) ([]entity.Link, error)
}

func (u *UseCase) ListMergeRequests(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.MRQueueQuery) (entity.MRQueuePage, error) {
	if q.Limit < 1 {
		return entity.MRQueuePage{}, entity.ErrInvalidCursor
	}
	now := u.d.Clock.Now()
	at := now
	var known *int64
	if q.After != nil {
		at, known = q.After.AsOf, &q.After.Seq
		if at.IsZero() || at.After(now) || now.Sub(at) > _keepFor {
			return entity.MRQueuePage{}, entity.ErrInvalidCursor
		}
	}
	seq, err := u.d.Code.ReportKnowledge(ctx, s.OrganizationID, known)
	if err != nil {
		return entity.MRQueuePage{}, err
	}
	areas, err := u.queueAreas(ctx, s.OrganizationID, scope, &q, now)
	if err != nil || len(areas) == 0 {
		return entity.MRQueuePage{}, err
	}
	scope, restricted, err := u.queueScope(ctx, s, scope, areas)
	if err != nil {
		return entity.MRQueuePage{}, err
	}
	page := entity.MRQueuePage{Notes: scope.Notes()}
	if restricted && len(page.Notes) == 0 {
		page.Notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
	}
	var rows []entity.MRReview
	for _, area := range areas {
		if !scope.Area(area) {
			continue
		}
		list, err := u.d.ReviewCalculator.MRReviewsAt(ctx, scope, s.OrganizationID, area, seq, at)
		if err != nil {
			return entity.MRQueuePage{}, err
		}
		for i := range list {
			m := &list[i]
			if m.Version.State != q.State || m.Version.OpenedAt.After(at) || (q.Wait != "" && (m.Wait == nil || m.Wait.Kind != q.Wait)) {
				continue
			}
			if q.After == nil || compareQueue(queuePosition(m, at, seq), *q.After, q.Sort) > 0 {
				rows = append(rows, *m)
			}
		}
	}
	slices.SortFunc(rows, func(a, b entity.MRReview) int {
		return compareQueue(queuePosition(&a, at, seq), queuePosition(&b, at, seq), q.Sort)
	})
	if len(rows) > q.Limit {
		next := queuePosition(&rows[q.Limit-1], at, seq)
		page.Next = &next
		rows = rows[:q.Limit]
	}
	page.Items, err = u.queueDetails(ctx, s.OrganizationID, scope, rows)
	return page, err
}

func (u *UseCase) queueAreas(ctx context.Context, org entity.ID, scope entity.Scope, q *entity.MRQueueQuery, now time.Time) ([]entity.ID, error) {
	if q.Project == nil && q.Area == nil && scope.Whole() {
		return u.d.Code.AllRepositoryAreas(ctx, org)
	}
	if q.Project == nil && q.Area != nil {
		return u.d.Code.RepositoryAreas(ctx, org, []entity.ID{*q.Area})
	}
	projects := scope.Projects()
	if q.Project != nil && !scope.Whole() && !slices.Contains(projects, *q.Project) {
		return nil, nil
	}
	var links []entity.Link
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		links, err = u.d.ProjectAreas.ProjectAreas(ctx, org, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	var ids []entity.ID
	for _, link := range links {
		if (q.Project != nil && link.From != *q.Project) || (q.Project == nil && !slices.Contains(projects, link.From)) || (q.Area != nil && link.To != *q.Area) {
			continue
		}
		ids = append(ids, link.To)
	}
	return u.d.Code.RepositoryAreas(ctx, org, ids)
}

func (u *UseCase) queueScope(ctx context.Context, s entity.Subject, scope entity.Scope, areas []entity.ID) (entity.Scope, bool, error) {
	const op = "listMergeRequests"
	scope, err := u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: op, Areas: areas})
	if err != nil {
		return scope, false, err
	}
	objects, err := u.d.Code.RestrictedObjects(ctx, scope, s.OrganizationID, areas)
	if err != nil {
		return scope, false, err
	}
	objects = slices.DeleteFunc(objects, func(o entity.RestrictedObject) bool { return o.Kind != entity.ObjectMergeRequest })
	if len(objects) > 0 {
		scope, err = u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: op, Objects: objects})
		if err != nil {
			return scope, false, err
		}
	}
	allowed := scope.Objects()
	return scope, slices.ContainsFunc(areas, func(id entity.ID) bool { return !scope.Area(id) }) ||
		slices.ContainsFunc(objects, func(o entity.RestrictedObject) bool { return !slices.Contains(allowed, o.ID) }), nil
}

func (u *UseCase) queueDetails(ctx context.Context, org entity.ID, scope entity.Scope, rows []entity.MRReview) ([]entity.MRDetail, error) {
	ids := make([]entity.ID, len(rows))
	for i := range rows {
		ids[i] = rows[i].Version.MRID
	}
	ends, err := u.d.Code.CurrentObjects(ctx, org, ids)
	if err != nil {
		return nil, err
	}
	instances := make(map[entity.ID]entity.ID, len(ends))
	for _, end := range ends {
		instances[end.ID] = end.InstanceID
	}
	repos := map[entity.ID]entity.RepositoryVersion{}
	providers := map[entity.ID]entity.Provider{}
	out := make([]entity.MRDetail, len(rows))
	for i := range rows {
		row := &rows[i]
		v := &row.Version
		if _, ok := repos[v.AreaID]; !ok {
			repository, err := u.d.Code.Repository(ctx, scope, org, v.AreaID)
			if err != nil && !errors.Is(err, entity.ErrNotFound) {
				return nil, err
			}
			repos[v.AreaID] = repository
		}
		detail := entity.MRDetail{Version: *v, Repository: repos[v.AreaID], AuthorKind: row.AuthorKind, ReviewWait: row.Wait}
		if instance := instances[v.MRID]; !instance.IsZero() && v.IID != nil && detail.Repository.WebURL != "" {
			if _, ok := providers[instance]; !ok {
				source, err := u.d.Instances.Instance(ctx, org, instance)
				if err != nil {
					return nil, err
				}
				providers[instance] = source.Provider
			}
			detail.SourceURL = mergeRequestURL(detail.Repository.WebURL, providers[instance], *v.IID)
		}
		out[i] = detail
	}
	return out, nil
}

func queuePosition(m *entity.MRReview, at time.Time, seq int64) entity.MRQueueCursor {
	p := entity.MRQueueCursor{AsOf: at, Seq: seq, OpenedAt: m.Version.OpenedAt, ID: m.Version.MRID, WaitSeconds: -1}
	if m.Wait != nil {
		p.WaitSeconds = m.Wait.WaitSeconds
	}
	return p
}

func compareQueue(a, b entity.MRQueueCursor, sort string) int {
	var order int
	switch sort {
	case "wait":
		order = cmp.Compare(a.WaitSeconds, b.WaitSeconds)
	case "opened_at":
		order = a.OpenedAt.Compare(b.OpenedAt)
	case "-opened_at":
		order = b.OpenedAt.Compare(a.OpenedAt)
	default:
		order = cmp.Compare(b.WaitSeconds, a.WaitSeconds)
	}
	if order == 0 {
		order = bytes.Compare(a.ID[:], b.ID[:])
	}
	return order
}
