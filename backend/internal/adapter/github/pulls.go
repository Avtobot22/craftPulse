package github

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _pullPageLimit = 3

type coordinate struct {
	ID        int64     `json:"id"`
	Number    int       `json:"number"`
	UpdatedAt time.Time `json:"updated_at"`
}

type branch struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo struct {
		ID int64 `json:"id"`
	} `json:"repo"`
}

type pull struct {
	coordinate
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	User      account    `json:"user"`
	Base      branch     `json:"base"`
	Head      branch     `json:"head"`
	Reviewers *[]account `json:"requested_reviewers"`
	OpenedAt  time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

func (p pull) validate(repoID int64) error {
	if p.ID <= 0 || p.Number <= 0 || p.UpdatedAt.IsZero() || p.Base.Repo.ID != repoID {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub pull request lacks identity or update time")
	}
	return nil
}

func compare(a, b coordinate) int {
	if c := a.UpdatedAt.Compare(b.UpdatedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.ID, b.ID)
}

var errBeforeWindow = errors.New("GitHub listing reached the lower bound")

// GitHub lists newest first. Keep only the earliest pending coordinates plus
// lookahead so advancing the cursor cannot skip older items or timestamp ties.
func (s source) selectPulls(ctx context.Context, repo repository, period adaptersdk.Period, cursor *adaptersdk.Cursor,
	keys []adaptersdk.ObjectKey, limit int,
) ([]coordinate, error) {
	var after coordinate
	if cursor != nil {
		n, ok := numeric(cursor.NativeId)
		if !ok && (cursor.NativeId != "" || len(keys) != 0) {
			return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub pull request cursor")
		}
		after = coordinate{ID: n, UpdatedAt: cursor.ModifiedAt}
	}
	targeted := len(keys) > 0
	wanted := make(map[int64]bool)
	if targeted {
		var pending []int64
		for _, key := range keys {
			n, ok := numeric(key.NativeId)
			if !ok || key.NativeNs != "" || key.ObjectKind != adaptersdk.ObjectKeyObjectKindMergeRequest {
				return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitHub selection requires global pull request IDs")
			}
			if n > after.ID {
				pending = append(pending, n)
			}
		}
		slices.Sort(pending)
		pending = slices.Compact(pending)
		for _, n := range pending[:min(len(pending), limit)] {
			wanted[n] = true
		}
		if len(wanted) == 0 {
			return nil, nil
		}
	}
	items := make([]coordinate, 0, limit)
	selected := make(map[int64]struct{}, limit)
	query := url.Values{"state": {"all"}, "sort": {"updated"}, "direction": {"desc"}, "per_page": {"30"}}
	if !targeted && period.From != nil && period.From.After(after.UpdatedAt) {
		after = coordinate{UpdatedAt: *period.From}
	}
	err := walk(ctx, s, "pulls", repo.path()+"/pulls", query, false, func(p pull) error {
		if err := p.validate(repo.ID); err != nil {
			return err
		}
		if targeted {
			if wanted[p.ID] {
				items = append(items, p.coordinate)
				delete(wanted, p.ID)
				if len(wanted) == 0 {
					return errBeforeWindow
				}
			}
			return nil
		}
		if p.UpdatedAt.Before(after.UpdatedAt) {
			return errBeforeWindow
		}
		if compare(p.coordinate, after) <= 0 || period.To != nil && !p.UpdatedAt.Before(*period.To) {
			return nil
		}
		if _, exists := selected[p.ID]; exists {
			return nil
		}
		at, _ := slices.BinarySearchFunc(items, p.coordinate, compare)
		if at < limit {
			if len(items) == limit {
				delete(selected, items[limit-1].ID)
				items = items[:limit-1]
			}
			items = slices.Insert(items, at, p.coordinate)
			selected[p.ID] = struct{}{}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errBeforeWindow) {
		return nil, err
	}
	if len(wanted) != 0 {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "GitHub pull request is not visible in this repository")
	}
	if targeted {
		slices.SortFunc(items, func(a, b coordinate) int { return cmp.Compare(a.ID, b.ID) })
	}
	return items, nil
}

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	if in.SchemaId != adaptersdk.FetchInputSchemaIdMergeRequests && in.SchemaId != adaptersdk.FetchInputSchemaIdAccounts {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitHub serves merge_requests and accounts")
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub Fetch limit")
	}
	if in.SchemaId == adaptersdk.FetchInputSchemaIdAccounts && len(in.ObjectKeys) != 0 {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitHub accounts are read within the repository")
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return nil, err
	}
	repo, err := s.repository(ctx, in.Scope)
	if err != nil {
		return nil, err
	}
	m := manifest(in)
	m.Area.Key = &repo.FullName
	if in.SchemaId == adaptersdk.FetchInputSchemaIdMergeRequests {
		visibility := envelope.RepositoryV1Visibility(repo.Visibility)
		if !visibility.Valid() {
			visibility = envelope.RepositoryV1VisibilityUnknown
		}
		if err := writeRecord(page, object("repository", "", id(repo.ID), m.CreatedAt), envelope.RepositoryV1{
			Path: repo.FullName, Name: repo.Name, DefaultBranch: optional(repo.DefaultBranch), Visibility: visibility, WebUrl: repo.URL, UpdatedAt: repo.UpdatedAt,
		}); err != nil {
			return nil, err
		}
	}
	limit := min(in.Limit, _pullPageLimit)
	items, err := s.selectPulls(ctx, repo, in.Period, in.Cursor, in.ObjectKeys, limit+1)
	if err != nil {
		return nil, err
	}
	writtenAccounts := make(map[int64]bool)
	for _, item := range items[:min(len(items), limit)] {
		if err := s.fetchPull(ctx, repo, item, in, m, page, writtenAccounts); err != nil {
			return nil, err
		}
	}
	if len(items) > limit {
		last := items[limit-1]
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: last.UpdatedAt, NativeId: id(last.ID)}
	}
	return m, nil
}

func (s source) fetchPull(ctx context.Context, repo repository, item coordinate, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page, writtenAccounts map[int64]bool) error {
	var p pull
	path := repo.path() + "/pulls/" + id(int64(item.Number))
	if err := s.get(ctx, "pull", path, &p); err != nil {
		return err
	}
	if err := p.validate(repo.ID); err != nil {
		return err
	}
	if p.ID != item.ID || p.Number != item.Number || p.OpenedAt.IsZero() || !validSHA(p.Head.SHA) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub pull request details do not match listing")
	}
	if len(in.ObjectKeys) == 0 && in.Period.To != nil && !p.UpdatedAt.Before(*in.Period.To) {
		return nil // The next overlapping round will read the moved PR.
	}
	accountsOnly := in.SchemaId == adaptersdk.FetchInputSchemaIdAccounts
	h := history{pull: p, page: page, observed: m.CreatedAt, accounts: make(map[int64]account), accountsOnly: accountsOnly}
	h.remember(p.User)
	reviewers := make([]string, 0)
	if p.Reviewers != nil {
		for _, reviewer := range *p.Reviewers {
			if reviewer.ID <= 0 {
				continue
			}
			h.remember(reviewer)
			reviewers = append(reviewers, id(reviewer.ID))
		}
	}
	slices.Sort(reviewers)
	reviewers = slices.Compact(reviewers)
	if err := s.readHistory(ctx, repo, &h); err != nil {
		return err
	}
	if err := s.writeAccounts(ctx, &h, writtenAccounts); err != nil {
		return err
	}
	if accountsOnly {
		if h.truncated {
			m.Completeness = envelope.ManifestCompletenessPartial
		}
		return nil
	}
	if h.truncated {
		m.Completeness = envelope.ManifestCompletenessPartial
		m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{RecordKind: "merge_request", NativeId: id(p.ID)})
	}
	state := envelope.MergeRequestV1State(p.State)
	if p.MergedAt != nil {
		state = envelope.MergeRequestV1StateMerged
	}
	if !state.Valid() {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "unknown GitHub pull request state")
	}
	reviewerField := p.Reviewers != nil
	record := envelope.MergeRequestV1{
		ProjectNativeId: id(repo.ID), Iid: &p.Number, Title: optional(p.Title), State: state, Draft: p.Draft,
		AuthorAccountNativeId: p.User.nativeID(), SourceBranch: p.Head.Ref, TargetBranch: p.Base.Ref,
		HeadSha: p.Head.SHA, BaseSha: optional(p.Base.SHA), OpenedAt: p.OpenedAt, ReadyAt: h.readyAt,
		ClosedAt: p.ClosedAt, MergedAt: p.MergedAt, UpdatedAt: &p.UpdatedAt, ReviewerField: &reviewerField,
	}
	if reviewerField {
		record.ReviewerAccountNativeIds = &reviewers
	}
	return writeRecord(page, object("merge_request", "", id(p.ID), m.CreatedAt), record)
}
