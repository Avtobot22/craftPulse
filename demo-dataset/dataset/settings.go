package dataset

import (
	"fmt"
	"slices"
	"time"
)

// Settings are what the administrator and the process owner confirm on step 2 of the demo: the workflow
// mapping in the shape of the MappingRule of the API, targets by type and size, the reopen window of T23
// and the tasks shown in the mapping preview.
type Settings struct {
	SchemaVersion  string          `json:"schema_version"`
	OrganizationID string          `json:"organization_id"`
	TeamID         string          `json:"team_id"`
	Mapping        MappingSettings `json:"mapping"`
	Targets        []TargetVersion `json:"targets"`
	Reopen         ReopenWindow    `json:"reopen"`
	PreviewTaskIDs []string        `json:"preview_task_ids"`
}

type MappingSettings struct {
	VersionNo   int           `json:"version_no"`
	ConfirmedAt string        `json:"confirmed_at"`
	ConfirmedBy string        `json:"confirmed_by"`
	Rules       []MappingRule `json:"rules"`
}

// MappingRule follows the MappingRule of the API; native values are the names the dataset gives to
// statuses, fields and options, and the loader turns them into the IDs the source assigns.
type MappingRule struct {
	RuleKind    string `json:"rule_kind"`
	InstanceID  string `json:"instance_id"`
	NativeRef   string `json:"native_ref"`
	NativeValue string `json:"native_value"`
	Meaning     string `json:"meaning"`
	Rank        *int   `json:"rank"`
}

// TargetVersion takes effect when it is confirmed and is never set back in time (V-16).
type TargetVersion struct {
	TeamID          string `json:"team_id"`
	TaskType        string `json:"task_type"`
	SizeLabel       string `json:"size_label"`
	VersionNo       int    `json:"version_no"`
	DurationSeconds int64  `json:"duration_seconds"`
	Basis           string `json:"basis"`
	ConfirmedAt     string `json:"confirmed_at"`
	ConfirmedBy     string `json:"confirmed_by"`
}

// ReopenWindow is the follow-up window of T23 in the given definition version, in calendar days.
type ReopenWindow struct {
	MetricID          string `json:"metric_id"`
	DefinitionVersion string `json:"definition_version"`
	WindowDays        int    `json:"window_days"`
}

const _day = int64(24 * time.Hour / time.Second)

var _previewTasks = []string{"PA-1", "PA-2", "PA-3", "PA-4", "PA-5", "PA-6", "PA-8", "PA-10", "PA-12", "PA-13"}

func GenerateSettings(tasks *TaskFile) (*Settings, error) {
	const (
		org     = "org-1"
		owner   = "org-1/person-01"
		manager = "org-1/person-02"
	)
	s := &Settings{
		SchemaVersion: "1", OrganizationID: org, TeamID: "team-1",
		Mapping: MappingSettings{VersionNo: 1, ConfirmedAt: "2026-01-05T09:00:00Z", ConfirmedBy: owner},
		Reopen:  ReopenWindow{MetricID: "T23", DefinitionVersion: "1", WindowDays: 30},
	}
	for _, instance := range []string{"jira-dc-1", "universal-1"} {
		s.Mapping.Rules = append(s.Mapping.Rules, mappingRules(instance)...)
	}
	v1, v2 := "2026-02-01T09:00:00Z", "2026-07-01T09:00:00Z"
	for _, t := range []struct {
		typ, size string
		days      int64
	}{{"Story", "S", 2}, {"Story", "M", 5}, {"Story", "L", 10}, {"Bug", "S", 1}, {"Bug", "M", 3}, {"Bug", "L", 6}} {
		s.Targets = append(s.Targets, TargetVersion{TeamID: "team-1", TaskType: t.typ, SizeLabel: t.size, VersionNo: 1,
			DurationSeconds: t.days * _day, Basis: "manual", ConfirmedAt: v1, ConfirmedBy: manager})
	}
	s.Targets = append(s.Targets, TargetVersion{TeamID: "team-1", TaskType: "Story", SizeLabel: "S", VersionNo: 2,
		DurationSeconds: 4 * _day, Basis: "manual", ConfirmedAt: v2, ConfirmedBy: manager})
	for _, key := range _previewTasks {
		s.PreviewTaskIDs = append(s.PreviewTaskIDs, org+"/jira-dc-1/"+key)
	}
	if err := ValidateSettings(s, tasks); err != nil {
		return nil, err
	}
	return s, nil
}

func mappingRules(instance string) []MappingRule {
	var rules []MappingRule
	add := func(kind, ref, value, meaning string, rank *int) {
		rules = append(rules, MappingRule{RuleKind: kind, InstanceID: instance, NativeRef: ref, NativeValue: value, Meaning: meaning, Rank: rank})
	}
	for _, s := range []struct{ status, meaning string }{
		{StatusBacklog, "intake"}, {StatusTriage, "intake"}, {StatusInProgress, "active"}, {StatusInReview, "active"},
		{StatusWaiting, "wait"}, {StatusBlocked, "blocked"}, {StatusDone, "done"}, {StatusCancelled, "cancelled"},
		{StatusDuplicate, "duplicate"},
	} {
		add("status", s.status, "", s.meaning, nil)
	}
	add("resolution", ResolutionDone, "", "accepted", nil)
	add("resolution", ResolutionWontDo, "", "cancelled", nil)
	add("resolution", ResolutionDuplicate, "", "duplicate", nil)
	for _, t := range []string{"Story", "Bug", "Task"} {
		add("issue_type", t, "", t, nil)
	}
	// A20 and T28 count stories and bugs; plain tasks do not need a MR.
	add("code_required_type", "Story", "", "code_required", nil)
	add("code_required_type", "Bug", "", "code_required", nil)
	add("size_field", FieldSize, "", FieldSize, nil)
	for i, size := range []string{"S", "M", "L"} {
		rank := i + 1
		add("size_value", FieldSize, size, size, &rank)
	}
	add("team_field", FieldTeam, "", FieldTeam, nil)
	add("team_value", FieldTeam, "Platform", "team-1", nil)
	add("team_value", FieldTeam, "Payments", "team-2", nil)
	add("blocked_flag", FieldFlagged, FlagImpediment, "blocked", nil)
	return rules
}

// ValidateSettings checks that every status of the first organization except UnknownStatus is mapped,
// that targets keep their order of confirmation and that the preview shows ten existing tasks.
func ValidateSettings(s *Settings, tasks *TaskFile) error {
	mapped := map[[2]string]bool{}
	for _, r := range s.Mapping.Rules {
		if r.RuleKind == "status" {
			mapped[[2]string{r.InstanceID, r.NativeRef}] = true
		}
	}
	byID := map[string]Task{}
	for _, task := range tasks.Tasks {
		byID[task.ID] = task
		if task.OrganizationID != s.OrganizationID {
			continue
		}
		for _, e := range task.Events {
			status := e.To
			if e.Kind == EventCreated {
				status = e.Initial.Status
			} else if e.Field != FieldStatus {
				continue
			}
			if status != UnknownStatus && !mapped[[2]string{task.InstanceID, status}] {
				return fmt.Errorf("status %q of %s has no mapping rule", status, task.InstanceID)
			}
			if status == UnknownStatus && mapped[[2]string{task.InstanceID, status}] {
				return fmt.Errorf("status %q must stay unmapped", status)
			}
		}
	}
	versions := map[[2]string][]TargetVersion{}
	for _, t := range s.Targets {
		key := [2]string{t.TaskType, t.SizeLabel}
		versions[key] = append(versions[key], t)
		if t.DurationSeconds <= 0 {
			return fmt.Errorf("target %v must be positive", key)
		}
	}
	multiple := false
	for key, vs := range versions {
		for i, v := range vs {
			if v.VersionNo != i+1 {
				return fmt.Errorf("target %v: versions must be numbered in order", key)
			}
			if i > 0 && v.ConfirmedAt <= vs[i-1].ConfirmedAt {
				return fmt.Errorf("target %v: a version cannot be confirmed before the previous one", key)
			}
		}
		multiple = multiple || len(vs) > 1
	}
	if !multiple {
		return fmt.Errorf("targets need a group with two versions")
	}
	if len(s.PreviewTaskIDs) != 10 || len(slices.Compact(slices.Sorted(slices.Values(s.PreviewTaskIDs)))) != 10 {
		return fmt.Errorf("preview needs ten different tasks")
	}
	for _, id := range s.PreviewTaskIDs {
		if task, ok := byID[id]; !ok || task.OrganizationID != s.OrganizationID {
			return fmt.Errorf("preview task %s is not a task of the organization", id)
		}
	}
	return nil
}
