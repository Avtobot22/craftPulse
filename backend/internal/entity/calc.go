package entity

import "time"

// DataState is the state of a value of a point (TZ 4.3): only ASSESSED and PROVISIONAL carry a value and only
// BOUNDED a bound.
type DataState string

const (
	StateAssessed      DataState = "ASSESSED"
	StateProvisional   DataState = "PROVISIONAL"
	StateBounded       DataState = "BOUNDED"
	StateUnknown       DataState = "UNKNOWN"
	StateNotEvaluated  DataState = "NOT_EVALUATED"
	StateNotApplicable DataState = "NOT_APPLICABLE"
)

// Reasons of a state or of a contribution.
const (
	ReasonMappingMissing     = "mapping_missing"
	ReasonHistoryIncomplete  = "history_incomplete"
	ReasonEventOrderConflict = "event_order_conflict"
	ReasonNoStart            = "no_start"
	ReasonRepeatCompletion   = "repeat_completion"
	ReasonFullAge            = "full_age"
	ReasonStarted            = "started"
	ReasonCompleted          = "completed"
)

// ContributionRole is what an object gives a point (TZ 12.5.8).
type ContributionRole string

const (
	RoleNumerator   ContributionRole = "numerator"
	RoleDenominator ContributionRole = "denominator"
	RoleExcluded    ContributionRole = "excluded"
	RoleMissing     ContributionRole = "missing"
)

// IntervalClass is the class of a workflow interval of an episode (DR-007).
type IntervalClass string

const (
	IntervalActive  IntervalClass = "active"
	IntervalWait    IntervalClass = "wait"
	IntervalBlocked IntervalClass = "blocked"
)

// Step is the length of a point of a series.
type Step string

const (
	StepWeek  Step = "week"
	StepMonth Step = "month"
)

// GroupBy splits the series of a report by an attribute of a task.
type GroupBy string

const (
	GroupNone      GroupBy = ""
	GroupTaskType  GroupBy = "task_type"
	GroupSizeLabel GroupBy = "size_label"
)

// GroupingCurrentStructure is the M0 grouping by the current composition of the team (BR-007).
const GroupingCurrentStructure = "current_structure"

// Bound is the proven lower or upper bound of a BOUNDED value.
type Bound struct {
	Kind  string
	Value float64
}

const (
	BoundLower = "lower"
	BoundUpper = "upper"
)

// Contribution is what one object gives a point; ItemNo tells several contributions of one object apart.
type Contribution struct {
	ObjectID   ID
	ItemNo     int
	ObjectKind ObjectKind
	Role       ContributionRole
	Value      *float64
	ReasonCode string
}

// Point is a value of a series over [Start, End); a metric of a state at a date is taken at End.
type Point struct {
	Start         time.Time
	End           time.Time
	State         DataState
	Value         *float64
	Numerator     *float64
	Denominator   *float64
	N             *int
	Bound         *Bound
	ExcludedCount int
	MissingCount  int
	LowSample     bool
	ReasonCode    string
	Contributions []Contribution
}

// SeriesGroup is the group of a split series; a nil label is the group of tasks without it.
type SeriesGroup struct {
	TaskType  *string
	SizeLabel *string
}

// Series is the points of one metric and estimator, for example T08 with p85.
type Series struct {
	MetricID          string
	DefinitionVersion int
	Unit              string
	Estimator         string
	Group             *SeriesGroup
	Points            []Point
}

// WorkflowInterval is an interval [From, To) of an episode of a task (DR-007); an open one ends at as_of.
type WorkflowInterval struct {
	TaskID         ID
	EpisodeNo      int
	IntervalNo     int
	Class          IntervalClass
	From           time.Time
	To             time.Time
	SourceEventIDs []ID
}

// TaskSlice is what a calculation knows of a task: the attributes a series is cut by, the clocks of its
// workflow and its intervals. Unmapped are the native statuses it passed without a rule.
type TaskSlice struct {
	TaskID          ID
	ProjectID       ID
	TaskType        *string
	SizeLabel       *string
	Created         time.Time
	FirstStart      *time.Time
	FirstCompletion *time.Time
	FinalCompletion *time.Time
	Episodes        int
	HistoryFrom     *time.Time
	Conflict        bool
	Unmapped        []string
	Intervals       []WorkflowInterval
}

// Window is the period [From, To) of a series cut into points of Step.
type Window struct {
	From time.Time
	To   time.Time
	Step Step
}

// AreaFacts is every task of a source area known at a slice with its events, restricted ones included.
type AreaFacts struct {
	AreaID     ID
	InstanceID ID
	Tasks      []TaskVersion
	Events     []TaskEvent
}

// CalcRequest asks for the flow of a team over a window at the knowledge number KnowledgeSeq.
type CalcRequest struct {
	OrganizationID ID
	TeamID         ID
	KnowledgeSeq   int64
	Window         Window
	AsOf           time.Time
	GroupBy        GroupBy
}

// CalcResult is a calculation of the flow metrics: the series and the tasks they were computed from.
type CalcResult struct {
	Grouping       string
	MappingVersion *int
	Series         []Series
	Tasks          []TaskSlice
}
