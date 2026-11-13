package dataset_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func tasks(t *testing.T, seed uint64) (*dataset.Model, *dataset.TaskFile) {
	t.Helper()
	m := model(t, seed)
	file, err := dataset.GenerateTasks(m)
	if err != nil {
		t.Fatal(err)
	}
	return m, file
}

func byKey(t *testing.T, file *dataset.TaskFile, instance, key string) dataset.Task {
	t.Helper()
	for _, task := range file.Tasks {
		if task.InstanceID == instance && task.Key == key {
			return task
		}
	}
	t.Fatalf("task %s/%s is missing", instance, key)
	return dataset.Task{}
}

func at(t *testing.T, e dataset.TaskEvent) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, e.EventTime.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// statusAt replays the history and returns the status in force at the moment.
func statusAt(t *testing.T, task dataset.Task, moment time.Time) string {
	t.Helper()
	status := ""
	for _, e := range task.Events {
		if at(t, e).After(moment) {
			break
		}
		switch {
		case e.Kind == dataset.EventCreated:
			status = e.Initial.Status
		case e.Field == dataset.FieldStatus:
			status = e.To
		}
	}
	return status
}

func changes(task dataset.Task, field string) []dataset.TaskEvent {
	var out []dataset.TaskEvent
	for _, e := range task.Events {
		if e.Field == field {
			out = append(out, e)
		}
	}
	return out
}

func TestGenerateTasks_Volumes(t *testing.T) {
	t.Parallel()
	_, file := tasks(t, 21)
	counts := map[string][2]int{}
	for _, task := range file.Tasks {
		c := counts[task.OrganizationID+"/"+task.ProjectID]
		counts[task.OrganizationID+"/"+task.ProjectID] = [2]int{c[0] + 1, c[1] + len(task.Events)}
	}
	want := map[string][2]int{
		"org-1/jira-project-a": {40, 400}, "org-1/jira-project-b": {40, 400}, "org-1/import-project": {20, 200},
		"org-2/private-project": {1, 10},
	}
	if len(counts) != len(want) {
		t.Fatalf("projects: %v", counts)
	}
	for key, w := range want {
		if counts[key] != w {
			t.Fatalf("%s: %v, want %v", key, counts[key], w)
		}
	}
}

func TestGenerateTasks_DeterministicForEverySeed(t *testing.T) {
	t.Parallel()
	_, first := tasks(t, 21)
	_, again := tasks(t, 21)
	_, other := tasks(t, 22)
	h1, err := dataset.Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := dataset.Hash(again)
	h3, _ := dataset.Hash(other)
	if h1 != h2 || h1 == h3 {
		t.Fatal("the seed must control the task model byte for byte")
	}
	for seed := uint64(1); seed <= 200; seed++ {
		m := model(t, seed)
		file, err := dataset.GenerateTasks(m)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if _, err := dataset.GenerateSettings(file); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestGenerateTasks_EveryTaskVariantHasTasks(t *testing.T) {
	t.Parallel()
	m, file := tasks(t, 21)
	marked := map[string][]string{}
	for _, v := range file.Variants {
		marked[v.VariantID] = v.ObjectIDs
	}
	for _, v := range m.Variants {
		if !slices.Contains(v.Sources, "jira_cloud") && !slices.Contains(v.Sources, "universal") {
			continue
		}
		if len(marked[v.ID]) == 0 {
			t.Errorf("variant %s has no task", v.ID)
		}
	}
	if n := len(marked["multi-period-history"]); n < dataset.MinFullFlow {
		t.Fatalf("%d tasks with a full start and completion, want at least %d", n, dataset.MinFullFlow)
	}
}

func TestGenerateTasks_FlowVariants(t *testing.T) {
	t.Parallel()
	m, file := tasks(t, 21)
	asOf, _ := time.Parse(time.RFC3339, m.AsOf)
	from, _ := time.Parse(time.RFC3339, m.Period.From)
	const jira, imp = "jira-cloud-1", "universal-1"

	if task := byKey(t, file, jira, "PA-1"); task.Events[0].Initial.Status != dataset.StatusInProgress {
		t.Error("created-active: the task is created in work")
	}
	intake := byKey(t, file, jira, "PA-2")
	if intake.Events[0].Initial.Status != dataset.StatusTriage || file.Projects[0].Statuses[1] != (dataset.TaskStatus{Name: dataset.StatusTriage, Category: "indeterminate"}) {
		t.Error("intake-active-category: Triage is an intake the source reports as active")
	}

	blocked := byKey(t, file, jira, "PA-3")
	start := changes(blocked, dataset.FieldStatus)[0]
	if start.EventTime != (dataset.SourceTime{UTC: "2026-10-02T00:00:00Z", OriginalOffset: "+03:00"}) {
		t.Errorf("blocked: the start must be 2026-10-02T03:00:00+03:00, got %+v", start.EventTime)
	}
	flagOn, flagOff := at(t, changes(blocked, dataset.FieldFlagged)[0]), at(t, changes(blocked, dataset.FieldFlagged)[1])
	var statusBlocked []time.Time
	for _, e := range changes(blocked, dataset.FieldStatus) {
		if e.To == dataset.StatusBlocked {
			statusBlocked = append(statusBlocked, at(t, e))
		}
	}
	if !statusBlocked[0].After(flagOn) || !statusBlocked[0].Before(flagOff) {
		t.Error("blocked: the status block must start while the flag is set")
	}

	if task := byKey(t, file, jira, "PA-4"); changes(task, dataset.FieldResolution)[0].To != dataset.ResolutionWontDo {
		t.Error("cancelled: resolution Won't Do")
	}
	if task := byKey(t, file, jira, "PA-5"); changes(task, dataset.FieldStatus)[0].To != dataset.StatusDuplicate {
		t.Error("duplicate: closed as a duplicate without start")
	}

	reopened := byKey(t, file, jira, "PA-6")
	var done []time.Time
	for _, e := range changes(reopened, dataset.FieldStatus) {
		if e.To == dataset.StatusDone {
			done = append(done, at(t, e))
		}
	}
	if len(done) != 2 || statusAt(t, reopened, done[0].Add(6*24*time.Hour)) != dataset.StatusInProgress {
		t.Error("done-reopen-done: two completions with a reopen between them")
	}
	if !done[0].AddDate(0, 0, 30).Before(asOf) {
		t.Error("complete-reopen-followup: the 30-day window must end before as-of")
	}
	if late := byKey(t, file, jira, "PA-14"); !at(t, late.Events[len(late.Events)-1]).AddDate(0, 0, 30).After(asOf) {
		t.Error("incomplete-reopen-followup: the window must still be open at as-of")
	}

	sized := byKey(t, file, jira, "PA-8")
	startAt := at(t, changes(sized, dataset.FieldStatus)[0])
	if sized.Events[0].Initial.Size != "S" || !at(t, changes(sized, dataset.FieldSize)[0]).After(startAt) || changes(sized, dataset.FieldSize)[0].To != "M" {
		t.Error("size-change: S at start, M after start")
	}
	if team := changes(byKey(t, file, jira, "PA-9"), dataset.FieldTeam); len(team) != 1 || team[0].To != "Payments" {
		t.Error("team-change: one change of team")
	}

	old := byKey(t, file, jira, "PA-10")
	if !at(t, old.Events[len(old.Events)-1]).Before(from) || statusAt(t, old, asOf) != dataset.StatusInProgress {
		t.Error("old-open: started before the period, no updates inside it, still open")
	}
	if n := len(byKey(t, file, jira, "PA-11").Events); n <= 2*m.PageSize {
		t.Errorf("incomplete-history-page: %d events fit into two pages", n)
	}
	if task := byKey(t, file, jira, "PA-12"); statusAt(t, task, asOf) != dataset.UnknownStatus {
		t.Error("unknown-status: the task stays in the unmapped status")
	}
	if task := byKey(t, file, jira, "PA-13"); task.Events[0].Initial.Size != "" || len(changes(task, dataset.FieldSize)) != 0 {
		t.Error("types-sizes-targets: a task without size")
	}

	same, other := byKey(t, file, jira, "PA-7"), byKey(t, file, imp, "PA-7")
	if same.ID == other.ID || same.Key != other.Key {
		t.Error("same-key-instances: one key, two identities")
	}
	lateTask := byKey(t, file, jira, "PA-15")
	last := lateTask.Events[len(lateTask.Events)-1]
	if last.ObservedTime == nil || last.ObservedTime.UTC <= last.EventTime.UTC {
		t.Error("late-event: the completion is observed after it happened")
	}
	for _, task := range file.Tasks {
		for _, e := range task.Events {
			if e.ObservedTime != nil && task.ID != lateTask.ID && task.Key != "IMP-7" {
				t.Errorf("%s: only late-event tasks have an observed time", e.ID)
			}
		}
	}
}

func TestGenerateTasks_Rejects(t *testing.T) {
	t.Parallel()
	m, file := tasks(t, 21)
	broken := clone(t, *file)
	broken.Tasks[0].Events[1].EventTime.UTC = "2025-01-01T00:00:00Z"
	if err := dataset.ValidateTasks(&broken, m); err == nil {
		t.Error("accepted an event before the previous one")
	}
	broken = clone(t, *file)
	broken.Tasks[0].Events[1].From = "Nowhere"
	if err := dataset.ValidateTasks(&broken, m); err == nil {
		t.Error("accepted a change that does not continue the history")
	}
	broken = clone(t, *file)
	broken.Tasks = broken.Tasks[1:]
	if err := dataset.ValidateTasks(&broken, m); err == nil {
		t.Error("accepted a model below the volume")
	}
}

func TestGenerateSettings(t *testing.T) {
	t.Parallel()
	_, file := tasks(t, 21)
	s, err := dataset.GenerateSettings(file)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, r := range s.Mapping.Rules {
		kinds[r.RuleKind]++
		if r.RuleKind == "status" && r.NativeRef == dataset.UnknownStatus {
			t.Error("the unknown status must stay unmapped")
		}
		if r.RuleKind == "size_value" && r.Rank == nil {
			t.Error("size values need a rank")
		}
	}
	if kinds["blocked_flag"] != 2 || kinds["code_required_type"] != 4 || kinds["status"] != 18 {
		t.Errorf("rules by kind: %v", kinds)
	}
	var storyS []dataset.TargetVersion
	for _, v := range s.Targets {
		if v.TaskType == "Story" && v.SizeLabel == "S" {
			storyS = append(storyS, v)
		}
	}
	if len(storyS) != 2 || storyS[0].DurationSeconds != 2*86400 || storyS[1].DurationSeconds != 4*86400 || storyS[1].ConfirmedAt <= storyS[0].ConfirmedAt {
		t.Errorf("Story S needs v1 = 2 days and a later v2 = 4 days: %+v", storyS)
	}
	if s.Reopen.WindowDays != 30 || len(s.PreviewTaskIDs) != 10 {
		t.Error("reopen window of 30 days and ten preview tasks")
	}

	broken := clone(t, *s)
	broken.Mapping.Rules = broken.Mapping.Rules[1:]
	if err := dataset.ValidateSettings(&broken, file); err == nil {
		t.Error("accepted a status of the model without mapping")
	}
	broken = clone(t, *s)
	broken.Targets[len(broken.Targets)-1].ConfirmedAt = "2026-01-01T00:00:00Z"
	if err := dataset.ValidateSettings(&broken, file); err == nil {
		t.Error("accepted a target version confirmed before the previous one")
	}
}
