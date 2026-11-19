package entity

import "time"

// ReportFilter cuts a revision by the attributes its contributions keep (TZ 11): projects, the type and the
// size at start of a task. UnknownSize selects the tasks without a size; GroupBy splits every series by type
// or size.
type ReportFilter struct {
	Projects    []ID
	TaskType    *string
	SizeLabel   *string
	UnknownSize bool
	GroupBy     GroupBy
}

// VisibleSeries are the series of a revision computed by their definitions over the part of the cohort the
// reader may see; Notes mark what is hidden without the number or names (FR-001).
type VisibleSeries struct {
	Series []Series
	Notes  []AccessNote
}

// CohortRow is a stored contribution of a revision with the attributes of its task; a contribution of an
// object that is not a task has no project.
type CohortRow struct {
	MetricID   string
	Estimator  string
	PointStart time.Time
	Contribution
	ProjectID *ID
	TaskType  *string
	SizeLabel *string
}

// CohortQuery reads one point of a revision: its contributions of a role after the key.
type CohortQuery struct {
	MetricID   string
	Estimator  string
	PointStart time.Time
	Role       ContributionRole
	After      *ContributionKey
	Limit      int
}

// CohortObject is a counted object as the reader sees it; Deleted marks an object removed on request, shown
// without its content (V-09).
type CohortObject struct {
	ID         ID
	Kind       ObjectKind
	InstanceID ID
	DisplayKey *string
	Title      *string
	Deleted    bool
}

// CohortItem is a contribution of the cohort page with its object, the attributes the report cuts it by and
// the interval of the task open at the end of a point of a state.
type CohortItem struct {
	Contribution
	Object    CohortObject
	TaskType  *string
	SizeLabel *string
	Interval  *WorkflowInterval
}

// CohortPage is a page of the visible cohort of a point; Next is set when more follow.
type CohortPage struct {
	Items []CohortItem
	Next  *ContributionKey
	Notes []AccessNote
}

// MetricDefinition is a version of a metric of the catalog as the registry gives it (DR-011).
type MetricDefinition struct {
	ID         string
	Version    int
	Name       string
	Unit       string
	CatalogRef string
	Parameters map[string]any
}

// ReportQuery asks for the report of a team over [From, To) by the current revision of the series of its step
// or by a given revision of the team.
type ReportQuery struct {
	TeamID     ID
	Step       Step
	From, To   time.Time
	RevisionID *ID
	Filter     ReportFilter
}

// TeamReport is a revision of the series of a team narrowed to the reader, with the points of the period.
type TeamReport struct {
	Revision CalcRevision
	Series   []Series
	Notes    []AccessNote
}

// RevisionPassport is a revision with the definitions and the versions of the inputs it was computed with.
type RevisionPassport struct {
	Revision    CalcRevision
	Definitions []MetricDefinition
	Inputs      []RevisionInput
}
