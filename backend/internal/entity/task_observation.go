package entity

import "time"

// ObjectRef identifies an accessible object and carries its display values from the source.
type ObjectRef struct {
	Kind       ObjectKind
	ID         ID
	DisplayKey *string
	Title      *string
	SourceURL  *string
	Indirect   bool
}

// TaskObservationFacts contains facts read at one knowledge number after checking access to every object.
type TaskObservationFacts struct {
	Task             TaskVersion
	InstanceID       ID
	TaskEvents       []TaskEvent
	HistoryFrom      *time.Time
	Mapping          *MappingVersion
	Targets          []TargetVersion
	Links            []LinkEdge
	Reviews          []ReviewFacts
	ReviewCollection []AreaCollection
	CI               CIFacts
	Refs             map[ID]ObjectRef
	// TaskMRsUnavailable means the input cannot establish the absence of a confirmed MR.
	TaskMRsUnavailable bool
	KnowledgeSeq       int64
	AsOf               time.Time
}

// ObservationKind is the fact or open question shown on a task.
type ObservationKind string

const (
	ObservationAwaitingReviewer ObservationKind = "awaiting_reviewer"
	ObservationAwaitingFirst    ObservationKind = "awaiting_first_response"
	ObservationCIFailed         ObservationKind = "ci_failed"
	ObservationCIRunning        ObservationKind = "ci_running"
	ObservationCIRetried        ObservationKind = "ci_passed_after_retry"
	ObservationMissingReport    ObservationKind = "missing_report"
	ObservationNewFinding       ObservationKind = "new_finding"
	ObservationBlocked          ObservationKind = "blocked"
	ObservationNoMR             ObservationKind = "no_merge_request"
	ObservationOverTarget       ObservationKind = "over_target"
	ObservationUnknown          ObservationKind = "unknown"
)

// ObservationAction is the next step supported by an observation.
type ObservationAction string

const (
	ObservationAssignReviewer ObservationAction = "assign_reviewer"
	ObservationRequestReview  ObservationAction = "request_review"
	ObservationCheckPipeline  ObservationAction = "check_pipeline"
	ObservationRestoreReport  ObservationAction = "restore_report"
	ObservationReviewFinding  ObservationAction = "review_finding"
	ObservationResolveBlocker ObservationAction = "resolve_blocker"
	ObservationLinkMR         ObservationAction = "link_merge_request"
	ObservationReviewTarget   ObservationAction = "review_target"
	ObservationClarifyReason  ObservationAction = "clarify_reason"
)

// ObservationQuestion is the missing fact to ask about when the action is clarify_reason.
type ObservationQuestion string

const (
	QuestionReviewerAssigned ObservationQuestion = "reviewer_assigned"
	QuestionMRExists         ObservationQuestion = "merge_request_exists"
	QuestionDelayReason      ObservationQuestion = "delay_reason"
)

// TaskObservation has its subject first in Basis. Event and version IDs are calculation evidence.
type TaskObservation struct {
	Kind           ObservationKind
	Since          *time.Time
	Basis          []ObjectRef
	Action         ObservationAction
	Question       ObservationQuestion
	SourceEventIDs []ID
	TargetVersion  *TargetVersion
	Retry          *TaskRetry
}

// TaskRetry gives observed I04 counts and execution time, including any incompleteness of each value.
type TaskRetry struct {
	Attempts            int64
	Seconds             int64
	AttemptsLowerBound  bool
	SecondsLowerBound   bool
	ObservedOutcomeFlip bool
}
