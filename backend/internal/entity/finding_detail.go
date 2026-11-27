package entity

import "time"

// FindingDetail combines a canonical finding with a stored observation and its report.
type FindingDetail struct {
	FindingIdentity
	Observation FindingOccurrence
	Report      TechnicalReport
	ReportKind  string
	ComponentID *ID
	State       string
	Exceptions  []ExceptionVersion
	Actions     []string
	SourceURL   *string
}

type GitLabFindingSource struct {
	InstanceURL   string
	RepositoryURL string
	ProjectPath   string
}

type FindingQuery struct {
	ComponentID, ProjectID, ServiceID ID
	Kind, State, Severity, Sort       string
	Limit                             int
	After                             *FindingPosition
}

type FindingPosition struct {
	ID          ID
	FirstSeenAt time.Time
	Severity    int
}

type FindingPage struct {
	Items []FindingDetail
	Next  *FindingPosition
	Notes []AccessNote
}

type FindingSelection struct {
	Areas, SecurityAreas []ID
	Components           []ID
	Kind                 string
}

type ComponentFindingCount struct {
	Kind, State string
	Count       int
}

type ComponentCodeFact struct {
	SeriesPoint
	Source string
	Window Window
}

type ComponentOverview struct {
	Component Component
	Counts    []ComponentFindingCount
	Facts     []ComponentCodeFact
	Notes     []AccessNote
}
