package jiracloud

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_inventoryFields = "updated,created,project"
	_taskFields      = "*all,-description,-attachment,-comment,-worklog"
)

type jiraTime struct{ time.Time }

func (t *jiraTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var err error
	t.Time, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Time, err = time.Parse("2006-01-02T15:04:05.999999999-0700", value)
	}
	if err != nil {
		return invalidResponse("Jira timestamp is invalid")
	}
	return nil
}

type responseRows[T any] struct {
	items []T
	limit int
}

func (r *responseRows[T]) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	start, err := dec.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('[') {
		return invalidResponse("Jira page items must be an array")
	}
	r.items = make([]T, 0, r.limit)
	for dec.More() {
		if len(r.items) == r.limit {
			return invalidResponse("Jira page exceeds the requested size")
		}
		var item T
		if err := dec.Decode(&item); err != nil {
			return err
		}
		r.items = append(r.items, item)
	}
	_, err = dec.Token()
	return err
}

type issueSearch struct {
	Issues        responseRows[jiraIssue] `json:"issues"`
	IsLast        *bool                   `json:"isLast"`
	NextPageToken string                  `json:"nextPageToken"`
}

func (s source) search(ctx context.Context, jql, fields, token string, limit int) (issueSearch, error) {
	query := url.Values{"jql": {jql}, "fields": {fields}, "maxResults": {strconv.Itoa(limit)}}
	if token != "" {
		query.Set("nextPageToken", token)
	}
	out := issueSearch{Issues: responseRows[jiraIssue]{limit: limit}}
	if err := s.get(ctx, "search", "search/jql", query, &out); err != nil {
		return issueSearch{}, err
	}
	if out.Issues.items == nil || out.IsLast == nil || len(out.NextPageToken) > 8192 ||
		!*out.IsLast && (out.NextPageToken == "" || out.NextPageToken == token) ||
		*out.IsLast && out.NextPageToken != "" {
		return issueSearch{}, invalidResponse("Jira search page or continuation is invalid")
	}
	return out, nil
}

type position struct {
	at time.Time
	id int64
}

func (p position) compare(q position) int {
	if order := p.at.Compare(q.at); order != 0 {
		return order
	}
	return cmp.Compare(p.id, q.id)
}

func (p position) cursor() *envelope.Cursor {
	return &envelope.Cursor{ModifiedAt: p.at, NativeId: strconv.FormatInt(p.id, 10)}
}

func readPosition(cur *adaptersdk.Cursor) (position, error) {
	if cur == nil {
		return position{}, nil
	}
	if cur.ModifiedAt.IsZero() {
		return position{}, invalidRequest("Jira issue cursor requires a timestamp")
	}
	if cur.NativeId == "" {
		return position{at: cur.ModifiedAt}, nil
	}
	id, ok := numericID(cur.NativeId)
	if !ok {
		return position{}, invalidRequest("Jira issue cursor requires a numeric issue ID")
	}
	return position{at: cur.ModifiedAt, id: id}, nil
}

func issueJQL(projectID string, lower, now time.Time) string {
	jql := "project = " + projectID
	// Relative JQL dates avoid assuming the collector's time zone. Compare the exact
	// timestamp and ID locally because JQL filters have minute precision.
	if !lower.IsZero() {
		ago := max(0, now.Sub(lower)/time.Minute) + 2
		jql += ` AND updated >= "-` + strconv.FormatInt(int64(ago), 10) + `m"`
	}
	return jql + " ORDER BY updated ASC, id ASC"
}

func (s source) scanIssues(ctx context.Context, projectID, fields string, period adaptersdk.Period, after position, limit int, visit func(*jiraIssue) error) (position, error) {
	lower := after.at
	if period.From != nil && period.From.After(lower) {
		lower = *period.From
	}
	if period.To != nil && !lower.Before(*period.To) {
		return position{}, nil
	}
	jql := issueJQL(projectID, lower, time.Now())
	var last position
	token, remaining := "", limit
	seenTokens := make(map[string]struct{})
	for {
		res, err := s.search(ctx, jql, fields, token, min(_pageSize, max(remaining, 2)))
		if err != nil {
			return position{}, err
		}
		issues := res.Issues.items
		for i := range issues {
			issue := &issues[i]
			if issue.Fields.Project.ID != projectID {
				return position{}, invalidResponse("Jira search returned an issue outside the project")
			}
			p, err := issue.position()
			if err != nil {
				return position{}, err
			}
			if p.at.Before(lower) || p.compare(after) <= 0 || p.compare(last) == 0 {
				continue
			}
			if p.compare(last) < 0 {
				return position{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "Jira search order changed during collection")
			}
			if period.To != nil && !p.at.Before(*period.To) {
				return last, nil
			}
			if err := visit(issue); err != nil {
				return position{}, err
			}
			last = p
			remaining--
			if remaining == 0 {
				return last, nil
			}
		}
		if *res.IsLast {
			return last, nil
		}
		token = res.NextPageToken
		if _, seen := seenTokens[token]; seen || len(seenTokens) == 1000 {
			return position{}, invalidResponse("Jira search pagination does not advance")
		}
		seenTokens[token] = struct{}{}
	}
}

func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	if in.SchemaId != adaptersdk.ManifestInputSchemaIdTasks {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "Jira Cloud provides a tasks manifest")
	}
	projectID, err := projectScope(in.Scope)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	if err := validateWindow(in.Period, in.Limit); err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	return s.inventory(ctx, projectID, in)
}

func (s source) inventory(ctx context.Context, projectID string, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	var after position
	if in.Cursor != nil {
		id, ts, ok := strings.Cut(*in.Cursor, "/")
		at, err := time.Parse(time.RFC3339Nano, ts)
		if !ok || err != nil || id == "" {
			return adaptersdk.ManifestPage{}, invalidRequest("Jira manifest cursor is invalid")
		}
		after, err = readPosition(&adaptersdk.Cursor{ModifiedAt: at, NativeId: id})
		if err != nil {
			return adaptersdk.ManifestPage{}, err
		}
	}
	limit := min(in.Limit, 100)
	out := adaptersdk.ManifestPage{Items: make([]adaptersdk.ManifestItem, 0, limit)}
	last, err := s.scanIssues(ctx, projectID, _inventoryFields, in.Period, after, limit, func(issue *jiraIssue) error {
		out.Items = append(out.Items, adaptersdk.ManifestItem{RecordKind: "task", NativeId: issue.ID, UpdatedAt: &issue.Fields.Updated.Time})
		return nil
	})
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	if len(out.Items) == limit {
		key := last.cursor()
		out.NextCursor = new(key.NativeId + "/" + key.ModifiedAt.UTC().Format(time.RFC3339Nano))
	}
	return out, nil
}

func validateWindow(period adaptersdk.Period, limit int) error {
	if limit < 1 || limit > 1000 || period.From != nil && period.To != nil && !period.From.Before(*period.To) {
		return invalidRequest("Jira selection requires a valid limit and period")
	}
	return nil
}
