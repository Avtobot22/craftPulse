package calc

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_operationPreview = "previewMapping"
	// _previewTasks bounds the sample of a preview (FR-017).
	_previewTasks = 20
)

// _controls are the control metrics of a preview, the three of the manual check of the demo set.
var _controls = []struct{ metric, estimator string }{{"T05", ""}, {"T08", EstimatorP50}, {"T12", ""}}

// _sampleKinds are the kinds of tasks a process owner checks a mapping on; a sample takes one of each first.
var _sampleKinds = []func(t *entity.PreviewTask) bool{
	func(t *entity.PreviewTask) bool { return t.Outcome == entity.PreviewOpen && t.StartedAt != nil },
	func(t *entity.PreviewTask) bool { return t.Outcome == entity.PreviewOpen && t.StartedAt == nil },
	func(t *entity.PreviewTask) bool { return t.Outcome == entity.PreviewCompleted },
	func(t *entity.PreviewTask) bool {
		return t.Outcome == entity.PreviewCancelled || t.Outcome == entity.PreviewDuplicate
	},
	func(t *entity.PreviewTask) bool { return t.BlockedSeconds != nil && *t.BlockedSeconds > 0 },
	// The size is taken at the first start (BR-007), so a task without a size is a started one.
	func(t *entity.PreviewTask) bool { return t.StartedAt != nil && t.SizeLabel == nil },
	func(t *entity.PreviewTask) bool { return t.Outcome == entity.PreviewUnknown || len(t.Unmapped) > 0 },
}

// PreviewMapping applies a draft of the mapping of a team to the tasks of its projects the subject may read now
// and computes the control metrics by the definitions of the report (UC-07, FR-017). Nothing is saved: the
// draft is checked by process, read at the current knowledge number and dropped.
func (u *UseCase) PreviewMapping(ctx context.Context, s entity.Subject, scope entity.Scope, teamID entity.ID,
	d entity.MappingDraft,
) (entity.MappingPreview, error) {
	org := s.OrganizationID
	if err := u.d.Settings.CheckDraft(ctx, org, teamID, d); err != nil {
		return entity.MappingPreview{}, err
	}
	loc, err := time.LoadLocation(d.Timezone)
	if err != nil {
		return entity.MappingPreview{}, fmt.Errorf("calc: time zone of the draft: %w", err)
	}
	var seq int64
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		seq, _, err = u.d.Tenants.Clock(ctx, org)
		return err
	})
	if err != nil {
		return entity.MappingPreview{}, err
	}
	now := u.d.Clock.Now()
	asOf := now.UTC().Truncate(time.Second)
	tasks, err := u.visibleTasks(ctx, s, scope, teamID, seq, now)
	if err != nil {
		return entity.MappingPreview{}, err
	}
	mapping := entity.MappingVersion{TeamID: teamID, Timezone: d.Timezone, CalendarVersionNo: d.CalendarVersionNo, Rules: d.Rules}
	in := input{mapping: &mapping, team: teamID, window: controlWindow(asOf, loc), asOf: asOf, location: loc, tasks: tasks}
	return preview(in), nil
}

// visibleTasks are the tasks of the projects of the team the sources allow the subject now: the areas first,
// then the objects with their own restriction in the open ones (TZ 13.4, 13.5). The projects are those of the
// current structure, as in Calculate.
func (u *UseCase) visibleTasks(ctx context.Context, s entity.Subject, scope entity.Scope, teamID entity.ID, seq int64,
	now time.Time,
) ([]taskInput, error) {
	org := s.OrganizationID
	links, err := u.d.Structure.TeamProjectAreas(ctx, org, teamID, now)
	if err != nil {
		return nil, err
	}
	var areas []entity.ID
	project := map[entity.ID]entity.ID{}
	for _, l := range links {
		if _, ok := project[l.To]; ok {
			continue
		}
		project[l.To] = l.From
		areas = append(areas, l.To)
	}
	if len(areas) == 0 {
		return nil, nil
	}
	if scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _operationPreview, Areas: areas}); err != nil {
		return nil, err
	}
	objects, err := u.d.Facts.RestrictedObjects(ctx, scope, org, areas)
	if err != nil {
		return nil, err
	}
	if len(objects) > 0 {
		scope, err = u.d.Rights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _operationPreview, Objects: objects})
		if err != nil {
			return nil, err
		}
	}
	var out []taskInput
	for _, a := range areas {
		facts, err := u.d.Facts.VisibleAreaFacts(ctx, scope, org, a, seq)
		if err != nil {
			return nil, err
		}
		out = append(out, tasksOf(&facts, project[a])...)
	}
	return out, nil
}

// controlWindow is the week of the report up to as_of: from Monday 00:00 in the time zone of the team, the week
// before when as_of is that midnight itself.
func controlWindow(asOf time.Time, loc *time.Location) entity.Window {
	t := asOf.In(loc)
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
	if !start.Before(asOf) {
		start = start.AddDate(0, 0, -7)
	}
	return entity.Window{From: start.UTC(), To: asOf, Step: entity.StepWeek}
}

// preview is the draft of the input applied to its tasks: the sample, the control metrics over the window and
// the statuses without a rule the tasks passed.
func preview(in input) entity.MappingPreview {
	pr := prepare(in)
	res := pr.calculate(periods(in.window, in.asOf, in.location))
	byTask := make(map[entity.ID]*taskInput, len(in.tasks))
	for i := range in.tasks {
		byTask[in.tasks[i].version.TaskID] = &in.tasks[i]
	}
	return entity.MappingPreview{
		AsOf: in.asOf, Period: in.window, Tasks: sample(pr.tls, byTask), Controls: controls(res.Series),
		Unmapped: unmappedStatuses(pr.tls, byTask),
	}
}

// sample takes up to _previewTasks tasks, the most recently changed first: one of every kind of _sampleKinds the
// team has, then the rest. The tasks come in the order of their creation.
func sample(tls []*timeline, byTask map[entity.ID]*taskInput) []entity.PreviewTask {
	order := slices.Clone(tls)
	slices.SortFunc(order, func(a, b *timeline) int {
		return cmp.Or(b.lastChange().Compare(a.lastChange()), bytes.Compare(a.slice.TaskID[:], b.slice.TaskID[:]))
	})
	views := make([]entity.PreviewTask, len(order))
	for i, tl := range order {
		views[i] = tl.preview(&byTask[tl.slice.TaskID].version)
	}
	picked := make([]bool, len(views))
	out := make([]entity.PreviewTask, 0, min(len(views), _previewTasks))
	for _, kind := range _sampleKinds {
		for i := range views {
			if !picked[i] && kind(&views[i]) {
				picked[i] = true
				out = append(out, views[i])
				break
			}
		}
	}
	for i := range views {
		if len(out) == _previewTasks {
			break
		}
		if !picked[i] {
			out = append(out, views[i])
		}
	}
	slices.SortFunc(out, func(a, b entity.PreviewTask) int {
		return cmp.Or(a.Created.Compare(b.Created), bytes.Compare(a.TaskID[:], b.TaskID[:]))
	})
	return out
}

// lastChange is the moment of the last change replayed, the creation without one.
func (tl *timeline) lastChange() time.Time {
	if n := len(tl.events); n > 0 {
		return tl.events[n-1].EventTime
	}
	return tl.slice.Created
}

// preview is the task as the draft reads it at as_of.
func (tl *timeline) preview(v *entity.TaskVersion) entity.PreviewTask {
	s := &tl.slice
	out := entity.PreviewTask{
		TaskID: s.TaskID, DisplayKey: v.TaskKey, Title: v.Title, Created: s.Created, Outcome: tl.outcomeAt(),
		TaskType: s.TaskType, SizeLabel: s.SizeLabel, Unmapped: slices.Clone(s.Unmapped),
	}
	if start := tl.observedStart(); start != nil {
		at := *start
		out.StartedAt = &at
	}
	if len(tl.completions) > 0 {
		at := tl.completions[0]
		out.CompletedAt = &at
	}
	if !s.Conflict {
		var blocked time.Duration
		for _, iv := range s.Intervals {
			if iv.Class == entity.IntervalBlocked {
				blocked += iv.To.Sub(iv.From)
			}
		}
		seconds := int64(blocked / time.Second)
		out.BlockedSeconds = &seconds
	}
	return out
}

// outcomeAt is the state of the task at as_of; a current status or resolution without a rule leaves it unknown,
// as a conflict of the order of its events does.
func (tl *timeline) outcomeAt() entity.PreviewOutcome {
	if tl.slice.Conflict {
		return entity.PreviewUnknown
	}
	meaning, ok := tl.rules.status[tl.values[_fieldStatus]]
	if !ok || (meaning == entity.MeaningDone && tl.rules.resolved(tl.values[_fieldResolution]) == outcomeUnknown) {
		return entity.PreviewUnknown
	}
	if tl.phase != phaseTerminal {
		return entity.PreviewOpen
	}
	switch tl.term {
	case outcomeCompleted:
		return entity.PreviewCompleted
	case outcomeCancelled:
		return entity.PreviewCancelled
	case outcomeDuplicate:
		return entity.PreviewDuplicate
	case outcomeUnknown, outcomeIntake, outcomeWork:
	}
	return entity.PreviewUnknown
}

// controls are the points of the control metrics over the window of the preview.
func controls(series []entity.Series) []entity.ControlValue {
	out := make([]entity.ControlValue, 0, len(_controls))
	for _, c := range _controls {
		i := slices.IndexFunc(series, func(s entity.Series) bool {
			return s.MetricID == c.metric && s.Estimator == c.estimator && s.Group == nil
		})
		if i < 0 || len(series[i].Points) == 0 {
			continue
		}
		s := &series[i]
		out = append(out, entity.ControlValue{MetricID: s.MetricID, Estimator: s.Estimator, Unit: s.Unit, Point: s.Points[len(s.Points)-1]})
	}
	return out
}

// unmappedStatuses are the statuses without a rule every task passed, each once, by instance and ID.
func unmappedStatuses(tls []*timeline, byTask map[entity.ID]*taskInput) []entity.NativeStatusRef {
	seen := map[entity.NativeStatusRef]bool{}
	out := []entity.NativeStatusRef{}
	for _, tl := range tls {
		for _, st := range tl.slice.Unmapped {
			ref := entity.NativeStatusRef{InstanceID: byTask[tl.slice.TaskID].instance, NativeID: st}
			if !seen[ref] {
				seen[ref] = true
				out = append(out, ref)
			}
		}
	}
	slices.SortFunc(out, func(a, b entity.NativeStatusRef) int {
		return cmp.Or(bytes.Compare(a.InstanceID[:], b.InstanceID[:]), cmp.Compare(a.NativeID, b.NativeID))
	})
	return out
}
