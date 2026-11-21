package jiracloud

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _historyIssueLimit = 5

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	if err := validateWindow(in.Period, in.Limit); err != nil {
		return nil, err
	}
	projectID, err := projectScope(in.Scope)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return nil, err
	}
	return s.fetch(ctx, projectID, in, page)
}

func (s source) fetch(ctx context.Context, projectID string, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	m := s.manifest(in)
	switch in.SchemaId {
	case adaptersdk.FetchInputSchemaIdTasks, adaptersdk.FetchInputSchemaIdTaskEvents:
		if len(in.ObjectKeys) > 0 {
			return m, s.fetchTargets(ctx, projectID, in, m, page)
		}
		return m, s.fetchIssues(ctx, projectID, in, m, page)
	case adaptersdk.FetchInputSchemaIdAccounts:
		if len(in.ObjectKeys) != 0 {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "Jira accounts are read within the project")
		}
		return m, s.fetchAccounts(ctx, projectID, in, m, page)
	default:
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "Jira Cloud serves tasks, task_events and accounts")
	}
}

func (s source) fetchIssues(ctx context.Context, projectID string, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	after, err := readPosition(in.Cursor)
	if err != nil {
		return err
	}
	limit, fields := min(in.Limit, _pageSize), _taskFields
	isHistory := in.SchemaId == adaptersdk.FetchInputSchemaIdTaskEvents
	if isHistory {
		limit, fields = min(in.Limit, _historyIssueLimit), _inventoryFields
	} else if after.id == 0 {
		m.ReadId = new(m.CreatedAt.Format(time.RFC3339Nano))
		if err := s.dictionaries(ctx, projectID, page, m.CreatedAt); err != nil {
			return err
		}
	}
	last, err := s.scanIssues(ctx, projectID, fields, in.Period, after, limit, func(issue *jiraIssue) error {
		if !isHistory {
			return issue.task(page, m.CreatedAt, projectID)
		}
		next, err := s.writeHistory(ctx, page, issue, 0, m.CreatedAt)
		if err != nil && !hidden(err) && !errors.Is(err, adaptersdk.ErrPageFull) {
			return err
		}
		if next != 0 || err != nil {
			m.Completeness = envelope.ManifestCompletenessPartial
			m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{
				NativeId: issue.ID, RecordKind: envelope.TruncatedHistoryRecordKindTask,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	// The core stores the last non-final cursor as its watermark and widens it
	// before the next round. An empty terminal page preserves the last issue.
	if last.id != 0 {
		m.Selection.End = false
		m.Selection.NextCursor = last.cursor()
	}
	return nil
}

type targetPosition struct {
	id     string
	offset int
	at     time.Time
}

func (p targetPosition) cursor() *envelope.Cursor {
	return &envelope.Cursor{NativeId: p.id + "/" + strconv.Itoa(p.offset), ModifiedAt: p.at}
}

func targetKeys(in adaptersdk.FetchInput) ([]string, targetPosition, error) {
	ids := make([]string, 0, len(in.ObjectKeys))
	for _, key := range in.ObjectKeys {
		if _, ok := numericID(key.NativeId); !ok || key.ObjectKind != adaptersdk.ObjectKeyObjectKindTask || key.NativeNs != "" {
			return nil, targetPosition{}, invalidRequest("Jira object_keys require task identities with numeric IDs")
		}
		ids = append(ids, key.NativeId)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if in.Cursor == nil {
		return ids, targetPosition{}, nil
	}
	id, raw, found := strings.Cut(in.Cursor.NativeId, "/")
	offset, err := strconv.Atoi(raw)
	i, exists := slices.BinarySearch(ids, id)
	if !found || err != nil || offset < 0 || !exists || in.Cursor.ModifiedAt.IsZero() {
		return nil, targetPosition{}, invalidRequest("Jira target cursor is invalid")
	}
	if offset == 0 {
		i++
	}
	return ids[i:], targetPosition{id: id, offset: offset, at: in.Cursor.ModifiedAt}, nil
}

func (s source) fetchTargets(ctx context.Context, projectID string, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	ids, after, err := targetKeys(in)
	if err != nil {
		return err
	}
	limit := min(in.Limit, _historyIssueLimit)
	fields := _taskFields
	isHistory := in.SchemaId == adaptersdk.FetchInputSchemaIdTaskEvents
	if isHistory {
		fields = _inventoryFields
	} else if after.offset != 0 {
		return invalidRequest("a Jira changelog cursor requires task_events")
	}
	last := after
	for _, id := range ids[:min(len(ids), limit)] {
		var issue jiraIssue
		err := s.get(ctx, "issue", "issue/"+id, url.Values{"fields": {fields}}, &issue)
		if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) && after.offset == 0 {
			last = targetPosition{id: id, at: m.CreatedAt}
			continue
		}
		if err != nil {
			return err
		}
		at, err := issue.position()
		if err != nil || issue.ID != id || issue.Fields.Project.ID == "" {
			return invalidResponse("Jira returned a different issue or omitted its identity")
		}
		last = targetPosition{id: id, at: at.at}
		if issue.Fields.Project.ID != projectID {
			if id == after.id && after.offset != 0 {
				return invalidResponse("Jira issue moved during changelog collection")
			}
			continue
		}
		if !isHistory {
			if err := issue.task(page, m.CreatedAt, projectID); err != nil {
				return err
			}
			continue
		}
		offset := 0
		// Changes to the source revision invalidate the offset. Replay the immutable
		// changelog entries; ingest deduplicates by their native identities.
		if id == after.id && after.at.Equal(at.at) {
			offset = after.offset
		}
		next, err := s.writeHistory(ctx, page, &issue, offset, m.CreatedAt)
		if err != nil {
			return err
		}
		if next > 0 {
			last.offset = next
			break
		}
	}
	if last.offset > 0 || len(ids) > limit {
		m.Selection.End, m.Selection.NextCursor = false, last.cursor()
	}
	return nil
}
