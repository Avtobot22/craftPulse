// Package expect calculates the expected values of the task metrics of the demo set from the task model and
// the demo settings alone (AC-04, NFR-06). It reads neither the code nor the API of craftPulse, so its values
// are an independent check of the calculation. Definitions: section 1 of docs/product/metrics/mvp.md,
// docs/product/metrics/tasks.md, business rules 4.1 and control sets 4.4 of the specification.
package expect

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

// Meanings of statuses and resolutions in a confirmed mapping (MappingRule.meaning).
const (
	meaningIntake    = "intake"
	meaningActive    = "active"
	meaningWait      = "wait"
	meaningBlocked   = "blocked"
	meaningDone      = "done"
	meaningCancelled = "cancelled"
	meaningDuplicate = "duplicate"
	meaningAccepted  = "accepted"
)

// Outcomes of an episode or of a task that ends without one.
const (
	outcomeCompleted = "completed"
	outcomeCancelled = "cancelled"
	outcomeDuplicate = "duplicate"
)

// unknownLabel groups a task without a size or a team: it is shown, not dropped (BR-007).
const unknownLabel = "unknown"

// mapping is the confirmed mapping of one instance.
type mapping struct {
	status     map[string]string
	resolution map[string]string
	sizeField  string
	teamField  string
	teams      map[string]string
	flagField  string
	flagValue  string
}

func mappings(s *dataset.Settings) map[string]*mapping {
	out := map[string]*mapping{}
	get := func(instance string) *mapping {
		if m, ok := out[instance]; ok {
			return m
		}
		m := &mapping{status: map[string]string{}, resolution: map[string]string{}, teams: map[string]string{}}
		out[instance] = m
		return m
	}
	for _, r := range s.Mapping.Rules {
		m := get(r.InstanceID)
		switch r.RuleKind {
		case "status":
			m.status[r.NativeRef] = r.Meaning
		case "resolution":
			m.resolution[r.NativeRef] = r.Meaning
		case "size_field":
			m.sizeField = r.NativeRef
		case "team_field":
			m.teamField = r.NativeRef
		case "team_value":
			m.teams[r.NativeValue] = r.Meaning
		case "blocked_flag":
			m.flagField, m.flagValue = r.NativeRef, r.NativeValue
		}
	}
	return out
}

// span is a half-open interval [from, to); offset is the UTC offset of the event that opened it.
type span struct {
	from, to time.Time
	offset   string
}

// episode is one work episode from a valid start to a terminal outcome; an open one has a zero end.
type episode struct {
	span
	// known is false for an episode a snapshot found already started: its start is not known.
	known   bool
	reopen  bool
	outcome string
}

type event struct {
	at      time.Time
	offset  string
	outcome string
}

// attrs are the slice attributes of a task: type and size at its first start (BR-007), team by the current
// structure (M0 groups by the current team, labelled current_structure).
type attrs struct {
	typ, size, team string
}

// timeline is what the history of a task says about its work.
type timeline struct {
	id, org, project string
	created          time.Time
	firstEvent       time.Time
	lastObserved     time.Time
	// partial is set when the history before historyFrom is unknown: starts and first completions are not
	// proven first then (V-04, V-05).
	partial      bool
	historyFrom  time.Time
	episodes     []episode
	completions  []event
	nonCompleted []event
	reopens      []time.Time
	blocked      []span
	attrs        attrs
	// transitions counts the statuses the task entered, mapped those with a confirmed meaning (T41).
	transitions, mapped int
	conflict            bool
	invalid             bool
}

// source is a task with the time its known history starts; a zero historyFrom is a full history.
type source struct {
	task        dataset.Task
	historyFrom time.Time
}

type state struct {
	status, resolution, typ, size, team, flag string
}

// build replays the history of a task in the order of event time. Events that happen at one instant apply
// together: a finish sets the status and the resolution at once. A repeated event with the same ID and
// content is one fact; the same ID with other content is a conflict, and the first accepted fact stays (BR-002).
func build(src source, m *mapping, asOf time.Time) (*timeline, error) {
	t := &timeline{id: src.task.ID, org: src.task.OrganizationID, project: src.task.ProjectID, historyFrom: src.historyFrom}
	t.partial = !src.historyFrom.IsZero()
	events, err := t.accepted(src.task.Events, asOf)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("task %s has no events before as-of", t.id)
	}
	t.firstEvent = events[0].at
	var (
		st          state
		open        = -1
		terminal    = ""
		lastOutcome = ""
		statusBlock *span
		flagBlock   *span
		blocks      []span
		snapshots   []snapshot
	)
	for i := 0; i < len(events); {
		at := events[i].at
		created := false
		j := i
		for j < len(events) && events[j].at.Equal(at) {
			j++
		}
		group := events[i:j]
		i = j
		offset := group[0].offset
		for _, e := range group {
			created = created || e.raw.Kind == dataset.EventCreated
			if err := t.apply(&st, e, m); err != nil {
				return nil, err
			}
		}
		class := classify(st, m)
		outcome := outcomeOf(class, st, m)
		first := len(snapshots) == 0
		snapshots = append(snapshots, snapshot{at: at, st: st})
		// Without the history before it, the first event is a snapshot of the state, not the creation.
		snap := first && t.partial && !at.After(t.historyFrom)
		if first && created && !snap {
			t.created = at
		}
		working := class == meaningActive || class == meaningWait || class == meaningBlocked
		switch {
		case open < 0 && working:
			known := !snap
			t.episodes = append(t.episodes, episode{span: span{from: at, offset: offset}, known: known, reopen: len(t.episodes) > 0 || terminal != ""})
			open = len(t.episodes) - 1
			if lastOutcome == outcomeCompleted {
				t.reopens = append(t.reopens, at)
			}
			terminal, lastOutcome = "", ""
		case open >= 0 && outcome != "":
			t.episodes[open].to, t.episodes[open].outcome = at, outcome
			t.record(outcome, at, offset)
			open, terminal, lastOutcome = -1, outcome, outcome
		case open < 0 && outcome != "" && terminal == "":
			t.record(outcome, at, offset)
			terminal, lastOutcome = outcome, outcome
		case open < 0 && outcome == "" && terminal != "":
			if lastOutcome == outcomeCompleted {
				t.reopens = append(t.reopens, at)
			}
			terminal, lastOutcome = "", ""
		}
		statusBlock = toggle(statusBlock, class == meaningBlocked, at, offset, &blocks)
		flagBlock = toggle(flagBlock, m != nil && m.flagField != "" && st.flag == m.flagValue, at, offset, &blocks)
	}
	toggle(statusBlock, false, asOf, "", &blocks)
	toggle(flagBlock, false, asOf, "", &blocks)
	t.blocked = clip(union(blocks), t.episodes, asOf)
	t.attrs = t.attributes(snapshots, m)
	return t, nil
}

type snapshot struct {
	at time.Time
	st state
}

type accepted struct {
	at, observed time.Time
	offset       string
	raw          dataset.TaskEvent
}

// accepted dedupes events by ID, drops those not known by as-of and orders the rest by event time. An event
// before the creation is a conflict of order: the task is invalid until the source corrects it (BR-006).
func (t *timeline) accepted(raw []dataset.TaskEvent, asOf time.Time) ([]accepted, error) {
	seen := map[string]string{}
	var out []accepted
	for _, e := range raw {
		body, err := json.Marshal(e)
		if err != nil {
			return nil, fmt.Errorf("encode event: %w", err)
		}
		if prev, ok := seen[e.ID]; ok {
			if prev != string(body) {
				t.conflict = true
			}
			continue
		}
		seen[e.ID] = string(body)
		at, err := time.Parse(time.RFC3339, e.EventTime.UTC)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", e.ID, err)
		}
		observed := at
		if e.ObservedTime != nil {
			if observed, err = time.Parse(time.RFC3339, e.ObservedTime.UTC); err != nil {
				return nil, fmt.Errorf("event %s: %w", e.ID, err)
			}
		}
		if !at.Before(asOf) || observed.After(asOf) {
			continue
		}
		if observed.After(t.lastObserved) {
			t.lastObserved = observed
		}
		out = append(out, accepted{at: at.UTC(), observed: observed, offset: e.EventTime.OriginalOffset, raw: e})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	for i, e := range out {
		if e.raw.Kind == dataset.EventCreated && i > 0 && out[0].at.Before(e.at) {
			t.invalid = true
		}
	}
	return out, nil
}

func (t *timeline) apply(st *state, e accepted, m *mapping) error {
	switch e.raw.Kind {
	case dataset.EventCreated:
		if e.raw.Initial == nil {
			return fmt.Errorf("created event %s has no initial fields", e.raw.ID)
		}
		st.status, st.typ, st.size, st.team = e.raw.Initial.Status, e.raw.Initial.Type, e.raw.Initial.Size, e.raw.Initial.Team
		t.countStatus(st.status, m)
	case dataset.EventFieldChange:
		switch e.raw.Field {
		case dataset.FieldStatus:
			st.status = e.raw.To
			t.countStatus(st.status, m)
		case dataset.FieldResolution:
			st.resolution = e.raw.To
		case dataset.FieldType:
			st.typ = e.raw.To
		case dataset.FieldSize:
			st.size = e.raw.To
		case dataset.FieldTeam:
			st.team = e.raw.To
		case dataset.FieldFlagged:
			st.flag = e.raw.To
		}
	default:
		return fmt.Errorf("event %s has unknown kind", e.raw.ID)
	}
	return nil
}

func (t *timeline) countStatus(status string, m *mapping) {
	t.transitions++
	if m != nil && m.status[status] != "" {
		t.mapped++
	}
}

func (t *timeline) record(outcome string, at time.Time, offset string) {
	if outcome == outcomeCompleted {
		t.completions = append(t.completions, event{at: at, offset: offset, outcome: outcome})
		return
	}
	t.nonCompleted = append(t.nonCompleted, event{at: at, offset: offset, outcome: outcome})
}

// classify gives the meaning of the current status; a status without a rule has none and neither starts
// nor ends work (BR-003).
func classify(st state, m *mapping) string {
	if m == nil {
		return ""
	}
	return m.status[st.status]
}

// outcomeOf is the terminal outcome of a status: done with an accepted or no resolution is a completion;
// done with another resolution, cancelled and duplicate are non-completions by their reason.
func outcomeOf(class string, st state, m *mapping) string {
	switch class {
	case meaningDone:
		switch m.resolution[st.resolution] {
		case meaningCancelled:
			return outcomeCancelled
		case meaningDuplicate:
			return outcomeDuplicate
		}
		return outcomeCompleted
	case meaningCancelled:
		if m.resolution[st.resolution] == meaningDuplicate {
			return outcomeDuplicate
		}
		return outcomeCancelled
	case meaningDuplicate:
		return outcomeDuplicate
	}
	return ""
}

func toggle(open *span, on bool, at time.Time, offset string, out *[]span) *span {
	switch {
	case on && open == nil:
		return &span{from: at, offset: offset}
	case !on && open != nil:
		if at.After(open.from) {
			open.to = at
			*out = append(*out, *open)
		}
		return nil
	}
	return open
}

// union merges overlapping blocks: a block by status and by flag at once counts once (BR-005, V-41).
func union(spans []span) []span {
	slices.SortFunc(spans, func(a, b span) int { return a.from.Compare(b.from) })
	var out []span
	for _, s := range spans {
		if n := len(out); n > 0 && !s.from.After(out[n-1].to) {
			if s.to.After(out[n-1].to) {
				out[n-1].to = s.to
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// clip keeps the blocked time inside work episodes: blocked is a subset of WIP (BR-005).
func clip(blocks []span, episodes []episode, asOf time.Time) []span {
	var out []span
	for _, b := range blocks {
		for _, e := range episodes {
			end := e.to
			if end.IsZero() {
				end = asOf
			}
			from, to := later(b.from, e.from), earlier(b.to, end)
			if to.After(from) {
				offset := b.offset
				if from.Equal(e.from) && !b.from.Equal(e.from) {
					offset = e.offset
				}
				out = append(out, span{from: from, to: to, offset: offset})
			}
		}
	}
	return out
}

func (t *timeline) attributes(snapshots []snapshot, m *mapping) attrs {
	at := time.Time{}
	switch {
	case len(t.episodes) > 0:
		at = t.episodes[0].from
	case len(t.completions) > 0:
		at = t.completions[0].at
	case len(t.nonCompleted) > 0:
		at = t.nonCompleted[0].at
	}
	pick := snapshots[len(snapshots)-1].st
	if !at.IsZero() {
		for _, s := range snapshots {
			if s.at.After(at) {
				break
			}
			pick = s.st
		}
	}
	a := attrs{typ: pick.typ, size: pick.size, team: unknownLabel}
	if a.size == "" {
		a.size = unknownLabel
	}
	if a.typ == "" {
		a.typ = unknownLabel
	}
	if m != nil {
		if team, ok := m.teams[snapshots[len(snapshots)-1].st.team]; ok {
			a.team = team
		}
	}
	return a
}

// firstStart is the first valid start, known only for a full history.
func (t *timeline) firstStart() (time.Time, bool) {
	if len(t.episodes) == 0 || t.partial || !t.episodes[0].known {
		return time.Time{}, false
	}
	return t.episodes[0].from, true
}

// openAt is the episode in work at the instant: started at or before it and not ended by it.
func (t *timeline) openAt(at time.Time) (episode, bool) {
	for _, e := range t.episodes {
		if !e.from.After(at) && (e.to.IsZero() || at.Before(e.to)) {
			return e, true
		}
	}
	return episode{}, false
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
