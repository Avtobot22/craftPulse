package calc

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_fieldStatus     = "status"
	_fieldResolution = "resolution"
)

// outcome is what the values of a task at a moment mean under the confirmed mapping (BR-003).
type outcome int

const (
	// outcomeUnknown is a status or resolution without a rule: it gives neither a start nor an end (FR-017 A3).
	outcomeUnknown outcome = iota + 1
	outcomeIntake
	outcomeWork
	outcomeCompleted
	outcomeCancelled
	outcomeDuplicate
)

type phase int

const (
	phaseNotStarted phase = iota + 1
	phaseEpisode
	phaseTerminal
)

// rules are the rules of one instance of a mapping version.
type rules struct {
	status     map[string]string
	resolution map[string]string
	types      map[string]string
	sizeField  string
	sizes      map[string]string
	flags      map[string]map[string]bool
	teamField  string
	teams      map[string]string
}

func rulesOf(m *entity.MappingVersion) map[entity.ID]*rules {
	out := map[entity.ID]*rules{}
	for _, r := range m.Rules {
		rs, ok := out[r.InstanceID]
		if !ok {
			rs = &rules{
				status: map[string]string{}, resolution: map[string]string{}, types: map[string]string{}, sizes: map[string]string{},
				flags: map[string]map[string]bool{}, teams: map[string]string{},
			}
			out[r.InstanceID] = rs
		}
		switch r.Kind {
		case entity.RuleStatus:
			rs.status[r.NativeRef] = r.Meaning
		case entity.RuleResolution:
			rs.resolution[r.NativeRef] = r.Meaning
		case entity.RuleIssueType:
			rs.types[r.NativeRef] = r.Meaning
		case entity.RuleSizeField:
			rs.sizeField = r.NativeRef
		case entity.RuleSizeValue:
			rs.sizes[r.NativeValue] = r.Meaning
		case entity.RuleBlockedFlag:
			if rs.flags[r.NativeRef] == nil {
				rs.flags[r.NativeRef] = map[string]bool{}
			}
			rs.flags[r.NativeRef][r.NativeValue] = true
		case entity.RuleTeamField:
			rs.teamField = r.NativeRef
		case entity.RuleTeamValue:
			rs.teams[r.NativeValue] = r.Meaning
		case entity.RuleCodeRequiredType, entity.RuleAcceptanceGate, entity.RuleEpicField, entity.RuleBusinessEpicValue:
		}
	}
	return out
}

// derive is the outcome of the values of the fields of a task with the class of work it gives.
func (rs *rules) derive(values map[string]string) (outcome, entity.IntervalClass) {
	meaning, ok := rs.status[values[_fieldStatus]]
	if !ok || meaning == entity.MeaningIgnored {
		return outcomeUnknown, ""
	}
	switch meaning {
	case entity.MeaningIntake:
		return outcomeIntake, ""
	case entity.MeaningActive, entity.MeaningWait, entity.MeaningBlocked:
		if meaning == entity.MeaningBlocked || rs.flagged(values) {
			return outcomeWork, entity.IntervalBlocked
		}
		return outcomeWork, entity.IntervalClass(meaning)
	case entity.MeaningCancelled:
		return outcomeCancelled, ""
	case entity.MeaningDuplicate:
		return outcomeDuplicate, ""
	case entity.MeaningDone:
		return rs.resolved(values[_fieldResolution]), ""
	}
	return outcomeUnknown, ""
}

// resolved is the outcome of a done status by its resolution; a resolution without a rule decides nothing.
func (rs *rules) resolved(resolution string) outcome {
	if resolution == "" {
		return outcomeCompleted
	}
	switch rs.resolution[resolution] {
	case entity.MeaningAccepted, entity.MeaningIgnored:
		return outcomeCompleted
	case entity.MeaningCancelled:
		return outcomeCancelled
	case entity.MeaningDuplicate:
		return outcomeDuplicate
	}
	return outcomeUnknown
}

func (rs *rules) flagged(values map[string]string) bool {
	for field, blocking := range rs.flags {
		if blocking[values[field]] {
			return true
		}
	}
	return false
}

func (rs *rules) tracked(field string) bool {
	if field == _fieldStatus || field == _fieldResolution || (rs.sizeField != "" && field == rs.sizeField) {
		return true
	}
	_, ok := rs.flags[field]
	return ok
}

type segment struct {
	class  entity.IntervalClass
	from   time.Time
	events []entity.ID
}

// episode is a started period of work; an episode whose start precedes the known history has startKnown false
// and starts at the beginning of that history.
type episode struct {
	start      time.Time
	startKnown bool
	end        *time.Time
	segments   []segment
}

type termination struct {
	at     time.Time
	reason string
}

// timeline is the reconstructed workflow of one task up to as_of (DR-007).
type timeline struct {
	slice  entity.TaskSlice
	rules  *rules
	asOf   time.Time
	values map[string]string
	phase  phase
	term   outcome

	// preStart and preCompletion prove a start or a completion before the known history.
	preStart      bool
	preCompletion bool
	episodes      []*episode
	completions   []time.Time
	terminations  []termination
	// events are the changes replayed, for the hash of the input of a part.
	events []entity.TaskEvent
}

type taskInput struct {
	version     entity.TaskVersion
	instance    entity.ID
	project     entity.ID
	events      []entity.TaskEvent
	historyFrom *time.Time
}

// buildTimeline replays the history of a task; events at one moment apply together, so a status and its
// resolution set by one change decide once. An event that does not continue the values it changes, or one
// before the creation, is a conflict of order: the task stays UNKNOWN until reconciliation (BR-006).
func buildTimeline(in *taskInput, rs *rules, asOf time.Time) *timeline {
	v := &in.version
	tl := &timeline{
		slice: entity.TaskSlice{TaskID: v.TaskID, ProjectID: in.project, Created: v.CreatedSrcAt.UTC(), HistoryFrom: utc(in.historyFrom)},
		rules: rs, asOf: asOf, phase: phaseNotStarted,
	}
	if label, ok := rs.types[v.TypeNativeID]; ok {
		tl.slice.TaskType = &label
	}
	from := tl.slice.Created
	if h := tl.slice.HistoryFrom; h != nil && h.After(from) {
		from = *h
	} else {
		tl.slice.HistoryFrom = nil
	}
	all := history(in.events, rs)
	tl.values = initialValues(v, all, rs, from)
	end := slices.IndexFunc(all, func(e entity.TaskEvent) bool { return e.EventTime.After(asOf) })
	if end < 0 {
		end = len(all)
	}
	events := all[:end]
	tl.events = events
	unmapped := map[string]bool{}
	tl.note(unmapped)

	start := 0
	for start < len(events) && events[start].EventTime.Before(from) {
		if tl.slice.HistoryFrom == nil {
			tl.slice.Conflict = true
		}
		start++
	}
	tl.begin(from)
	for i := start; i < len(events); {
		at := events[i].EventTime
		var ids []entity.ID
		for ; i < len(events) && events[i].EventTime.Equal(at); i++ {
			e := &events[i]
			if deref(e.BeforeValue) != tl.values[e.FieldNativeID] {
				tl.slice.Conflict = true
			}
			tl.values[e.FieldNativeID] = deref(e.AfterValue)
			ids = append(ids, e.ID)
		}
		tl.note(unmapped)
		tl.step(at, ids)
	}
	tl.slice.Unmapped = slices.Sorted(func(yield func(string) bool) {
		for s := range unmapped {
			if !yield(s) {
				return
			}
		}
	})
	tl.finish()
	return tl
}

// history keeps the changes of the tracked fields once per event in the order of event time. The values at the
// beginning come from all of them, as the current version is the state after the latest; only the changes up to
// as_of are replayed, and a change at as_of belongs to the state at as_of and gives no interval.
func history(events []entity.TaskEvent, rs *rules) []entity.TaskEvent {
	seen := make(map[entity.ID]bool, len(events))
	out := make([]entity.TaskEvent, 0, len(events))
	for i := range events {
		e := &events[i]
		if e.Kind != entity.TaskFieldChange || !rs.tracked(e.FieldNativeID) || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, *e)
		out[len(out)-1].EventTime = e.EventTime.UTC()
	}
	slices.SortStableFunc(out, func(a, b entity.TaskEvent) int {
		return cmp.Or(a.EventTime.Compare(b.EventTime), cmp.Compare(a.EventKey, b.EventKey))
	})
	return out
}

// initialValues are the values at the beginning of the known history: the value before the first change of a
// field, or its current value when the field did not change.
func initialValues(v *entity.TaskVersion, events []entity.TaskEvent, rs *rules, from time.Time) map[string]string {
	raw := rawFields(v)
	values := map[string]string{_fieldStatus: v.StatusNativeID, _fieldResolution: v.ResolutionNative}
	if rs.sizeField != "" {
		values[rs.sizeField] = cmp.Or(deref(raw[rs.sizeField]), v.SizeNative)
	}
	for field := range rs.flags {
		values[field] = deref(raw[field])
	}
	first := map[string]bool{}
	for i := range events {
		e := &events[i]
		if e.EventTime.Before(from) || first[e.FieldNativeID] {
			continue
		}
		first[e.FieldNativeID] = true
		values[e.FieldNativeID] = deref(e.BeforeValue)
	}
	return values
}

// rawFields are the other fields of the task as the source sent them; a malformed set leaves them unknown.
func rawFields(v *entity.TaskVersion) map[string]*string {
	raw := map[string]*string{}
	if len(v.RawFields) > 0 && json.Unmarshal(v.RawFields, &raw) != nil {
		return map[string]*string{}
	}
	return raw
}

func (tl *timeline) note(unmapped map[string]bool) {
	if status := tl.values[_fieldStatus]; status != "" {
		if _, ok := tl.rules.status[status]; !ok {
			unmapped[status] = true
		}
	}
}

// begin places the task at the beginning of its known history. Without the earlier history a task already at
// work has an episode with an unknown start, and a finished one an earlier outcome of an unknown time.
func (tl *timeline) begin(at time.Time) {
	o, class := tl.rules.derive(tl.values)
	if tl.slice.HistoryFrom == nil {
		tl.transition(at, o, class, nil)
		return
	}
	switch o {
	case outcomeWork:
		tl.preStart = true
		tl.open(at, class, false, nil)
	case outcomeCompleted:
		tl.preCompletion = true
		tl.phase, tl.term = phaseTerminal, o
	case outcomeCancelled, outcomeDuplicate:
		tl.phase, tl.term = phaseTerminal, o
	case outcomeIntake, outcomeUnknown:
	}
}

func (tl *timeline) step(at time.Time, events []entity.ID) {
	o, class := tl.rules.derive(tl.values)
	tl.transition(at, o, class, events)
}

func (tl *timeline) transition(at time.Time, o outcome, class entity.IntervalClass, events []entity.ID) {
	switch tl.phase {
	case phaseNotStarted:
		switch o {
		case outcomeWork:
			tl.open(at, class, true, events)
		case outcomeCompleted:
			tl.complete(at)
		case outcomeCancelled, outcomeDuplicate:
			tl.terminate(at, o)
		case outcomeIntake, outcomeUnknown:
		}
	case phaseEpisode:
		switch o {
		case outcomeWork:
			tl.reclass(at, class, events)
		case outcomeIntake:
			tl.reclass(at, entity.IntervalWait, events)
		case outcomeCompleted:
			tl.close(at)
			tl.complete(at)
		case outcomeCancelled, outcomeDuplicate:
			tl.close(at)
			tl.terminate(at, o)
		case outcomeUnknown:
		}
	case phaseTerminal:
		switch o {
		case outcomeWork:
			tl.open(at, class, true, events)
		case outcomeIntake:
			tl.phase = phaseNotStarted
		case outcomeCompleted, outcomeCancelled, outcomeDuplicate:
			if tl.term == o {
				return
			}
			if o == outcomeCompleted {
				tl.complete(at)
			} else {
				tl.terminate(at, o)
			}
		case outcomeUnknown:
		}
	}
}

// open starts an episode; a known first start fixes the size the task is counted by (BR-007).
func (tl *timeline) open(at time.Time, class entity.IntervalClass, known bool, events []entity.ID) {
	tl.episodes = append(tl.episodes, &episode{start: at, startKnown: known, segments: []segment{{class: class, from: at, events: events}}})
	tl.phase = phaseEpisode
	if !known || tl.slice.FirstStart != nil {
		return
	}
	first := at
	tl.slice.FirstStart = &first
	if tl.slice.HistoryFrom != nil || tl.rules.sizeField == "" {
		return
	}
	if label, ok := tl.rules.sizes[tl.values[tl.rules.sizeField]]; ok {
		tl.slice.SizeLabel = &label
	}
}

func (tl *timeline) reclass(at time.Time, class entity.IntervalClass, events []entity.ID) {
	ep := tl.episodes[len(tl.episodes)-1]
	if last := &ep.segments[len(ep.segments)-1]; last.class == class {
		return
	}
	ep.segments = append(ep.segments, segment{class: class, from: at, events: events})
}

func (tl *timeline) close(at time.Time) {
	end := at
	tl.episodes[len(tl.episodes)-1].end = &end
}

func (tl *timeline) complete(at time.Time) {
	tl.completions = append(tl.completions, at)
	tl.phase, tl.term = phaseTerminal, outcomeCompleted
}

func (tl *timeline) terminate(at time.Time, o outcome) {
	reason := entity.MeaningCancelled
	if o == outcomeDuplicate {
		reason = entity.MeaningDuplicate
	}
	tl.terminations = append(tl.terminations, termination{at: at, reason: reason})
	tl.phase, tl.term = phaseTerminal, o
}

// finish fixes the clocks and the intervals of the slice; an open interval ends at as_of.
func (tl *timeline) finish() {
	s := &tl.slice
	s.Episodes = len(tl.episodes)
	if len(tl.completions) > 0 {
		last := tl.completions[len(tl.completions)-1]
		s.FinalCompletion = &last
		if s.HistoryFrom == nil {
			first := tl.completions[0]
			s.FirstCompletion = &first
		}
	}
	for no, ep := range tl.episodes {
		for i, seg := range ep.segments {
			to := tl.asOf
			switch {
			case i+1 < len(ep.segments):
				to = ep.segments[i+1].from
			case ep.end != nil:
				to = *ep.end
			}
			if !seg.from.Before(to) {
				continue
			}
			s.Intervals = append(s.Intervals, entity.WorkflowInterval{
				TaskID: s.TaskID, EpisodeNo: no + 1, IntervalNo: len(s.Intervals) + 1, Class: seg.class, From: seg.from, To: to,
				SourceEventIDs: seg.events,
			})
		}
	}
}

// episodeAt is the episode open at t, if any.
func (tl *timeline) episodeAt(t time.Time) *episode {
	for _, ep := range tl.episodes {
		if !ep.start.After(t) && (ep.end == nil || t.Before(*ep.end)) {
			return ep
		}
	}
	return nil
}

// classAt is the class of the open episode at t.
func (ep *episode) classAt(t time.Time) entity.IntervalClass {
	class := ep.segments[0].class
	for _, seg := range ep.segments {
		if seg.from.After(t) {
			break
		}
		class = seg.class
	}
	return class
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
