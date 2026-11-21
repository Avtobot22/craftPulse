package jiracloud

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type jiraIssue struct {
	ID     string     `json:"id"`
	Key    string     `json:"key"`
	Fields jiraFields `json:"fields"`
}

type reference struct {
	ID string `json:"id"`
}

type jiraLink struct {
	ID      string     `json:"id"`
	Type    reference  `json:"type"`
	Inward  *reference `json:"inwardIssue,omitempty"`
	Outward *reference `json:"outwardIssue,omitempty"`
}

type issueFields struct {
	Created    jiraTime    `json:"created"`
	Updated    jiraTime    `json:"updated"`
	Summary    string      `json:"summary"`
	Project    reference   `json:"project"`
	Type       reference   `json:"issuetype"`
	Status     reference   `json:"status"`
	Resolution reference   `json:"resolution"`
	Security   reference   `json:"security"`
	Parent     reference   `json:"parent"`
	Assignee   jiraAccount `json:"assignee"`
	Links      *[]jiraLink `json:"issuelinks"`
}

type jiraFields struct {
	issueFields
	raw map[string]json.RawMessage
}

func (f *jiraFields) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &f.issueFields); err != nil {
		return err
	}
	return json.Unmarshal(data, &f.raw)
}

func (i *jiraIssue) position() (position, error) {
	id, ok := numericID(i.ID)
	if !ok || i.Fields.Updated.IsZero() {
		return position{}, invalidResponse("Jira issue has no numeric ID or updated timestamp")
	}
	return position{at: i.Fields.Updated.Time, id: id}, nil
}

func (i *jiraIssue) task(page *adaptersdk.Page, observed time.Time, projectID string) error {
	f := i.Fields
	if f.Created.IsZero() || f.Status.ID == "" || f.Type.ID == "" || f.Project.ID != projectID {
		return invalidResponse("Jira issue fields are incomplete or refer to another project")
	}
	raw := make(map[string]*string)
	for fieldID, value := range f.raw {
		if !strings.HasPrefix(fieldID, "customfield_") {
			continue
		}
		if text, ok := fieldValue(value); ok {
			raw[fieldID] = text
		}
	}
	if f.Links != nil {
		for _, link := range *f.Links {
			if link.ID == "" || link.Type.ID == "" || (link.Inward == nil) == (link.Outward == nil) {
				return invalidResponse("Jira issue link is missing its identity, type or destination")
			}
			direction, target := "outward", link.Outward
			if link.Inward != nil {
				direction, target = "inward", link.Inward
			}
			if _, ok := numericID(target.ID); !ok {
				return invalidResponse("Jira issue link has no numeric destination ID")
			}
			raw["issuelink/"+link.ID] = new(direction + "/" + link.Type.ID + "/" + target.ID)
		}
	}
	return addRecord(page, "task", "object", "", i.ID, nil, nil, observed, envelope.TaskV1{
		ProjectNativeId: projectID, TaskKey: optional(i.Key), Title: bounded(f.Summary),
		TypeNativeId: f.Type.ID, StatusNativeId: f.Status.ID, ResolutionNativeId: optional(f.Resolution.ID),
		CreatedAt: f.Created.Time, UpdatedAt: &f.Updated.Time, AssigneeAccountNativeId: optional(f.Assignee.AccountID),
		ParentNativeId: optional(f.Parent.ID), Restriction: optional(f.Security.ID), RawFields: &raw,
	})
}

func fieldValue(data json.RawMessage) (*string, bool) {
	if len(data) == 0 {
		return nil, false
	}
	if string(data) == "null" {
		return nil, true
	}
	switch data[0] {
	case '"':
		var value string
		if json.Unmarshal(data, &value) == nil {
			return bounded(value), true
		}
	case '{':
		var option struct {
			ID    string  `json:"id"`
			Value *string `json:"value"`
		}
		if json.Unmarshal(data, &option) == nil {
			if option.ID != "" {
				return bounded(option.ID), true
			}
			if option.Value != nil {
				return bounded(*option.Value), true
			}
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil {
			return nil, false
		}
		values := make([]string, 0, len(items))
		for _, item := range items {
			if len(item) == 0 || item[0] == '[' {
				return nil, false
			}
			value, ok := fieldValue(item)
			if !ok || value == nil {
				return nil, false
			}
			values = append(values, *value)
		}
		slices.Sort(values)
		data, err := json.Marshal(values)
		if err == nil && len(data) <= 1024 {
			return new(string(data)), true
		}
	default:
		return bounded(string(data)), true
	}
	return nil, false
}

func bounded(value string) *string {
	if len(value) > 1024 {
		end := 1024
		for !utf8.RuneStart(value[end]) {
			end--
		}
		value = value[:end]
	}
	return optional(value)
}

func addRecord(page *adaptersdk.Page, kind, class, namespace, id string, revision *string, eventTime *time.Time, observed time.Time, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	rec := &envelope.Record{
		RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: kind + "/1",
		RecordClass: envelope.RecordRecordClass(class), NativeNs: namespace, NativeId: id,
		SourceRevision: revision, EventTime: eventTime, ObservedTime: &observed, Payload: payload,
	}
	if class == "object" && revision == nil {
		hash, err := envelope.ContentHash(rec)
		if err != nil {
			return err
		}
		rec.SourceRevision = &hash
	}
	return page.AddRecord(rec)
}
