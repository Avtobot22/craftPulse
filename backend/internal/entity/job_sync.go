package entity

const (
	KindSyncPage      = "sync.page"
	KindSyncHistory   = "sync.history"
	KindSyncReconcile = "sync.reconcile"
)

// SyncPageJob collects one page of a data kind (DataKind of the contract) of a connection; the cursor
// is the checkpoint of sync. Retry counts the failures in a row the job is the repeat of.
type SyncPageJob struct {
	OrganizationID ID     `json:"organization_id" river:"unique"`
	ConnectionID   ID     `json:"connection_id" river:"unique"`
	DataKind       string `json:"data_kind" river:"unique"`
	CorrelationID  ID     `json:"correlation_id"`
	Retry          int    `json:"retry,omitempty"`
}

func (SyncPageJob) Kind() string { return KindSyncPage }

// UniqueWhileActive keeps one page of a data kind of a connection waiting at a time (TZ 10.6): a schedule
// or a request during an attempt adds nothing.
func (SyncPageJob) UniqueWhileActive() {}

// SyncHistoryJob reads the history of one object the source returned cut; Cursor continues its pages.
type SyncHistoryJob struct {
	OrganizationID ID            `json:"organization_id"`
	ConnectionID   ID            `json:"connection_id"`
	ScopeID        ID            `json:"scope_id"`
	SchemaID       string        `json:"schema_id"`
	ObjectKind     string        `json:"object_kind"`
	NativeNS       string        `json:"native_ns"`
	NativeID       string        `json:"native_id"`
	Cursor         *SourceCursor `json:"cursor,omitempty"`
	CorrelationID  ID            `json:"correlation_id"`
	Retry          int           `json:"retry,omitempty"`
}

func (SyncHistoryJob) Kind() string { return KindSyncHistory }

// UniqueWhileActive makes one history job per object of the source.
func (SyncHistoryJob) UniqueWhileActive() {}

// SyncReconcileJob compares what was collected of a data kind with the manifest of the source. RunID is
// the run written when it was queued; Round counts the reconciliations in a row that found differences.
type SyncReconcileJob struct {
	OrganizationID ID     `json:"organization_id"`
	ConnectionID   ID     `json:"connection_id"`
	DataKind       string `json:"data_kind"`
	RunID          ID     `json:"run_id"`
	Round          int    `json:"round,omitempty"`
	CorrelationID  ID     `json:"correlation_id"`
}

func (SyncReconcileJob) Kind() string { return KindSyncReconcile }

const (
	KindSyncDirty    = "sync.dirty"
	KindSyncTick     = "sync.tick"
	KindSyncSchedule = "sync.schedule"
)

// SyncDirtyJob reads again the objects in the set of a connection.
type SyncDirtyJob struct {
	OrganizationID ID  `json:"organization_id"`
	ConnectionID   ID  `json:"connection_id"`
	Retry          int `json:"retry,omitempty"`
}

func (SyncDirtyJob) Kind() string { return KindSyncDirty }

// UniqueWhileActive keeps one read of the set of a connection waiting at a time.
func (SyncDirtyJob) UniqueWhileActive() {}

// SyncTickJob is the periodic job that plans the collection of every organization.
type SyncTickJob struct{}

func (SyncTickJob) Kind() string { return KindSyncTick }

// SyncScheduleJob starts the increments of an organization that are due.
type SyncScheduleJob struct {
	OrganizationID ID `json:"organization_id"`
}

func (SyncScheduleJob) Kind() string { return KindSyncSchedule }

// UniqueWhileActive keeps one plan of an organization waiting at a time.
func (SyncScheduleJob) UniqueWhileActive() {}
