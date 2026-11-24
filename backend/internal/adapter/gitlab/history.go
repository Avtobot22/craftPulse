package gitlab

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type discussion struct {
	Notes []note `json:"notes"`
}

type note struct {
	ID        int64      `json:"id"`
	Type      string     `json:"type"`
	Body      string     `json:"body"`
	System    bool       `json:"system"`
	Author    account    `json:"author"`
	CreatedAt *time.Time `json:"created_at"`
}

type stateEvent struct {
	ID        int64      `json:"id"`
	State     string     `json:"state"`
	User      account    `json:"user"`
	CreatedAt *time.Time `json:"created_at"`
}

type diffVersion struct {
	ID        int64     `json:"id"`
	MRID      int64     `json:"merge_request_id"`
	HeadSHA   string    `json:"head_commit_sha"`
	BaseSHA   string    `json:"base_commit_sha"`
	StartSHA  string    `json:"start_commit_sha"`
	CreatedAt time.Time `json:"created_at"`
}

type mrHistory struct {
	readyAt   *time.Time
	closedAt  *time.Time
	hasDraft  bool
	truncated bool
}

func (s source) fetchMR(ctx context.Context, item listedMR, m *envelope.Manifest, page *adaptersdk.Page, targeted bool) error {
	root := projectPath(item.ProjectID) + "/merge_requests/" + id(item.IID)
	var mr mergeRequest
	if err := s.get(ctx, "merge_request", root, &mr); err != nil {
		return err
	}
	if !mr.valid(item.ProjectID) || mr.ID != item.ID || mr.IID != item.IID {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab returned a different merge request")
	}
	for _, user := range []account{mr.Author, mr.MergeUser, mr.MergedBy, mr.ClosedBy} {
		if err := user.record(page, m.CreatedAt); err != nil {
			return err
		}
	}
	if mr.Reviewers != nil {
		for _, user := range *mr.Reviewers {
			if err := user.record(page, m.CreatedAt); err != nil {
				return err
			}
		}
	}
	h := historyReader{source: s, page: page, observed: m.CreatedAt, mr: id(item.ID)}
	if err := h.read(ctx, root, mr); err != nil {
		return err
	}
	payload, err := mr.payload(h.mrHistory)
	if err != nil {
		return err
	}
	if targeted && mr.State == "opened" && h.currentDiff == nil && (h.latestDiffSeen || mr.DiffRefs.HeadSHA != payload.HeadSha) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitLab is preparing the diff for the current head")
	}
	if h.currentDiff != nil {
		payload.BaseSha = optional(h.currentDiff.BaseSHA)
		payload.MergeBaseSha = payload.BaseSha
		if err := h.diff(ctx, root, *h.currentDiff); err != nil {
			return err
		}
	}
	if h.truncated || (!payload.Draft && h.hasDraft && h.readyAt == nil) {
		m.Completeness = envelope.ManifestCompletenessPartial
		m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{
			RecordKind: envelope.TruncatedHistoryRecordKindMergeRequest, NativeId: h.mr,
		})
	}
	return addRecord(page, "merge_request", "merge_request/1", "object", "", h.mr, nil, nil, m.CreatedAt, payload)
}

type historyReader struct {
	source
	mrHistory
	page           *adaptersdk.Page
	observed       time.Time
	mr             string
	pushes         map[int64]pushNote
	currentDiff    *diffVersion
	latestDiffSeen bool
}

type pushNote struct {
	actorID int64
	at      *time.Time
}

func (h *historyReader) read(ctx context.Context, root string, mr mergeRequest) error {
	if err := historyPages(ctx, h, root, "discussions", func(d discussion) error {
		for i := range d.Notes {
			if err := h.note(ctx, &d.Notes[i], i == 0); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := historyPages(ctx, h, root, "resource_state_events", h.stateEvent); err != nil {
		return err
	}
	var previous *diffVersion
	if err := historyPages(ctx, h, root, "versions", func(v diffVersion) error {
		if err := h.revision(v); err != nil {
			return err
		}
		if previous == nil && mr.State == "opened" {
			h.latestDiffSeen = true
			if mr.isCurrentDiff(v) {
				h.currentDiff = &v
			}
		}
		// GitLab lists the most recent diff version first. Each version with an older
		// predecessor and a changed head records a push, even when the notes are absent.
		if previous != nil && previous.HeadSHA != v.HeadSHA && validSHA(previous.HeadSHA) {
			if err := h.push(*previous); err != nil {
				return err
			}
		}
		previous = &v
		return nil
	}); err != nil {
		return err
	}
	if mr.MergedAt != nil {
		actor := mr.MergeUser
		if actor.ID == 0 {
			actor = mr.MergedBy
		}
		if err := h.event("merge/"+h.mr, "merged", actor, nil, nil, nil, true, mr.MergedAt); err != nil {
			return err
		}
	}
	if h.closedAt == nil && mr.ClosedAt != nil && mr.State == "closed" {
		h.closedAt = mr.ClosedAt
		return h.event("close/"+h.mr, "closed", mr.ClosedBy, nil, nil, nil, true, mr.ClosedAt)
	}
	return nil
}

func historyPages[T any](ctx context.Context, h *historyReader, root, collection string, visit func(T) error) error {
	query := url.Values{}
	for current := 1; current > 0; {
		next, err := readPage(ctx, h.source, collection, root+"/"+collection, query, current, _sourcePageSize, visit)
		if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
			h.truncated = true
			return nil
		}
		if err != nil {
			return err
		}
		current = next
	}
	return nil
}

func (h *historyReader) stateEvent(event stateEvent) error {
	var kind envelope.MrEventV2EventKind
	switch event.State {
	case "closed":
		kind = envelope.MrEventV2EventKindClosed
		if event.CreatedAt != nil && (h.closedAt == nil || event.CreatedAt.After(*h.closedAt)) {
			h.closedAt = event.CreatedAt
		}
	case "opened", "reopened":
		kind = envelope.MrEventV2EventKindReopened
	default:
		return nil
	}
	return h.event("state/"+id(event.ID), kind, event.User, nil, nil, nil, true, event.CreatedAt)
}

func (h *historyReader) revision(v diffVersion) error {
	if v.ID <= 0 || v.MRID > 0 && id(v.MRID) != h.mr || !validSHA(v.HeadSHA) || v.CreatedAt.IsZero() {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab returned an invalid merge request diff version")
	}
	return addRecord(h.page, "mr_revision", "mr_revision/1", "object", h.mr, id(v.ID), optional(id(v.ID)), nil, h.observed,
		envelope.MrRevisionV1{MrNativeId: h.mr, HeadSha: v.HeadSHA, BaseSha: optional(v.BaseSHA), StartSha: optional(v.StartSHA), CreatedAt: v.CreatedAt})
}

var (
	_pushNoteVersion  = regexp.MustCompile(`(?s)^added [0-9]+ commits?\n.*\[Compare with previous version\]\([^)]*[?&]diff_id=([0-9]+)[&)]`)
	_usernameMentions = regexp.MustCompile(`@([a-zA-Z0-9_][a-zA-Z0-9_.-]*)`)
)

func (h *historyReader) note(ctx context.Context, n *note, first bool) error {
	if n.ID < 1 {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab note has no ID")
	}
	key := "note/" + id(n.ID)
	if !n.System {
		if first && n.Type == "DiffNote" {
			state := envelope.MrEventV2ReviewStateCommented
			return h.event(key, "review", n.Author, nil, nil, &state, false, n.CreatedAt)
		}
		return h.event(key, "comment", n.Author, nil, nil, nil, false, n.CreatedAt)
	}

	if match := _pushNoteVersion.FindStringSubmatch(n.Body); len(match) == 2 {
		return h.rememberPush(match[1], n)
	}
	body := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(n.Body)), "**", "")
	switch {
	case body == "marked this merge request as ready" || body == "marked this merge request as ready for review" || body == "removed the draft status":
		if n.CreatedAt != nil && (h.readyAt == nil || n.CreatedAt.Before(*h.readyAt)) {
			h.readyAt = n.CreatedAt
		}
		return h.event(key, "ready", n.Author, nil, nil, nil, true, n.CreatedAt)
	case body == "marked this merge request as draft", strings.HasPrefix(body, "marked this merge request as draft from "):
		h.hasDraft = true
		return h.event(key, "draft", n.Author, nil, nil, nil, true, n.CreatedAt)
	case body == "approved this merge request":
		return h.event(key, "approval", n.Author, nil, nil, nil, true, n.CreatedAt)
	case body == "requested changes" || body == "requested changes for this merge request":
		state := envelope.MrEventV2ReviewStateChangesRequested
		return h.event(key, "review", n.Author, nil, nil, &state, true, n.CreatedAt)
	case strings.HasPrefix(body, "requested review from "), strings.HasPrefix(body, "removed review request for "):
		return h.reviewerNotes(ctx, n, key)
	}
	return h.event(key, "comment", n.Author, nil, nil, nil, true, n.CreatedAt)
}

func (h *historyReader) rememberPush(version string, n *note) error {
	versionID, ok := numericID(version)
	if !ok || len(h.pushes) >= 10000 {
		h.truncated = true
		return nil
	}
	if h.pushes == nil {
		h.pushes = make(map[int64]pushNote)
	}
	h.pushes[versionID] = pushNote{actorID: n.Author.ID, at: n.CreatedAt}
	return n.Author.record(h.page, h.observed)
}

func (h *historyReader) push(v diffVersion) error {
	at := &v.CreatedAt
	var actor account
	n, found := h.pushes[v.ID]
	if found {
		actor.ID = n.actorID
		if n.at != nil {
			at = n.at
		}
	}
	return h.event("version/"+id(v.ID), "head_pushed", actor, nil, &v.HeadSHA, nil, found, at)
}

func (h *historyReader) reviewerNotes(ctx context.Context, n *note, key string) error {
	for i, action := range []string{"requested review from ", "removed review request for "} {
		kind := envelope.MrEventV2EventKindReviewerAssigned
		if i == 1 {
			kind = envelope.MrEventV2EventKindReviewerRemoved
		}
		_, text, found := strings.Cut(n.Body, action)
		if !found {
			continue
		}
		text, _, _ = strings.Cut(text, " and removed review request for ")
		for _, mention := range _usernameMentions.FindAllStringSubmatch(text, -1) {
			login := strings.TrimRight(mention[1], ".")
			target, err := h.byUsername(ctx, login)
			if err != nil {
				return err
			}
			if target.ID == 0 {
				h.truncated = true
				continue
			}
			if err := target.record(h.page, h.observed); err != nil {
				return err
			}
			if err := h.event(key+"/"+string(kind)+"/"+id(target.ID), kind, n.Author, target.nativeID(), nil, nil, true, n.CreatedAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s source) byUsername(ctx context.Context, login string) (account, error) {
	var found account
	_, err := readPage(ctx, s, "user_by_login", "users", url.Values{"username": {login}}, 1, 2, func(u account) error {
		if strings.EqualFold(u.Username, login) {
			found = u
		}
		return nil
	})
	return found, err
}

func (h *historyReader) event(key string, kind envelope.MrEventV2EventKind, actor account, target, head *string,
	review *envelope.MrEventV2ReviewState, system bool, at *time.Time,
) error {
	unknownTime := at == nil || at.IsZero()
	if unknownTime {
		h.truncated = true
		at = nil
	}
	if err := actor.record(h.page, h.observed); err != nil {
		return err
	}
	return addRecord(h.page, "mr_event", "mr_event/2", "event", h.mr, key, nil, at, h.observed,
		envelope.MrEventV2{
			MrNativeId: h.mr, EventKind: kind, ActorAccountNativeId: actor.nativeID(),
			TargetAccountNativeId: target, HeadSha: head, ReviewState: review, SystemNote: &system, EventTimeUnknown: &unknownTime,
		})
}

func validSHA(value string) bool {
	return (len(value) == 40 || len(value) == 64) && strings.IndexFunc(value, func(c rune) bool {
		return (c < '0' || c > '9') && (c < 'a' || c > 'f')
	}) == -1
}
