package entity

import (
	"errors"
	"time"
)

var ErrScopeTaken = errors.New("connect: the scope is collected by another active connection")

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
	RowVersion      int
	CreatedBy       ID
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

// NewConnection is a draft; AdapterKey is the plain service key of an adapter of a client.
type NewConnection struct {
	ID              ID
	OrganizationID  ID
	InstanceID      ID
	Name            string
	AdapterKind     AdapterKind
	AdapterURL      string
	AdapterKey      Secret
	ContractVersion string
	AccessMode      AccessMode
	CreatedBy       ID
	CreatedAt       time.Time
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
