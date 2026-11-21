package jiracloud

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type jiraStatus struct {
	named
	Category struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type jiraType struct {
	named
	Subtask  bool         `json:"subtask"`
	Statuses []jiraStatus `json:"statuses"`
}

type jiraField struct {
	named
	Custom bool `json:"custom"`
	Schema struct {
		Type  string `json:"type"`
		Items string `json:"items"`
	} `json:"schema"`
}

func (s source) dictionaries(ctx context.Context, projectID string, page *adaptersdk.Page, now time.Time) error {
	var types []jiraType
	if err := s.get(ctx, "statuses", "project/"+projectID+"/statuses", nil, &types); err != nil {
		return err
	}
	seen := make(map[string]struct{})
	for _, item := range types {
		if err := addSnapshot(page, "task_type", item.named, now, envelope.TaskTypeV1{Name: item.Name, Subtask: item.Subtask}); err != nil {
			return err
		}
		for _, status := range item.Statuses {
			if _, ok := seen[status.ID]; ok {
				continue
			}
			seen[status.ID] = struct{}{}
			category := envelope.TaskStatusV1Category(status.Category.Key)
			if !category.Valid() {
				category = envelope.TaskStatusV1CategoryUnknown
			}
			if err := addSnapshot(page, "task_status", status.named, now, envelope.TaskStatusV1{
				Name: status.Name, Category: category, ProjectNativeId: &projectID,
			}); err != nil {
				return err
			}
		}
	}
	if err := s.resolutions(ctx, page, now); err != nil {
		return err
	}
	var fields []jiraField
	if err := s.get(ctx, "fields", "field", nil, &fields); err != nil {
		return err
	}
	for _, field := range fields {
		kind := field.Schema.Type
		if kind == "array" && field.Schema.Items != "" {
			kind += "/" + field.Schema.Items
		}
		if kind == "" {
			kind = "unknown"
		}
		if err := addSnapshot(page, "task_field", field.named, now,
			envelope.TaskFieldV1{Name: field.Name, Custom: field.Custom, ValueType: kind}); err != nil {
			return err
		}
	}
	return nil
}

func (s source) resolutions(ctx context.Context, page *adaptersdk.Page, now time.Time) error {
	for start := 0; ; {
		var out struct {
			StartAt *int     `json:"startAt"`
			IsLast  *bool    `json:"isLast"`
			Values  *[]named `json:"values"`
		}
		err := s.get(ctx, "resolutions", "resolution/search", url.Values{
			"maxResults": {"50"}, "startAt": {strconv.Itoa(start)},
		}, &out)
		if err != nil {
			return err
		}
		if out.StartAt == nil || *out.StartAt != start || out.IsLast == nil || out.Values == nil {
			return invalidResponse("Jira resolution page metadata is invalid")
		}
		for _, item := range *out.Values {
			if err := addSnapshot(page, "task_resolution", item, now, envelope.TaskResolutionV1{Name: item.Name}); err != nil {
				return err
			}
		}
		if *out.IsLast {
			return nil
		}
		if len(*out.Values) == 0 {
			return invalidResponse("Jira resolution pagination does not advance")
		}
		start += len(*out.Values)
	}
}

func addSnapshot(page *adaptersdk.Page, kind string, item named, now time.Time, value any) error {
	if item.ID == "" || strings.TrimSpace(item.Name) == "" {
		return invalidResponse("Jira dictionary entry has no ID or name")
	}
	return addRecord(page, kind, "snapshot", "", item.ID, nil, nil, now, value)
}
