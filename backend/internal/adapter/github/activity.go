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
	query := url.Values{"ref": {ref}, "activity_type": {"push"}, "direction": {"desc"}, "per_page": {"30"}}
	if after != "" {
		query.Set("after", after)
	}
	collection := "activity/" + id(repo.ID)
	links, err := arrayPage(ctx, s, "repository_activity", repo.path()+"/activity", query, after != "", func(item repositoryActivity) error {
		if item.Type != "push" || item.Ref != ref {
			return nil
		}
		if item.Timestamp.Before(h.pull.OpenedAt) && !item.Timestamp.IsZero() {
			h.activityCovered = true
			return errBeforeWindow
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
