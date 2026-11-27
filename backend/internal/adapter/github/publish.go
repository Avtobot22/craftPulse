package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const _writePermissionMissing = "write_permission_missing"

var errPublicationFound = errors.New("GitHub publication target found")

type publicationKey struct {
	org    string
	pullID int64
}

type publicationPull struct {
	repo repository
	pull pull
}

type issueComment struct {
	ID   int64   `json:"id"`
	Body string  `json:"body"`
	User account `json:"user"`
}

func publicationTarget(action string, key adaptersdk.ObjectKey, marker string) (int64, error) {
	if action != "mr_comment" {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitHub publishes pull request comments")
	}
	n, ok := numeric(key.NativeId)
	if !ok || key.NativeNs != "" || key.ObjectKind != adaptersdk.ObjectKeyObjectKindMergeRequest || strings.TrimSpace(marker) == "" {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitHub publication requires a global pull request ID and a marker")
	}
	return n, nil
}

func commentMarker(key publicationKey, marker string) string {
	return "<!-- craftpulse:mr-summary:" + key.org + ":" + id(key.pullID) + " " +
		base64.RawStdEncoding.EncodeToString([]byte(marker)) + " -->"
}

func (p publicationPull) webURL(commentID int64) *string {
	return optional(_baseURL + strings.TrimPrefix(p.repo.path(), "/repos") + "/pull/" +
		strconv.Itoa(p.pull.Number) + "#issuecomment-" + id(commentID))
}

func (a *Adapter) publicationBot(ctx context.Context, call adaptersdk.Call) (writeBot, error) {
	reader, err := a.bound(ctx, call)
	if err != nil {
		return writeBot{}, err
	}
	return a.writeBot(ctx, call, reader.token)
}

func (b writeBot) publicationPull(ctx context.Context, pullID int64) (publicationPull, error) {
	// The contract stores a global ID, while GitHub's comment API uses a repository and PR number.
	var target publicationPull
	err := b.walkRepositories(ctx, func(repo repository) error {
		query := url.Values{"state": {"all"}, "per_page": {"100"}}
		err := walk(ctx, b.source, "publication_pulls", repo.path()+"/pulls", query, false, func(p pull) error {
			if p.ID != pullID {
				return nil
			}
			if err := p.validate(repo.ID); err != nil {
				return err
			}
			target = publicationPull{repo: repo, pull: p}
			return errPublicationFound
		})
		// A repository can lose access between listing the installation and listing its PRs.
		// walk reports a missing continuation separately so an incomplete traversal still fails.
		if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
			return nil
		}
		return err
	})
	if errors.Is(err, errPublicationFound) {
		return target, nil
	}
	if err != nil {
		return publicationPull{}, err
	}
	return publicationPull{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "pull request is not visible to the GitHub write bot")
}

func (p publicationPull) commentsPath() string {
	return p.repo.path() + "/issues/" + strconv.Itoa(p.pull.Number) + "/comments"
}

func (b writeBot) owns(comment issueComment) bool {
	return comment.User.Type == "Bot" && strings.EqualFold(comment.User.Login, b.login)
}

func (c issueComment) hasMarker(marker string) bool {
	first, _, _ := strings.Cut(c.Body, "\n")
	return strings.TrimSuffix(first, "\r") == marker
}

func (b writeBot) findComment(ctx context.Context, p publicationPull, marker string) (issueComment, error) {
	var found issueComment
	err := walk(ctx, b.source, "publication_comments", p.commentsPath(), url.Values{"per_page": {"50"}}, false, func(c issueComment) error {
		if !b.owns(c) || !c.hasMarker(marker) {
			return nil
		}
		if c.ID <= 0 || found.ID != 0 && found.ID != c.ID {
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub summary comment identity is ambiguous")
		}
		found = c
		return nil
	})
	return found, err
}

func (a *Adapter) FindPublished(ctx context.Context, call adaptersdk.Call, in adaptersdk.FindPublishedInput) (adaptersdk.FindPublishedResult, error) {
	pullID, err := publicationTarget(string(in.Action), in.Target, in.Marker)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()

	bot, err := a.publicationBot(ctx, call)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, err
	}
	p, err := bot.publicationPull(ctx, pullID)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, publicationFindError(err)
	}
	c, err := bot.findComment(ctx, p, commentMarker(publicationKey{org: call.Organization.String(), pullID: pullID}, in.Marker))
	if err != nil || c.ID == 0 {
		return adaptersdk.FindPublishedResult{}, publicationFindError(err)
	}
	return adaptersdk.FindPublishedResult{Found: true, ExternalId: optional(id(c.ID)), ExternalUrl: p.webURL(c.ID)}, nil
}

func (a *Adapter) Publish(ctx context.Context, call adaptersdk.Call, in adaptersdk.PublishInput) (adaptersdk.PublishResult, error) {
	pullID, err := publicationTarget(string(in.Action), in.Target, in.Marker)
	if err != nil {
		return adaptersdk.PublishResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()

	key := publicationKey{org: call.Organization.String(), pullID: pullID}
	release, err := a.lockPublication(ctx, key)
	if err != nil {
		return adaptersdk.PublishResult{}, err
	}
	defer release()

	bot, err := a.publicationBot(ctx, call)
	if err != nil {
		return publicationError(err)
	}
	p, err := bot.publicationPull(ctx, pullID)
	if err != nil {
		return publicationError(err)
	}
	if p.repo.Archived {
		return failedPublication(_writePermissionMissing), nil
	}
	marker := commentMarker(key, in.Marker)
	comment, err := bot.findComment(ctx, p, marker)
	if err != nil {
		return publicationError(err)
	}
	if comment.ID == 0 && in.ExternalId != nil {
		return failedPublication(_writePermissionMissing), nil
	}
	if in.ExpectedHeadSha != nil {
		if err := bot.checkPublicationHead(ctx, p, *in.ExpectedHeadSha); err != nil {
			return publicationError(err)
		}
	}
	body := marker + "\n\n" + in.BodyMarkdown
	if comment.ID > 0 && comment.Body == body {
		return succeededPublication(p, comment.ID), nil
	}
	return bot.writeComment(ctx, p, comment.ID, body)
}

func (a *Adapter) lockPublication(ctx context.Context, key publicationKey) (func(), error) {
	for ctx.Err() == nil {
		a.publicationMu.Lock()
		done, busy := a.publications[key]
		if !busy {
			done = make(chan struct{})
			if a.publications == nil {
				a.publications = make(map[publicationKey]chan struct{})
			}
			a.publications[key] = done
			a.publicationMu.Unlock()
			return func() {
				a.publicationMu.Lock()
				defer a.publicationMu.Unlock()
				delete(a.publications, key)
				close(done)
			}, nil
		}
		a.publicationMu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "GitHub publication wait expired")
}

func (b writeBot) checkPublicationHead(ctx context.Context, p publicationPull, expected string) error {
	path := p.repo.path() + "/pulls/" + strconv.Itoa(p.pull.Number)
	var current pull
	if err := b.get(ctx, "publication_head", path, &current); err != nil {
		return err
	}
	if current.ID != p.pull.ID || current.Number != p.pull.Number || current.Base.Repo.ID != p.repo.ID || current.Head.SHA == "" {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub pull request identity or head could not be verified")
	}
	if current.Head.SHA != expected {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeHeadConflict, "GitHub pull request head has changed")
	}
	return nil
}

func (b writeBot) writeComment(ctx context.Context, p publicationPull, commentID int64, body string) (adaptersdk.PublishResult, error) {
	method, path := http.MethodPost, p.commentsPath()
	if commentID > 0 {
		method, path = http.MethodPatch, p.repo.path()+"/issues/comments/"+id(commentID)
	}
	var c issueComment
	err := b.sendJSON(ctx, "write_pr_comment", method, path, map[string]string{"body": body}, &c)
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		switch status := e.SourceStatusCode(); {
		case unavailableScope(err), e.Code == adaptersdk.AdapterErrorCodeRateLimited:
			return publicationError(err)
		case status >= 400 && status < 500 && status != http.StatusRequestTimeout:
			return failedPublication("provider_error"), nil
		}
	}
	if err != nil || c.ID <= 0 || commentID > 0 && c.ID != commentID || !b.owns(c) || c.Body != body {
		// A lost or incomplete reply can follow a committed write.
		return adaptersdk.PublishResult{Outcome: adaptersdk.PublishResultOutcomeUnknown, ReasonCode: optional("provider_error")}, nil //nolint:nilerr // the write may have committed
	}
	return succeededPublication(p, c.ID), nil
}

func publicationFindError(err error) error {
	if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodePermissionMissing, "pull request or its comments are not visible to the GitHub write bot")
	}
	return err
}

func publicationError(err error) (adaptersdk.PublishResult, error) {
	if unavailableScope(err) {
		return failedPublication(_writePermissionMissing), nil
	}
	// Publish has no invalid_response variant in the HTTP contract.
	if hasCode(err, adaptersdk.AdapterErrorCodeInvalidResponse) {
		return adaptersdk.PublishResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub publication could not be verified")
	}
	return adaptersdk.PublishResult{}, err
}

func failedPublication(reason string) adaptersdk.PublishResult {
	return adaptersdk.PublishResult{Outcome: adaptersdk.PublishResultOutcomeFailed, ReasonCode: optional(reason)}
}

func succeededPublication(p publicationPull, commentID int64) adaptersdk.PublishResult {
	return adaptersdk.PublishResult{
		Outcome: adaptersdk.PublishResultOutcomeSucceeded, ExternalId: optional(id(commentID)), ExternalUrl: p.webURL(commentID),
	}
}
