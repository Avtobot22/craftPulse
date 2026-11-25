package github

import (
	"cmp"
	"context"
	"errors"
	"hash"
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
	Title          string     `json:"title"`
	State          string     `json:"state"`
	Draft          bool       `json:"draft"`
	User           account    `json:"user"`
	Base           branch     `json:"base"`
	Head           branch     `json:"head"`
	Reviewers      *[]account `json:"requested_reviewers"`
	RequestedTeams *[]team    `json:"requested_teams"`
	OpenedAt       time.Time  `json:"created_at"`
	MergedAt       *time.Time `json:"merged_at"`
	ClosedAt       *time.Time `json:"closed_at"`
}

type team struct {
	ID int64 `json:"id"`
}

func (t team) nativeID() *string {
	if t.ID <= 0 {
		return nil
	}
	// Team and user IDs can overlap. The canonical references need distinct keys.
	return optional("team/" + id(t.ID))
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
	keys []adaptersdk.ObjectKey, limit int, fingerprint hash.Hash,
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
		if fingerprint != nil {
			line := id(p.ID) + "/" + id(int64(p.Number)) + "/" + p.UpdatedAt.UTC().Format(time.RFC3339Nano) + "\n"
			_, _ = fingerprint.Write([]byte(line))
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
	resume, err := parseHistoryCursor(in)
	if err != nil {
		return nil, err
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
	writtenAccounts := make(map[int64]bool)
	if resume != nil {
		if err := s.fetchPull(ctx, repo, resume.pull, in, m, page, writtenAccounts, resume); err != nil {
			return nil, err
		}
		if m.Selection.End && slices.ContainsFunc(in.ObjectKeys, func(key adaptersdk.ObjectKey) bool {
			n, ok := numeric(key.NativeId)
			return ok && n > resume.pull.ID
		}) {
			m.Selection.End = false
			m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: resume.pull.UpdatedAt, NativeId: id(resume.pull.ID)}
		}
		return m, nil
	}
	items, err := s.selectPulls(ctx, repo, in.Period, in.Cursor, in.ObjectKeys, limit+1, nil)
	if err != nil {
		return nil, err
	}
	for _, item := range items[:min(len(items), limit)] {
		if err := s.fetchPull(ctx, repo, item, in, m, page, writtenAccounts, nil); err != nil {
			return nil, err
		}
		if m.Selection.NextCursor != nil {
			return m, nil
		}
	}
	if len(items) > limit {
		last := items[limit-1]
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: last.UpdatedAt, NativeId: id(last.ID)}
	}
	return m, nil
}

func (s source) fetchPull(ctx context.Context, repo repository, item coordinate, in adaptersdk.FetchInput,
	m *envelope.Manifest, page *adaptersdk.Page, writtenAccounts map[int64]bool, resume *historyCursor,
) error {
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
	var pos historyPosition
	var reviewers *[]string
	if resume == nil {
		h.remember(p.User)
		reviewers = h.reviewers()
	} else {
		pos = resume.position
		h.truncated = !item.UpdatedAt.Equal(p.UpdatedAt)
		if resume.activityRef != "" && resume.activityRef != activityRefKey(p.Head) {
			pos.page = ""
			h.truncated = true
		}
		h.currentRevision = resume.currentRevision && !h.truncated
	}
	next, err := s.readHistory(ctx, repo, &h, pos)
	if err != nil {
		return err
	}
	if err := s.writeAccounts(ctx, &h, writtenAccounts); err != nil {
		return err
	}
	if next != nil {
		if len(in.ObjectKeys) == 0 {
			h.truncated = true
		} else {
			m.Selection.End = false
			m.Selection.NextCursor = historyNextCursor(p, *next, h.currentRevision)
		}
	}
	if h.truncated {
		m.Completeness = envelope.ManifestCompletenessPartial
		if !accountsOnly {
			m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{
				RecordKind: envelope.TruncatedHistoryRecordKindMergeRequest, NativeId: id(p.ID),
			})
		}
	}
	if accountsOnly || resume != nil {
		return nil
	}
	return h.writePull(repo.ID, reviewers)
}

func (h *history) reviewers() *[]string {
	var ids []string
	complete := h.pull.Reviewers != nil && h.pull.RequestedTeams != nil
	if h.pull.Reviewers != nil {
		for _, reviewer := range *h.pull.Reviewers {
			h.remember(reviewer)
			native := reviewer.nativeID()
			if native == nil {
				complete = false
				continue
			}
			ids = append(ids, *native)
		}
	}
	if h.pull.RequestedTeams != nil {
		for _, reviewer := range *h.pull.RequestedTeams {
			native := reviewer.nativeID()
			if native == nil {
				complete = false
				continue
			}
			ids = append(ids, *native)
		}
	}
	if !complete {
		return nil
	}
	if len(ids) == 0 {
		ids = []string{}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	return &ids
}

func (h *history) writePull(repoID int64, reviewers *[]string) error {
	p := h.pull
	state := envelope.MergeRequestV1State(p.State)
	if p.MergedAt != nil {
		state = envelope.MergeRequestV1StateMerged
	}
	if !state.Valid() {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "unknown GitHub pull request state")
	}
	reviewerField := reviewers != nil
	record := envelope.MergeRequestV1{
		ProjectNativeId: id(repoID), Iid: &p.Number, Title: optional(p.Title), State: state, Draft: p.Draft,
		AuthorAccountNativeId: p.User.nativeID(), SourceBranch: p.Head.Ref, TargetBranch: p.Base.Ref,
		HeadSha: p.Head.SHA, BaseSha: optional(p.Base.SHA), OpenedAt: p.OpenedAt, ReadyAt: h.readyAt,
		ClosedAt: p.ClosedAt, MergedAt: p.MergedAt, UpdatedAt: &p.UpdatedAt,
		ReviewerField: &reviewerField, ReviewerAccountNativeIds: reviewers,
	}
	return writeRecord(h.page, object("merge_request", "", id(p.ID), h.observed), record)
}
