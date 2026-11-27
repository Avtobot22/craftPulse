package entity

import (
	"encoding/json"
	"time"
)

const (
	WikiInventoryProfile   = "wiki-inventory"
	GitDocsProfile         = "git-markdown-openapi"
	DocsFormatInventory    = "docs_json"
	MetricSetDocumentation = "documentation"

	ObjectDocument             ObjectKind = "document"
	ObjectDocumentationRoot    ObjectKind = "documentation_root"
	ObjectDocumentationFinding ObjectKind = "documentation_finding"
	ObjectDocumentationReport  ObjectKind = "documentation_report"

	DocsRuleMissingFile   = "missing-file"
	DocsRuleMissingAnchor = "missing-anchor"
)

// DocumentationScope selects one confirmed root of a service at a Git commit.
type DocumentationScope struct {
	ServiceID    ID     `json:"service_id"`
	RootID       ID     `json:"root_id"`
	RepositoryID ID     `json:"repository_id"`
	SHA          string `json:"sha"`
}

type DocumentVersion struct {
	ID           ID      `json:"id"`
	DocumentID   ID      `json:"document_id"`
	Path         string  `json:"path"`
	VersionRef   string  `json:"version_ref"`
	Format       string  `json:"format"`
	Title        string  `json:"title,omitempty"`
	ContentHash  []byte  `json:"content_hash"`
	Owner        string  `json:"owner,omitempty"`
	ACLState     string  `json:"acl_state"`
	PageNativeID string  `json:"page_native_id,omitempty"`
	SourceURL    *string `json:"source_url,omitempty"`
	Restricted   bool    `json:"-"`
}

type DocumentationInventory struct {
	Root      DocumentationRoot
	Documents []DocumentVersion
}

type DocsLinkCounts struct {
	Checked  int `json:"checked"`
	Broken   int `json:"broken"`
	External int `json:"external"`
}

type DocsReportDocument struct {
	DocumentVersion
	SpecValid    *bool      `json:"spec_valid,omitempty"`
	EventTime    *time.Time `json:"event_time,omitempty"`
	EventOffsetS int32      `json:"event_offset_s,omitempty"`
}

// DocsReportFacts keeps what one check observed, separately from immutable document versions.
type DocsReportFacts struct {
	RootID     *ID
	PathPrefix string
	OwnerRule  string
	Checks     []string
	Links      *DocsLinkCounts
	SARIFHash  []byte
	Documents  []DocsReportDocument
}

type DocsReport struct {
	ID           ID
	Kind         string
	SARIFID      ID
	Completeness string
	Skipped      int
	ObservedAt   time.Time
	KnownSeq     int64
	DocsReportFacts
}

type DocsFinding struct {
	ID            ID              `json:"finding_id"`
	ReportID      ID              `json:"report_id"`
	Rule          string          `json:"rule"`
	Kind          string          `json:"kind"`
	Path          string          `json:"path"`
	Line          *int            `json:"line,omitempty"`
	Severity      string          `json:"severity"`
	FirstSeenAt   time.Time       `json:"first_seen_at"`
	Evidence      json.RawMessage `json:"evidence"`
	HasLinkTarget bool            `json:"-"`
}

type DocsReceiptRoot struct {
	AreaID     ID
	RootID     *ID
	PathPrefix string
	ProfileID  string
	SHA        string
}

// DocumentationCheck is an observed check or the latest completed check of its kind.
type DocumentationCheck struct {
	SHA           string
	Kind          string
	Checks        []string
	ObservedAt    time.Time
	SARIFID       ID
	Complete      bool
	Latest        bool
	RepositoryURL string
	InstanceKey   string
	ProjectPath   string
	Provider      Provider
}

// ServiceDocument combines an accessible Git version with the last checked state.
type ServiceDocument struct {
	DocumentVersion
	LastCheckedAt  *time.Time
	OpenViolations *int
	SourceURL      *string
}

// ServiceDocuments is a page of a service's documentation corpus.
type ServiceDocuments struct {
	State DataState
	Items []ServiceDocument
	Notes []AccessNote
	Next  *ID
}

// WikiPage carries a source version and the access metadata observed with it.
type WikiPage struct {
	DocumentID   ID
	PageNativeID string
	VersionRef   string
	Title        *string
	ContentHash  []byte
	ACLState     string
	Restriction  *string
	AncestorIDs  []string
	SourceURL    *string
	UpdatedAt    time.Time
	ObservedAt   time.Time
	Deleted      bool
}
