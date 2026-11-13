package adaptersdk

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Source is what an adapter implements: the operations of its source. The library serves the contract,
// keeps credentials and classifies errors; an error that is not *Error answers unavailable.
type Source interface {
	// Probe reads with the current credentials; on success the library drops the replaced ones.
	Probe(ctx context.Context, call Call, in ProbeInput) (ProbeResult, error)
	// Fetch adds the records of one page and returns the manifest with the next cursor.
	Fetch(ctx context.Context, call Call, in FetchInput, page *Page) (*envelope.Manifest, error)
	Manifest(ctx context.Context, call Call, in ManifestInput) (ManifestPage, error)
	CheckAccess(ctx context.Context, call Call, in CheckAccessInput) (CheckAccessResult, error)
	AuthorizeURL(ctx context.Context, call Call, in AuthorizeUrlInput) (AuthorizeUrlResult, error)
	// ExchangeCode turns a one-time OAuth code of a user into tokens kept for CheckAccess.
	ExchangeCode(ctx context.Context, call Call, code, redirectURI string) (UserToken, error)
	Publish(ctx context.Context, call Call, in PublishInput) (PublishResult, error)
	FindPublished(ctx context.Context, call Call, in FindPublishedInput) (FindPublishedResult, error)
	ParseWebhook(ctx context.Context, call Call, in WebhookInput) (WebhookResult, error)
}

// Call is one contract call: who it is for and access to the credentials of its connection.
type Call struct {
	Organization  uuid.UUID
	Connection    uuid.UUID
	CorrelationID string
	store         *Store
}

// Credential returns the value of the purpose; a missing one is auth, so the core stops collection
// until the administrator sets it.
func (c Call) Credential(ctx context.Context, purpose CredentialsInputPurpose) (string, error) {
	v, err := c.store.Credential(ctx, c.Organization, c.Connection, purpose)
	if errors.Is(err, ErrNoCredential) {
		return "", NewError(AdapterErrorCodeAuth, "credentials "+string(purpose)+" are not set")
	}
	return v, err
}

// UserToken returns the token of a user, or ErrNoCredential when the user has not linked an account.
func (c Call) UserToken(ctx context.Context, user uuid.UUID) (UserToken, error) {
	return c.store.UserToken(ctx, c.Organization, c.Connection, user)
}

// SaveUserToken keeps a refreshed token of a user.
func (c Call) SaveUserToken(ctx context.Context, user uuid.UUID, tok UserToken) error {
	_, err := c.store.PutUserToken(ctx, c.Organization, c.Connection, user, tok)
	return err
}

// Log returns the logger of the call with its correlation ID.
func (Call) Log(ctx context.Context) *zerolog.Logger { return zerolog.Ctx(ctx) }

// Unimplemented answers not_supported to every operation; a source embeds it and overrides what it serves.
type Unimplemented struct{}

var errNotSupported = NewError(AdapterErrorCodeNotSupported, "operation is not supported by this adapter")

func (Unimplemented) Probe(context.Context, Call, ProbeInput) (ProbeResult, error) {
	return ProbeResult{}, errNotSupported
}

func (Unimplemented) Fetch(context.Context, Call, FetchInput, *Page) (*envelope.Manifest, error) {
	return nil, errNotSupported
}

func (Unimplemented) Manifest(context.Context, Call, ManifestInput) (ManifestPage, error) {
	return ManifestPage{}, errNotSupported
}

func (Unimplemented) CheckAccess(context.Context, Call, CheckAccessInput) (CheckAccessResult, error) {
	return CheckAccessResult{}, errNotSupported
}

func (Unimplemented) AuthorizeURL(context.Context, Call, AuthorizeUrlInput) (AuthorizeUrlResult, error) {
	return AuthorizeUrlResult{}, errNotSupported
}

func (Unimplemented) ExchangeCode(context.Context, Call, string, string) (UserToken, error) {
	return UserToken{}, errNotSupported
}

func (Unimplemented) Publish(context.Context, Call, PublishInput) (PublishResult, error) {
	return PublishResult{}, errNotSupported
}

func (Unimplemented) FindPublished(context.Context, Call, FindPublishedInput) (FindPublishedResult, error) {
	return FindPublishedResult{}, errNotSupported
}

func (Unimplemented) ParseWebhook(context.Context, Call, WebhookInput) (WebhookResult, error) {
	return WebhookResult{}, errNotSupported
}
