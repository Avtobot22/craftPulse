package entity

const KindSyncPage = "sync.page"

// SyncPageJob collects one page of a data kind of a connection; the cursor is the checkpoint of sync.
type SyncPageJob struct {
	OrganizationID ID     `json:"organization_id"`
	ConnectionID   ID     `json:"connection_id"`
	SchemaID       string `json:"schema_id"`
	CorrelationID  ID     `json:"correlation_id"`
}

func (SyncPageJob) Kind() string { return KindSyncPage }
