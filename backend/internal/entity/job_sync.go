package entity

const (
	KindSyncPage      = "sync.page"
	KindSyncHistory   = "sync.history"
	KindSyncReconcile = "sync.reconcile"
)

// SyncPageJob collects one page of a data kind (DataKind of the contract) of a connection; the cursor
// is the checkpoint of sync. Retry counts the failures in a row the job is the repeat of.
type SyncPageJob struct {
	OrganizationID ID     `json:"organization_id"`
	ConnectionID   ID     `json:"connection_id"`
	DataKind       string `json:"data_kind"`
	CorrelationID  ID     `json:"correlation_id"`
	Retry          int    `json:"retry,omitempty"`
}

func (SyncPageJob) Kind() string { return KindSyncPage }

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

// SyncReconcileJob compares what was collected of a data kind with the manifest of the source.
type SyncReconcileJob struct {
	OrganizationID ID     `json:"organization_id"`
	ConnectionID   ID     `json:"connection_id"`
	DataKind       string `json:"data_kind"`
	CorrelationID  ID     `json:"correlation_id"`
}

func (SyncReconcileJob) Kind() string { return KindSyncReconcile }
