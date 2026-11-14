package expect

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

// The control sets of section 4.4: d0 = 2026-10-01T00:00:00Z, dN = d0 + N days, window [d0, d10), as-of d10,
// mapping m1 and the default cohort with nearest rank.
var d0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

func d(n float64) time.Time { return d0.Add(time.Duration(n * float64(day))) }

// story writes the history of one task of the control sets.
type story struct {
	id     string
	events []dataset.TaskEvent
}

func newStory(id string) *story { return &story{id: "org-1/jira-cloud-1/" + id} }

func (s *story) add(at time.Time, kind, field, from, to string, initial *dataset.TaskFields) *story {
	s.events = append(s.events, dataset.TaskEvent{
		ID: fmt.Sprintf("%s/e%02d", s.id, len(s.events)+1), Kind: kind, EventTime: dataset.SourceTime{UTC: at.Format(time.RFC3339), OriginalOffset: "Z"},
		AuthorID: "org-1/person-01", Field: field, From: from, To: to, Initial: initial,
	})
	return s
}

func (s *story) create(at time.Time, status, size string) *story {
	return s.add(at, dataset.EventCreated, "", "", "", &dataset.TaskFields{Status: status, Type: "Story", Size: size, Team: "Platform"})
}

func (s *story) move(at time.Time, status string) *story {
	return s.add(at, dataset.EventFieldChange, dataset.FieldStatus, "", status, nil)
}

func (s *story) set(at time.Time, field, value string) *story {
	return s.add(at, dataset.EventFieldChange, field, "", value, nil)
}

func (s *story) done(at time.Time) *story {
	return s.move(at, dataset.StatusDone).set(at, dataset.FieldResolution, dataset.ResolutionDone)
}

func (s *story) cancel(at time.Time) *story {
	return s.move(at, dataset.StatusCancelled).set(at, dataset.FieldResolution, dataset.ResolutionWontDo)
}

func (s *story) task() dataset.Task {
	return dataset.Task{ID: s.id, OrganizationID: "org-1", InstanceID: "jira-cloud-1", ProjectID: "project-p", Key: s.id, Events: s.events}
}

func (s *story) source() source { return source{task: s.task()} }

// m1 is the mapping of the control sets with a reopen window of H days and the given targets of Story.
func m1(windowDays int, targets ...dataset.TargetVersion) *dataset.Settings {
	s := &dataset.Settings{OrganizationID: "org-1", TeamID: "team-1", Reopen: dataset.ReopenWindow{MetricID: "T23", DefinitionVersion: "1", WindowDays: windowDays}, Targets: targets}
	rule := func(kind, ref, value, meaning string) {
		s.Mapping.Rules = append(s.Mapping.Rules, dataset.MappingRule{RuleKind: kind, InstanceID: "jira-cloud-1", NativeRef: ref, NativeValue: value, Meaning: meaning})
	}
	for status, meaning := range map[string]string{
		dataset.StatusBacklog: meaningIntake, dataset.StatusTriage: meaningIntake, dataset.StatusInProgress: meaningActive,
		dataset.StatusWaiting: meaningWait, dataset.StatusBlocked: meaningBlocked, dataset.StatusDone: meaningDone,
		dataset.StatusCancelled: meaningCancelled, dataset.StatusDuplicate: meaningDuplicate,
	} {
		rule("status", status, "", meaning)
	}
	rule("resolution", dataset.ResolutionDone, "", meaningAccepted)
	rule("resolution", dataset.ResolutionWontDo, "", meaningCancelled)
	rule("resolution", dataset.ResolutionDuplicate, "", meaningDuplicate)
	rule("team_value", dataset.FieldTeam, "Platform", "team-1")
	rule("blocked_flag", dataset.FieldFlagged, dataset.FlagImpediment, "blocked")
	return s
}

func targetS(version int, days int64, known time.Time) dataset.TargetVersion {
	return dataset.TargetVersion{TeamID: "team-1", TaskType: "Story", SizeLabel: "S", VersionNo: version, DurationSeconds: days * int64(day/time.Second), ConfirmedAt: known.Format(time.RFC3339)}
}

func newTestCalc(t *testing.T, s *dataset.Settings, asOf time.Time, sources ...source) *calc {
	t.Helper()
	c, err := newCalc("org-1", sources, s, dataset.Window{From: d0.Format(time.RFC3339), To: asOf.Format(time.RFC3339)}, asOf)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// get returns the record of a metric of a window; reason and target knowledge pick variants.
func get(t *testing.T, records []dataset.Expected, metric string, match ...func(dataset.Expected) bool) dataset.Expected {
	t.Helper()
	for _, r := range records {
		if r.MetricID != metric {
			continue
		}
		if len(match) == 0 && (r.Filters["reason"] != nil || r.Filters["field"] != nil || r.Filters["project"] != nil) {
			continue
		}
		if slices.ContainsFunc(match, func(m func(dataset.Expected) bool) bool { return !m(r) }) {
			continue
		}
		return r
	}
	t.Fatalf("no record %s", metric)
	return dataset.Expected{}
}

func value(t *testing.T, r dataset.Expected) string {
	t.Helper()
	if r.Value == nil {
		return "null"
	}
	return string(*r.Value)
}

func days(n float64) string { return fmt.Sprint(int64(n * float64(day/time.Second))) }

func check(t *testing.T, r dataset.Expected, wantValue, wantState string) {
	t.Helper()
	if got := value(t, r); got != wantValue || r.State != wantState {
		t.Errorf("%s %s: value %s state %s, want %s %s", r.MetricID, r.Period.From, got, r.State, wantValue, wantState)
	}
}

func windowOf(c *calc, from, to time.Time) []dataset.Expected {
	return c.window("week", from, to, slice{})
}

func v03() (a, b *story) {
	a = newStory("A").create(d(0), dataset.StatusBacklog, "S").move(d(1), dataset.StatusInProgress).
		move(d(2), dataset.StatusBlocked).move(d(3), dataset.StatusInProgress).done(d(4))
	b = newStory("B").create(d(0), dataset.StatusBacklog, "S").move(d(1), dataset.StatusInProgress).cancel(d(3))
	return a, b
}

func TestV02V03_UsualPathCancelAndReplay(t *testing.T) {
	t.Parallel()
	a, b := v03()
	c := newTestCalc(t, m1(30), d(10), a.source(), b.source())
	all := windowOf(c, d(0), d(10))
	check(t, get(t, all, "T04"), "2", stateAssessed)
	check(t, get(t, all, "T05"), "1", stateAssessed)
	check(t, get(t, all, "T07"), "1", stateAssessed)
	for _, p := range []string{"T08.p50", "T08.p85"} {
		r := get(t, all, p)
		check(t, r, days(3), stateAssessed)
		if r.N != 1 || !slices.Contains(r.Flags, "low_sample") {
			t.Errorf("%s: n %d flags %v, want n 1 with low_sample", p, r.N, r.Flags)
		}
	}
	if got := get(t, all, "T15"); value(t, got) != days(1) || string(*got.Denominator) != days(3) {
		t.Errorf("blocked duration of A %s of %v, want 1 day of a 3 day cycle", value(t, got), got.Denominator)
	}
	if !slices.Equal(get(t, all, "T07").Cohort, []string{b.id}) || slices.Contains(get(t, all, "T05").Cohort, b.id) {
		t.Error("the cancelled task is a non-completion, not a completion")
	}
	if r := get(t, c.window("week", d(0), d(10), slice{dim: "size", value: "S"}), "T05"); value(t, r) != "1" {
		t.Error("the size at start is S")
	}

	snapshots := []struct {
		at                 float64
		wip, blocked, size string
	}{{2, "2", "1", "0.500000"}, {3, "1", "0", "0.000000"}, {4, "0", "0", "null"}}
	for _, s := range snapshots {
		w := windowOf(c, d(0), d(s.at))
		check(t, get(t, w, "T12"), s.wip, stateAssessed)
		if r := get(t, w, "T13"); string(*r.Numerator) != s.blocked || value(t, r) != s.size {
			t.Errorf("T13 at d%v: %s blocked, share %s; want %s and %s", s.at, *r.Numerator, value(t, r), s.blocked, s.size)
		}
	}

	replayed := a.task()
	replayed.Events = slices.Concat(replayed.Events, replayed.Events, replayed.Events)
	c2 := newTestCalc(t, m1(30), d(10), source{task: replayed}, b.source())
	if diff := differ(windowOf(c, d(0), d(10)), windowOf(c2, d(0), d(10))); diff != "" {
		t.Errorf("three deliveries change the values: %s", diff)
	}

	conflicting := a.task()
	changed := conflicting.Events[len(conflicting.Events)-2]
	changed.EventTime.UTC = d(5).Format(time.RFC3339)
	conflicting.Events = append(conflicting.Events, changed)
	c3 := newTestCalc(t, m1(30), d(10), source{task: conflicting}, b.source())
	if diff := differ(windowOf(c, d(0), d(10)), windowOf(c3, d(0), d(10))); diff != "" {
		t.Errorf("a conflicting revision overwrote the accepted fact: %s", diff)
	}
	if r := get(t, c3.completeness(), "T42", func(e dataset.Expected) bool { return true }); string(*r.Numerator) != "1" {
		t.Errorf("the conflict is a reconciliation variance: %s", *r.Numerator)
	}
}

// differ compares the values, states and cohorts of two windows.
func differ(left, right []dataset.Expected) string {
	if len(left) != len(right) {
		return "different number of records"
	}
	for i := range left {
		l, r := left[i], right[i]
		if l.ID != r.ID || fmt.Sprint(l.Value, l.State, l.Cohort, l.Numerator, l.Denominator) != fmt.Sprint(r.Value, r.State, r.Cohort, r.Numerator, r.Denominator) {
			return l.ID
		}
	}
	return ""
}

func v04(reopen float64) (a *story) {
	return newStory("A").create(d(0), dataset.StatusBacklog, "S").move(d(1), dataset.StatusInProgress).done(d(3)).
		move(d(reopen), dataset.StatusInProgress).set(d(reopen), dataset.FieldResolution, "").done(d(8))
}

func TestV04_ReopenedTask(t *testing.T) {
	t.Parallel()
	a := v04(5)
	c := newTestCalc(t, m1(4), d(10), a.source())
	first := windowOf(c, d(0), d(5))
	check(t, get(t, first, "T05"), "1", stateAssessed)
	check(t, get(t, first, "T08.p50"), days(2), stateAssessed)
	check(t, get(t, first, "T08.p85"), days(2), stateAssessed)

	second := windowOf(c, d(5), d(10))
	check(t, get(t, second, "T05"), "0", stateAssessed)
	check(t, get(t, second, "T05.repeat"), "1", stateAssessed)
	check(t, get(t, second, "T04.reopen"), "1", stateAssessed)
	if r := get(t, second, "T08.p50"); r.N != 0 || r.Value != nil || r.State != stateNotApplicable {
		t.Errorf("no completion cohort gives no percentile, not 0: n %d value %s state %s", r.N, value(t, r), r.State)
	}

	at6 := windowOf(c, d(5), d(6))
	check(t, get(t, at6, "T12"), "1", stateAssessed)
	check(t, get(t, at6, "T14"), days(1), stateAssessed)
	check(t, get(t, at6, "T14.full"), days(5), stateAssessed)
	check(t, get(t, windowOf(c, d(5), d(8)), "T12"), "0", stateAssessed)

	finished := newStory("C").create(d(0), dataset.StatusBacklog, "S").move(d(6), dataset.StatusInProgress).done(d(9))
	c = newTestCalc(t, m1(4), d(10), a.source(), finished.source())
	all := windowOf(c, d(0), d(10))
	if r := get(t, all, "T23"); value(t, r) != "1.000000" || string(*r.Denominator) != "1" {
		t.Errorf("T23 of A with a reopen inside [d3, d7): %s of %v", value(t, r), r.Denominator)
	}
	if r := get(t, all, "T23.immature"); !slices.Equal(r.Cohort, []string{finished.id}) {
		t.Errorf("C has no full window and stays out of the denominator: %v", r.Cohort)
	}

	boundary := newTestCalc(t, m1(4), d(10), v04(7).source())
	at := windowOf(boundary, d(0), d(10))
	if r := get(t, at, "T23"); value(t, r) != "0.000000" || string(*r.Denominator) != "1" {
		t.Errorf("a reopen exactly at the end of [d3, d7) is outside it: %s", value(t, r))
	}
	check(t, get(t, windowOf(boundary, d(7), d(8)), "T04.reopen"), "1", stateAssessed)

	unknown := newStory("A").create(d(2), dataset.StatusInProgress, "S").done(d(3)).
		move(d(5), dataset.StatusInProgress).set(d(5), dataset.FieldResolution, "").done(d(8))
	partial := newTestCalc(t, m1(4), d(10), source{task: unknown.task(), historyFrom: d(2)})
	w := windowOf(partial, d(0), d(5))
	if r := get(t, w, "T05"); r.State != stateBounded || r.Value != nil {
		t.Errorf("without the history before d2 the first completion is not known: %s %s", value(t, r), r.State)
	}
	check(t, get(t, w, "T08.p50"), "null", stateUnknown)
}

func TestV05_IncompleteHistoryAndLateEvent(t *testing.T) {
	t.Parallel()
	snapshot := newStory("H").create(d(3), dataset.StatusInProgress, "S").done(d(4))
	r0 := newTestCalc(t, m1(30), d(5), source{task: snapshot.task(), historyFrom: d(3)})
	w := windowOf(r0, d(0), d(5))
	check(t, get(t, w, "T12"), "0", stateAssessed)
	t05 := get(t, w, "T05")
	if t05.State != stateBounded || fmt.Sprint(t05.Conditions) != "[{ge 0} {le 1}]" {
		t.Errorf("R0: T05 bounded from 0 to 1, got %s %v", t05.State, t05.Conditions)
	}
	t08 := get(t, w, "T08.p50")
	if t08.State != stateUnknown || t08.Value != nil || len(t08.Intervals) != 1 || t08.Intervals[0].Kind != "bounded_cycle" ||
		t08.Intervals[0].From != d(3).Format(time.RFC3339) {
		t.Errorf("R0: T08 unknown without a value, the known episode at least one day: %+v", t08)
	}
	check(t, get(t, w, "T04"), "null", stateBounded)

	recovered := newStory("H").create(d(0), dataset.StatusBacklog, "S").move(d(1), dataset.StatusInProgress).done(d(4))
	r1 := newTestCalc(t, m1(30), d(5), recovered.source())
	w = windowOf(r1, d(0), d(5))
	check(t, get(t, w, "T05"), "1", stateAssessed)
	check(t, get(t, w, "T08.p50"), days(3), stateAssessed)

	again := windowOf(newTestCalc(t, m1(30), d(5), source{task: snapshot.task(), historyFrom: d(3)}), d(0), d(5))
	if diff := differ(windowOf(r0, d(0), d(5)), again); diff != "" {
		t.Errorf("R0 is reproduced as it was: %s", diff)
	}

	onlyStart := newStory("H").create(d(1), dataset.StatusInProgress, "S")
	partial := newTestCalc(t, m1(30), d(5), source{task: onlyStart.task(), historyFrom: d(1)})
	if r := get(t, partial.completeness(), "T39"); value(t, r) != "0.000000" {
		t.Errorf("without confirmed pages the history is not complete: %s", value(t, r))
	}
}

func TestV14_WorkflowMapping(t *testing.T) {
	t.Parallel()
	active := newStory("A").create(d(0), dataset.StatusInProgress, "S")
	intake := newStory("B").create(d(0), dataset.StatusTriage, "S").move(d(2), dataset.StatusInProgress)
	unmapped := newStory("X").create(d(0), dataset.StatusBacklog, "").move(d(1), dataset.StatusLegacyQA).move(d(3), dataset.StatusDone)
	c := newTestCalc(t, m1(30), d(10), active.source(), intake.source(), unmapped.source())
	if r := get(t, windowOf(c, d(0), d(1)), "T04"); !slices.Equal(r.Cohort, []string{active.id}) {
		t.Errorf("a task created in work starts at its creation, the intake status does not start: %v", r.Cohort)
	}
	if r := get(t, windowOf(c, d(2), d(3)), "T04"); !slices.Equal(r.Cohort, []string{intake.id}) {
		t.Errorf("B starts when it moves into work: %v", r.Cohort)
	}
	all := windowOf(c, d(0), d(10))
	if r := get(t, all, "T04"); slices.Contains(r.Cohort, unmapped.id) {
		t.Error("an unmapped status gives no start")
	}
	if r := get(t, all, "T05"); value(t, r) != "1" || !slices.Equal(r.Cohort, []string{unmapped.id}) {
		t.Errorf("Done is mapped, so X completes: %v", r.Cohort)
	}
	if r := get(t, c.completeness(), "T41.transitions"); string(*r.Numerator) != "5" || string(*r.Denominator) != "6" {
		t.Errorf("the unmapped status is not covered: %v of %v", *r.Numerator, *r.Denominator)
	}
	if r := get(t, c.window("week", d(0), d(10), slice{dim: "size", value: unknownLabel}), "T05"); !slices.Equal(r.Cohort, []string{unmapped.id}) {
		t.Errorf("no size is the group unknown, not an exclusion: %v", r.Cohort)
	}
}

func TestV16_TargetVersionsAndSize(t *testing.T) {
	t.Parallel()
	a := newStory("A").create(d(0), dataset.StatusBacklog, "S").move(d(1), dataset.StatusInProgress).set(d(2), dataset.FieldSize, "M").done(d(4))
	noSize := newStory("N").create(d(0), dataset.StatusBacklog, "").move(d(1), dataset.StatusInProgress).done(d(2))
	k1, k2 := d(-30), d(-10)
	s := m1(30, targetS(1, 2, k1), targetS(2, 4, k2))
	c := newTestCalc(t, s, d(10), a.source(), noSize.source())
	all := windowOf(c, d(0), d(10))
	known := func(k time.Time) func(dataset.Expected) bool {
		return func(e dataset.Expected) bool { return e.Scope["targets_known_at"] == k.Format(time.RFC3339) }
	}
	if r := get(t, all, "T10", known(k1)); value(t, r) != "0.000000" || string(*r.Denominator) != "1" {
		t.Errorf("a cycle of 3 days breaks v1 = 2 days: %s of %v", value(t, r), r.Denominator)
	}
	if r := get(t, all, "T10", known(k2)); value(t, r) != "1.000000" {
		t.Errorf("v2 = 4 days is its own revision: %s", value(t, r))
	}
	if r := get(t, all, "T10.missing", known(k1)); !slices.Equal(r.Cohort, []string{noSize.id}) {
		t.Errorf("a task without size has no target and no denominator: %v", r.Cohort)
	}
	if r := get(t, c.window("week", d(0), d(10), slice{dim: "size", value: "S"}), "T05"); !slices.Equal(r.Cohort, []string{a.id}) {
		t.Errorf("the size of A is S at its start: %v", r.Cohort)
	}
	if _, err := newCalc("org-1", nil, m1(30, targetS(1, 0, k1)), dataset.Window{From: d0.Format(time.RFC3339), To: d(10).Format(time.RFC3339)}, d(10)); err == nil {
		t.Error("a target of zero is refused")
	}
}

func TestV41_BoundariesAndUnits(t *testing.T) {
	t.Parallel()
	var sources []source
	for i, n := range []float64{1, 2, 3, 4} {
		sources = append(sources, newStory(fmt.Sprint("D", i)).create(d(1), dataset.StatusInProgress, "S").done(d(1+n)).source())
	}
	atEnd := newStory("E").create(d(1), dataset.StatusInProgress, "S").done(d(9))
	c := newTestCalc(t, m1(30), d(10), append(sources, atEnd.source())...)
	w := windowOf(c, d(0), d(9))
	check(t, get(t, w, "T08.p50"), days(2), stateAssessed)
	check(t, get(t, w, "T08.p85"), days(4), stateAssessed)
	if r := get(t, w, "T08.p85"); r.N != 4 || !slices.Contains(r.Flags, "low_sample") || slices.Contains(get(t, w, "T05").Cohort, atEnd.id) {
		t.Errorf("n 4 with low_sample, completion at the end of the window outside it: %d %v", r.N, r.Flags)
	}

	shifted := newStory("Z")
	shifted.add(d(0), dataset.EventCreated, "", "", "", &dataset.TaskFields{Status: dataset.StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	shifted.events = append(shifted.events, dataset.TaskEvent{ID: shifted.id + "/e02", Kind: dataset.EventFieldChange, Field: dataset.FieldStatus,
		To: dataset.StatusInProgress, EventTime: dataset.SourceTime{UTC: "2026-10-02T03:00:00+03:00", OriginalOffset: "+03:00"}})
	if r := get(t, windowOf(newTestCalc(t, m1(30), d(10), shifted.source()), d(1), d(2)), "T04"); value(t, r) != "1" {
		t.Error("2026-10-02T03:00:00+03:00 is d1 in UTC")
	}

	overlap := newStory("B").create(d(0), dataset.StatusInProgress, "S").move(d(1), dataset.StatusBlocked).
		set(d(2), dataset.FieldFlagged, dataset.FlagImpediment).move(d(3), dataset.StatusInProgress).
		set(d(4), dataset.FieldFlagged, "").done(d(5))
	b := get(t, windowOf(newTestCalc(t, m1(30), d(10), overlap.source()), d(0), d(10)), "T15")
	check(t, b, days(3), stateAssessed)

	early := newStory("N").create(d(3), dataset.StatusInProgress, "S")
	early.events = append(early.events, dataset.TaskEvent{ID: early.id + "/e02", Kind: dataset.EventFieldChange, Field: dataset.FieldStatus,
		To: dataset.StatusDone, EventTime: dataset.SourceTime{UTC: d(2).Format(time.RFC3339), OriginalOffset: "Z"}})
	if r := get(t, windowOf(newTestCalc(t, m1(30), d(10), early.source()), d(0), d(10)), "T08.p50"); r.State == stateAssessed {
		t.Errorf("an event before the creation is not assessed: %s", r.State)
	}
}

func TestBuildIsDeterministicAndValid(t *testing.T) {
	t.Parallel()
	model, err := dataset.Generate(21, "2026-12-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := dataset.GenerateTasks(model)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := dataset.GenerateSettings(tasks)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(tasks, settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(tasks, settings)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatal("two runs on the same model differ")
	}
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range first.Records {
		if strings.HasPrefix(r.ID, "org-2/") && r.State == stateAssessed && !strings.HasPrefix(r.MetricID, "T39") &&
			!strings.HasPrefix(r.MetricID, "T40") && !strings.HasPrefix(r.MetricID, "T41") && !strings.HasPrefix(r.MetricID, "T42") {
			t.Fatalf("the closed project of the second organization has no confirmed mapping: %s", r.ID)
		}
	}
}
