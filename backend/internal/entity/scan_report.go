package entity

import (
	"encoding/json"
	"time"
)

// Reasons normalize rejects a record of a report file with (AC-10): the former report of the same area, SHA and kind
// stays in force.
const (
	RecordReportInvalid            = "invalid_report"
	RecordReportFormatUnsupported  = "unsupported_report_format"
	RecordReportVersionUnsupported = "unsupported_report_version"
)

// Kinds of findings (normalize.finding_occurrence.finding_kind).
const (
	FindingCode       = "code"
	FindingSecurity   = "security"
	FindingSecret     = "secret"
	FindingDependency = "dependency"
	FindingIaC        = "iac"
	FindingDocs       = "docs"
	FindingOpenAPI    = "openapi"
)

// ScanReport is a report of analysis, tests or coverage with its provenance (DR-009). Empty Producer and nil ScopeHash
// are unknown: a report file an adapter reads from the CI of the source has no scan manifest.
type ScanReport struct {
	Record          RecordRef
	KnownSeq        int64
	AreaID          ID
	ComponentID     *ID
	Kind            string
	Mode            string
	Producer        string
	ProducerVersion string
	Format          string
	SchemaVersion   string
	ParserVersion   string
	ProfileID       string
	RulesHash       []byte
	ConfigHash      []byte
	DBVersion       string
	DBPublishedAt   *time.Time
	ScopeHash       []byte
	SHA             string
	ArtifactDigest  string
	RunID           *ID
	AttemptID       *ID
	StartedAt       *time.Time
	FinishedAt      *time.Time
	ExitClass       string
	Completeness    string
	ScannedCount    *int
	SkippedCount    *int
	SkippedTargets  []SkippedTarget
	History         *ScanHistory
}

type SkippedTarget struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ScanHistory is the range of commits a check of history read and whether it was cut.
type ScanHistory struct {
	FromSHA   string
	ToSHA     string
	Truncated bool
}

// FindingOccurrence is a finding in one report. Empty texts and nil lines are unknown; Evidence keeps safe
// coordinates and the description of the rule only, never code or the value of a secret (BR-017).
type FindingOccurrence struct {
	NativeID         string
	RuleID           string
	Kind             string
	Path             string
	LineStart        *int
	LineEnd          *int
	PackagePURL      string
	NativeSeverity   string
	NativeConfidence string
	Fingerprint      []byte
	Evidence         json.RawMessage
}
