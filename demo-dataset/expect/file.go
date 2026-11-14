package expect

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

// Build calculates the expected values of the task metrics of the set: every week and month of the period in
// all and by type, size and team, and the completeness of the whole period. An organization without a
// confirmed mapping gives NOT_EVALUATED, not zero (section 4.3).
func Build(tasks *dataset.TaskFile, settings *dataset.Settings) (*dataset.ExpectedFile, error) {
	asOf, err := time.Parse(time.RFC3339, tasks.AsOf)
	if err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	from, err := time.Parse(time.RFC3339, tasks.Period.From)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	to, err := time.Parse(time.RFC3339, tasks.Period.To)
	if err != nil || !to.After(from) || asOf.Before(to) {
		return nil, fmt.Errorf("period must be nonempty and end by as-of")
	}
	sources := map[string][]source{}
	for _, t := range tasks.Tasks {
		sources[t.OrganizationID] = append(sources[t.OrganizationID], source{task: t})
	}
	orgs := make([]string, 0, len(sources))
	for org := range sources {
		orgs = append(orgs, org)
	}
	sort.Strings(orgs)
	file := &dataset.ExpectedFile{SchemaVersion: "1", Records: []dataset.Expected{}}
	for _, org := range orgs {
		var s *dataset.Settings
		if org == settings.OrganizationID {
			s = settings
		}
		c, err := newCalc(org, sources[org], s, tasks.Period, asOf)
		if err != nil {
			return nil, err
		}
		file.Records = append(file.Records, c.records(tasks)...)
	}
	if err := file.Validate(); err != nil {
		return nil, fmt.Errorf("expected values: %w", err)
	}
	return file, nil
}

// newCalc builds the timelines of an organization; settings are nil without a confirmed mapping.
func newCalc(org string, sources []source, settings *dataset.Settings, period dataset.Window, asOf time.Time) (*calc, error) {
	from, err := time.Parse(time.RFC3339, period.From)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	c := &calc{org: org, asOf: asOf, period: period, periodFrom: from, mapped: settings != nil}
	var byInstance map[string]*mapping
	if settings != nil {
		byInstance = mappings(settings)
		days := settings.Reopen.WindowDays
		if days <= 0 {
			return nil, fmt.Errorf("the reopen window of T23 must be positive")
		}
		c.followUp = time.Duration(days) * 24 * time.Hour
		for _, t := range settings.Targets {
			if t.DurationSeconds <= 0 {
				return nil, fmt.Errorf("target %s/%s/%s v%d must be positive", t.TeamID, t.TaskType, t.SizeLabel, t.VersionNo)
			}
			known, err := time.Parse(time.RFC3339, t.ConfirmedAt)
			if err != nil {
				return nil, fmt.Errorf("target confirmation: %w", err)
			}
			c.targets = append(c.targets, target{team: t.TeamID, typ: t.TaskType, size: t.SizeLabel, version: t.VersionNo, seconds: t.DurationSeconds, known: known})
		}
	}
	projects := map[string]bool{}
	for _, src := range sources {
		var m *mapping
		if byInstance != nil {
			m = byInstance[src.task.InstanceID]
		}
		t, err := build(src, m, asOf)
		if err != nil {
			return nil, err
		}
		c.timelines = append(c.timelines, t)
		projects[src.task.ProjectID] = true
	}
	c.scope = strings.Join(slices.Sorted(func(yield func(string) bool) {
		for p := range projects {
			if !yield(p) {
				return
			}
		}
	}), ",")
	return c, nil
}

func (c *calc) records(tasks *dataset.TaskFile) []dataset.Expected {
	var out []dataset.Expected
	if !c.mapped {
		return append(c.notEvaluated(), c.completeness()...)
	}
	for _, step := range []string{"week", "month"} {
		for _, w := range c.buckets(step) {
			for _, s := range c.slices(tasks) {
				out = append(out, c.window(step, w[0], w[1], s)...)
			}
		}
	}
	return append(out, c.completeness()...)
}

// buckets split the period into ISO weeks from Monday and calendar months in UTC; the first and the last one
// are cut by the period.
func (c *calc) buckets(step string) [][2]time.Time {
	from, _ := time.Parse(time.RFC3339, c.period.From)
	to, _ := time.Parse(time.RFC3339, c.period.To)
	var out [][2]time.Time
	for start := from; start.Before(to); {
		var next time.Time
		if step == "week" {
			day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
			next = day.AddDate(0, 0, (8-int(day.Weekday()))%7)
			if !next.After(start) {
				next = next.AddDate(0, 0, 7)
			}
		} else {
			next = time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		}
		out = append(out, [2]time.Time{start, earlier(next, to)})
		start = next
	}
	return out
}

// slices are the whole scope and each type, size and team of the projects; unknown size is its own group.
func (c *calc) slices(tasks *dataset.TaskFile) []slice {
	out := []slice{{}}
	var types, sizes []string
	for _, p := range tasks.Projects {
		if p.OrganizationID != c.org {
			continue
		}
		types = append(types, p.Types...)
		sizes = append(sizes, p.Sizes...)
	}
	teams := map[string]bool{}
	for _, t := range c.timelines {
		teams[t.attrs.team] = true
	}
	for _, dim := range []struct {
		name   string
		values []string
	}{
		{"type", unique(types)}, {"size", append(unique(sizes), unknownLabel)}, {"team", unique(keys(teams))},
	} {
		for _, v := range dim.values {
			out = append(out, slice{dim: dim.name, value: v})
		}
	}
	return out
}

func unique(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (c *calc) periodRecord(metric, unit string) *builder {
	from, _ := time.Parse(time.RFC3339, c.period.From)
	to, _ := time.Parse(time.RFC3339, c.period.To)
	return c.record(metric, "period", from, to, slice{}, unit)
}

// notEvaluated are the flow metrics of an organization without a confirmed mapping: no start or completion
// is derived from an unconfirmed status (BR-003).
func (c *calc) notEvaluated() []dataset.Expected {
	var out []dataset.Expected
	for _, m := range []struct{ id, unit string }{
		{"T04", "tasks"}, {"T05", "tasks"}, {"T07", "tasks"}, {"T08.p50", "seconds"}, {"T08.p85", "seconds"},
		{"T10", "ratio"}, {"T11", "tasks"}, {"T12", "tasks"}, {"T13", "ratio"}, {"T14", "seconds"}, {"T15", "seconds"},
		{"T18", "tasks"}, {"T23", "ratio"},
	} {
		b := c.periodRecord(m.id, m.unit).flag("mapping_missing")
		b.rec.State = stateNotEvaluated
		out = append(out, b.done())
	}
	return out
}

// completeness is T38-T42 over the period. T40 depends on when the stand collects the set, so it is given by
// conditions: the watermark and the last success are not earlier than the last event the set has, and the
// lag is not negative.
func (c *calc) completeness() []dataset.Expected {
	var out []dataset.Expected
	if c.mapped {
		out = append(out, c.fields()...)
	}
	complete := c.periodRecord("T39", "ratio")
	var full int64
	var last time.Time
	for _, t := range c.timelines {
		if !t.partial && !t.conflict && !t.invalid {
			full++
			complete.interval(t, "history", span{from: t.firstEvent, to: c.asOf, offset: "Z"})
		}
		complete.member(t)
		if t.lastObserved.After(last) {
			last = t.lastObserved
		}
	}
	out = append(out, complete.ratio(full, int64(len(c.timelines))).done())
	bound := json.Number(strconv.FormatInt(last.Unix(), 10))
	for _, m := range []struct {
		id, unit string
		bound    json.Number
	}{
		{"T40.watermark", "unix_seconds", bound}, {"T40.last_success", "unix_seconds", bound}, {"T40.lag_p95", "seconds", "0"},
	} {
		b := c.periodRecord(m.id, m.unit)
		b.rec.Conditions = []dataset.Condition{{Op: "ge", Value: m.bound}}
		out = append(out, b.done())
	}
	byTask, byTransition := c.periodRecord("T41.tasks", "ratio"), c.periodRecord("T41.transitions", "ratio")
	var mappedTasks, transitions, mappedTransitions int64
	for _, t := range c.timelines {
		byTask.member(t)
		byTransition.member(t)
		if t.transitions > 0 && t.mapped == t.transitions {
			mappedTasks++
		}
		transitions += int64(t.transitions)
		mappedTransitions += int64(t.mapped)
	}
	out = append(out, byTask.ratio(mappedTasks, int64(len(c.timelines))).done(), byTransition.ratio(mappedTransitions, transitions).done())
	projects := map[string][]*timeline{}
	for _, t := range c.timelines {
		projects[t.project] = append(projects[t.project], t)
	}
	for _, p := range unique(keysOf(projects)) {
		b := c.periodRecord("T42", "tasks").filter("project", p)
		var differing int64
		for _, t := range projects[p] {
			b.member(t)
			if t.conflict || t.invalid {
				differing++
			}
		}
		out = append(out, b.value(differing).parts(differing, int64(len(projects[p]))).done())
	}
	return out
}

// fields is T38: tasks with each field known, field by field; epic has no mapped field in the set.
func (c *calc) fields() []dataset.Expected {
	var out []dataset.Expected
	for _, field := range []string{"size", "type", "team", "start"} {
		b := c.periodRecord("T38", "ratio").filter("field", field)
		var known, eligible int64
		for _, t := range c.timelines {
			if field == "start" && len(t.episodes) == 0 && len(t.completions) == 0 {
				continue
			}
			eligible++
			b.member(t)
			ok := false
			switch field {
			case "size":
				ok = t.attrs.size != unknownLabel
			case "type":
				ok = t.attrs.typ != unknownLabel
			case "team":
				ok = t.attrs.team != unknownLabel
			case "start":
				_, ok = t.firstStart()
			}
			if ok {
				known++
			}
		}
		out = append(out, b.ratio(known, eligible).done())
	}
	epic := c.periodRecord("T38", "ratio").filter("field", "epic").flag("mapping_missing")
	epic.rec.State = stateNotEvaluated
	return append(out, epic.done())
}

func keysOf(m map[string][]*timeline) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
