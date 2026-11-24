package entity

import (
	"encoding/json"
	"time"
)

const (
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
	ID          ID     `json:"id"`
	DocumentID  ID     `json:"document_id"`
	Path        string `json:"path"`
	VersionRef  string `json:"version_ref"`
	Format      string `json:"format"`
	Title       string `json:"title,omitempty"`
	ContentHash []byte `json:"content_hash"`
	Owner       string `json:"owner,omitempty"`
	ACLState    string `json:"acl_state"`
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
	SpecValid *bool `json:"spec_valid,omitempty"`
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
