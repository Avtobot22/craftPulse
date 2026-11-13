package entity

const KindNormalizeBatch = "normalize.batch"

// NormalizeBatchJob turns the accepted records of a receipt into canonical facts, a part at a time:
// AfterNo is the last record number a committed part took. ResolvedNo is the record a conflict resolution
// accepted, which may replace the version of the same source revision.
type NormalizeBatchJob struct {
	OrganizationID ID          `json:"organization_id"`
	ReceiptID      ID          `json:"receipt_id"`
	AfterNo        int         `json:"after_no,omitempty"`
	ObservedVia    ObservedVia `json:"observed_via,omitempty"`
	ResolvedNo     int         `json:"resolved_no,omitempty"`
	CorrelationID  ID          `json:"correlation_id"`
}

func (NormalizeBatchJob) Kind() string { return KindNormalizeBatch }

// UniqueWhileActive keeps one job of a receipt and its part waiting at a time.
func (NormalizeBatchJob) UniqueWhileActive() {}
