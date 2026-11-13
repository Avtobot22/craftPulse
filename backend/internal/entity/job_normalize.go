package entity

const KindNormalizeBatch = "normalize.batch"

// NormalizeBatchJob turns the accepted records of a receipt into canonical facts.
type NormalizeBatchJob struct {
	OrganizationID ID `json:"organization_id"`
	ReceiptID      ID `json:"receipt_id"`
	CorrelationID  ID `json:"correlation_id"`
}

func (NormalizeBatchJob) Kind() string { return KindNormalizeBatch }
