package expect

import (
	"encoding/json"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

const (
	_definitionVersion = "1"
	// _lowSample marks a distribution of fewer observations (BR-006); 20 is a product guard, not statistics.
	_lowSample = 20
	// _ratioDigits is the precision of a ratio; the numerator and the denominator stay exact.
	_ratioDigits = 6
)

// Data states of section 4.3.
const (
	stateAssessed      = "ASSESSED"
	stateBounded       = "BOUNDED"
	stateUnknown       = "UNKNOWN"
	stateNotEvaluated  = "NOT_EVALUATED"
	stateNotApplicable = "NOT_APPLICABLE"
)

// slice is one breakdown of a window: the whole scope, or one value of type, size or team.
type slice struct {
	dim, value string
}

func (s slice) key() string {
	if s.dim == "" {
		return "all"
	}
	return s.dim + "=" + s.value
}

func (s slice) filters() map[string][]string {
	if s.dim == "" {
		return map[string][]string{}
	}
	return map[string][]string{s.dim: {s.value}}
}

func (s slice) match(t *timeline) bool {
	switch s.dim {
	case "type":
		return t.attrs.typ == s.value
	case "size":
		return t.attrs.size == s.value
	case "team":
		return t.attrs.team == s.value
	}
	return true
}

// target is a version of the target of a group, known since its confirmation.
type target struct {
	team, typ, size string
	version         int
	seconds         int64
	known           time.Time
}

// calc holds the timelines of one organization with its settings for the as-of of the set.
type calc struct {
	org, scope string
	asOf       time.Time
	period     dataset.Window
	periodFrom time.Time
	timelines  []*timeline
	mapped     bool
	targets    []target
	followUp   time.Duration
}

// targetsAt is the version of each group known at the moment: a later version does not rewrite what was
// known before it (BR-007, FR-019).
func (c *calc) targetsAt(known time.Time) map[[3]string]target {
	out := map[[3]string]target{}
	for _, t := range c.targets {
		key := [3]string{t.team, t.typ, t.size}
		if !t.known.After(known) && t.version > out[key].version {
			out[key] = t
		}
	}
	return out
}

// knowledge is the distinct moments a target version became known, in order.
func (c *calc) knowledge() []time.Time {
	var out []time.Time
	for _, t := range c.targets {
		if !slices.ContainsFunc(out, t.known.Equal) {
			out = append(out, t.known)
		}
	}
	slices.SortFunc(out, time.Time.Compare)
	return out
}

// builder accumulates one record.
type builder struct {
	rec dataset.Expected
	// n is set by a distribution; otherwise n is the number of objects of the cohort.
	n bool
}

func (c *calc) record(metric, step string, from, to time.Time, s slice, unit string) *builder {
	scope := map[string]string{"organization_id": c.org, "projects": c.scope, "team_grouping": "current_structure"}
	return &builder{rec: dataset.Expected{
		ID:       c.org + "/" + metric + "/" + step + "/" + from.UTC().Format(time.RFC3339) + "/" + s.key(),
		MetricID: metric, DefinitionVersion: _definitionVersion, Scope: scope, Filters: s.filters(),
		Period: dataset.Window{From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339)},
		Step:   step, AsOf: c.asOf.UTC().Format(time.RFC3339), Unit: unit, State: stateAssessed,
		Cohort: []string{}, Intervals: []dataset.Interval{}, Flags: []string{}, Conditions: []dataset.Condition{},
	}}
}

func (b *builder) id(suffix string) *builder {
	b.rec.ID += "/" + suffix
	return b
}

func (b *builder) scope(key, value string) *builder {
	b.rec.Scope[key] = value
	return b
}

func (b *builder) filter(key, value string) *builder {
	b.rec.Filters[key] = []string{value}
	b.rec.ID += "/" + key + "=" + value
	return b
}

func (b *builder) member(t *timeline) {
	if !slices.Contains(b.rec.Cohort, t.id) {
		b.rec.Cohort = append(b.rec.Cohort, t.id)
	}
}

func (b *builder) interval(t *timeline, kind string, s span) {
	if !s.to.After(s.from) {
		return
	}
	b.member(t)
	offset := s.offset
	if offset == "" {
		offset = "Z"
	}
	b.rec.Intervals = append(b.rec.Intervals, dataset.Interval{
		ObjectID: t.id, Kind: kind, From: s.from.UTC().Format(time.RFC3339), To: s.to.UTC().Format(time.RFC3339), OriginalOffset: offset,
	})
}

func (b *builder) value(v int64) *builder {
	n := json.Number(strconv.FormatInt(v, 10))
	b.rec.Value = &n
	return b
}

func (b *builder) parts(num, den int64) *builder {
	n, d := json.Number(strconv.FormatInt(num, 10)), json.Number(strconv.FormatInt(den, 10))
	b.rec.Numerator, b.rec.Denominator = &n, &d
	return b
}

// ratio sets the value num/den rounded to six digits with halves away from zero; no denominator is no value.
func (b *builder) ratio(num, den int64) *builder {
	b.parts(num, den)
	if den == 0 {
		b.rec.State = stateNotApplicable
		return b
	}
	v := json.Number(new(big.Rat).SetFrac64(num, den).FloatString(_ratioDigits))
	b.rec.Value = &v
	return b
}

func (b *builder) bounded(lower, upper int64) *builder {
	b.rec.Value, b.rec.State = nil, stateBounded
	b.rec.Conditions = []dataset.Condition{
		{Op: "ge", Value: json.Number(strconv.FormatInt(lower, 10))}, {Op: "le", Value: json.Number(strconv.FormatInt(upper, 10))},
	}
	return b
}

func (b *builder) flag(f string) *builder {
	if !slices.Contains(b.rec.Flags, f) {
		b.rec.Flags = append(b.rec.Flags, f)
	}
	return b
}

func (b *builder) done() dataset.Expected {
	if !b.n {
		b.rec.N = int64(len(b.rec.Cohort))
	}
	sort.Strings(b.rec.Cohort)
	sort.SliceStable(b.rec.Intervals, func(i, j int) bool {
		x, y := b.rec.Intervals[i], b.rec.Intervals[j]
		if x.ObjectID != y.ObjectID {
			return x.ObjectID < y.ObjectID
		}
		return x.From < y.From
	})
	return b.rec
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }

func in(at, from, to time.Time) bool { return !at.Before(from) && at.Before(to) }

// percentile is nearest-rank sorted[ceil(p*n)-1] with p in percent, without rounding before it (BR-006).
func percentile(sorted []int64, p int) int64 {
	rank := (p*len(sorted) + 99) / 100
	return sorted[rank-1]
}

// window gives the records of a window [from, to) and of its end as the instant of the point metrics.
func (c *calc) window(step string, from, to time.Time, s slice) []dataset.Expected {
	var tasks []*timeline
	for _, t := range c.timelines {
		if s.match(t) {
			tasks = append(tasks, t)
		}
	}
	var out []dataset.Expected
	add := func(b *builder) { out = append(out, b.done()) }
	add(c.started(step, from, to, s, tasks))
	add(c.reopenStarts(step, from, to, s, tasks))
	completed := c.completedRecord(step, from, to, s, tasks)
	add(completed)
	add(c.repeated(step, from, to, s, tasks))
	out = append(out, c.nonCompletions(step, from, to, s, tasks)...)
	out = append(out, c.cycle(step, from, to, s, tasks)...)
	for _, known := range c.knowledge() {
		out = append(out, c.onTarget(step, from, to, s, tasks, known)...)
		add(c.aging(step, from, to, s, tasks, known))
	}
	add(c.wip(step, from, to, s, tasks))
	add(c.blockedWIP(step, from, to, s, tasks))
	out = append(out, c.age(step, from, to, s, tasks)...)
	add(c.blockedDuration(step, from, to, s, tasks))
	add(c.blockedOpen(step, from, to, s, tasks))
	add(c.balance(step, from, to, s, tasks))
	out = append(out, c.reopened(step, from, to, s, tasks)...)
	return out
}

// started is T04: unique tasks whose first valid start falls in the window. A task whose start is before its
// known history may have started in any window up to that history: the count is then bounded.
func (c *calc) started(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T04", step, from, to, s, "tasks")
	var exact, possible int64
	for _, t := range tasks {
		if start, ok := t.firstStart(); ok {
			if in(start, from, to) {
				exact++
				b.member(t)
			}
			continue
		}
		if len(t.episodes) > 0 && !from.After(t.historyFrom) {
			possible++
			b.member(t)
		}
	}
	if possible > 0 {
		return b.bounded(exact, exact+possible)
	}
	return b.value(exact)
}

// reopenStarts counts the episodes a reopen starts in the window; T04 counts first starts only.
func (c *calc) reopenStarts(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T04.reopen", step, from, to, s, "episodes")
	var n int64
	for _, t := range tasks {
		for _, e := range t.episodes {
			if e.reopen && in(e.from, from, to) {
				n++
				b.member(t)
			}
		}
	}
	return b.value(n)
}

// firstCompleted is the first accepted completion of a task in the window and whether it is proven first.
func firstCompleted(t *timeline, from, to time.Time) (event, bool, bool) {
	if len(t.completions) == 0 || !in(t.completions[0].at, from, to) {
		return event{}, false, false
	}
	return t.completions[0], true, !t.partial
}

// completedRecord is T05: unique tasks with their first accepted completion in the window. Without the
// history before a completion it may not be the first one: the count is bounded (V-05).
func (c *calc) completedRecord(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T05", step, from, to, s, "tasks")
	var exact, possible int64
	for _, t := range tasks {
		_, ok, proven := firstCompleted(t, from, to)
		switch {
		case ok && proven:
			exact++
			b.member(t)
		case ok:
			possible++
			b.member(t)
		}
	}
	if possible > 0 {
		return b.bounded(exact, exact+possible)
	}
	return b.value(exact)
}

// repeated counts accepted completions after a reopen in the window; they do not enter T05 (BR-004).
func (c *calc) repeated(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T05.repeat", step, from, to, s, "completions")
	var n int64
	for _, t := range tasks {
		for _, e := range t.completions[min(1, len(t.completions)):] {
			if in(e.at, from, to) {
				n++
				b.member(t)
			}
		}
	}
	return b.value(n)
}

// nonCompletions is T07: unique tasks with a terminal outcome without an accepted result, in all and by reason.
func (c *calc) nonCompletions(step string, from, to time.Time, s slice, tasks []*timeline) []dataset.Expected {
	all := c.record("T07", step, from, to, s, "tasks")
	byReason := map[string]*builder{
		outcomeCancelled: c.record("T07", step, from, to, s, "tasks").filter("reason", outcomeCancelled),
		outcomeDuplicate: c.record("T07", step, from, to, s, "tasks").filter("reason", outcomeDuplicate),
	}
	for _, t := range tasks {
		for _, e := range t.nonCompleted {
			if in(e.at, from, to) {
				all.member(t)
				byReason[e.outcome].member(t)
			}
		}
	}
	out := []dataset.Expected{all.value(int64(len(all.rec.Cohort))).done()}
	for _, reason := range []string{outcomeCancelled, outcomeDuplicate} {
		r := byReason[reason]
		out = append(out, r.value(int64(len(r.rec.Cohort))).done())
	}
	return out
}

// cycle is T08: completed - started over the tasks with the first accepted completion in the window, p50 and
// p85 by nearest rank. A task of the cohort without a known start or a proven first completion makes the exact
// distribution unknown; its known part is shown as a bounded interval.
func (c *calc) cycle(step string, from, to time.Time, s slice, tasks []*timeline) []dataset.Expected {
	p50, p85 := c.record("T08.p50", step, from, to, s, "seconds"), c.record("T08.p85", step, from, to, s, "seconds")
	var (
		samples []int64
		unknown bool
	)
	for _, t := range tasks {
		done, ok, proven := firstCompleted(t, from, to)
		if !ok {
			continue
		}
		start, known := t.firstStart()
		switch {
		case !proven || t.invalid:
			unknown = true
			if len(t.episodes) > 0 {
				for _, b := range []*builder{p50, p85} {
					b.interval(t, "bounded_cycle", span{from: t.episodes[0].from, to: done.at, offset: t.episodes[0].offset})
				}
			}
		case known && done.at.Before(start):
			unknown = true
		case known:
			samples = append(samples, seconds(done.at.Sub(start)))
			for _, b := range []*builder{p50, p85} {
				b.interval(t, "cycle", span{from: start, to: done.at, offset: t.episodes[0].offset})
			}
		}
	}
	slices.Sort(samples)
	for p, b := range map[int]*builder{50: p50, 85: p85} {
		b.rec.N, b.n = int64(len(samples)), true
		switch {
		case unknown:
			b.rec.State = stateUnknown
		case len(samples) == 0:
			b.rec.State = stateNotApplicable
		default:
			b.value(percentile(samples, p))
			if len(samples) < _lowSample {
				b.flag("low_sample")
			}
		}
	}
	return []dataset.Expected{p50.done(), p85.done()}
}

// onTarget is T10 for the target versions known at a moment: completed within the applicable target / completed
// with a known start, size and target. A task without size or target is missing, no other group is
// substituted (BR-007); T10.missing lists them.
func (c *calc) onTarget(step string, from, to time.Time, s slice, tasks []*timeline, known time.Time) []dataset.Expected {
	at := known.UTC().Format(time.RFC3339)
	b := c.record("T10", step, from, to, s, "ratio").scope("targets_known_at", at).id("targets@" + at)
	missing := c.record("T10.missing", step, from, to, s, "tasks").scope("targets_known_at", at).id("targets@" + at)
	targets := c.targetsAt(known)
	var num, den int64
	for _, t := range tasks {
		done, ok, proven := firstCompleted(t, from, to)
		if !ok || !proven {
			continue
		}
		start, startKnown := t.firstStart()
		limit, hasTarget := targets[[3]string{t.attrs.team, t.attrs.typ, t.attrs.size}]
		if !startKnown || !hasTarget || t.attrs.size == unknownLabel {
			missing.member(t)
			continue
		}
		den++
		cycle := seconds(done.at.Sub(start))
		b.interval(t, "cycle", span{from: start, to: done.at, offset: t.episodes[0].offset})
		if cycle <= limit.seconds {
			num++
		}
	}
	return []dataset.Expected{b.ratio(num, den).done(), missing.value(int64(len(missing.rec.Cohort))).done()}
}

// aging is T11 at the end of the window: open tasks whose current episode is older than the applicable
// target, among the open tasks with a target. Without a target there is no excess.
func (c *calc) aging(step string, from, to time.Time, s slice, tasks []*timeline, known time.Time) *builder {
	at := known.UTC().Format(time.RFC3339)
	b := c.record("T11", step, from, to, s, "tasks").scope("targets_known_at", at).id("targets@" + at)
	targets := c.targetsAt(known)
	var over, with int64
	for _, t := range tasks {
		e, ok := t.openAt(to)
		limit, hasTarget := targets[[3]string{t.attrs.team, t.attrs.typ, t.attrs.size}]
		if !ok || !hasTarget || t.attrs.size == unknownLabel || !e.known || t.partial {
			continue
		}
		with++
		b.interval(t, "current_episode", span{from: e.from, to: to, offset: e.offset})
		b.member(t)
		if seconds(to.Sub(e.from)) > limit.seconds {
			over++
		}
	}
	b.parts(over, with)
	if with == 0 {
		b.rec.State = stateNotApplicable
		return b
	}
	return b.value(over)
}

// wip is T12 at the end of the window: tasks with an open episode, waiting and blocked ones included.
func (c *calc) wip(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T12", step, from, to, s, "tasks")
	for _, t := range tasks {
		if e, ok := t.openAt(to); ok {
			b.member(t)
			b.interval(t, "episode", span{from: e.from, to: to, offset: e.offset})
		}
	}
	return b.value(int64(len(b.rec.Cohort)))
}

// blockedWIP is T13 at the end of the window: blocked tasks / tasks in WIP; the value is the share, the
// numerator the count. The cohort of a share is its denominator, the blocked ones have their block interval.
func (c *calc) blockedWIP(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T13", step, from, to, s, "ratio")
	var blocked, wip int64
	for _, t := range tasks {
		if _, ok := t.openAt(to); !ok {
			continue
		}
		wip++
		b.member(t)
		for _, bl := range t.blocked {
			if !bl.from.After(to) && to.Before(bl.to) {
				blocked++
				b.interval(t, "blocked", span{from: bl.from, to: to, offset: bl.offset})
				break
			}
		}
	}
	return b.ratio(blocked, wip)
}

// age is T14 at the end of the window: the age of the current episode of each open task, the age since the
// first start separately, and the open tasks started before the period separately. The value of a point is
// the oldest age; every age is an interval. An unknown start gives no exact age (BR-005).
func (c *calc) age(step string, from, to time.Time, s slice, tasks []*timeline) []dataset.Expected {
	current := c.record("T14", step, from, to, s, "seconds")
	full := c.record("T14.full", step, from, to, s, "seconds")
	old := c.record("T14.before_period", step, from, to, s, "tasks")
	var oldest, oldestFull int64
	unknown := false
	for _, t := range tasks {
		e, ok := t.openAt(to)
		if !ok {
			continue
		}
		current.member(t)
		full.member(t)
		start, known := t.firstStart()
		if !e.known || !known {
			unknown = true
			continue
		}
		current.interval(t, "current_episode", span{from: e.from, to: to, offset: e.offset})
		full.interval(t, "since_first_start", span{from: start, to: to, offset: t.episodes[0].offset})
		oldest, oldestFull = max(oldest, seconds(to.Sub(e.from))), max(oldestFull, seconds(to.Sub(start)))
		if e.from.Before(c.periodFrom) {
			old.member(t)
		}
	}
	for b, v := range map[*builder]int64{current: oldest, full: oldestFull} {
		switch {
		case unknown:
			b.rec.State = stateUnknown
		case len(b.rec.Cohort) == 0:
			b.rec.State = stateNotApplicable
		default:
			b.value(v)
		}
	}
	return []dataset.Expected{current.done(), full.done(), old.value(int64(len(old.rec.Cohort))).done()}
}

// blockedDuration is T15 over the tasks with the first accepted completion in the window: the blocked time of
// the first cycle; the numerator is the same sum, the denominator the elapsed cycle time.
func (c *calc) blockedDuration(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T15", step, from, to, s, "seconds")
	var blocked, elapsed int64
	for _, t := range tasks {
		done, ok, proven := firstCompleted(t, from, to)
		start, known := t.firstStart()
		if !ok || !proven || !known {
			continue
		}
		b.member(t)
		elapsed += seconds(done.at.Sub(start))
		for _, bl := range t.blocked {
			f, e := later(bl.from, start), earlier(bl.to, done.at)
			if e.After(f) {
				blocked += seconds(e.Sub(f))
				b.interval(t, "blocked", span{from: f, to: e, offset: bl.offset})
			}
		}
	}
	return b.value(blocked).parts(blocked, elapsed)
}

// blockedOpen is T15 of the open episodes at the end of the window: an open block is bounded by that instant.
func (c *calc) blockedOpen(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T15.open", step, from, to, s, "seconds")
	var blocked, elapsed int64
	for _, t := range tasks {
		e, ok := t.openAt(to)
		if !ok || !e.known || t.partial {
			continue
		}
		b.member(t)
		elapsed += seconds(to.Sub(e.from))
		for _, bl := range t.blocked {
			f, en := later(bl.from, e.from), earlier(bl.to, to)
			if en.After(f) {
				blocked += seconds(en.Sub(f))
				b.interval(t, "blocked", span{from: f, to: en, offset: bl.offset})
			}
		}
	}
	return b.value(blocked).parts(blocked, elapsed)
}

// balance is T18: first starts minus first completions in the window; WIP at its end is T12.
func (c *calc) balance(step string, from, to time.Time, s slice, tasks []*timeline) *builder {
	b := c.record("T18", step, from, to, s, "tasks")
	var started, completed int64
	for _, t := range tasks {
		if start, ok := t.firstStart(); ok && in(start, from, to) {
			started++
			b.member(t)
		}
		if _, ok, proven := firstCompleted(t, from, to); ok && proven {
			completed++
			b.member(t)
		}
	}
	return b.value(started-completed).parts(started, completed)
}

// reopened is T23 of the tasks with the first accepted completion in the window: reopened within
// [completed, completed+H) / tasks whose window ended by as-of. The window includes its start and excludes its
// end; tasks with a window still open are immature and listed separately (BR-008).
func (c *calc) reopened(step string, from, to time.Time, s slice, tasks []*timeline) []dataset.Expected {
	b := c.record("T23", step, from, to, s, "ratio")
	immature := c.record("T23.immature", step, from, to, s, "tasks").flag("immature")
	var num, den int64
	for _, t := range tasks {
		done, ok, proven := firstCompleted(t, from, to)
		if !ok || !proven {
			continue
		}
		end := done.at.Add(c.followUp)
		if end.After(c.asOf) {
			immature.member(t)
			continue
		}
		den++
		b.interval(t, "follow_up", span{from: done.at, to: end, offset: done.offset})
		if slices.ContainsFunc(t.reopens, func(r time.Time) bool { return in(r, done.at, end) }) {
			num++
		}
	}
	return []dataset.Expected{b.ratio(num, den).done(), immature.value(int64(len(immature.rec.Cohort))).done()}
}
