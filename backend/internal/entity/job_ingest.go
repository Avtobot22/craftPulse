package entity

const KindIngestPackage = "ingest.package"

// IngestPackageJob receives a package waiting in quarantine; a repeat finds the receipt done.
type IngestPackageJob struct {
	OrganizationID ID `json:"organization_id"`
	ReceiptID      ID `json:"receipt_id"`
	CorrelationID  ID `json:"correlation_id"`
}

func (IngestPackageJob) Kind() string { return KindIngestPackage }
