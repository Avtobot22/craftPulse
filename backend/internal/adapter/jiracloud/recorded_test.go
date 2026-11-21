package jiracloud

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRecordedCloudResponses(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/cloud-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Search     []json.RawMessage `json:"search"`
		History    []json.RawMessage `json:"history"`
		Restricted jiraIssue         `json:"restricted"`
		Accounts   []jiraAccount     `json:"accounts"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Search) != 2 || len(fixture.History) != 3 {
		t.Fatal("recorded page counts changed")
	}
	for _, body := range fixture.Search {
		page := issueSearch{Issues: responseRows[jiraIssue]{limit: 2}}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if page.IsLast == nil || len(page.Issues.items) == 0 {
			t.Fatal("recorded search page has no issues or end marker")
		}
		for i := range page.Issues.items {
			issue := &page.Issues.items[i]
			if _, err := issue.position(); err != nil || issue.Fields.Project.ID == "" {
				t.Fatal("recorded issue has no position or project")
			}
		}
	}
	start := 0
	for i, body := range fixture.History {
		page := changelogPage{Values: responseRows[change]{limit: _changelogPageSize}}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if page.StartAt == nil || *page.StartAt != start || page.Total == nil || *page.Total != 122 ||
			page.IsLast == nil || *page.IsLast != (i == len(fixture.History)-1) {
			t.Fatal("invalid recorded changelog metadata")
		}
		for row := range page.Values.items {
			change := &page.Values.items[row]
			if change.ID == "" || change.Created.IsZero() || len(change.Items) == 0 {
				t.Fatal("invalid recorded changelog entry")
			}
			for _, item := range change.Items {
				event := item.event("80000", item.fieldKey(), change.Author.AccountID)
				if event.FieldNativeId == nil || *event.FieldNativeId == "" || !event.EventKind.Valid() {
					t.Fatal("invalid canonical event from recorded changelog")
				}
			}
		}
		start += len(page.Values.items)
	}
	if start != 122 || fixture.Restricted.Fields.Security.ID == "" || len(fixture.Accounts) == 0 {
		t.Fatal("recorded fixture lacks changelog entries, issue security or accounts")
	}
	for _, account := range fixture.Accounts {
		if account.AccountID == "" || account.Email != "" {
			t.Fatal("recorded account has no accountId or still contains a real email")
		}
	}
}
