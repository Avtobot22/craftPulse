package dataset

import (
	"fmt"
	"slices"
	"time"
)

// MinFullFlow is the least number of tasks with a first start and a first completion inside the period.
const MinFullFlow = 20

// ValidateTasks checks the task model against the volumes, the vocabularies of its projects and the
// order of every history; the generator runs it before writing anything.
func ValidateTasks(file *TaskFile, model *Model) error {
	asOf, err := parseSecond(model.AsOf)
	if err != nil {
		return err
	}
	if file.SchemaVersion != "1" || file.AsOf != model.AsOf || file.Period != model.Period {
		return fmt.Errorf("task model does not match the model period and as_of")
	}
	projects := make(map[string]TaskProject, len(file.Projects))
	for _, p := range file.Projects {
		projects[p.ID] = p
	}
	people := map[string][]string{}
	for _, person := range model.People {
		if !person.Bot {
			people[person.OrganizationID] = append(people[person.OrganizationID], person.ID)
		}
	}
	tasks, events := map[string]int{}, map[string]int{}
	seen := map[string]string{}
	for _, task := range file.Tasks {
		p, ok := projects[task.ProjectID]
		if !ok || task.OrganizationID != p.OrganizationID || task.InstanceID != p.InstanceID {
			return fmt.Errorf("task %s: unknown project", task.ID)
		}
		if seen[task.ID] != "" {
			return fmt.Errorf("task %s: repeated ID", task.ID)
		}
		seen[task.ID] = task.OrganizationID
		tasks[p.ID]++
		events[p.ID] += len(task.Events)
		if err := validateHistory(task, p, people[p.OrganizationID], asOf); err != nil {
			return fmt.Errorf("task %s: %w", task.ID, err)
		}
	}
	for _, task := range file.Tasks {
		if err := validateLinks(task, seen); err != nil {
			return fmt.Errorf("task %s: %w", task.ID, err)
		}
	}
	for _, p := range file.Projects {
		if want := volume(model, p.ID, "task"); tasks[p.ID] != want {
			return fmt.Errorf("%s: %d tasks, the volume is %d", p.ID, tasks[p.ID], want)
		}
		if want := volume(model, p.ID, "task_event"); events[p.ID] != want {
			return fmt.Errorf("%s: %d events, the volume is %d", p.ID, events[p.ID], want)
		}
	}
	catalog := map[string]bool{}
	for _, v := range model.Variants {
		catalog[v.ID] = true
	}
	for _, v := range file.Variants {
		if !catalog[v.VariantID] || len(v.ObjectIDs) == 0 {
			return fmt.Errorf("variant %s is unknown or has no tasks", v.VariantID)
		}
		for _, id := range v.ObjectIDs {
			if seen[id] == "" {
				return fmt.Errorf("variant %s refers to unknown task %s", v.VariantID, id)
			}
		}
		if v.VariantID == "multi-period-history" && len(v.ObjectIDs) < MinFullFlow {
			return fmt.Errorf("only %d tasks have a full start and completion in the period", len(v.ObjectIDs))
		}
	}
	return nil
}

// validateLinks checks that every link points once to another known task of the same organization.
func validateLinks(task Task, orgOf map[string]string) error {
	seen := map[TaskLink]bool{}
	for _, l := range task.Links {
		switch {
		case l.Kind != LinkBlocks && l.Kind != LinkRelates && l.Kind != LinkDuplicates:
			return fmt.Errorf("link kind %q is unknown", l.Kind)
		case l.TaskID == task.ID || orgOf[l.TaskID] != task.OrganizationID:
			return fmt.Errorf("link to %s leaves the tasks of the organization", l.TaskID)
		case seen[l]:
			return fmt.Errorf("link %s %s is repeated", l.Kind, l.TaskID)
		}
		seen[l] = true
	}
	return nil
}

func validateHistory(task Task, p TaskProject, people []string, asOf time.Time) error {
	if len(task.Events) == 0 || task.Events[0].Kind != EventCreated || task.Events[0].Initial == nil {
		return fmt.Errorf("history must start with the created event")
	}
	if task.Epic != "" && !slices.ContainsFunc(p.Epics, func(e TaskEpic) bool { return e.Key == task.Epic }) {
		return fmt.Errorf("epic %q is not in the project", task.Epic)
	}
	statuses := make([]string, len(p.Statuses))
	for i, s := range p.Statuses {
		statuses[i] = s.Name
	}
	allowed := map[string][]string{
		FieldStatus:     statuses,
		FieldType:       p.Types,
		FieldSize:       append([]string{""}, p.Sizes...),
		FieldTeam:       p.Teams,
		FieldResolution: append([]string{""}, p.Resolutions...),
		FieldFlagged:    {"", FlagImpediment},
		FieldAssignee:   append([]string{""}, people...),
	}
	initial := task.Events[0].Initial
	current := map[string]string{
		FieldStatus: initial.Status, FieldType: initial.Type, FieldSize: initial.Size, FieldTeam: initial.Team, FieldAssignee: initial.Assignee,
	}
	for field, value := range current {
		if !slices.Contains(allowed[field], value) {
			return fmt.Errorf("initial %s %q is not in the project", field, value)
		}
	}
	var previous int64
	for i, e := range task.Events {
		if e.ID != fmt.Sprintf("%s/e%02d", task.ID, i+1) {
			return fmt.Errorf("event %d has an unstable ID", i)
		}
		at, err := parseSecond(e.EventTime.UTC)
		if err != nil || at.Format("Z07:00") != "Z" || !validOffset(e.EventTime.OriginalOffset) {
			return fmt.Errorf("event %s: invalid event time", e.ID)
		}
		if at.Unix() < previous || at.Unix() >= asOf.Unix() {
			return fmt.Errorf("event %s: out of order or not before as_of", e.ID)
		}
		previous = at.Unix()
		if e.ObservedTime != nil {
			observed, err := parseSecond(e.ObservedTime.UTC)
			if err != nil || observed.Before(at) || observed.Unix() >= asOf.Unix() {
				return fmt.Errorf("event %s: observed time must be between event time and as_of", e.ID)
			}
		}
		if (e.Kind == EventComment) != (e.Body != "") {
			return fmt.Errorf("event %s: only a comment has a body, and it is not empty", e.ID)
		}
		if i == 0 {
			continue
		}
		if e.Kind == EventComment {
			if e.Field != "" || e.From != "" || e.To != "" || e.Initial != nil {
				return fmt.Errorf("event %s: a comment changes no field", e.ID)
			}
			continue
		}
		if e.Kind != EventFieldChange || e.Initial != nil {
			return fmt.Errorf("event %s: only the first event creates the task", e.ID)
		}
		values, ok := allowed[e.Field]
		if !ok || !slices.Contains(values, e.To) || e.From != current[e.Field] || e.From == e.To {
			return fmt.Errorf("event %s: invalid change of %s", e.ID, e.Field)
		}
		current[e.Field] = e.To
	}
	return nil
}
