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

// RevisionStatus is the life of a revision of a series (TZ 12.7).
type RevisionStatus string

const (
	RevisionBuilding   RevisionStatus = "building"
	RevisionPublished  RevisionStatus = "published"
	RevisionSuperseded RevisionStatus = "superseded"
	RevisionFailed     RevisionStatus = "failed"
)

// Reasons a revision cannot be reproduced; its values stay.
const (
	IrreproducibleBasisDeleted      = "basis_deleted"
	IrreproducibleBasisExpired      = "basis_expired"
	IrreproducibleDefinitionRetired = "definition_retired"
)

// Inputs of a revision kept in calc.revision_input.
const (
	InputKindDefinition = "definition"
	InputKindMapping    = "mapping"
	InputKindCalendar   = "calendar"
	InputKindStructure  = "structure"
)

// MetricSetFlow is the set of the flow metrics of a team.
const MetricSetFlow = "task_flow"

// CalcSeries is an area, a set of metrics and a step; Scope is canonical JSON and ScopeHash its SHA-256.
type CalcSeries struct {
	ID                ID
	Scope             []byte
	ScopeHash         []byte
	MetricSet         string
	Step              Step
	CurrentRevisionID *ID
	ConsumedSeq       int64
}

// CalcRevision is a revision of a series; it does not change after publication.
type CalcRevision struct {
	ID                   ID
	SeriesID             ID
	RevisionNo           int
	Status               RevisionStatus
	ProgressAt           time.Time
	Window               Window
	AsOf                 time.Time
	KnowledgeSeq         int64
	Timezone             string
	InputHash            []byte
	Loading              bool
	StartedAt            time.Time
	PublishedAt          *time.Time
	SupersededAt         *time.Time
	IrreproducibleReason string
	PreviousRevisionID   *ID
}

// RevisionInput is a version of an input a revision was computed with.
type RevisionInput struct {
	Kind         string
	ID           string
	Version      string
	Watermark    *time.Time
	Completeness string
}

// SeriesPoint is a stored point of a metric and an estimator of a revision.
type SeriesPoint struct {
	MetricID  string
	Estimator string
	Unit      string
	Point
}

// ContributionKey is the position of a page of contributions of a point.
type ContributionKey struct {
	ObjectID ID
	ItemNo   int
}

// ContributionQuery reads the contributions of one point of a revision after the key.
type ContributionQuery struct {
	MetricID   string
	Estimator  string
	PointStart time.Time
	After      *ContributionKey
	Limit      int
}

// ContributionPage is a page of contributions; Next is set when more follow.
type ContributionPage struct {
	Items []Contribution
	Next  *ContributionKey
}

// StoredPart is a computed part of a series ready to be saved: its points with their contributions, the
// intervals of its tasks inside the part and the attributes the report cuts it by.
type StoredPart struct {
	InputHash []byte
	MetricSet string
	From, To  time.Time
	Points    []SeriesPoint
	Intervals []WorkflowInterval
	Tasks     []TaskSlice
}

const (
	KindCalcSlice  = "calc.slice"
	KindCalcGC     = "calc.gc"
	KindCalcGCTick = "calc.gc.tick"
)

// CalcSliceJob builds a revision of a series part by part (TZ 10.6). Without RevisionID it starts a chain; the
// part number is not in the uniqueness, so a series has one chain of a window at a time.
type CalcSliceJob struct {
	OrganizationID ID        `json:"organization_id" river:"unique"`
	SeriesID       ID        `json:"series_id" river:"unique"`
	From           time.Time `json:"from" river:"unique"`
	To             time.Time `json:"to" river:"unique"`
	AsOf           time.Time `json:"as_of"`
	RevisionID     ID        `json:"revision_id,omitzero"`
	PartNo         int       `json:"part_no,omitempty"`
	CorrelationID  ID        `json:"correlation_id"`
}

func (CalcSliceJob) Kind() string { return KindCalcSlice }

func (CalcSliceJob) UniqueWhileActive() {}

// CalcGCJob deletes a portion of the parts of an organization no revision refers to; AfterPartID is the
// cursor of the portion before.
type CalcGCJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	AfterPartID    ID `json:"after_part_id,omitzero"`
}

func (CalcGCJob) Kind() string { return KindCalcGC }

func (CalcGCJob) UniqueWhileActive() {}

// CalcGCTickJob queues the collection of parts of every organization once a day.
type CalcGCTickJob struct{}

func (CalcGCTickJob) Kind() string { return KindCalcGCTick }

// PreviewOutcome is the state of a task of a preview at as_of.
type PreviewOutcome string

const (
	PreviewOpen      PreviewOutcome = "open"
	PreviewCompleted PreviewOutcome = "completed"
	PreviewCancelled PreviewOutcome = "cancelled"
	PreviewDuplicate PreviewOutcome = "duplicate"
	// PreviewUnknown is a task whose current status or resolution has no rule, or whose history conflicts.
	PreviewUnknown PreviewOutcome = "unknown"
)

// PreviewTask is how a draft of the mapping reads one task: the first start and completion seen in its known
// history, its outcome at as_of and the statuses it passed without a rule. BlockedSeconds is nil when the
// order of its events conflicts.
type PreviewTask struct {
	TaskID         ID
	DisplayKey     string
	Title          string
	Created        time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	Outcome        PreviewOutcome
	BlockedSeconds *int64
	TaskType       *string
	SizeLabel      *string
	Unmapped       []string
}

// ControlValue is a control metric of a preview over its period, computed by the definition of the report.
type ControlValue struct {
	MetricID  string
	Estimator string
	Unit      string
	Point     Point
}

// MappingPreview is a draft of the mapping applied to the visible tasks of a team without saving it (FR-017):
// a sample of them and the control metrics over Period, the week of the report up to AsOf.
type MappingPreview struct {
	AsOf     time.Time
	Period   Window
	Tasks    []PreviewTask
	Controls []ControlValue
	Unmapped []NativeStatusRef
}
