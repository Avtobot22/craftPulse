package entity

import "time"

type OperationKind string

const OperationSyncRun OperationKind = "sync_run"

type OperationStatus string

const (
	OperationQueued    OperationStatus = "queued"
	OperationRunning   OperationStatus = "running"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
)

// Safe classes of a failed operation; the cause stays in the logs under the correlation ID.
const (
	OperationSourceUnavailable = "source_unavailable"
	OperationAccessLost        = "access_lost"
)

// Operation is the public descriptor of a run a user asked for (TZ 12.5.8): its job in the queue is not shown.
type Operation struct {
	ID             ID
	OrganizationID ID
	RequestedBy    ID
	Kind           OperationKind
	ResourceKind   string
	ResourceID     ID
	Action         string
	Parameters     []byte
	RequestHash    []byte
	Status         OperationStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
	ResultURI      string
	ErrorCode      string
	CorrelationID  *ID
}

// Done tells a finished operation.
func (o Operation) Done() bool { return o.Status == OperationSucceeded || o.Status == OperationFailed }

// SyncRunRequest is what an administrator asks of the collection of a connection (UI-11).
type SyncRunRequest struct {
	Kind     SyncRunKind `json:"kind"`
	DataKind string      `json:"data_kind"`
}
