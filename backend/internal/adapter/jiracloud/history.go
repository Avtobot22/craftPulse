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

const (
	_changelogPageSize  = 50
	_inlineHistoryPages = 2
)

type change struct {
	ID      string       `json:"id"`
	Created jiraTime     `json:"created"`
	Author  jiraAccount  `json:"author"`
	Items   []changeItem `json:"items"`
}

type changeItem struct {
	FieldID    string  `json:"fieldId"`
	Field      string  `json:"field"`
	From       *string `json:"from"`
	FromString *string `json:"fromString"`
	To         *string `json:"to"`
	ToString   *string `json:"toString"`
}

type changelogPage struct {
	StartAt *int                 `json:"startAt"`
	Total   *int                 `json:"total"`
	IsLast  *bool                `json:"isLast"`
	Values  responseRows[change] `json:"values"`
}

func (s source) changelog(ctx context.Context, id string, start, limit int) (changelogPage, error) {
	out := changelogPage{Values: responseRows[change]{limit: limit}}
	err := s.get(ctx, "changelog", "issue/"+id+"/changelog", url.Values{
		"startAt": {strconv.Itoa(start)}, "maxResults": {strconv.Itoa(limit)},
	}, &out)
	if err != nil {
		return changelogPage{}, err
	}
	if out.StartAt == nil || *out.StartAt != start || out.Total == nil || out.IsLast == nil || out.Values.items == nil {
		return changelogPage{}, invalidResponse("Jira changelog page metadata is invalid")
	}
	count := len(out.Values.items)
	if count > limit || *out.Total < start || count > *out.Total-start ||
		*out.IsLast != (count == *out.Total-start) || count == 0 && !*out.IsLast {
		return changelogPage{}, invalidResponse("Jira changelog page does not match its reported size")
	}
	return out, nil
}

func (s source) writeHistory(ctx context.Context, page *adaptersdk.Page, issue *jiraIssue, start int, now time.Time) (int, error) {
	if issue.Fields.Created.IsZero() {
		return 0, invalidResponse("Jira issue has no created timestamp")
	}
	if start == 0 {
		if err := addRecord(page, "task_event", "event", issue.ID, issue.ID+"/created", new("created"),
			&issue.Fields.Created.Time, now, envelope.TaskEventV1{TaskNativeId: issue.ID, EventKind: envelope.TaskEventV1EventKindCreated}); err != nil {
			return 0, err
		}
	}
	var previousTotal *int
	seen := make(map[string]struct{}, _inlineHistoryPages*_changelogPageSize)
	for range _inlineHistoryPages {
		res, err := s.changelog(ctx, issue.ID, start, _changelogPageSize)
		if err != nil {
			return 0, err
		}
		if previousTotal != nil && *res.Total != *previousTotal {
			return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "Jira changelog changed during collection")
		}
		previousTotal = res.Total
		rows := res.Values.items
		for i := range rows {
			row := &rows[i]
			if _, exists := seen[row.ID]; exists {
				return 0, invalidResponse("Jira returned a duplicate changelog entry")
			}
			seen[row.ID] = struct{}{}
			if err := row.records(page, issue.ID, now); err != nil {
				return 0, err
			}
		}
		if *res.IsLast {
			return 0, nil
		}
		start += len(res.Values.items)
	}
	return start, nil
}

func (r *change) records(page *adaptersdk.Page, issueID string, now time.Time) error {
	if _, ok := numericID(r.ID); !ok || r.Created.IsZero() || r.Items == nil {
		return invalidResponse("Jira changelog entry has no ID, timestamp or items")
	}
	occurrences := make(map[string]int)
	for _, item := range r.Items {
		field := item.fieldKey()
		if field == "" || len(field) > 200 {
			return invalidResponse("Jira changelog item has no valid field ID")
		}
		key := r.ID + "/" + url.PathEscape(field)
		occurrences[field]++
		if n := occurrences[field]; n > 1 {
			key += "/" + strconv.Itoa(n)
		}
		value := item.event(issueID, field, r.Author.AccountID)
		if err := addRecord(page, "task_event", "event", issueID, key, &r.ID, &r.Created.Time, now, value); err != nil {
			return err
		}
	}
	return nil
}

func (i changeItem) fieldKey() string {
	if i.FieldID != "" {
		return i.FieldID
	}
	switch strings.ToLower(i.Field) {
	case "status", "resolution", "assignee", "reporter", "creator", "project", "security", "description", "attachment", "comment":
		return strings.ToLower(i.Field)
	case "issue link", "link":
		return "issuelinks"
	default:
		return i.Field
	}
}

func (i changeItem) event(issueID, field, accountID string) envelope.TaskEventV1 {
	out := envelope.TaskEventV1{
		TaskNativeId: issueID, EventKind: envelope.TaskEventV1EventKindFieldChange,
		FieldNativeId: &field, ActorAccountNativeId: optional(accountID),
	}
	switch field {
	case "assignee", "reporter", "creator":
		out.BeforeAccountNativeId, out.AfterAccountNativeId = i.From, i.To
	case "issuelinks", "Link":
		out.EventKind = envelope.TaskEventV1EventKindLinkChange
		out.BeforeValue = changeValue(i.From, i.FromString)
		out.AfterValue = changeValue(i.To, i.ToString)
	case "project":
		out.EventKind = envelope.TaskEventV1EventKindMoved
		out.BeforeValue = changeValue(i.From, i.FromString)
		out.AfterValue = changeValue(i.To, i.ToString)
	case "description", "attachment", "comment":
		// Keep the change identity and timestamp without transferring the content.
	default:
		out.BeforeValue = changeValue(i.From, i.FromString)
		out.AfterValue = changeValue(i.To, i.ToString)
	}
	return out
}

func changeValue(native, display *string) *string {
	if native != nil {
		return bounded(*native)
	}
	if display != nil {
		return bounded(*display)
	}
	return nil
}
