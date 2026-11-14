package calc

import (
	"cmp"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// input is everything a calculation of the flow of a team reads; without a mapping every point is
// NOT_EVALUATED with mapping_missing.
type input struct {
	mapping  *entity.MappingVersion
	team     entity.ID
	window   entity.Window
	asOf     time.Time
	location *time.Location
	groupBy  entity.GroupBy
	tasks    []taskInput
}

// pointCalc builds the point of one metric over a period from the timelines.
type pointCalc func(tls []*timeline, p period) []entity.Point

var _metrics = map[string]pointCalc{
	"T04": startedPoint, "T05": completedPoint, "T07": cancelledPoint, "T08": cycleTimePoints, "T12": wipPoint,
	"T13": blockedWIPPoint, "T14": agePoints, "T15": blockedTimePoint, "T18": balancePoint,
}

// compute is the flow of a team as a pure function of its facts, settings, window and as_of (TZ 10.3, rule 5).
func compute(in input) entity.CalcResult {
	return prepare(in).calculate(periods(in.window, in.asOf, in.location))
}

// prepared is the timelines of the tasks of a calculation; a window is computed from them part by part.
type prepared struct {
	in  input
	tls []*timeline
}

// prepare replays the history of every task of the team known at as_of; without a mapping nothing is replayed.
func prepare(in input) *prepared {
	pr := &prepared{in: in}
	if in.mapping == nil {
		return pr
	}
	byInstance := rulesOf(in.mapping)
	for i := range in.tasks {
		t := &in.tasks[i]
		rs, ok := byInstance[t.instance]
		if !ok {
			rs = &rules{}
		}
		if t.version.CreatedSrcAt.After(in.asOf) || !rs.ofTeam(&t.version, in.team) {
			continue
		}
		pr.tls = append(pr.tls, buildTimeline(t, rs, in.asOf))
	}
	return pr
}

// calculate is the series over the periods with the tasks they were computed from.
func (pr *prepared) calculate(periods []period) entity.CalcResult {
	out := entity.CalcResult{Grouping: entity.GroupingCurrentStructure}
	if pr.in.mapping == nil {
		out.Series = notEvaluated(periods)
		return out
	}
	no := pr.in.mapping.VersionNo
	out.MappingVersion = &no
	for _, tl := range pr.tls {
		out.Tasks = append(out.Tasks, tl.slice)
	}
	if pr.in.groupBy == entity.GroupNone {
		out.Series = series(pr.tls, periods, nil)
		return out
	}
	for _, g := range groups(pr.tls, pr.in.groupBy) {
		out.Series = append(out.Series, series(g.tasks, periods, &g.group)...)
	}
	return out
}

// ofTeam applies the rule of the team field: a task of an instance with such a rule belongs to the team whose
// value matched; without the rule the projects of the team decide (TZ 12.5.6).
func (rs *rules) ofTeam(v *entity.TaskVersion, team entity.ID) bool {
	if rs.teamField == "" {
		return true
	}
	value := cmp.Or(deref(rawFields(v)[rs.teamField]), v.TeamNative)
	return rs.teams[value] == team.String()
}

func series(tls []*timeline, periods []period, group *entity.SeriesGroup) []entity.Series {
	var out []entity.Series
	for i := range _definitions {
		def := &_definitions[i]
		calc := _metrics[def.ID]
		estimators := def.Estimators
		if len(estimators) == 0 {
			estimators = []string{""}
		}
		ss := make([]entity.Series, len(estimators))
		for i, est := range estimators {
			ss[i] = entity.Series{MetricID: def.ID, DefinitionVersion: def.Version, Unit: def.Unit, Estimator: est, Group: group}
		}
		for _, p := range periods {
			pts := calc(tls, p)
			for i := range pts {
				ss[i].Points = append(ss[i].Points, pts[i])
			}
		}
		out = append(out, ss...)
	}
	return out
}

func notEvaluated(periods []period) []entity.Series {
	var out []entity.Series
	for i := range _definitions {
		def := &_definitions[i]
		estimators := def.Estimators
		if len(estimators) == 0 {
			estimators = []string{""}
		}
		for _, est := range estimators {
			s := entity.Series{MetricID: def.ID, DefinitionVersion: def.Version, Unit: def.Unit, Estimator: est}
			for _, p := range periods {
				s.Points = append(s.Points, entity.Point{
					Start: p.start, End: p.end, State: entity.StateNotEvaluated, ReasonCode: entity.ReasonMappingMissing,
				})
			}
			out = append(out, s)
		}
	}
	return out
}

// periods cut [from, min(to, as_of)) into steps in the time zone of the team.
func periods(w entity.Window, asOf time.Time, loc *time.Location) []period {
	end := w.To
	if asOf.Before(end) {
		end = asOf
	}
	var out []period
	for start := w.From.In(loc); start.Before(end); {
		next := start.AddDate(0, 0, 7)
		if w.Step == entity.StepMonth {
			next = start.AddDate(0, 1, 0)
		}
		p := period{start: start.UTC(), end: next.UTC()}
		if end.Before(p.end) {
			p.end = end.UTC()
		}
		out = append(out, p)
		start = next
	}
	return out
}

type taskGroup struct {
	group entity.SeriesGroup
	tasks []*timeline
}

// groups split the tasks by type or by the size at start; tasks without it form the group with a nil label
// (BR-007).
func groups(tls []*timeline, by entity.GroupBy) []taskGroup {
	var out []taskGroup
	for _, tl := range tls {
		label := tl.slice.SizeLabel
		if by == entity.GroupTaskType {
			label = tl.slice.TaskType
		}
		i := slices.IndexFunc(out, func(g taskGroup) bool { return sameLabel(g.label(by), label) })
		if i < 0 {
			g := taskGroup{}
			if by == entity.GroupTaskType {
				g.group.TaskType = label
			} else {
				g.group.SizeLabel = label
			}
			out = append(out, g)
			i = len(out) - 1
		}
		out[i].tasks = append(out[i].tasks, tl)
	}
	slices.SortFunc(out, func(a, b taskGroup) int { return compareLabels(a.label(by), b.label(by)) })
	return out
}

func (g taskGroup) label(by entity.GroupBy) *string {
	if by == entity.GroupTaskType {
		return g.group.TaskType
	}
	return g.group.SizeLabel
}

func sameLabel(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// compareLabels orders labels by name with the group without a label last.
func compareLabels(a, b *string) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Compare(*a, *b)
}
