package gitlab

import (
	"cmp"
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

const _fetchMRLimit = 5

var errSelectionFilled = errors.New("GitLab merge request selection filled")

type listedMR struct {
	ID        int64     `json:"id"`
	IID       int64     `json:"iid"`
	ProjectID int64     `json:"project_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type mergeRequest struct {
	listedMR
	Title          string  `json:"title"`
	State          string  `json:"state"`
	Draft          *bool   `json:"draft"`
	WorkInProgress bool    `json:"work_in_progress"`
	Author         account `json:"author"`
	MergeUser      account `json:"merge_user"`
	MergedBy       account `json:"merged_by"`
	ClosedBy       account `json:"closed_by"`
	SourceBranch   string  `json:"source_branch"`
	TargetBranch   string  `json:"target_branch"`
	SHA            string  `json:"sha"`
	DiffRefs       struct {
		HeadSHA  string `json:"head_sha"`
		BaseSHA  string `json:"base_sha"`
		StartSHA string `json:"start_sha"`
	} `json:"diff_refs"`
	Reviewers *[]account `json:"reviewers"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

func (mr listedMR) valid(projectID int64) bool {
	return mr.ID > 0 && mr.IID > 0 && mr.ProjectID == projectID && !mr.UpdatedAt.IsZero()
}

func compareMR(a, b listedMR) int {
	if byTime := a.UpdatedAt.Compare(b.UpdatedAt); byTime != 0 {
		return byTime
	}
	return cmp.Compare(a.ID, b.ID)
}

func (s source) listMRs(ctx context.Context, projectID int64, period adaptersdk.Period, cursor *adaptersdk.Cursor, limit int) ([]listedMR, error) {
	var after listedMR
	if cursor != nil {
		n, ok := numericID(cursor.NativeId)
		if !ok && cursor.NativeId != "" {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab merge request cursor")
		}
		after = listedMR{ID: n, UpdatedAt: cursor.ModifiedAt}
	}
	query := url.Values{"scope": {"all"}, "state": {"all"}, "order_by": {"updated_at"}, "sort": {"asc"}}
	from := period.From
	// The core subtracts the overlap before a new round and sends an empty native_id.
	if !after.UpdatedAt.IsZero() && (from == nil || after.UpdatedAt.After(*from)) {
		from = &after.UpdatedAt
	}
	if from != nil {
		query.Set("updated_after", from.UTC().Format(time.RFC3339Nano))
	}
	if period.To != nil {
		query.Set("updated_before", period.To.UTC().Format(time.RFC3339Nano))
	}
	path := projectPath(projectID) + "/merge_requests"
	items := make([]listedMR, 0, limit)
	for page := 1; page > 0; {
		next, err := readPage(ctx, s, "merge_requests", path, query, page, _sourcePageSize, func(mr listedMR) error {
			if !mr.valid(projectID) {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab merge request has no identity or update time")
			}
			if compareMR(mr, after) <= 0 || (from != nil && mr.UpdatedAt.Before(*from)) ||
				(period.To != nil && !mr.UpdatedAt.Before(*period.To)) {
				return nil
			}
			if len(items) == limit && mr.UpdatedAt.After(items[len(items)-1].UpdatedAt) {
				return errSelectionFilled
			}
			at, found := slices.BinarySearchFunc(items, mr, compareMR)
			if !found && at < limit {
				if len(items) == limit {
					items = items[:limit-1]
				}
				items = slices.Insert(items, at, mr)
			}
			return nil
		})
		if errors.Is(err, errSelectionFilled) {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		page = next
	}
	return items, nil
}

func (s source) findMRs(ctx context.Context, projectID int64, keys []adaptersdk.ObjectKey, cursor *adaptersdk.Cursor, limit int) ([]listedMR, error) {
	var after int64
	if cursor != nil {
		var ok bool
		after, ok = numericID(cursor.NativeId)
		if !ok {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab object selection cursor")
		}
	}
	var ids []int64
	for _, key := range keys {
		n, ok := numericID(key.NativeId)
		if !ok || key.NativeNs != "" || key.ObjectKind != adaptersdk.ObjectKeyObjectKindMergeRequest {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab object selection requires global merge request IDs")
		}
		if n > after {
			ids = append(ids, n)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	ids = ids[:min(len(ids), limit)]
	found := make(map[int64]listedMR, len(ids))
	query := url.Values{"scope": {"all"}, "state": {"all"}}
	path := projectPath(projectID) + "/merge_requests"
	for page := 1; page > 0 && len(found) < len(ids); {
		next, err := readPage(ctx, s, "find_merge_requests", path, query, page, _sourcePageSize, func(mr listedMR) error {
			if _, ok := slices.BinarySearch(ids, mr.ID); ok {
				if !mr.valid(projectID) {
					return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab merge request has no identity or update time")
				}
				found[mr.ID] = mr
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		page = next
	}
	items := make([]listedMR, 0, len(ids))
	for _, key := range ids {
		mr, ok := found[key]
		if !ok {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "merge request is not visible in this GitLab project")
		}
		items = append(items, mr)
	}
	return items, nil
}

func (s source) fetchMRs(ctx context.Context, projectID int64, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return err
	}
	m.Area.Key = &p.Path
	visibility := envelope.RepositoryV1Visibility(p.Visibility)
	if !visibility.Valid() {
		visibility = envelope.RepositoryV1VisibilityUnknown
	}
	if err := addRecord(page, "repository", "repository/1", "object", "", id(projectID), nil, nil, m.CreatedAt, envelope.RepositoryV1{
		Path: p.Path, Name: p.Name, DefaultBranch: optional(p.DefaultBranch), Visibility: visibility, WebUrl: p.WebURL, UpdatedAt: p.UpdatedAt,
	}); err != nil {
		return err
	}

	limit := min(in.Limit, _fetchMRLimit)
	var items []listedMR
	if len(in.ObjectKeys) == 0 {
		items, err = s.listMRs(ctx, projectID, in.Period, in.Cursor, limit+1)
	} else {
		items, err = s.findMRs(ctx, projectID, in.ObjectKeys, in.Cursor, limit+1)
	}
	if err != nil {
		return err
	}
	for _, item := range items[:min(len(items), limit)] {
		if err := s.fetchMR(ctx, item, m, page); err != nil {
			return err
		}
	}
	if len(items) > limit {
		last := items[limit-1]
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: last.UpdatedAt, NativeId: id(last.ID)}
	}
	return nil
}

func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	if in.SchemaId != adaptersdk.ManifestInputSchemaIdMergeRequests && in.SchemaId != adaptersdk.ManifestInputSchemaIdCiRuns {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitLab manifest serves merge_requests and ci_runs")
	}
	if in.Limit < 1 || in.Limit > 10000 {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid Manifest limit")
	}
	projectID, err := scopeID(in.Scope)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	cursor, err := decodeManifestCursor(in.Cursor)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	if in.SchemaId == adaptersdk.ManifestInputSchemaIdCiRuns {
		return s.manifestCIRuns(ctx, projectID, in, cursor)
	}
	limit := min(in.Limit, 100)
	items, err := s.listMRs(ctx, projectID, in.Period, cursor, limit+1)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	out := adaptersdk.ManifestPage{Items: make([]adaptersdk.ManifestItem, 0, min(len(items), limit))}
	for _, item := range items[:min(len(items), limit)] {
		out.Items = append(out.Items, adaptersdk.ManifestItem{
			NativeId: id(item.ID), RecordKind: "merge_request", UpdatedAt: &item.UpdatedAt,
		})
	}
	if len(items) > limit {
		last := items[limit-1]
		out.NextCursor = optional(last.UpdatedAt.UTC().Format(time.RFC3339Nano) + "/" + id(last.ID))
	}
	return out, nil
}

func decodeManifestCursor(raw *string) (*adaptersdk.Cursor, error) {
	if raw == nil {
		return nil, nil
	}
	stamp, nativeID, ok := strings.Cut(*raw, "/")
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if _, valid := numericID(nativeID); ok && valid && err == nil {
		return &adaptersdk.Cursor{ModifiedAt: at, NativeId: nativeID}, nil
	}
	return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab manifest cursor")
}

func (mr mergeRequest) payload(history mrHistory) (envelope.MergeRequestV1, error) {
	var state envelope.MergeRequestV1State
	switch mr.State {
	case "opened":
		state = envelope.MergeRequestV1StateOpen
	case "closed":
		state = envelope.MergeRequestV1StateClosed
	case "merged":
		state = envelope.MergeRequestV1StateMerged
	case "locked":
		// GitLab briefly locks a merge request while merging. A later collection sees its final state.
		return envelope.MergeRequestV1{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitLab merge request is temporarily locked")
	default:
		return envelope.MergeRequestV1{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "unknown GitLab merge request state")
	}
	draft := mr.WorkInProgress
	if mr.Draft != nil {
		draft = *mr.Draft
	}
	ready, closed := history.readyAt, mr.ClosedAt
	if closed == nil && state == envelope.MergeRequestV1StateClosed {
		closed = history.closedAt
	}
	iid, err := strconv.Atoi(id(mr.IID))
	if err != nil {
		return envelope.MergeRequestV1{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "invalid GitLab merge request IID")
	}
	hasReviewers := mr.Reviewers != nil
	head := cmp.Or(mr.SHA, mr.DiffRefs.HeadSHA)
	var base *string
	if mr.DiffRefs.HeadSHA == head {
		base = optional(mr.DiffRefs.BaseSHA)
	}
	out := envelope.MergeRequestV1{
		ProjectNativeId: id(mr.ProjectID), Iid: &iid, Title: optional(mr.Title), State: state, Draft: draft,
		AuthorAccountNativeId: mr.Author.nativeID(), SourceBranch: mr.SourceBranch, TargetBranch: mr.TargetBranch,
		OpenedAt: mr.CreatedAt, UpdatedAt: &mr.UpdatedAt, ReadyAt: ready, MergedAt: mr.MergedAt, ClosedAt: closed,
		HeadSha: head, BaseSha: base, MergeBaseSha: base,
		ReviewerField: &hasReviewers,
	}
	if hasReviewers {
		reviewers := make([]string, 0, len(*mr.Reviewers))
		for _, user := range *mr.Reviewers {
			if user.ID > 0 {
				reviewers = append(reviewers, id(user.ID))
			}
		}
		slices.Sort(reviewers)
		reviewers = slices.Compact(reviewers)
		out.ReviewerAccountNativeIds = &reviewers
	}
	if !validSHA(out.HeadSha) || out.OpenedAt.IsZero() || state == envelope.MergeRequestV1StateMerged && mr.MergedAt == nil {
		return envelope.MergeRequestV1{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab merge request has no head, creation or merge time")
	}
	return out, nil
}
