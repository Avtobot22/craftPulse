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
	_accountWindow      = 100
	_accountSearchLimit = 1000
)

type jiraAccount struct {
	AccountID   string `json:"accountId"`
	AccountType string `json:"accountType"`
	Name        string `json:"displayName"`
	Email       string `json:"emailAddress"`
	Active      bool   `json:"active"`
}

func (u jiraAccount) record(page *adaptersdk.Page, now time.Time) error {
	if u.AccountID == "" {
		return invalidResponse("Jira account has no accountId")
	}
	kind := envelope.AccountV1AccountKindUnknown
	switch u.AccountType {
	case "atlassian", "customer":
		kind = envelope.AccountV1AccountKindHuman
	case "app":
		kind = envelope.AccountV1AccountKindBot
	}
	return addRecord(page, "account", "object", "", u.AccountID, nil, nil, now, envelope.AccountV1{
		DisplayName: optional(u.Name), Email: optional(u.Email), Active: u.Active, AccountKind: kind,
	})
}

func (s source) fetchAccounts(ctx context.Context, projectID string, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	start, offset, err := accountsPosition(in.Cursor)
	if err != nil {
		return err
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return err
	}
	users := responseRows[jiraAccount]{limit: _accountWindow}
	err = s.get(ctx, "project_accounts", "user/viewissue/search", url.Values{
		"projectKey": {p.Key}, "query": {"*"}, "startAt": {strconv.Itoa(start)}, "maxResults": {strconv.Itoa(_accountWindow)},
	}, &users)
	if err != nil {
		return err
	}
	out := users.items
	if out == nil || offset > len(out) {
		return invalidResponse("Jira project account page is invalid or has changed")
	}
	end := min(offset+in.Limit, len(out))
	for _, user := range out[offset:end] {
		if err := user.record(page, m.CreatedAt); err != nil {
			return err
		}
	}
	if end == len(out) {
		// The endpoint filters a site-wide window, so an empty window is not a
		// terminal page. Its documented coverage stops at the thousandth user.
		start, end = start+_accountWindow, 0
	}
	m.Completeness = envelope.ManifestCompletenessPartial
	if start < _accountSearchLimit {
		at := m.CreatedAt
		if in.Cursor != nil && in.Cursor.NativeId != "" {
			at = in.Cursor.ModifiedAt
		}
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: at, NativeId: strconv.Itoa(start) + "/" + strconv.Itoa(end)}
	}
	return nil
}

func accountsPosition(cur *adaptersdk.Cursor) (start int, offset int, err error) {
	if cur == nil || cur.NativeId == "" {
		return 0, 0, nil
	}
	rawStart, rawOffset, ok := strings.Cut(cur.NativeId, "/")
	start, startErr := strconv.Atoi(rawStart)
	offset, offsetErr := strconv.Atoi(rawOffset)
	if cur.ModifiedAt.IsZero() || !ok || startErr != nil || offsetErr != nil ||
		start < 0 || start >= _accountSearchLimit || start%_accountWindow != 0 || offset < 0 || offset >= _accountWindow {
		return 0, 0, invalidRequest("Jira accounts cursor is invalid")
	}
	return start, offset, nil
}
