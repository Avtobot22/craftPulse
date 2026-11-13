package entity

import "time"

type SyncPhase string

const (
	SyncBackfill    SyncPhase = "backfill"
	SyncIncremental SyncPhase = "incremental"
	SyncPaused      SyncPhase = "paused"
	SyncStopped     SyncPhase = "stopped"
)

// SyncState is the state of collection of a data kind of a connection; Cursor is opaque to everyone but sync.
type SyncState struct {
	ConnectionID  ID
	DataKind      string
	Phase         SyncPhase
	Cursor        string
	CheckpointAt  *time.Time
	FailedCursor  string
	LastAttemptAt *time.Time
	LastOutcome   string
	LastErrorCode string
	LastSuccessAt *time.Time
	Watermark     *time.Time
	HistoryFrom   *time.Time
	Completeness  string
}

// Outcomes of a reconciliation with the manifest of the source (TZ 12.6, step 3).
const (
	ReconcileRunning    = "running"
	ReconcileComplete   = "complete"
	ReconcileIncomplete = "incomplete"
	ReconcileFailed     = "failed"
)

// ReconcileRun compares the identities and versions of a manifest with the current versions of normalize.
type ReconcileRun struct {
	ID            ID
	ConnectionID  ID
	DataKind      string
	AsOf          time.Time
	StartedAt     time.Time
	FinishedAt    *time.Time
	ExpectedCount *int
	ObservedCount *int
	MissingCount  *int
	ChangedCount  *int
	Outcome       string
	ReasonCode    string
}

// DirtyKey is an object of a scope to read again; Added tells a key added during the read from a read one.
type DirtyKey struct {
	Hash     []byte
	ScopeID  ID
	DataKind string
	Object   ObjectKey
	Added    time.Time
}

type SyncRunKind string

const (
	SyncResume    SyncRunKind = "resume"
	SyncReconcile SyncRunKind = "reconcile"
)

// SyncRunStatus is where a requested run is: queued until sync takes it, then running, then done or failed.
type SyncRunStatus struct {
	Status     OperationStatus
	ErrorCode  string
	FinishedAt *time.Time
}

// SyncStatus is the state of collection of a connection for its administrator (FR-020): the error, the
// last success and the completeness of every data kind stay apart.
type SyncStatus struct {
	ConnectionID     ID
	ConnectionStatus ConnectionStatus
	PausedUntil      *time.Time
	DataKinds        []SyncDataKindStatus
	LastReconcile    *ReconcileRun
}

type SyncDataKindStatus struct {
	SyncState
	FailedPagePresent bool
	LagSeconds        *int64
	AffectedMetrics   []string
}
