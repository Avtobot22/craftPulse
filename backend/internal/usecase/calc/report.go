package calc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_operationReport = "getTeamReport"
	// _readAge is how long a cohort with the current places of its objects and an aggregate over it are kept
	// (TZ 10.4, 12.7); a decision of access lives as long.
	_readAge = 60 * time.Second
	// _cohortEntries and _reportEntries bound the caches of read revisions and of aggregates by reader and filter.
	_cohortEntries = 64
	_reportEntries = 512
	_cohortPage    = 50
)

// _stateMetrics describe a state at the end of a point: the cohort shows the interval of a task open then.
var _stateMetrics = map[string]bool{"T11": true, "T12": true, "T13": true, "T14": true}

type pointKey struct {
	metric, estimator string
	start             int64
}

func keyOf(metric, estimator string, start time.Time) pointKey {
	return pointKey{metric: metric, estimator: estimator, start: start.UnixNano()}
}

// cohort is a published revision as a reader sees it: its stored points, its contributions with the
// attributes of their tasks and where the objects are now. Nothing in it changes after it is built.
type cohort struct {
	at      time.Time
	points  []entity.SeriesPoint
	rows    map[pointKey][]entity.CohortRow
	objects map[entity.ID]entity.LinkEnd
	tasks   map[entity.ID]entity.CohortRow
	cross   map[crossKey]crossEvidence
}

// visibility is what the sources and the roles let a reader see of a cohort now.
type visibility struct {
	whole        bool
	projects     []entity.ID
	areas        []entity.ID
	objects      []entity.ID
	notes        []entity.AccessNote
	areaProjects map[entity.ID][]entity.ID
}

type report struct {
	at     time.Time
	series []entity.Series
	hidden bool
}

// VisibleSeries computes the series of a published revision over the part of its cohort the reader may see
// and the filter selects, by the definitions of the metrics; the facts are not read again (TZ 12.7). The
// result is kept 60 s by revision, visible areas and objects and filter, and is shared: callers do not change
// it.
func (u *UseCase) VisibleSeries(ctx context.Context, s entity.Subject, scope entity.Scope, rev entity.CalcRevision, teamID entity.ID,
	f entity.ReportFilter,
) (entity.VisibleSeries, error) {
	c, err := u.cohortOf(ctx, s.OrganizationID, rev.ID)
	if err != nil {
		return entity.VisibleSeries{}, err
	}
	v, err := u.visibility(ctx, s, scope, teamID, c)
	if err != nil {
		return entity.VisibleSeries{}, err
	}
	key := reportKey(rev.ID, &v, &f)
	if r, ok := u.reports.Get(key); ok && u.fresh(r.at) {
		return entity.VisibleSeries{Series: r.series, Notes: notesOf(&v, r.hidden)}, nil
	}
	out, err, _ := u.flight.Do("report:"+key, func() (any, error) {
		series, hidden := c.series(&v, &f)
		r := &report{at: u.d.Clock.Now(), series: series, hidden: hidden}
		u.reports.Add(key, r)
		return r, nil
	})
	if err != nil {
		return entity.VisibleSeries{}, err
	}
	r, _ := out.(*report) // the flight returns only reports
	return entity.VisibleSeries{Series: r.series, Notes: notesOf(&v, r.hidden)}, nil
}

// VisibleCohort pages the contributions of one point of a published revision the reader may see, with their
// objects; an unknown point is entity.ErrNotFound. A deleted object is shown without its content.
func (u *UseCase) VisibleCohort(ctx context.Context, s entity.Subject, scope entity.Scope, rev entity.CalcRevision, teamID entity.ID,
	q entity.CohortQuery,
) (entity.CohortPage, error) {
	org := s.OrganizationID
	c, err := u.cohortOf(ctx, org, rev.ID)
	if err != nil {
		return entity.CohortPage{}, err
	}
	if q.Estimator == "" {
		q.Estimator = firstEstimator(q.MetricID)
	}
	key := keyOf(q.MetricID, q.Estimator, q.PointStart)
	i := slices.IndexFunc(c.points, func(p entity.SeriesPoint) bool { return keyOf(p.MetricID, p.Estimator, p.Start) == key })
	if i < 0 {
		return entity.CohortPage{}, entity.ErrNotFound
	}
	end := c.points[i].End
	v, err := u.visibility(ctx, s, scope, teamID, c)
	if err != nil {
		return entity.CohortPage{}, err
	}
	if q.MetricID == "A22" && eventGroup(q.Estimator) != nil && !c.visibleEventGroup(&v, q.Estimator) {
		return entity.CohortPage{}, entity.ErrNotFound
	}
	if q.Limit <= 0 {
		q.Limit = _cohortPage
	}
	var page entity.CohortPage
	hidden := false
	rows := c.rows[key]
	for i := range rows {
		row := &rows[i]
		if !c.visible(&v, row) {
			hidden = true
			continue
		}
		contribution := row.Contribution
		var basis crossEvidence
		if isCrossMetric(q.MetricID) {
			var restricted bool
			basis, restricted = c.crossBasis(row, &v)
			if restricted {
				crossScore(&contribution, q.Estimator, &basis)
				hidden = true
			}
			contribution.Evidence = nil
		}
		if (q.Role != "" && contribution.Role != q.Role) || (q.After != nil && !after(&contribution, q.After)) || page.Next != nil {
			continue
		}
		if len(page.Items) == q.Limit {
			last := page.Items[len(page.Items)-1]
			page.Next = &entity.ContributionKey{ObjectID: last.ObjectID, ItemNo: last.ItemNo}
			continue
		}
		it := c.item(row)
		it.Contribution = contribution
		if isCrossMetric(q.MetricID) && !it.Object.Deleted {
			c.crossItem(&it, &basis)
		}
		page.Items = append(page.Items, it)
	}
	page.Notes = notesOf(&v, hidden)
	if _stateMetrics[q.MetricID] && len(page.Items) > 0 {
		if err := u.openIntervals(ctx, org, rev.ID, end, page.Items); err != nil {
			return entity.CohortPage{}, err
		}
	}
	return page, nil
}

// cohortOf reads a revision once per 60 s for all readers: one read of its points and contributions and one
// query of the current places of its objects.
func (u *UseCase) cohortOf(ctx context.Context, org, revisionID entity.ID) (*cohort, error) {
	if c, ok := u.cohorts.Get(revisionID); ok && u.fresh(c.at) {
		return c, nil
	}
	out, err, _ := u.flight.Do("cohort:"+revisionID.String(), func() (any, error) {
		c := &cohort{
			at: u.d.Clock.Now(), rows: map[pointKey][]entity.CohortRow{}, objects: map[entity.ID]entity.LinkEnd{},
			tasks: map[entity.ID]entity.CohortRow{}, cross: map[crossKey]crossEvidence{},
		}
		var rows []entity.CohortRow
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			var err error
			if c.points, err = u.d.Repo.RevisionPoints(ctx, org, revisionID); err != nil {
				return err
			}
			rows, err = u.d.Repo.RevisionCohort(ctx, org, revisionID)
			return err
		})
		if err != nil {
			return nil, err
		}
		ids, err := c.index(rows)
		if err != nil {
			return nil, err
		}
		objects, err := u.d.Facts.CurrentObjects(ctx, org, ids)
		if err != nil {
			return nil, err
		}
		for i := range objects {
			c.objects[objects[i].ID] = objects[i]
		}
		u.cohorts.Add(revisionID, c)
		return c, nil
	})
	if err != nil {
		return nil, err
	}
	c, _ := out.(*cohort) // the flight returns only cohorts
	return c, nil
}

func (c *cohort) index(rows []entity.CohortRow) ([]entity.ID, error) {
	seen := map[entity.ID]bool{}
	for i := range rows {
		r := &rows[i]
		k := keyOf(r.MetricID, r.Estimator, r.PointStart)
		c.rows[k] = append(c.rows[k], *r)
		switch r.ObjectKind {
		case entity.ObjectTask:
			seen[r.ObjectID] = true
			if r.ProjectID != nil {
				c.tasks[r.ObjectID] = *r
			}
		case entity.ObjectMergeRequest:
			seen[r.ObjectID] = true
		default:
		}
		if !isCrossMetric(r.MetricID) {
			continue
		}
		key := crossKey{start: r.PointStart.UnixNano(), task: r.ObjectID}
		if _, ok := c.cross[key]; ok {
			continue
		}
		var b crossEvidence
		if err := json.Unmarshal(r.Evidence, &b); err != nil {
			return nil, fmt.Errorf("calc: read cross-metric evidence: %w", err)
		}
		c.cross[key] = b
		for i := range b.MRs {
			m := &b.MRs[i]
			seen[m.ID] = true
			if m.PipelineID != nil {
				seen[*m.PipelineID] = true
			}
		}
	}
	ids := make([]entity.ID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids, nil
}

// visibility decides the areas of the team and the areas the objects of the cohort are in now, then the
// objects with their own restriction in the open ones (TZ 13.4, 13.5).
func (u *UseCase) visibility(ctx context.Context, s entity.Subject, scope entity.Scope, teamID entity.ID, c *cohort) (visibility, error) {
	org := s.OrganizationID
	links, err := u.d.Structure.TeamProjectAreas(ctx, org, teamID, u.d.Clock.Now())
	if err != nil {
		return visibility{}, err
	}
	var areas []entity.ID
	add := func(a entity.ID) {
		if !a.IsZero() && !slices.Contains(areas, a) {
			areas = append(areas, a)
		}
	}
	areaProjects := map[entity.ID][]entity.ID{}
	for _, l := range links {
		add(l.To)
		areaProjects[l.To] = append(areaProjects[l.To], l.From)
	}
	for _, o := range c.objects {
		add(o.AreaID)
	}
	if len(areas) > 0 {
		if scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _operationReport, Areas: areas}); err != nil {
			return visibility{}, err
		}
	}
	var restricted []entity.RestrictedObject
	for _, o := range c.objects {
		if o.Restricted && scope.Area(o.AreaID) {
			restricted = append(restricted, entity.RestrictedObject{ID: o.ID, AreaID: o.AreaID, Kind: o.Kind, NativeNS: o.NativeNS, NativeID: o.NativeID})
		}
	}
	if len(restricted) > 0 {
		slices.SortFunc(restricted, func(a, b entity.RestrictedObject) int { return strings.Compare(a.ID.String(), b.ID.String()) })
		if scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _operationReport, Objects: restricted}); err != nil {
			return visibility{}, err
		}
	}
	v := visibility{whole: scope.Whole(), projects: sortedIDs(scope.Projects()), objects: sortedIDs(scope.Objects()), notes: scope.Notes(), areaProjects: areaProjects}
	for _, a := range areas {
		if scope.Area(a) {
			v.areas = append(v.areas, a)
		}
	}
	v.areas = sortedIDs(v.areas)
	return v, nil
}

// visible tells whether a reader sees a contribution: by the current area of the object and its own
// restriction, a deleted object by the project it was counted in, a connection always.
func (c *cohort) visible(v *visibility, r *entity.CohortRow) bool {
	switch r.ObjectKind {
	case entity.ObjectTask:
		if _, ok := c.objects[r.ObjectID]; !ok {
			return r.ProjectID != nil && (v.whole || slices.Contains(v.projects, *r.ProjectID))
		}
		return c.objectVisible(v, r.ObjectID)
	case entity.ObjectMergeRequest:
		return c.objectVisible(v, r.ObjectID)
	default:
		return true
	}
}

func (c *cohort) objectVisible(v *visibility, id entity.ID) bool {
	o, ok := c.objects[id]
	return ok && slices.Contains(v.areas, o.AreaID) && (!o.Restricted || slices.Contains(v.objects, o.ID))
}

func (c *cohort) visibleEventGroup(v *visibility, estimator string) bool {
	for key, rows := range c.rows {
		if key.metric != "A22" || key.estimator != estimator {
			continue
		}
		for i := range rows {
			if c.visible(v, &rows[i]) {
				return true
			}
		}
	}
	return false
}

// series are the series of the cohort over what the reader sees and the filter selects, split by group; a
// point whose contributions all stay is the stored one. hidden tells that the rights hid a contribution.
func (c *cohort) series(v *visibility, f *entity.ReportFilter) (out []entity.Series, hidden bool) {
	groups := []*entity.SeriesGroup{nil}
	if f.GroupBy != entity.GroupNone {
		groups = c.groups(v, f)
	}
	for _, g := range groups {
		var cur *entity.Series
		for i := range c.points {
			sp := &c.points[i]
			if cur == nil || cur.MetricID != sp.MetricID || cur.Estimator != sp.Estimator {
				version := 0
				if def, ok := lookup(sp.MetricID); ok {
					version = def.Version
				}
				group := g
				if sp.MetricID == "A22" {
					if events := eventGroup(sp.Estimator); events != nil {
						if g != nil {
							events.TaskType, events.SizeLabel = g.TaskType, g.SizeLabel
						}
						group = events
					}
				}
				out = append(out, entity.Series{MetricID: sp.MetricID, DefinitionVersion: version, Unit: sp.Unit, Estimator: sp.Estimator, Group: group})
				cur = &out[len(out)-1]
			}
			pt, h := c.point(v, f, g, sp)
			hidden = hidden || h
			cur.Points = append(cur.Points, pt)
		}
	}
	out = slices.DeleteFunc(out, func(s entity.Series) bool {
		return s.MetricID == "A22" && !slices.ContainsFunc(s.Points, func(p entity.Point) bool { return p.N != nil && *p.N > 0 })
	})
	return out, hidden
}

func (c *cohort) point(v *visibility, f *entity.ReportFilter, g *entity.SeriesGroup, sp *entity.SeriesPoint) (entity.Point, bool) {
	rows := c.rows[keyOf(sp.MetricID, sp.Estimator, sp.Start)]
	kept := make([]entity.Contribution, 0, len(rows))
	hidden := false
	for i := range rows {
		r := &rows[i]
		if !c.visible(v, r) {
			hidden = true
			continue
		}
		if !selected(r, f, g) || r.ObjectKind == entity.ObjectMergeRequest && !c.mrSelected(v, r, f, g) {
			continue
		}
		row := r.Contribution
		if isCrossMetric(sp.MetricID) {
			b, restricted := c.crossBasis(r, v)
			if restricted {
				hidden = true
				crossScore(&row, sp.Estimator, &b)
			}
		}
		kept = append(kept, row)
	}
	pt := sp.Point
	if regathered(sp) && (hidden || len(kept) < len(rows)) {
		pt = regather(sp, kept)
	}
	pt.Contributions = nil
	return pt, hidden
}

// selected tells whether a contribution of a task passes the filter and belongs to the group; a connection
// belongs to every one.
func selected(r *entity.CohortRow, f *entity.ReportFilter, g *entity.SeriesGroup) bool {
	if r.ObjectKind != entity.ObjectTask {
		return true
	}
	switch {
	case len(f.Projects) > 0 && (r.ProjectID == nil || !slices.Contains(f.Projects, *r.ProjectID)):
		return false
	case f.TaskType != nil && !sameLabel(f.TaskType, r.TaskType):
		return false
	case f.UnknownSize && r.SizeLabel != nil:
		return false
	case f.SizeLabel != nil && !sameLabel(f.SizeLabel, r.SizeLabel):
		return false
	case g == nil:
		return true
	case f.GroupBy == entity.GroupTaskType:
		return sameLabel(g.TaskType, r.TaskType)
	}
	return sameLabel(g.SizeLabel, r.SizeLabel)
}

// groups are the labels of the visible tasks the filter selects, by name with the group without one last.
func (c *cohort) groups(v *visibility, f *entity.ReportFilter) []*entity.SeriesGroup {
	var labels []*string
	seen := map[string]bool{}
	for _, rows := range c.rows {
		for i := range rows {
			r := &rows[i]
			if r.ObjectKind != entity.ObjectTask || !c.visible(v, r) || !selected(r, f, nil) {
				continue
			}
			label := r.SizeLabel
			if f.GroupBy == entity.GroupTaskType {
				label = r.TaskType
			}
			name := "\x00"
			if label != nil {
				name = *label
			}
			if !seen[name] {
				seen[name] = true
				labels = append(labels, label)
			}
		}
	}
	slices.SortFunc(labels, compareLabels)
	out := make([]*entity.SeriesGroup, len(labels))
	for i, l := range labels {
		if f.GroupBy == entity.GroupTaskType {
			out[i] = &entity.SeriesGroup{TaskType: l}
		} else {
			out[i] = &entity.SeriesGroup{SizeLabel: l}
		}
	}
	return out
}

func (c *cohort) item(r *entity.CohortRow) entity.CohortItem {
	it := entity.CohortItem{Contribution: r.Contribution, TaskType: r.TaskType, SizeLabel: r.SizeLabel}
	it.Object = entity.CohortObject{ID: r.ObjectID, Kind: r.ObjectKind}
	if r.ObjectKind != entity.ObjectTask && r.ObjectKind != entity.ObjectMergeRequest {
		return it
	}
	o, ok := c.objects[r.ObjectID]
	if !ok {
		it.Object.Deleted = true
		it.TaskType, it.SizeLabel = nil, nil
		return it
	}
	it.Object.InstanceID, it.Object.DisplayKey, it.Object.Title = o.InstanceID, o.DisplayKey, o.Title
	return it
}

// openIntervals sets on every item the interval of its task open at the end of the point.
func (u *UseCase) openIntervals(ctx context.Context, org, revisionID entity.ID, end time.Time, items []entity.CohortItem) error {
	var tasks []entity.ID
	for i := range items {
		if !items[i].Object.Deleted && !slices.Contains(tasks, items[i].ObjectID) {
			tasks = append(tasks, items[i].ObjectID)
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	var ivs []entity.WorkflowInterval
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		ivs, err = u.d.Repo.TasksIntervals(ctx, org, revisionID, tasks)
		return err
	})
	if err != nil {
		return err
	}
	for i := range items {
		j := slices.IndexFunc(ivs, func(iv entity.WorkflowInterval) bool {
			return iv.TaskID == items[i].ObjectID && iv.From.Before(end) && !iv.To.Before(end)
		})
		if j >= 0 {
			iv := ivs[j]
			items[i].Interval = &iv
		}
	}
	return nil
}

func (u *UseCase) fresh(at time.Time) bool { return u.d.Clock.Now().Sub(at) < _readAge }

// notesOf marks a limited answer: the notes of access, or that the rights hid a part of the cohort.
func notesOf(v *visibility, hidden bool) []entity.AccessNote {
	if len(v.notes) > 0 || !hidden {
		return v.notes
	}
	return []entity.AccessNote{{Code: entity.NoteRestricted}}
}

func reportKey(revisionID entity.ID, v *visibility, f *entity.ReportFilter) string {
	var b strings.Builder
	b.WriteString(revisionID.String())
	for _, list := range [][]entity.ID{v.areas, v.objects, v.projects, sortedIDs(f.Projects)} {
		b.WriteByte('|')
		for _, id := range list {
			b.WriteString(id.String())
		}
	}
	b.WriteString("|" + strconv.FormatBool(v.whole) + "|" + strconv.FormatBool(f.UnknownSize) + "|" + string(f.GroupBy))
	for _, label := range []*string{f.TaskType, f.SizeLabel} {
		b.WriteByte('|')
		if label != nil {
			b.WriteString(hex.EncodeToString([]byte(*label)))
		}
	}
	return b.String()
}

func after(c *entity.Contribution, k *entity.ContributionKey) bool {
	if c.ObjectID != k.ObjectID {
		return c.ObjectID.String() > k.ObjectID.String()
	}
	return c.ItemNo > k.ItemNo
}

func sortedIDs(ids []entity.ID) []entity.ID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b entity.ID) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(out)
}

func firstEstimator(metric string) string {
	if def, ok := lookup(metric); ok && len(def.Estimators) > 0 {
		return def.Estimators[0]
	}
	return ""
}
