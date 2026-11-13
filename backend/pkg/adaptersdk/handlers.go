package adaptersdk

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type server struct {
	src      Source
	store    *Store
	spoolDir string
	limits   envelope.Limits
}

var _ StrictServerInterface = (*server)(nil)

func (s *server) call(ctx context.Context, conn, org uuid.UUID) Call {
	var id string
	if c, ok := ctx.(*gin.Context); ok {
		id = c.GetString(_correlationKey)
	}
	return Call{Organization: org, Connection: conn, CorrelationID: id, store: s.store}
}

func invalid(detail string) *Error { return NewError(AdapterErrorCodeInvalidRequest, detail) }

// PutCredentials stores the value and logs only that it is set and when it was replaced (TZ 13.6).
func (s *server) PutCredentials(ctx context.Context, req PutCredentialsRequestObject) (PutCredentialsResponseObject, error) {
	in := req.Body
	call := s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization)
	if !in.Purpose.Valid() {
		return nil, invalid("purpose is not supported")
	}
	event := zerolog.Ctx(ctx).Info().Str("purpose", string(in.Purpose))
	if in.Purpose == CredentialsInputPurposeUserToken {
		if in.UserId == nil {
			return nil, invalid("user_id is required for user_token")
		}
		tok, err := s.userToken(ctx, call, in)
		if err != nil {
			return nil, err
		}
		at, err := s.store.PutUserToken(ctx, call.Organization, call.Connection, *in.UserId, tok)
		if err != nil {
			return nil, err
		}
		event.Str("user_id", in.UserId.String()).Bool("set", true).Time("replaced_at", at).Msg("credentials replaced")
		return PutCredentials204Response{}, nil
	}
	if in.Secret == nil || *in.Secret == "" {
		return nil, invalid("secret is required")
	}
	at, err := s.store.PutCredential(ctx, call.Organization, call.Connection, in.Purpose, *in.Secret)
	if err != nil {
		return nil, err
	}
	event.Bool("set", true).Time("replaced_at", at).Msg("credentials replaced")
	return PutCredentials204Response{}, nil
}

func (s *server) userToken(ctx context.Context, call Call, in *CredentialsInput) (UserToken, error) {
	hasSecret, hasCode := in.Secret != nil && *in.Secret != "", in.OauthCode != nil && *in.OauthCode != ""
	switch {
	case hasSecret == hasCode:
		return UserToken{}, invalid("user_token needs either secret or oauth_code")
	case hasSecret:
		return UserToken{Access: *in.Secret}, nil
	case in.RedirectUri == nil || *in.RedirectUri == "":
		return UserToken{}, invalid("redirect_uri is required with oauth_code")
	}
	tok, err := s.src.ExchangeCode(ctx, call, *in.OauthCode, *in.RedirectUri)
	if err != nil {
		return UserToken{}, err
	}
	if tok.Access == "" {
		return UserToken{}, errors.New("adaptersdk: code exchange returned no access token")
	}
	return tok, nil
}

func (s *server) DeleteCredentials(ctx context.Context, req DeleteCredentialsRequestObject) (DeleteCredentialsResponseObject, error) {
	p := req.Params
	var purpose *CredentialsInputPurpose
	if p.Purpose != nil {
		if !p.Purpose.Valid() {
			return nil, invalid("purpose is not supported")
		}
		v := CredentialsInputPurpose(*p.Purpose)
		purpose = &v
	}
	if err := s.store.DeleteCredentials(ctx, p.XCraftpulseOrganization, req.ConnectionId, purpose, p.UserId); err != nil {
		return nil, err
	}
	event := zerolog.Ctx(ctx).Info()
	if purpose != nil {
		event = event.Str("purpose", string(*purpose))
	}
	if p.UserId != nil {
		event = event.Str("user_id", p.UserId.String())
	}
	event.Msg("credentials deleted")
	return DeleteCredentials204Response{}, nil
}

//nolint:revive,staticcheck // the name is set by the generated interface
func (s *server) AuthorizeUrl(ctx context.Context, req AuthorizeUrlRequestObject) (AuthorizeUrlResponseObject, error) {
	res, err := s.src.AuthorizeURL(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	return AuthorizeUrl200JSONResponse(res), nil
}

// Probe drops the credentials kept since the last replacement once the new ones read the source.
func (s *server) Probe(ctx context.Context, req ProbeRequestObject) (ProbeResponseObject, error) {
	call := s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization)
	res, err := s.src.Probe(ctx, call, *req.Body)
	if err != nil {
		return nil, err
	}
	if len(res.ContractVersions) == 0 {
		res.ContractVersions = []string{CurrentContractVersion}
	}
	res.Capabilities = nonNil(res.Capabilities)
	res.Scopes = nonNil(res.Scopes)
	if err := s.store.DropPrevious(ctx, call.Organization, call.Connection); err != nil {
		return nil, err
	}
	return Probe200JSONResponse(res), nil
}

func (s *server) Fetch(ctx context.Context, req FetchRequestObject) (FetchResponseObject, error) {
	page := newPage(s.spoolDir, s.limits)
	manifest, err := s.src.Fetch(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body, page)
	if err == nil && manifest == nil {
		err = errors.New("adaptersdk: source returned no manifest")
	}
	if err != nil {
		_ = page.Close()
		return nil, err
	}
	return pageResponse{page: page, manifest: manifest}, nil
}

// pageResponse streams the package of a page; an error after the first byte leaves a truncated
// archive, which the core rejects and fetches again.
type pageResponse struct {
	page     *Page
	manifest *envelope.Manifest
}

func (r pageResponse) VisitFetchResponse(w http.ResponseWriter) error {
	defer func() { _ = r.page.Close() }()
	w.Header().Set("Content-Type", "application/gzip")
	w.WriteHeader(http.StatusOK)
	return r.page.writeTo(w, r.manifest)
}

func (s *server) Manifest(ctx context.Context, req ManifestRequestObject) (ManifestResponseObject, error) {
	res, err := s.src.Manifest(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	res.Items = nonNil(res.Items)
	return Manifest200JSONResponse(res), nil
}

func (s *server) CheckAccess(ctx context.Context, req CheckAccessRequestObject) (CheckAccessResponseObject, error) {
	res, err := s.src.CheckAccess(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	res.Decisions = nonNil(res.Decisions)
	return CheckAccess200JSONResponse(res), nil
}

func (s *server) Publish(ctx context.Context, req PublishRequestObject) (PublishResponseObject, error) {
	res, err := s.src.Publish(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	return Publish200JSONResponse(res), nil
}

func (s *server) FindPublished(ctx context.Context, req FindPublishedRequestObject) (FindPublishedResponseObject, error) {
	res, err := s.src.FindPublished(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	return FindPublished200JSONResponse(res), nil
}

func (s *server) ParseWebhook(ctx context.Context, req ParseWebhookRequestObject) (ParseWebhookResponseObject, error) {
	res, err := s.src.ParseWebhook(ctx, s.call(ctx, req.ConnectionId, req.Params.XCraftpulseOrganization), *req.Body)
	if err != nil {
		return nil, err
	}
	res.Keys = nonNil(res.Keys)
	return ParseWebhook200JSONResponse(res), nil
}

// nonNil keeps a required array of the contract from encoding as null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
