package gitlab

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const _writePermissionMissing = "write_permission_missing"

func (a *Adapter) botSource(ctx context.Context, call adaptersdk.Call) (source, error) {
	binding, collect, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, err
	}
	reader, err := tokenFrom(collect)
	if err != nil {
		return source{}, err
	}
	return a.bindBot(ctx, call, binding, reader)
}

func (a *Adapter) bindBot(ctx context.Context, call adaptersdk.Call, binding adaptersdk.SourceBinding, reader string) (source, error) {
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeWriteBot)
	if err != nil {
		return source{}, err
	}
	token := strings.TrimSpace(secret)
	if strings.HasPrefix(token, "{") {
		token, err = tokenFrom(token)
		if err != nil {
			return source{}, err
		}
	}
	if token == "" || token == reader || strings.ContainsAny(token, "\r\n") {
		return source{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab requires a separate write bot token")
	}
	return source{client: a.client, binding: binding, token: token}, nil
}

func publicationTarget(action string, target adaptersdk.ObjectKey, marker string) (int64, error) {
	if action != "mr_comment" {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitLab only publishes merge request comments")
	}
	nativeID, ok := numericID(target.NativeId)
	if !ok || target.NativeNs != "" || target.ObjectKind != adaptersdk.ObjectKeyObjectKindMergeRequest ||
		strings.TrimSpace(marker) == "" {
		return 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab publication requires a global merge request ID and a marker")
	}
	return nativeID, nil
}

func noteMarker(call adaptersdk.Call, mrID int64, marker string) string {
	return "<!-- craftpulse:mr-summary:" + call.Organization.String() + ":" + id(mrID) + " " +
		base64.RawStdEncoding.EncodeToString([]byte(marker)) + " -->"
}

type publishMR struct {
	mergeRequest
	WebURL string `json:"web_url"`
}

type publishedNote struct {
	ID     int64   `json:"id"`
	Body   string  `json:"body"`
	Author account `json:"author"`
	System bool    `json:"system"`
}

func (s source) botAccount(ctx context.Context) (account, error) {
	var bot account
	if err := s.get(ctx, "write_bot_user", "user", &bot); err != nil {
		return account{}, err
	}
	if bot.ID <= 0 || bot.State != "active" {
		return account{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab write bot is not active")
	}
	return bot, nil
}

func (s source) publicationMR(ctx context.Context, mrID int64) (publishMR, error) {
	// ObjectKey carries a global ID; the notes endpoints use project ID and IID.
	query := url.Values{"scope": {"all"}, "state": {"all"}}
	var found publishMR
	for page := 1; page > 0 && found.ID == 0; {
		next, err := readPage(ctx, s, "publication_merge_requests", "merge_requests", query, page, 100, func(mr publishMR) error {
			if mr.ID == mrID {
				found = mr
			}
			return nil
		})
		if err != nil {
			return publishMR{}, err
		}
		page = next
	}
	if found.ID == 0 {
		return publishMR{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "merge request is not visible to the GitLab write bot")
	}
	if found.ProjectID <= 0 || found.IID <= 0 {
		return publishMR{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab merge request identity is incomplete")
	}
	return found, nil
}

func (mr publishMR) path() string {
	return projectPath(mr.ProjectID) + "/merge_requests/" + id(mr.IID)
}

func (s source) noteURL(mr publishMR, noteID int64) *string {
	web, err := url.Parse(mr.WebURL)
	base, baseErr := url.Parse(s.binding.BaseURL)
	if err != nil || baseErr != nil || web.Scheme != base.Scheme || web.Hostname() != base.Hostname() ||
		(web.Port() != base.Port() && (base.Port() != "" || web.Port() != "443")) ||
		web.User != nil || web.RawQuery != "" || web.ForceQuery {
		return nil
	}
	web.Fragment, web.RawFragment = "note_"+id(noteID), ""
	return optional(web.String())
}

func (s source) findNote(ctx context.Context, mr publishMR, botID int64, marker string) (int64, error) {
	path := mr.path() + "/notes"
	query := url.Values{"sort": {"asc"}, "order_by": {"created_at"}}
	var noteID int64
	for page := 1; page > 0 && noteID == 0; {
		next, err := readPage(ctx, s, "publication_notes", path, query, page, _sourcePageSize, func(note publishedNote) error {
			first, _, _ := strings.Cut(note.Body, "\n")
			if note.System || note.Author.ID != botID || first != marker {
				return nil
			}
			if note.ID <= 0 {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab note ID is missing")
			}
			if noteID == 0 {
				noteID = note.ID
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
		page = next
	}
	return noteID, nil
}

func (a *Adapter) FindPublished(ctx context.Context, call adaptersdk.Call, in adaptersdk.FindPublishedInput) (adaptersdk.FindPublishedResult, error) {
	mrID, err := publicationTarget(string(in.Action), in.Target, in.Marker)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()

	s, err := a.botSource(ctx, call)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, err
	}
	bot, err := s.botAccount(ctx)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, err
	}
	mr, err := s.publicationMR(ctx, mrID)
	if err != nil {
		return adaptersdk.FindPublishedResult{}, writeBotLookupError(err)
	}
	noteID, err := s.findNote(ctx, mr, bot.ID, noteMarker(call, mrID, in.Marker))
	if err != nil || noteID == 0 {
		return adaptersdk.FindPublishedResult{}, writeBotLookupError(err)
	}
	return adaptersdk.FindPublishedResult{
		Found: true, ExternalId: optional(id(noteID)), ExternalUrl: s.noteURL(mr, noteID),
	}, nil
}

func writeBotLookupError(err error) error {
	if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "merge request or its notes are not visible to the GitLab write bot")
	}
	return err
}

func (a *Adapter) Publish(ctx context.Context, call adaptersdk.Call, in adaptersdk.PublishInput) (adaptersdk.PublishResult, error) {
	mrID, err := publicationTarget(string(in.Action), in.Target, in.Marker)
	if err != nil {
		return adaptersdk.PublishResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()

	s, err := a.botSource(ctx, call)
	if err != nil {
		return publicationLookupError(err)
	}
	bot, err := s.botAccount(ctx)
	if err != nil {
		return publicationLookupError(err)
	}
	mr, err := s.publicationMR(ctx, mrID)
	if err != nil {
		return publicationLookupError(err)
	}
	if err := s.requireCommentMember(ctx, mr.ProjectID, bot.ID); err != nil {
		return publicationLookupError(err)
	}
	marker := noteMarker(call, mrID, in.Marker)
	// Keep local callers from creating duplicate notes while an earlier write is in flight.
	limit, release := a.publishLimits.share(call.Organization.String()+"/"+s.binding.BaseURL+"/"+id(mrID), 1)
	defer release()
	if err := limit.acquire(ctx); err != nil {
		return adaptersdk.PublishResult{}, err
	}
	defer limit.release()

	// Rediscover the note after a lost reply or when the saved external ID is stale.
	noteID, err := s.findNote(ctx, mr, bot.ID, marker)
	if err != nil {
		return publicationLookupError(err)
	}
	if in.ExpectedHeadSha != nil {
		if err := s.checkPublicationHead(ctx, mr, *in.ExpectedHeadSha); err != nil {
			return publicationLookupError(err)
		}
	}
	return s.writeNote(ctx, mr, noteID, marker, in.BodyMarkdown)
}

func (s source) checkPublicationHead(ctx context.Context, mr publishMR, expected string) error {
	var current mergeRequest
	if err := s.get(ctx, "publication_head", mr.path(), &current); err != nil {
		return err
	}
	if current.ID != mr.ID || current.ProjectID != mr.ProjectID || current.IID != mr.IID || current.SHA == "" {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab merge request head could not be verified")
	}
	if current.SHA != expected {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeHeadConflict, "GitLab merge request head has changed")
	}
	return nil
}

func publicationLookupError(err error) (adaptersdk.PublishResult, error) {
	if hasCode(err, adaptersdk.AdapterErrorCodeAuth) || hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		return failedPublication(_writePermissionMissing), nil
	}
	return adaptersdk.PublishResult{}, err
}

func failedPublication(reason string) adaptersdk.PublishResult {
	return adaptersdk.PublishResult{Outcome: adaptersdk.PublishResultOutcomeFailed, ReasonCode: optional(reason)}
}

func unknownPublication() adaptersdk.PublishResult {
	return adaptersdk.PublishResult{Outcome: adaptersdk.PublishResultOutcomeUnknown, ReasonCode: optional("provider_error")}
}

func (s source) writeNote(ctx context.Context, mr publishMR, noteID int64, marker, markdown string) (adaptersdk.PublishResult, error) {
	method, path := http.MethodPost, "/api/v4/"+mr.path()+"/notes"
	if noteID > 0 {
		method, path = http.MethodPut, path+"/"+id(noteID)
	}
	body, err := json.Marshal(map[string]string{"body": commentBody(marker, markdown)})
	if err != nil {
		return adaptersdk.PublishResult{}, err
	}
	resp, err := s.send(ctx, "write_mr_note", method, path, nil, "application/json", _maxSourcePageBytes, bytes.NewReader(body)) //nolint:bodyclose // DecodeJSON owns the body
	if err != nil {
		if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
			status := e.SourceStatusCode()
			switch {
			case e.Code == adaptersdk.AdapterErrorCodeAuth || e.Code == adaptersdk.AdapterErrorCodeNotFound:
				return failedPublication(_writePermissionMissing), nil
			case e.Code == adaptersdk.AdapterErrorCodeRateLimited:
				return adaptersdk.PublishResult{}, err
			case status >= 400 && status < 500 && status != http.StatusRequestTimeout:
				return failedPublication("provider_error"), nil
			}
		}
		return unknownPublication(), nil //nolint:nilerr // the source may have committed the write
	}

	var note publishedNote
	if err := adaptersdk.DecodeJSON(resp, &note); err != nil || note.ID <= 0 || noteID > 0 && note.ID != noteID {
		return unknownPublication(), nil //nolint:nilerr // writing can succeed even if the response is incomplete
	}
	return adaptersdk.PublishResult{
		Outcome: adaptersdk.PublishResultOutcomeSucceeded, ExternalId: optional(id(note.ID)), ExternalUrl: s.noteURL(mr, note.ID),
	}, nil
}

func commentBody(marker, markdown string) string {
	// GitLab executes slash commands in note bodies. Escape a leading slash so a summary stays a comment.
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		text := strings.TrimLeftFunc(line, unicode.IsSpace)
		if strings.HasPrefix(text, "/") {
			lines[i] = line[:len(line)-len(text)] + "\\" + text
		}
	}
	return marker + "\n\n" + strings.Join(lines, "\n")
}

func (s source) requireCommentMember(ctx context.Context, projectID, botID int64) error {
	m, err := s.member(ctx, projectID, botID)
	if err == nil && (!m.active() || m.AccessLevel < _planner) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab write bot lacks project comment permission")
	}
	return err
}

func (s source) hasWriteScope(ctx context.Context) bool {
	var token struct {
		Scopes []string `json:"scopes"`
	}
	return s.get(ctx, "write_bot_scope", "personal_access_tokens/self", &token) == nil && slices.Contains(token.Scopes, "api")
}
