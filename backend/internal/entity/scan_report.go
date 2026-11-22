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
	// RecordManifestMismatch rejects an execution manifest of another build or SHA: the build keeps unknown
	// inner retries.
	RecordManifestMismatch = "execution_manifest_mismatch"
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
	// RefName is the branch or tag of the run and Language the language of the check; empty when unknown.
	RefName  string
	Language string
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
	// ContentFingerprint leaves the path and the lines out, so the same content at another place is found moved;
	// nil when the tool says nothing of the content.
	ContentFingerprint []byte
	Evidence           json.RawMessage
}

// Outcomes of a test execution (normalize.test_execution.outcome).
const (
	TestPassed  = "passed"
	TestFailed  = "failed"
	TestError   = "error"
	TestSkipped = "skipped"
)

// Kinds of coverage (normalize.coverage_fact.kind): line and branch coverage never mix (FR-062).
const (
	CoverageLine   = "line"
	CoverageBranch = "branch"
)

// SkippedOutsideRepository is the reason a file of a coverage report is left out: its path is not under the root
// of the repository.
const SkippedOutsideRepository = "outside_repository"

// ReportRows are the rows a report gives: findings of an analysis, executions of tests or coverage of files.
type ReportRows struct {
	Findings []FindingOccurrence
	Tests    []TestExecution
	Coverage []CoverageFact
	Code     *CodeFacts
}

// TestExecution is a test case of a JUnit report in the configuration of its attempt (DR-006): the same case in two
// configurations of a matrix is two executions, not two attempts of one test. Ordinal counts the same case within
// one report.
type TestExecution struct {
	Suite             *string
	CaseKey           string
	CaseKeyHash       []byte
	AttemptID         *ID
	ConfigFingerprint []byte
	Ordinal           int
	Outcome           string
	DurationMS        *int64
	RunStartedAt      time.Time
}

// CoverageFact is the coverage of a file by one kind (DR-008); its map of lines lies in the object store by
// LineMapKey.
type CoverageFact struct {
	Path       string
	Kind       string
	Covered    int
	Executable int
	LineMapKey string
}

// ReportFindings is a report as change reads it to identify its findings and to decide its baseline. Interrupted is
// an attempt of the report that was canceled; DefaultBranch is the main branch of its repository, empty when unknown.
type ReportFindings struct {
	ID            ID
	AreaID        ID
	ComponentID   *ID
	Kind          string
	Language      string
	RefName       string
	SHA           string
	ExitClass     string
	Completeness  string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	KnownSeq      int64
	Interrupted   bool
	DefaultBranch string
	Occurrences   []ReportOccurrence
}

// ReportOccurrence is a finding of a report with its coordinates and fingerprints.
type ReportOccurrence struct {
	ID                 ID
	RuleID             string
	Kind               string
	Path               string
	LineStart          *int
	PackagePURL        string
	Fingerprint        []byte
	ContentFingerprint []byte
}

// BranchReportQuery asks for the last report of a component, language and kind of analysis on a branch.
type BranchReportQuery struct {
	ComponentID ID
	Language    string
	Kind        string
	RefName     string
}
