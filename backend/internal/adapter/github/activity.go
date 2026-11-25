package github

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type repositoryActivity struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	Type      string    `json:"activity_type"`
	Ref       string    `json:"ref"`
	Before    string    `json:"before"`
	After     string    `json:"after"`
	Timestamp time.Time `json:"timestamp"`
	Actor     account   `json:"actor"`
}

func (s source) readActivityPage(ctx context.Context, repo repository, h *history, after string, seen map[string]struct{}) (string, error) {
	head := h.pull.Head
	basePath := repo.path()
	if head.Repo.ID <= 0 || head.Ref == "" {
		return "", nil
	}
	// A fork's pushes belong to the head repository, even if the base has a branch with the same name.
	if head.Repo.ID != repo.ID {
		var err error
		repo, err = s.repository(ctx, adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: id(head.Repo.ID)})
		if err != nil {
			if after != "" && hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
				return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub activity repository is no longer visible")
			}
			return "", err
		}
	}
	ref := "refs/heads/" + head.Ref
	// GitHub's activity_type filter accepts one type. The branch filter keeps
	// ordinary and force pushes in the same ordered stream and cursor.
	query := url.Values{"ref": {ref}, "direction": {"desc"}, "per_page": {"30"}}
	if after != "" {
		query.Set("after", after)
	}
	collection := "activity/" + id(repo.ID)
	links, err := arrayPage(ctx, s, "repository_activity", repo.path()+"/activity", query, after != "", func(item repositoryActivity) error {
		if item.Ref != ref {
			return nil
		}
		if item.Timestamp.Before(h.pull.OpenedAt) && !item.Timestamp.IsZero() {
			h.activityCovered = true
			return errBeforeWindow
		}
		if item.Type != "push" && item.Type != "force_push" {
			return nil
		}
		if h.pull.ClosedAt != nil && item.Timestamp.After(*h.pull.ClosedAt) ||
			h.pull.MergedAt != nil && item.Timestamp.After(*h.pull.MergedAt) {
			return nil
		}
		if item.Timestamp.Equal(h.pull.OpenedAt) {
			h.truncated = true // GitHub's timestamps cannot order this push relative to opening.
			return nil
		}
		native := historyID(item.ID, item.NodeID)
		if native == "" {
			h.truncated = true
			return nil
		}
		if h.duplicate(seen, collection, native) {
			return nil
		}
		if item.Type == "force_push" && !item.Timestamp.IsZero() && validSHA(item.After) {
			if err := s.loadTimelinePushes(ctx, basePath, h); err != nil {
				return err
			}
			if _, exists := h.timelinePushes[pushTransition{at: item.Timestamp.UTC(), head: item.After}]; exists {
				return nil // The timeline record already describes this transition.
			}
		}
		return h.addPush(collection+"/"+native, item)
	})
	if errors.Is(err, errBeforeWindow) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	next, err := nextQuery(links)
	if err != nil || next == nil {
		return "", err
	}
	token := next.Get("after")
	if token == "" || token == after || len(token) > _historyTokenLimit {
		return "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "invalid GitHub activity continuation")
	}
	return token, nil
}

func (s source) loadTimelinePushes(ctx context.Context, basePath string, h *history) error {
	if h.timelinePushes != nil {
		return nil
	}
	h.timelinePushes = make(map[pushTransition]struct{})
	path := basePath + "/issues/" + id(int64(h.pull.Number)) + "/timeline"
	err := walk(ctx, s, "timeline", path, url.Values{"per_page": {"30"}}, false, func(item historyItem) error {
		sha := item.headSHA()
		if timelineKinds[item.Event] == envelope.MrEventV2EventKindHeadPushed &&
			historyID(item.ID, item.NodeID) != "" && item.CreatedAt != nil && !item.CreatedAt.IsZero() && validSHA(sha) {
			h.timelinePushes[pushTransition{at: item.CreatedAt.UTC(), head: sha}] = struct{}{}
		}
		return nil
	})
	if unavailableScope(err) {
		h.truncated = true
		return nil
	}
	return err
}

func (h *history) addPush(key string, item repositoryActivity) error {
	// Repository activity proves the head change but does not carry the PR's historical base SHA.
	h.truncated = true
	if !validSHA(item.Before) || !validSHA(item.After) || item.Before == item.After {
		return nil
	}
	h.remember(item.Actor)
	at := &item.Timestamp
	unknown, system := at.IsZero(), true
	if unknown {
		at = nil
	} else if err := h.revision(key, item.After, nil, *at); err != nil {
		return err
	}
	return h.event(key, at, envelope.MrEventV2{
		MrNativeId: id(h.pull.ID), EventKind: envelope.MrEventV2EventKindHeadPushed, ActorAccountNativeId: item.Actor.nativeID(),
		HeadSha: &item.After, SystemNote: &system, EventTimeUnknown: &unknown,
	})
}
