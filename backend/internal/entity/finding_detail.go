package entity

// FindingDetail combines a canonical finding with a stored observation and its report.
type FindingDetail struct {
	FindingIdentity
	Observation FindingOccurrence
	Report      TechnicalReport
	ReportKind  string
	ComponentID *ID
	State       string
	Exceptions  []ExceptionVersion
}
