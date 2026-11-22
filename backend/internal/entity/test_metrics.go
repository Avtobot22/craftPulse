package entity

import "time"

// ReportScope selects one repository and SHA. For a pipeline the caller supplies the visible runs and attempts
// at KnownSeq. A nil Pipeline selects all reports for the repository and SHA.
type ReportScope struct {
	AreaID     ID
	SHA        string
	Pipeline   *ID
	RunIDs     []ID
	AttemptIDs []ID
	KnownSeq   int64
}

type ReportEvidence struct {
	ID              ID
	RunID           *ID
	ComponentID     *ID
	Kind            string
	Language        string
	Format          string
	Producer        string
	ProducerVersion string
	ScopeHash       string
	ExitClass       string
	Completeness    string
	KnownSeq        int64
	RejectedReason  string
	SkippedTargets  []SkippedTarget
	DependencyDelta string
	SecretScope     string
}

func (r *ReportEvidence) Partial() bool {
	return r.Completeness == "partial" || r.ExitClass == "partial" || r.ExitClass == "failed" || r.ExitClass == "timeout"
}

type ReportExecution struct {
	ID         ID
	ReportID   ID
	CaseID     ID
	Superseded bool
	TestExecution
}

type TestExecutionQuery struct {
	Limit    int
	Outcome  string
	After    *ID
	KnownSeq *int64
	IssuedAt *time.Time
}

type TestExecutionList struct {
	Items    []ReportExecution
	Next     *ID
	KnownSeq int64
	IssuedAt time.Time
}

type ReportCoverage struct {
	ID       ID
	ReportID ID
	CoverageFact
}

type ReportFacts struct {
	Scope      ReportScope
	Reports    []ReportEvidence
	Executions []ReportExecution
	Coverage   []ReportCoverage
}

type TestCounts struct {
	Passed        int
	Failed        int
	Errors        int
	Skipped       int
	DistinctCases int
	Executions    int
}

type TestGroup struct {
	Suite             *string
	ConfigFingerprint string
	AttemptID         *ID
	Counts            TestCounts
}

type TestMetric struct {
	State         DataState
	ReasonCode    string
	Counts        *TestCounts
	Groups        []TestGroup
	Contributions []TestContribution
}

type TestContribution struct {
	Execution  ReportExecution
	Role       ContributionRole
	ReasonCode string
}

type CoverageContribution struct {
	File       ReportCoverage
	Role       ContributionRole
	ReasonCode string
}

type CoverageMetric struct {
	Point
	Contributions []CoverageContribution
}

// TestCoverageRevision is reproducible from the report scope at its knowledge number and the definition versions.
type TestCoverageRevision struct {
	Scope          ReportScope
	Definitions    []RevisionInput
	Reports        []ReportEvidence
	Tests          TestMetric
	LineCoverage   CoverageMetric
	BranchCoverage CoverageMetric
}

type ExpectedReport struct {
	ComponentID ID
	Kind        string
	Language    string
}
