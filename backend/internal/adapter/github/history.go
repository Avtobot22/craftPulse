package github

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _historyLimit = 4000

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
	readyAt         *time.Time
	seenDraft       bool
	seenReady       bool
	currentRevision bool
}

type historyItem struct {
	ID                int64           `json:"id"`
	NodeID            string          `json:"node_id"`
	Event             string          `json:"event"`
	State             string          `json:"state"`
	Actor             account         `json:"actor"`
	User              account         `json:"user"`
	RequestedReviewer account         `json:"requested_reviewer"`
	CreatedAt         *time.Time      `json:"created_at"`
	SubmittedAt       *time.Time      `json:"submitted_at"`
	CommitID          string          `json:"commit_id"`
	HeadSHA           string          `json:"head_sha"`
	BaseSHA           string          `json:"base_sha"`
	PerformedViaApp   json.RawMessage `json:"performed_via_github_app"`
}

var errHistoryLimit = errors.New("GitHub history reached the per-PR item limit")

func (s source) readHistory(ctx context.Context, repo repository, h *history) error {
	for _, ep := range historyEndpoints(repo, h.pull) {
		seen := make(map[string]struct{})
		count := 0
		err := walk(ctx, s, ep.name, ep.path, url.Values{"per_page": {"30"}}, false, func(item historyItem) error {
			count++
			if count > _historyLimit {
				return errHistoryLimit
			}
			native := item.NodeID
			if item.ID > 0 {
				native = id(item.ID)
			}
			if native != "" {
				if _, ok := seen[native]; ok {
					h.truncated = true
					return nil
				}
				seen[native] = struct{}{}
			}
			return h.add(ep.name, native, item)
		})
		switch {
		case errors.Is(err, errHistoryLimit), unavailableScope(err):
			h.truncated = true
		case err != nil:
			return err
		}
	}
	if !h.seenDraft && !h.seenReady && !h.pull.Draft && !h.truncated {
		h.readyAt = &h.pull.OpenedAt
	}
	if h.accountsOnly || h.currentRevision {
		return nil
	}
	// Ordinary pushes need not appear in the issue timeline. The PR details
	// provide a snapshot at updated_at, without proving a push time.
	head, base := h.pull.Head.SHA, h.pull.Base.SHA
	if !validSHA(base) {
		h.truncated = true
		base = ""
	}
	return h.revision("snapshot/"+head+"/"+base, head, optional(base), h.pull.UpdatedAt)
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
		target = item.RequestedReviewer.nativeID()
		h.remember(item.RequestedReviewer)
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
		h.seenDraft = true
	case envelope.MrEventV2EventKindReady:
		h.seenReady = true
		if at != nil && (h.readyAt == nil || at.Before(*h.readyAt)) {
			h.readyAt = at
		}
	case envelope.MrEventV2EventKindHeadPushed:
		sha := item.HeadSHA
		if !validSHA(sha) {
			sha = item.CommitID
		}
		if !validSHA(sha) {
			h.truncated = true
			return nil // The canonical head transition requires a SHA.
		}
		head = &sha
		if at != nil {
			var base *string
			if validSHA(item.BaseSHA) {
				base = &item.BaseSHA
			} else {
				h.truncated = true // The timeline did not supply the historical base.
			}
			if err := h.revision(key, sha, base, *at); err != nil {
				return err
			}
			h.currentRevision = h.currentRevision || sha == h.pull.Head.SHA && base != nil && *base == h.pull.Base.SHA
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
	rec := object("mr_event", id(h.pull.ID), key, h.observed)
	rec.PayloadSchema, rec.RecordClass, rec.EventTime = "mr_event/2", envelope.RecordRecordClassEvent, at
	return writeRecord(h.page, rec, envelope.MrEventV2{
		MrNativeId: id(h.pull.ID), EventKind: kind, ActorAccountNativeId: actor.nativeID(),
		TargetAccountNativeId: target, HeadSha: head, ReviewState: review, SystemNote: &system, EventTimeUnknown: &unknown,
	})
}

func (h *history) revision(key, head string, base *string, at time.Time) error {
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
	if !exists && len(h.accounts) >= _historyLimit {
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

func (s source) writeAccounts(ctx context.Context, h *history, written map[int64]bool) error {
	for _, n := range slices.Sorted(maps.Keys(h.accounts)) {
		if written[n] {
			continue
		}
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
		err = writeRecord(h.page, object("account", "", id(n), h.observed), envelope.AccountV1{
			Login: optional(u.Login), DisplayName: optional(u.Name), Email: optional(u.Email), AccountKind: kind, Active: u.Login != "",
		})
		if err != nil {
			return err
		}
		written[n] = true
	}
	return nil
}
