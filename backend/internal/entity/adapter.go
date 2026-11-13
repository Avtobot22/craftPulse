package entity

import (
	"errors"
	"time"
)

type AdapterOp string

const (
	OpPutCredentials    AdapterOp = "put_credentials"
	OpDeleteCredentials AdapterOp = "delete_credentials" //nolint:gosec // an operation name, not a credential
	OpProbe             AdapterOp = "probe"
	OpFetch             AdapterOp = "fetch"
	OpManifest          AdapterOp = "manifest"
	OpCheckAccess       AdapterOp = "check_access"
)

// CallClass separates a failure of the adapter from a failure of the source behind it (TZ 10.7).
type CallClass string

const (
	// CallPaused is a call refused without a request: the connection waits out a rate limit.
	CallPaused              CallClass = "paused"
	CallAdapterUnavailable  CallClass = "adapter_unavailable"
	CallContractUnsupported CallClass = "contract_unsupported"
	// CallSource is an error of the source classified by the adapter; Code is the code of adapter.yaml.
	CallSource CallClass = "source"
)

// Reasons of CallAdapterUnavailable.
const (
	AdapterTimeout              = "timeout"
	AdapterCertificateUntrusted = "certificate_untrusted"
	AdapterOutsideNetworkScope  = "outside_network_scope"
	AdapterUnreachable          = "unreachable"
	AdapterContractViolation    = "contract_violation"
	AdapterKeyRejected          = "service_key_rejected"
	AdapterNotConfigured        = "not_configured"
)

// Codes of CallSource the connect module acts on; the others are passed to the caller as they are.
const (
	SourceRateLimited = "rate_limited"
	SourceAuth        = "auth"
	SourceTimeout     = "timeout"
	SourcePreparing   = "preparing"
)

var (
	ErrConnectionPaused    = errors.New("connect: connection is paused by a rate limit")
	ErrAdapterUnavailable  = errors.New("connect: adapter is unavailable")
	ErrContractUnsupported = errors.New("connect: contract version is not supported")
	ErrSourceFailed        = errors.New("connect: source call failed")
)

// AdapterError is a failed call of an adapter; Detail is for diagnostics and has no credentials.
type AdapterError struct {
	Class CallClass
	// Code is the reason of CallAdapterUnavailable or the contract code of CallSource.
	Code string
	// RetryAfter is the Retry-After of the answer; the pause counts it from the clock of the core.
	RetryAfter time.Duration
	// RetryAt is the end of the pause, or the retry_at of the answer when it has no Retry-After.
	RetryAt *time.Time
	Detail  string
}

func (e *AdapterError) Error() string {
	msg := "connect: " + string(e.Class)
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func (e *AdapterError) Is(target error) bool {
	switch e.Class {
	case CallPaused:
		return target == ErrConnectionPaused
	case CallAdapterUnavailable:
		return target == ErrAdapterUnavailable
	case CallContractUnsupported:
		return target == ErrContractUnsupported
	case CallSource:
		return target == ErrSourceFailed
	}
	return false
}

// AdapterTarget is what one call needs to reach the adapter of a connection.
type AdapterTarget struct {
	OrganizationID ID
	ConnectionID   ID
	Kind           AdapterKind
	// Provider selects a built-in adapter from the installation configuration.
	Provider Provider
	// URL and Key reach an adapter of a client.
	URL             string
	Key             Secret
	ContractVersion string
}

type ScopeRef struct {
	Kind          ScopeKind
	NativeScopeID string
}

type CredentialsInput struct {
	Purpose     string
	Secret      Secret
	UserID      *ID
	OAuthCode   Secret
	RedirectURI string
}

// CredentialsFilter deletes credentials of a purpose or a user; empty deletes everything of the connection.
type CredentialsFilter struct {
	Purpose string
	UserID  *ID
}

type ProbeInput struct {
	BaseURL string
	Scopes  []ScopeRef
}

type ProbeResult struct {
	// ContractVersion is the newest version both the core and the adapter support.
	ContractVersion  string
	ContractVersions []string
	Provider         Provider
	InstanceKey      string
	Edition          string
	Version          string
	// AccessMode is empty when the adapter cannot check access.
	AccessMode   AccessMode
	Capabilities []Capability
	Scopes       []ScopeCheck
}

type ScopeCheck struct {
	Kind          ScopeKind
	NativeScopeID string
	DisplayName   string
	Verified      bool
	ReasonCode    string
}

type SourceCursor struct {
	ModifiedAt time.Time
	NativeID   string
}

// Period is [From, To); a nil end is open.
type Period struct {
	From *time.Time
	To   *time.Time
}

type ObjectKey struct {
	ObjectKind string
	NativeNS   string
	NativeID   string
}

type FetchInput struct {
	SchemaID   string
	Scope      ScopeRef
	Cursor     *SourceCursor
	Period     Period
	Limit      int
	ObjectKeys []ObjectKey
}

type ManifestInput struct {
	SchemaID string
	Scope    ScopeRef
	Period   Period
	Cursor   string
	Limit    int
}

type ManifestPage struct {
	Items      []ManifestItem
	NextCursor string
	TotalCount *int
}

type ManifestItem struct {
	RecordKind string
	NativeNS   string
	NativeID   string
	Version    string
	UpdatedAt  *time.Time
}

type SourceAccessInput struct {
	UserID          ID
	AccountNativeID string
	Items           []SourceAccessItem
}

// SourceAccessItem is an area (AreaKind set) or an object (ObjectKind set) of the source.
type SourceAccessItem struct {
	AreaKind   ScopeKind
	ObjectKind string
	NativeNS   string
	NativeID   string
}

type SourceAccessResult struct {
	Mode          AccessMode
	CheckedAt     time.Time
	MaxAgeSeconds *int
	Decisions     []SourceAccessDecision
}

type SourceAccessDecision struct {
	Decision   string
	CheckedAt  time.Time
	ReasonCode string
}

// RetryLaterError asks the job to run again at At without spending an attempt (TZ 10.6).
type RetryLaterError struct {
	At  time.Time
	Err error
}

func (e *RetryLaterError) Error() string {
	return "retry at " + e.At.Format(time.RFC3339) + ": " + e.Err.Error()
}

func (e *RetryLaterError) Unwrap() error { return e.Err }
