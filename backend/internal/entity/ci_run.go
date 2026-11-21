package entity

import "time"

const (
	ObjectCIRun     ObjectKind = "ci_run"
	ObjectCIAttempt ObjectKind = "ci_attempt"
	ObjectCIJob     ObjectKind = "ci_job"
	// ObjectCIProject is the area of a CI system without a repository of its own, such as a job of Jenkins.
	ObjectCIProject ObjectKind = "ci_project"
)

const MetricSetCI = "ci"

// CIFacts is the canonical CI history of an area at a knowledge cutoff.
type CIFacts struct {
	AreaID   ID
	Provider Provider
	Runs     []CIRunVersion
	Attempts []CIAttemptVersion
}

// CIOutcome is the normalized result of a run or an attempt; the native status is kept beside it.
type CIOutcome string

const (
	CIRunning  CIOutcome = "running"
	CISuccess  CIOutcome = "success"
	CIFailed   CIOutcome = "failed"
	CICanceled CIOutcome = "canceled"
	CISkipped  CIOutcome = "skipped"
	CIUnknown  CIOutcome = "unknown"
)

// AttemptHistory tells whether all retries of the jobs of a run are known (FR-039 A3): a truncated history
// gives only a lower bound of the spent time.
type AttemptHistory string

const (
	AttemptHistoryComplete  AttemptHistory = "complete"
	AttemptHistoryTruncated AttemptHistory = "truncated"
)

type CIRunVersion struct {
	ID     ID
	RunID  ID
	AreaID ID
	// MRID is set only when the source links the run to the MR, both are in one repository and the SHA of
	// the run is a head of a revision of the MR.
	MRID *ID
	Knowledge
	Record            RecordRef
	Revision          Revision
	ObservedAt        time.Time
	SourceUpdatedAt   *time.Time
	RefName           string
	SHA               string
	ParentRunID       *ID
	DefinitionVersion string
	NativeStatus      string
	Outcome           CIOutcome
	StartedAt         *time.Time
	FinishedAt        *time.Time
	AttemptHistory    AttemptHistory
}

// CIAttemptVersion is an attempt of a job (DR-006): JobKey with ConfigFingerprint is the logical job, a
// retry is a new attempt with its own ID.
type CIAttemptVersion struct {
	ID        ID
	AttemptID ID
	RunID     ID
	Knowledge
	Record            RecordRef
	Revision          Revision
	ObservedAt        time.Time
	JobKey            string
	ConfigFingerprint []byte
	AttemptNo         int
	NativeStatus      string
	Outcome           CIOutcome
	AllowFailure      bool
	// FailureKind is empty when the cause is unknown; it is not attributed to the code (FR-039 A2).
	FailureKind string
	RunnerClass string
	StartedAt   *time.Time
	FinishedAt  *time.Time
	DurationS   *int64
}

// CIRun is a run with the attempts of its jobs and of the jobs of its child runs, each counted once.
type CIRun struct {
	Run      CIRunVersion
	Children []CIRunVersion
	CIAttempts
}

// CIAttempts are the known attempts of a run with the completeness of their history.
type CIAttempts struct {
	History  AttemptHistory
	Attempts []CIAttemptVersion
}

// RunMRMatch asks whether the MR a run names is the MR of the same repository with the run SHA among its heads.
type RunMRMatch struct {
	MRNativeID string
	AreaID     ID
	SHA        string
}

// RunsQuery selects the runs of a slice: of an MR, of child runs, or started in a window [From, Before).
type RunsQuery struct {
	MRID    *ID
	Parents []ID
	From    *time.Time
	Before  *time.Time
}
