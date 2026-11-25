package github

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_historyPageBudget   = 12
	_activityCollection  = 4
	_accountHistoryLimit = 4000
	_profilePageLimit    = 32
	_profileBudget       = 8 * time.Second
	_profilePageReserve  = time.Second
)

var errAccountHistoryLimit = errors.New("GitHub account history reached the per-collection item limit")

type endpoint struct{ name, path string }

func historyEndpoints(repo repository, p pull) [4]endpoint {
	number := id(int64(p.Number))
	issue := repo.path() + "/issues/" + number
	pr := repo.path() + "/pulls/" + number
	return [4]endpoint{
		{"timeline", issue + "/timeline"},
		{"reviews", pr + "/reviews"},
		{"review_comments", pr + "/comments"},
		{"comments", issue + "/comments"},
	}
}

type history struct {
	pull            pull
	page            *adaptersdk.Page
	observed        time.Time
	accounts        map[int64]account
	accountsOnly    bool
	truncated       bool
	timeline        timelineProgress
	timelineDone    bool
	timelinePushes  map[pushTransition]struct{}
	activityCovered bool
}

type timelineProgress struct {
	readyAt                          *time.Time
	seenDraft, seenReady, incomplete bool
	hasPushes                        bool
}

type pushTransition struct {
	at   time.Time
	head string
}

type historyPosition struct {
	collection int
	page       string
}

type historyItem struct {
	ID                int64           `json:"id"`
	NodeID            string          `json:"node_id"`
	Event             string          `json:"event"`
	State             string          `json:"state"`
	Actor             account         `json:"actor"`
	User              account         `json:"user"`
	RequestedReviewer account         `json:"requested_reviewer"`
	RequestedTeam     team            `json:"requested_team"`
	CreatedAt         *time.Time      `json:"created_at"`
	SubmittedAt       *time.Time      `json:"submitted_at"`
	CommitID          string          `json:"commit_id"`
	HeadSHA           string          `json:"head_sha"`
	BaseSHA           string          `json:"base_sha"`
	PerformedViaApp   json.RawMessage `json:"performed_via_github_app"`
}

func (s source) readHistory(ctx context.Context, repo repository, h *history, pos historyPosition) (*historyPosition, error) {
	if h.accountsOnly {
		return nil, s.readAccountHistory(ctx, repo, h)
	}
	endpoints := historyEndpoints(repo, h.pull)
	if !h.timeline.hasPushes {
		h.timelinePushes = make(map[pushTransition]struct{})
	}
	seen := make(map[string]struct{})
	for range _historyPageBudget {
		var next string
		var err error
		if pos.collection < len(endpoints) {
			next, err = s.readHistoryPage(ctx, endpoints[pos.collection], h, pos.page, seen)
		} else {
			next, err = s.readActivityPage(ctx, repo, h, pos.page, seen)
		}
		switch {
		case pos.page == "" && unavailableScope(err):
			h.truncated = true
		case err != nil:
			return nil, err
		}
		if pos.collection == 0 && h.truncated {
			h.timeline.incomplete = true
		}
		if next != "" {
			pos.page = next
			continue
		}
		if pos.collection == 0 {
			h.timelineDone = true
			if !h.timeline.seenDraft && !h.timeline.seenReady && !h.timeline.incomplete && !h.pull.Draft {
				h.timeline.readyAt = &h.pull.OpenedAt
			}
		}
		pos = historyPosition{collection: pos.collection + 1}
		if pos.collection > _activityCollection {
			h.truncated = h.truncated || !h.activityCovered
			return nil, nil
		}
	}
	return &pos, nil
}

func (s source) readAccountHistory(ctx context.Context, repo repository, h *history) error {
	for _, ep := range historyEndpoints(repo, h.pull) {
		count := 0
		seen := make(map[string]struct{})
		err := walk(ctx, s, ep.name, ep.path, url.Values{"per_page": {"30"}}, false, func(item historyItem) error {
			count++
			if count > _accountHistoryLimit {
				return errAccountHistoryLimit
			}
			return h.addHistoryItem(ep.name, item, seen)
		})
		switch {
		case errors.Is(err, errAccountHistoryLimit), unavailableScope(err):
			h.truncated = true
		case err != nil:
			return err
		}
	}
	return nil
}

func (s source) readHistoryPage(ctx context.Context, ep endpoint, h *history, page string, seen map[string]struct{}) (string, error) {
	query := url.Values{"per_page": {"30"}}
	current := 1
	if page != "" {
		query.Set("page", page)
		current, _ = strconv.Atoi(page) // The continuation cursor was validated at the boundary.
	}
	links, err := arrayPage(ctx, s, ep.name, ep.path, query, current > 1, func(item historyItem) error {
		return h.addHistoryItem(ep.name, item, seen)
	})
	if err != nil {
		return "", err
	}
	next, err := nextPage(links, current)
	if next == 0 {
		return "", err
	}
	return strconv.Itoa(next), nil
}

func (h *history) addHistoryItem(collection string, item historyItem, seen map[string]struct{}) error {
	native := historyID(item.ID, item.NodeID)
	if h.duplicate(seen, collection, native) {
		return nil
	}
	return h.add(collection, native, item)
}

func historyID(n int64, node string) string {
	if n > 0 {
		return id(n)
	}
	return node
}

func (h *history) duplicate(seen map[string]struct{}, collection, native string) bool {
	if native == "" {
		return false
	}
	key := collection + "/" + native
	_, exists := seen[key]
	if exists {
		h.truncated = true
	}
	seen[key] = struct{}{}
	return exists
}

var timelineKinds = map[string]envelope.MrEventV2EventKind{
	"convert_to_draft": "draft", "converted_to_draft": "draft",
	"ready_for_review": "ready",
	"review_requested": "reviewer_assigned", "review_request_removed": "reviewer_removed",
	"head_ref_force_pushed": "head_pushed", "head_pushed": "head_pushed",
	"merged": "merged", "closed": "closed", "reopened": "reopened",
}

func (h *history) add(collection, native string, item historyItem) error {
	kind := envelope.MrEventV2EventKindComment
	at, actor := item.CreatedAt, item.User
	var review *envelope.MrEventV2ReviewState
	var target, head *string
	system := collection == "timeline"
	switch collection {
	case "timeline":
		kind = timelineKinds[item.Event]
		if kind == "" {
			return nil
		}
		actor = item.Actor
		if kind == envelope.MrEventV2EventKindReviewerAssigned || kind == envelope.MrEventV2EventKindReviewerRemoved {
			target = item.RequestedReviewer.nativeID()
			h.remember(item.RequestedReviewer)
			if target == nil {
				target = item.RequestedTeam.nativeID()
			}
			if target == nil {
				h.truncated = true
				return nil // An unknown target cannot be matched to a later removal or assignment.
			}
		}
	case "reviews":
		switch state := envelope.MrEventV2ReviewState(strings.ToLower(item.State)); state {
		case "pending":
			return nil // Draft reviews have not been submitted.
		case envelope.MrEventV2ReviewStateApproved, envelope.MrEventV2ReviewStateChangesRequested,
			envelope.MrEventV2ReviewStateCommented, envelope.MrEventV2ReviewStateDismissed:
			review = &state
		default:
			h.truncated = true
		}
		kind, at = envelope.MrEventV2EventKindReview, item.SubmittedAt
		if validSHA(item.CommitID) {
			head = &item.CommitID
		}
	case "review_comments":
		if validSHA(item.CommitID) {
			head = &item.CommitID
		}
	}
	if native == "" {
		h.truncated = true
		return nil
	}
	if len(item.PerformedViaApp) != 0 && string(item.PerformedViaApp) != "null" && strings.HasSuffix(actor.Login, "[bot]") {
		actor.Type = "Bot"
	}
	h.remember(actor)
	unknown := at == nil || at.IsZero()
	if unknown {
		h.truncated = true
		at = nil
	}
	if h.accountsOnly {
		return nil
	}
	key := collection + "/" + native
	switch kind {
	case envelope.MrEventV2EventKindDraft:
		h.timeline.seenDraft = true
	case envelope.MrEventV2EventKindReady:
		h.timeline.seenReady = true
		if at != nil && (h.timeline.readyAt == nil || at.Before(*h.timeline.readyAt)) {
			h.timeline.readyAt = at
		}
	case envelope.MrEventV2EventKindHeadPushed:
		sha := item.headSHA()
		if !validSHA(sha) {
			// Repository activity supplies before/after for GitHub's SHA-less force-push marker.
			h.truncated = true
			return nil
		}
		head = &sha
		if at != nil {
			h.timeline.hasPushes = true
			if h.timelinePushes != nil {
				h.timelinePushes[pushTransition{at: at.UTC(), head: sha}] = struct{}{}
			}
			var base *string
			if validSHA(item.BaseSHA) {
				base = &item.BaseSHA
			} else {
				h.truncated = true // The timeline did not supply the historical base.
			}
			if err := h.revision(key, sha, base, *at); err != nil {
				return err
			}
		}
	case envelope.MrEventV2EventKindApproval,
		envelope.MrEventV2EventKindClosed,
		envelope.MrEventV2EventKindComment,
		envelope.MrEventV2EventKindMerged,
		envelope.MrEventV2EventKindReopened,
		envelope.MrEventV2EventKindReview,
		envelope.MrEventV2EventKindReviewerAssigned,
		envelope.MrEventV2EventKindReviewerRemoved:
		// These events require no additional history bookkeeping.
	}
	return h.event(key, at, envelope.MrEventV2{
		MrNativeId: id(h.pull.ID), EventKind: kind, ActorAccountNativeId: actor.nativeID(),
		TargetAccountNativeId: target, HeadSha: head, ReviewState: review, SystemNote: &system, EventTimeUnknown: &unknown,
	})
}

func (item historyItem) headSHA() string {
	if validSHA(item.HeadSHA) {
		return item.HeadSHA
	}
	return item.CommitID
}

func (h *history) event(key string, at *time.Time, payload envelope.MrEventV2) error {
	if h.page == nil {
		return nil // A profile continuation replays the source slice to recover its account IDs.
	}
	rec := object("mr_event", id(h.pull.ID), key, h.observed)
	rec.PayloadSchema, rec.RecordClass, rec.EventTime = "mr_event/2", envelope.RecordRecordClassEvent, at
	return writeRecord(h.page, rec, payload)
}

func (h *history) revision(key, head string, base *string, at time.Time) error {
	if h.page == nil {
		return nil
	}
	rec := object("mr_revision", id(h.pull.ID), key, h.observed)
	return writeRecord(h.page, rec, envelope.MrRevisionV1{MrNativeId: id(h.pull.ID), HeadSha: head, BaseSha: base, CreatedAt: at})
}

func validSHA(sha string) bool {
	return (len(sha) == 40 || len(sha) == 64) && !strings.ContainsFunc(sha, func(c rune) bool {
		return (c < '0' || c > '9') && (c < 'a' || c > 'f')
	})
}

type account struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Type  string `json:"type"`
}

func (u account) nativeID() *string {
	if u.ID <= 0 {
		return nil
	}
	return optional(id(u.ID))
}

func (h *history) remember(u account) {
	if u.ID <= 0 {
		return
	}
	old, exists := h.accounts[u.ID]
	if h.accountsOnly && !exists && len(h.accounts) >= _accountHistoryLimit {
		h.truncated = true
		return
	}
	if u.Login != "" {
		old.Login = u.Login
	}
	if u.Name != "" {
		old.Name = u.Name
	}
	if u.Email != "" {
		old.Email = u.Email
	}
	if old.Type != "Bot" && u.Type != "" {
		old.Type = u.Type
	}
	old.ID = u.ID
	h.accounts[u.ID] = old
}

func (s source) writeAccounts(ctx context.Context, page *adaptersdk.Page, h *history, written map[int64]bool, after int64, resumed bool) (next int64, pending bool, err error) {
	return enrichProfiles(ctx, slices.Sorted(maps.Keys(h.accounts)), written, after, resumed, func(ctx context.Context, n int64) error {
		var profile account
		err := s.get(ctx, "account", "/user/"+id(n), &profile)
		if err == nil {
			if profile.ID != n {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub account identity does not match")
			}
			h.remember(profile)
		} else if !unavailableScope(err) {
			return err
		}
		u := h.accounts[n]
		kind := envelope.AccountV1AccountKindUnknown
		switch {
		case u.Type == "Bot", u.Type == "App", strings.HasSuffix(u.Login, "[bot]"):
			kind = envelope.AccountV1AccountKindBot
		case u.Type == "User":
			kind = envelope.AccountV1AccountKindHuman
		}
		return writeRecord(page, object("account", "", id(n), h.observed), envelope.AccountV1{
			Login: optional(u.Login), DisplayName: optional(u.Name), Email: optional(u.Email), AccountKind: kind, Active: u.Login != "",
		})
	})
}

func enrichProfiles(ctx context.Context, ids []int64, written map[int64]bool, after int64, resumed bool,
	visit func(context.Context, int64) error,
) (next int64, pending bool, err error) {
	deadline := time.Now().Add(_profileBudget)
	if end, ok := ctx.Deadline(); ok {
		reserved := end.Add(-_profilePageReserve)
		if resumed || reserved.Before(deadline) {
			deadline = reserved
		}
	}
	profiles, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	last, count := after, 0
	for _, n := range ids {
		if n <= after || written[n] {
			continue
		}
		if count == _profilePageLimit || profiles.Err() != nil {
			return last, true, nil
		}
		err := visit(profiles, n)
		if errors.Is(profiles.Err(), context.DeadlineExceeded) && ctx.Err() == nil && hasCode(err, adaptersdk.AdapterErrorCodeTimeout) {
			return last, true, nil
		}
		if err != nil {
			return 0, false, err
		}
		written[n] = true
		last = n
		count++
	}
	return 0, false, nil
}
