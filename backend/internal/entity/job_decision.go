package entity

const (
	KindDecisionExpire = "decision.expire"
	KindChangeRecount  = "change.recount"
)

// DecisionExpireJob is the next scheduled change of a decision's effective interval.
type DecisionExpireJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	DecisionID     ID `json:"decision_id" river:"unique"`
	CorrelationID  ID `json:"correlation_id,omitzero"`
}

func (DecisionExpireJob) Kind() string       { return KindDecisionExpire }
func (DecisionExpireJob) UniqueWhileActive() {}

// ChangeRecountJob refreshes the effective exception inputs of an area.
type ChangeRecountJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	AreaID         ID `json:"area_id" river:"unique"`
	CauseID        ID `json:"cause_id" river:"unique"`
	CorrelationID  ID `json:"correlation_id,omitzero"`
}

func (ChangeRecountJob) Kind() string       { return KindChangeRecount }
func (ChangeRecountJob) UniqueWhileActive() {}
