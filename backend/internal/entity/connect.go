package entity

import (
	"errors"
	"time"
)

var (
	ErrScopeTaken = errors.New("connect: the scope is collected by another active connection")
	// ErrOutsideScope is an address the installation must not reach (TZ 13.7).
	ErrOutsideScope = errors.New("connect: the address is outside the network scope of the installation")
	// ErrStateConflict is a change the current state of the connection or of its probe does not allow.
	ErrStateConflict = errors.New("connect: the state of the connection does not allow this change")
)

type Provider string

const (
	ProviderJiraCloud       Provider = "jira_cloud"
	ProviderGitLab          Provider = "gitlab"
	ProviderGitHub          Provider = "github"
	ProviderJenkins         Provider = "jenkins"
	ProviderSonarQube       Provider = "sonarqube"
	ProviderConfluenceCloud Provider = "confluence_cloud"
	ProviderGit             Provider = "git"
	ProviderUniversal       Provider = "universal"
	ProviderClient          Provider = "client"
)

type AdapterKind string

const (
	AdapterBuiltin AdapterKind = "builtin"
	AdapterClient  AdapterKind = "client"
)

type ConnectionStatus string

const (
	ConnectionDraft      ConnectionStatus = "draft"
	ConnectionActive     ConnectionStatus = "active"
	ConnectionAccessLost ConnectionStatus = "access_lost"
	ConnectionDeleted    ConnectionStatus = "deleted"
)

type AccessMode string

const (
	AccessServiceAccount AccessMode = "service_account"
	AccessUserToken      AccessMode = "user_token"
	AccessSynced         AccessMode = "synced"
)

type ScopeKind string

const (
	ScopeTrackerProject  ScopeKind = "tracker_project"
	ScopeRepository      ScopeKind = "repository"
	ScopeWikiSpace       ScopeKind = "wiki_space"
	ScopeCIProject       ScopeKind = "ci_project"
	ScopeAnalysisProject ScopeKind = "analysis_project"
)

type CapabilityState string

const (
	CapabilityVerified CapabilityState = "verified"
	CapabilityPartial  CapabilityState = "partial"
	CapabilityMissing  CapabilityState = "missing"
)

// SourceInstance is the anchor of identity of external objects (TZ 12.4): a provider and an instance key.
type SourceInstance struct {
	ID             ID
	OrganizationID ID
	Provider       Provider
	InstanceKey    string
	BaseURL        string
	Edition        string
	SourceVersion  string
	VerifiedAt     *time.Time
}

type Connection struct {
	ID             ID
	OrganizationID ID
	InstanceID     ID
	Provider       Provider
	Name           string
	AdapterKind    AdapterKind
	// AdapterURL, AdapterKeyEnc and KeyVersion are set only for an adapter of a client.
	AdapterURL      string
	AdapterKeyEnc   []byte
	KeyVersion      uint32
	ContractVersion string
	AccessMode      AccessMode
	Status          ConnectionStatus
	PausedUntil     *time.Time
	AccessLostAt    *time.Time
	// CredentialsSetAt is when the adapter last accepted credentials; their value never reaches the core.
	CredentialsSetAt    *time.Time
	SyncPaused          bool
	RowVersion          int
	ActivatedProbeID    *ID
	ActivatedRowVersion int
	CreatedBy           ID
	CreatedAt           time.Time
	DeletedAt           *time.Time
}

// ConnectionInput creates a draft; AdapterKey is the plain service key of an adapter of a client.
type ConnectionInput struct {
	OrganizationID ID
	CreatedBy      ID
	Name           string
	Provider       Provider
	BaseURL        string
	AdapterKind    AdapterKind
	AdapterURL     string
	AdapterKey     Secret
	Credentials    map[string]Secret
	Scopes         []NewScope
}

// ConnectionPatch is a JSON Merge Patch; a nil field is left as it is.
type ConnectionPatch struct {
	Name        *string
	BaseURL     *string
	AdapterURL  *string
	AdapterKey  *Secret
	Credentials map[string]Secret
	Scopes      *[]NewScope
	SyncPaused  *bool
}

type ConnectionFilter struct {
	Status ConnectionStatus
	Limit  int
	After  *AuditKey
}

type ConnectionPage struct {
	Items []ConnectionState
	Next  *AuditKey
}

type ProbeStatus string

const (
	ProbePending   ProbeStatus = "pending"
	ProbeRunning   ProbeStatus = "running"
	ProbeSucceeded ProbeStatus = "succeeded"
	ProbeFailed    ProbeStatus = "failed"
)

// Reasons of a failed probe (ConnectionProbe.reason_code).
const (
	ProbeEditionMismatch      = "edition_mismatch"
	ProbeAuthFailed           = "auth_failed"
	ProbeOutsideNetworkScope  = "outside_network_scope"
	ProbeCertificateUntrusted = "certificate_untrusted"
	ProbeTimeout              = "timeout"
	ProbeAdapterUnavailable   = "adapter_unavailable"
	ProbeContractUnsupported  = "contract_unsupported"
	ProbeConfigurationChanged = "configuration_changed"
)

// Probe is a probe of a connection kept until it is activated or expires (TZ 12.5.3).
type Probe struct {
	ID                   ID
	ConnectionID         ID
	ConfigurationVersion int
	Status               ProbeStatus
	StartedAt            time.Time
	FinishedAt           *time.Time
	InstanceKey          string
	Edition              string
	SourceVersion        string
	ContractVersion      string
	AccessMode           AccessMode
	Capabilities         []Capability
	ReasonCode           string
}

func (p Probe) Running() bool { return p.Status == ProbePending || p.Status == ProbeRunning }

// AdapterInfo describes a built-in adapter for the connection form (listAdapters).
type AdapterInfo struct {
	Provider         Provider
	DisplayName      string
	Editions         []string
	ContractVersions []string
	DataKinds        []string
	ScopeKinds       []ScopeKind
	CredentialFields []CredentialField
}

type CredentialField struct {
	Name     string
	Label    string
	Required bool
	Secret   bool
}

type ConnectionScope struct {
	ID                ID
	ConnectionID      ID
	InstanceID        ID
	Kind              ScopeKind
	NativeScopeID     string
	DisplayName       string
	HistoryFrom       *time.Time
	RepositoryScopeID *ID
	// AreaID is the area of normalize; it appears after the first collection.
	AreaID    *ID
	Active    bool
	AddedAt   time.Time
	RemovedAt *time.Time
}

type NewScope struct {
	Kind              ScopeKind
	NativeScopeID     string
	DisplayName       string
	HistoryFrom       *time.Time
	RepositoryScopeID *ID
}

// Capabilities are every capability a connection may have (connect.capability).
var Capabilities = []string{
	"read_current", "read_status_history", "read_field_history", "read_resolution_history", "read_sprint_history",
	"read_links", "read_permissions", "read_deletions", "write_mr_comment", "write_tracker_issue", "webhook",
}

// CapabilityNotChecked is the reason of a capability the adapter did not report: it is not working.
const CapabilityNotChecked = "not_checked"

// Capability is a capability of a connection checked by Probe (FR-014).
type Capability struct {
	Name          string
	State         CapabilityState
	ReasonCode    string
	CheckedAt     time.Time
	SourceVersion string
}

type ConnectionState struct {
	Connection   Connection
	Instance     SourceInstance
	Scopes       []ConnectionScope
	Capabilities []Capability
}
