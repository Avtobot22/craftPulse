package entity

const KindChangeIdentify = "change.identify"

// ChangeIdentifyJob gives the findings of the reports of a receipt their canonical identities and decides the
// baselines the reports establish; normalize queues it after the last part of a receipt with reports (TZ 10.6).
type ChangeIdentifyJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	ReceiptID      ID `json:"receipt_id" river:"unique"`
	CorrelationID  ID `json:"correlation_id"`
}

func (ChangeIdentifyJob) Kind() string { return KindChangeIdentify }

// UniqueWhileActive keeps one job of a receipt waiting at a time.
func (ChangeIdentifyJob) UniqueWhileActive() {}
