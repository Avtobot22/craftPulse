package entity

const KindSyncPage = "sync.page"

// SyncPageJob collects one page of a data kind (DataKind of the contract) of a connection; the cursor
// is the checkpoint of sync.
type SyncPageJob struct {
	OrganizationID ID     `json:"organization_id"`
	ConnectionID   ID     `json:"connection_id"`
	DataKind       string `json:"data_kind"`
	CorrelationID  ID     `json:"correlation_id"`
}

func (SyncPageJob) Kind() string { return KindSyncPage }
