package entity

import "time"

// How a finding of a report matches its canonical finding (BR-011): uncertain is never both new and resolved.
const (
	MatchExact     = "exact"
	MatchMoved     = "moved"
	MatchUncertain = "uncertain"
)

// States of a baseline and the reasons a row of the matrix has none (FR-035 A3).
const (
	BaselineReady          = "ready"
	BaselinePending        = "pending"
	BaselineFailed         = "failed"
	BaselineRunMissing     = "full_run_missing"
	BaselineScanIncomplete = "scan_incomplete"
	BaselineScanFailed     = "scan_failed"
)

// FindingIdentity is a canonical finding: the same problem across the reports of an area.
type FindingIdentity struct {
	ID                 ID
	AreaID             ID
	MethodVersion      int
	Fingerprint        []byte
	ContentFingerprint []byte
	RuleID             string
	Kind               string
	FirstReportID      ID
	FirstSeenAt        time.Time
}

// OccurrenceIdentity tells which canonical finding a finding of a report is.
type OccurrenceIdentity struct {
	OccurrenceID      ID
	FindingIdentityID ID
	MatchKind         string
	ReportedAt        time.Time
	KnownSeq          int64
}

// Baseline is the last complete successful report of the main branch for a component, language and kind of analysis.
type Baseline struct {
	ID               ID
	ComponentID      ID
	Language         string
	AnalysisKind     string
	ProfileVersionID ID
	ReportID         ID
	EstablishedAt    time.Time
	SupersededAt     *time.Time
}

// BaselineState is the baseline of a row of the matrix of a project; a row without a complete run of the main
// branch is pending, never ready.
type BaselineState struct {
	ComponentID   ID
	Language      string
	AnalysisKind  string
	State         string
	ReportID      *ID
	Branch        string
	SHA           string
	EstablishedAt *time.Time
	ReasonCode    string
}
