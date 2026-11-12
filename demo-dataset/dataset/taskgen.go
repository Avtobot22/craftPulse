package dataset

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"time"
)

const (
	_maxLoopsPerTask = 9
	_lateDelivery    = 5 * 24 * time.Hour
)

// GenerateTasks builds the task model of the demo set from the people, period and volumes of the model.
// Tasks of the flow variants are fixed; background tasks spread over the period come from the seed, and
// review loops inside their work fill the history up to the exact event volume of each project.
func GenerateTasks(model *Model) (*TaskFile, error) {
	asOf, err := parseSecond(model.AsOf)
	if err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	from, err := parseSecond(model.Period.From)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	g := &taskGen{
		rng:       rand.New(rand.NewPCG(model.Seed, 95)),
		from:      from,
		asOf:      asOf,
		people:    map[string][]Person{},
		variants:  map[string][]string{},
		started:   map[string]time.Time{},
		completed: map[string]time.Time{},
	}
	for _, person := range model.People {
		if !person.Bot {
			g.people[person.OrganizationID] = append(g.people[person.OrganizationID], person)
		}
	}
	file := &TaskFile{SchemaVersion: "1", Seed: model.Seed, Period: model.Period, AsOf: model.AsOf, Projects: taskProjects()}
	for _, project := range file.Projects {
		taskTarget, eventTarget := volume(model, project.ID, "task"), volume(model, project.ID, "task_event")
		tasks, err := g.project(project, taskTarget, eventTarget)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", project.ID, err)
		}
		file.Tasks = append(file.Tasks, tasks...)
	}
	file.Variants = g.variantObjects(model.Variants)
	if err := ValidateTasks(file, model); err != nil {
		return nil, err
	}
	return file, nil
}

type taskGen struct {
	rng      *rand.Rand
	from     time.Time
	asOf     time.Time
	people   map[string][]Person
	variants map[string][]string
	// started and completed hold the first start and the first accepted completion of each task.
	started   map[string]time.Time
	completed map[string]time.Time
}

func (g *taskGen) mark(variant string, ids ...string) {
	g.variants[variant] = append(g.variants[variant], ids...)
}

func (g *taskGen) project(p TaskProject, taskTarget, eventTarget int) ([]Task, error) {
	var specials []Task
	switch p.ID {
	case "jira-project-a":
		specials = g.projectA(p)
	case "jira-project-b":
		specials = g.projectB(p)
	case "import-project":
		specials = g.importProject(p)
	case "private-project":
		return g.privateProject(p, taskTarget, eventTarget)
	}
	used := 0
	for _, task := range specials {
		used += len(task.Events)
	}
	backgrounds := make([]background, 0, taskTarget-len(specials))
	for number := len(specials) + 1; len(specials)+len(backgrounds) < taskTarget; number++ {
		backgrounds = append(backgrounds, g.background(p, fmt.Sprintf("%s-%d", p.Key, number)))
	}
	if err := fill(backgrounds, eventTarget-used); err != nil {
		return nil, err
	}
	tasks := specials
	groups := map[string]bool{}
	for _, b := range backgrounds {
		task := g.buildBackground(p, b)
		// One finished task per type and size with a target keeps every target group in the demo.
		if group := b.typ + "/" + b.size; p.ID == "jira-project-b" && b.outcome == _outcomeDone && b.typ != "Task" && b.size != "" && !groups[group] {
			groups[group] = true
			g.mark("types-sizes-targets", task.ID)
		}
		tasks = append(tasks, task)
	}
	if p.ID == "import-project" {
		g.mark("replay", tasks[len(specials)].ID, tasks[len(specials)+1].ID, tasks[len(specials)+2].ID)
	}
	return tasks, nil
}

// background is a generated task: created, maybe started and finished, with optional detours.
type background struct {
	key, typ, size, team string
	created, start, end  time.Time
	outcome              string
	blocked, waiting     bool
	preEstimate          bool
	loops                int
}

const (
	_outcomeDone      = "done"
	_outcomeCancelled = "cancelled"
	_outcomeOpen      = "open"
	_outcomeBacklog   = "backlog"
)

func (b *background) events() int {
	n := 1
	if b.outcome == _outcomeBacklog {
		return n
	}
	n++
	if b.preEstimate {
		n++
	}
	if b.blocked {
		n += 2
	}
	if b.waiting {
		n += 2
	}
	if b.outcome == _outcomeDone || b.outcome == _outcomeCancelled {
		n += 2
	}
	return n + 2*b.loops
}

func (b *background) canEstimate() bool {
	return b.outcome != _outcomeBacklog && b.size != "" && b.start.Sub(b.created) >= 2*time.Hour
}

func (g *taskGen) background(p TaskProject, key string) background {
	r := g.rng
	b := background{key: key, team: "Platform"}
	b.typ = pick(r, []string{"Story", "Story", "Story", "Story", "Story", "Bug", "Bug", "Bug", "Task", "Task"})
	b.size = pick(r, []string{"S", "S", "S", "S", "M", "M", "M", "M", "L", "L", "L", ""})
	if p.Key == "PB" && r.IntN(4) == 0 {
		b.team = "Payments"
	}
	earliest := g.from.AddDate(0, -1, 0)
	span := g.asOf.Add(-15 * 24 * time.Hour).Sub(earliest)
	b.created = workTime(earliest.Add(time.Duration(r.Int64N(int64(span)))), r)
	b.start = workTime(b.created.Add(time.Duration(r.IntN(6*24))*time.Hour), r)
	if !b.start.After(b.created) {
		b.start = b.created.Add(3 * time.Hour)
	}
	b.end = workTime(b.start.Add(time.Duration(24+r.IntN(13*24))*time.Hour), r)
	switch roll := r.IntN(100); {
	case roll < 70:
		b.outcome = _outcomeDone
	case roll < 80:
		b.outcome = _outcomeCancelled
	case roll < 92:
		b.outcome = _outcomeOpen
	default:
		b.outcome = _outcomeBacklog
	}
	if !b.end.Before(g.asOf.Add(-24 * time.Hour)) {
		b.outcome = _outcomeOpen
	}
	if b.outcome == _outcomeOpen {
		b.end = minTime(b.start.Add(20*24*time.Hour), g.asOf.Add(-2*time.Hour))
	}
	b.blocked = r.IntN(5) == 0
	b.waiting = r.IntN(7) == 0
	b.preEstimate = b.canEstimate() && r.IntN(10) == 0
	return b
}

// fill reaches the event target exactly: one optional estimate fixes the parity, review loops add pairs.
func fill(bs []background, need int) error {
	for i := range bs {
		need -= bs[i].events()
	}
	if need < 0 {
		return fmt.Errorf("base history has %d events more than the volume", -need)
	}
	if need%2 == 1 {
		fixed := false
		for i := range bs {
			if bs[i].canEstimate() {
				if bs[i].preEstimate {
					bs[i].preEstimate, need = false, need+1
				} else {
					bs[i].preEstimate, need = true, need-1
				}
				fixed = true
				break
			}
		}
		if !fixed {
			return fmt.Errorf("no task can take a single event to fix the volume parity")
		}
	}
	for loops := need / 2; loops > 0; {
		progressed := false
		for i := range bs {
			if loops == 0 {
				break
			}
			if bs[i].outcome == _outcomeBacklog || bs[i].loops >= _maxLoopsPerTask {
				continue
			}
			bs[i].loops++
			loops--
			progressed = true
		}
		if !progressed {
			return fmt.Errorf("background tasks cannot hold %d more review loops", loops)
		}
	}
	return nil
}

func (g *taskGen) buildBackground(p TaskProject, b background) Task {
	h := g.newHistory(p, b.key, "Background task "+b.key)
	h.create(b.created, TaskFields{Status: StatusBacklog, Type: b.typ, Size: b.size, Team: b.team})
	if b.outcome == _outcomeBacklog {
		return h.done()
	}
	if b.preEstimate {
		h.set(b.created.Add(b.start.Sub(b.created)/2), FieldSize, nextSize(b.size))
	}
	h.move(b.start, StatusInProgress)
	work := b.end.Sub(b.start)
	point := func(share float64) time.Time {
		return b.start.Add(time.Duration(float64(work) * share)).Truncate(time.Second)
	}
	h.loops(b.start, point(0.4), b.loops)
	if b.blocked {
		h.move(point(0.45), StatusBlocked)
		h.move(point(0.6), StatusInProgress)
	}
	if b.waiting {
		h.move(point(0.7), StatusWaiting)
		h.move(point(0.85), StatusInProgress)
	}
	switch b.outcome {
	case _outcomeDone:
		h.finish(b.end, StatusDone, ResolutionDone)
	case _outcomeCancelled:
		h.finish(b.end, StatusCancelled, ResolutionWontDo)
	}
	return h.done()
}

func (g *taskGen) projectA(p TaskProject) []Task {
	day := func(month, d int) time.Time { return time.Date(2026, time.Month(month), d, 7, 0, 0, 0, time.UTC) }
	var tasks []Task
	add := func(h *history, variants ...string) {
		task := h.done()
		for _, v := range variants {
			g.mark(v, task.ID)
		}
		tasks = append(tasks, task)
	}

	h := g.newHistory(p, "PA-1", "Created right in work")
	h.create(day(2, 2), TaskFields{Status: StatusInProgress, Type: "Story", Size: "M", Team: "Platform"})
	h.loops(day(2, 2), day(2, 9), 2)
	h.finish(day(2, 11), StatusDone, ResolutionDone)
	add(h, "created-active")

	h = g.newHistory(p, "PA-2", "Intake status the source reports as active")
	h.create(day(2, 3), TaskFields{Status: StatusTriage, Type: "Bug", Size: "S", Team: "Platform"})
	h.move(day(2, 6), StatusInProgress)
	h.finish(day(2, 13), StatusDone, ResolutionDone)
	add(h, "intake-active-category")

	// V-41: the start falls on 2026-10-02T03:00:00+03:00; flag and status blocks overlap, union is 4 days.
	h = g.newHistory(p, "PA-3", "Blocked by a flag and by a status at once")
	h.author = g.withOffset("org-1", "+03:00")
	midnight := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	h.create(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(midnight(2), StatusInProgress)
	h.set(midnight(3), FieldFlagged, FlagImpediment)
	h.move(midnight(4), StatusBlocked)
	h.set(midnight(5), FieldFlagged, "")
	h.move(midnight(6), StatusInProgress)
	h.move(midnight(8), StatusBlocked)
	h.move(midnight(9), StatusInProgress)
	h.finish(midnight(12), StatusDone, ResolutionDone)
	add(h, "blocked")

	h = g.newHistory(p, "PA-4", "Cancelled after start")
	h.create(day(3, 2), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(3, 4), StatusInProgress)
	h.finish(day(3, 10), StatusCancelled, ResolutionWontDo)
	add(h, "cancelled")

	h = g.newHistory(p, "PA-5", "Closed as a duplicate before start")
	h.create(day(3, 5), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "S", Team: "Platform"})
	h.finish(day(3, 6), StatusDuplicate, ResolutionDuplicate)
	add(h, "duplicate")

	h = g.newHistory(p, "PA-6", "Done, reopened and done again")
	h.create(day(4, 1), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(4, 2), StatusInProgress)
	h.finish(day(4, 6), StatusDone, ResolutionDone)
	h.reopen(day(4, 11))
	h.finish(day(4, 14), StatusDone, ResolutionDone)
	add(h, "done-reopen-done", "complete-reopen-followup")

	h = g.newHistory(p, "PA-7", "Same key as a task of the import project")
	h.create(day(5, 4), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(day(5, 5), StatusInProgress)
	h.loops(day(5, 5), day(5, 11), 1)
	h.finish(day(5, 12), StatusDone, ResolutionDone)
	add(h, "same-key-instances", "ambiguous-link")

	// V-16: S at start, M after start; 3 days of work break target v1(S) = 2 days.
	h = g.newHistory(p, "PA-8", "Size changed after start")
	h.create(day(3, 9), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(3, 10), StatusInProgress)
	h.set(day(3, 11), FieldSize, "M")
	h.finish(day(3, 13), StatusDone, ResolutionDone)
	add(h, "size-change", "types-sizes-targets")

	h = g.newHistory(p, "PA-9", "Moved to another team during work")
	h.create(day(5, 11), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(day(5, 12), StatusInProgress)
	h.set(day(5, 15), FieldTeam, "Payments")
	h.finish(day(5, 20), StatusDone, ResolutionDone)
	add(h, "team-change")

	h = g.newHistory(p, "PA-10", "Started long ago and never updated since")
	h.create(time.Date(2025, 9, 15, 8, 0, 0, 0, time.UTC), TaskFields{Status: StatusBacklog, Type: "Task", Size: "L", Team: "Platform"})
	h.move(time.Date(2025, 10, 1, 8, 0, 0, 0, time.UTC), StatusInProgress)
	add(h, "old-open")

	h = g.newHistory(p, "PA-11", "History longer than two API pages")
	h.create(day(6, 1), TaskFields{Status: StatusBacklog, Type: "Story", Size: "L", Team: "Platform"})
	h.move(day(6, 2), StatusInProgress)
	h.loops(day(6, 2), day(6, 26), 10)
	h.finish(day(6, 29), StatusDone, ResolutionDone)
	add(h, "incomplete-history-page")

	h = g.newHistory(p, "PA-12", "Moved into a status without mapping")
	h.create(day(7, 6), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "M", Team: "Platform"})
	h.move(day(7, 7), StatusInProgress)
	h.move(day(7, 10), UnknownStatus)
	add(h, "unknown-status")

	h = g.newHistory(p, "PA-13", "Task without size and without target")
	h.create(day(8, 3), TaskFields{Status: StatusBacklog, Type: "Task", Size: "", Team: "Platform"})
	h.move(day(8, 4), StatusInProgress)
	h.finish(day(8, 7), StatusDone, ResolutionDone)
	add(h, "types-sizes-targets")

	h = g.newHistory(p, "PA-14", "Done ten days before as-of")
	h.create(workTime(g.asOf.AddDate(0, 0, -20), nil), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(workTime(g.asOf.AddDate(0, 0, -18), nil), StatusInProgress)
	h.finish(workTime(g.asOf.AddDate(0, 0, -10), nil), StatusDone, ResolutionDone)
	add(h, "incomplete-reopen-followup")

	h = g.newHistory(p, "PA-15", "Completion delivered five days late")
	h.create(day(9, 7), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "S", Team: "Platform"})
	h.move(day(9, 8), StatusInProgress)
	h.late = _lateDelivery
	h.finish(day(9, 11), StatusDone, ResolutionDone)
	add(h, "late-event")

	h = g.newHistory(p, "PA-16", "Done without reopen, window fully observed")
	h.create(day(8, 10), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(day(8, 11), StatusInProgress)
	h.finish(day(8, 14), StatusDone, ResolutionDone)
	add(h, "complete-reopen-followup")

	h = g.newHistory(p, "PA-17", "Small story under target v2")
	h.create(day(7, 13), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(7, 14), StatusInProgress)
	h.finish(day(7, 17), StatusDone, ResolutionDone)
	add(h, "types-sizes-targets")

	h = g.newHistory(p, "PA-18", "Started before the period, done inside it")
	h.create(time.Date(2025, 12, 10, 8, 0, 0, 0, time.UTC), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "M", Team: "Platform"})
	h.move(time.Date(2025, 12, 15, 8, 0, 0, 0, time.UTC), StatusInProgress)
	h.finish(day(1, 8), StatusDone, ResolutionDone)
	add(h, "multi-period-history")
	return tasks
}

func (g *taskGen) projectB(p TaskProject) []Task {
	day := func(month, d int) time.Time { return time.Date(2026, time.Month(month), d, 8, 0, 0, 0, time.UTC) }
	var tasks []Task
	add := func(h *history, variants ...string) {
		task := h.done()
		for _, v := range variants {
			g.mark(v, task.ID)
		}
		tasks = append(tasks, task)
	}

	h := g.newHistory(p, "PB-1", "Bug created right in work")
	h.create(day(2, 16), TaskFields{Status: StatusInProgress, Type: "Bug", Size: "S", Team: "Platform"})
	h.finish(day(2, 18), StatusDone, ResolutionDone)
	add(h, "created-active")

	h = g.newHistory(p, "PB-2", "Blocked by status for four days")
	h.create(day(3, 16), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(day(3, 17), StatusInProgress)
	h.move(day(3, 19), StatusBlocked)
	h.move(day(3, 23), StatusInProgress)
	h.finish(day(3, 26), StatusDone, ResolutionDone)
	add(h, "blocked")

	h = g.newHistory(p, "PB-3", "Cancelled while waiting")
	h.create(day(4, 13), TaskFields{Status: StatusBacklog, Type: "Task", Size: "S", Team: "Platform"})
	h.move(day(4, 14), StatusInProgress)
	h.move(day(4, 16), StatusWaiting)
	h.finish(day(4, 20), StatusCancelled, ResolutionWontDo)
	add(h, "cancelled")

	h = g.newHistory(p, "PB-4", "Reopened close to as-of")
	h.create(workTime(g.asOf.AddDate(0, 0, -25), nil), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "M", Team: "Platform"})
	h.move(workTime(g.asOf.AddDate(0, 0, -24), nil), StatusInProgress)
	h.finish(workTime(g.asOf.AddDate(0, 0, -20), nil), StatusDone, ResolutionDone)
	h.reopen(workTime(g.asOf.AddDate(0, 0, -15), nil))
	h.finish(workTime(g.asOf.AddDate(0, 0, -12), nil), StatusDone, ResolutionDone)
	add(h, "done-reopen-done", "incomplete-reopen-followup")

	h = g.newHistory(p, "PB-5", "Old backlog item never started")
	h.create(time.Date(2025, 11, 3, 9, 0, 0, 0, time.UTC), TaskFields{Status: StatusBacklog, Type: "Story", Size: "L", Team: "Platform"})
	add(h, "old-open")

	h = g.newHistory(p, "PB-6", "Waits in the intake status the source reports as active")
	h.create(day(5, 18), TaskFields{Status: StatusTriage, Type: "Story", Size: "S", Team: "Platform"})
	add(h, "intake-active-category")
	return tasks
}

func (g *taskGen) importProject(p TaskProject) []Task {
	day := func(month, d int) time.Time { return time.Date(2026, time.Month(month), d, 9, 0, 0, 0, time.UTC) }
	var tasks []Task
	add := func(h *history, variants ...string) {
		task := h.done()
		for _, v := range variants {
			g.mark(v, task.ID)
		}
		tasks = append(tasks, task)
	}

	// The import project uses the key of a Jira DC task: two identities, no automatic link (V-17).
	h := g.newHistory(p, "PA-7", "Imported task with the key of a Jira DC task")
	h.create(day(5, 6), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(5, 7), StatusInProgress)
	h.finish(day(5, 15), StatusDone, ResolutionDone)
	add(h, "same-key-instances", "ambiguous-link")

	h = g.newHistory(p, "IMP-2", "Imported task created right in work")
	h.create(day(2, 9), TaskFields{Status: StatusInProgress, Type: "Story", Size: "M", Team: "Platform"})
	h.finish(day(2, 16), StatusDone, ResolutionDone)
	add(h, "created-active")

	h = g.newHistory(p, "IMP-3", "Imported task blocked by status")
	h.create(day(3, 2), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "S", Team: "Platform"})
	h.move(day(3, 3), StatusInProgress)
	h.move(day(3, 4), StatusBlocked)
	h.move(day(3, 6), StatusInProgress)
	h.finish(day(3, 9), StatusDone, ResolutionDone)
	add(h, "blocked")

	h = g.newHistory(p, "IMP-4", "Imported task cancelled")
	h.create(day(4, 6), TaskFields{Status: StatusBacklog, Type: "Task", Size: "M", Team: "Platform"})
	h.move(day(4, 7), StatusInProgress)
	h.finish(day(4, 9), StatusCancelled, ResolutionWontDo)
	add(h, "cancelled")

	h = g.newHistory(p, "IMP-5", "Imported task reopened inside a full window")
	h.create(day(6, 8), TaskFields{Status: StatusBacklog, Type: "Story", Size: "S", Team: "Platform"})
	h.move(day(6, 9), StatusInProgress)
	h.finish(day(6, 12), StatusDone, ResolutionDone)
	h.reopen(day(6, 20))
	h.finish(day(6, 24), StatusDone, ResolutionDone)
	add(h, "done-reopen-done", "complete-reopen-followup")

	h = g.newHistory(p, "IMP-6", "Imported task done five days before as-of")
	h.create(workTime(g.asOf.AddDate(0, 0, -12), nil), TaskFields{Status: StatusBacklog, Type: "Bug", Size: "S", Team: "Platform"})
	h.move(workTime(g.asOf.AddDate(0, 0, -11), nil), StatusInProgress)
	h.finish(workTime(g.asOf.AddDate(0, 0, -5), nil), StatusDone, ResolutionDone)
	add(h, "incomplete-reopen-followup")

	h = g.newHistory(p, "IMP-7", "Imported start delivered five days late")
	h.create(day(9, 14), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.late = _lateDelivery
	h.move(day(9, 15), StatusInProgress)
	h.late = 0
	h.finish(day(9, 25), StatusDone, ResolutionDone)
	add(h, "late-event")
	return tasks
}

// privateProject is the closed project of the second organization: one task with a fixed history.
func (g *taskGen) privateProject(p TaskProject, taskTarget, eventTarget int) ([]Task, error) {
	if taskTarget != 1 || eventTarget < 4 || eventTarget%2 != 0 {
		return nil, fmt.Errorf("private project expects one task with an even history of at least 4 events")
	}
	day := func(d int) time.Time { return time.Date(2026, 3, d, 10, 0, 0, 0, time.UTC) }
	h := g.newHistory(p, "SEC-1", "Task of the other organization")
	h.create(day(2), TaskFields{Status: StatusBacklog, Type: "Story", Size: "M", Team: "Platform"})
	h.move(day(3), StatusInProgress)
	h.loops(day(3), day(12), (eventTarget-4)/2)
	h.finish(day(13), StatusDone, ResolutionDone)
	return []Task{h.done()}, nil
}

// variantObjects lists the tasks of each catalog variant, adding the tasks with a full first start and
// first completion inside the period to multi-period-history.
func (g *taskGen) variantObjects(catalog []Variant) []VariantObjects {
	for id, start := range g.started {
		done, ok := g.completed[id]
		if ok && !start.Before(g.from) && done.Before(g.asOf) && !slices.Contains(g.variants["multi-period-history"], id) {
			g.mark("multi-period-history", id)
		}
	}
	var out []VariantObjects
	for _, v := range catalog {
		ids := slices.Clone(g.variants[v.ID])
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		out = append(out, VariantObjects{VariantID: v.ID, ObjectIDs: slices.Compact(ids)})
	}
	return out
}

// workTime moves an instant into the working day of the source: 07:00-16:59 UTC, whole seconds.
func workTime(t time.Time, r *rand.Rand) time.Time {
	t = t.UTC().Truncate(time.Hour)
	hour := t.Hour()
	if hour < 7 || hour > 16 {
		t = time.Date(t.Year(), t.Month(), t.Day(), 7+hour%10, 0, 0, 0, time.UTC)
	}
	if r != nil {
		t = t.Add(time.Duration(r.IntN(60)) * time.Minute)
	}
	return t
}

func volume(model *Model, area, kind string) int {
	n := 0
	for _, v := range model.Volumes {
		if v.AreaID == area && v.Kind == kind {
			n += v.Count
		}
	}
	return n
}

func pick(r *rand.Rand, values []string) string {
	return values[r.IntN(len(values))]
}

func nextSize(size string) string {
	if size == "S" {
		return "M"
	}
	return "S"
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
