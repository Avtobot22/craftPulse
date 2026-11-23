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
	ReasonMappingMissing      = "mapping_missing"
	ReasonHistoryIncomplete   = "history_incomplete"
	ReasonEventOrderConflict  = "event_order_conflict"
	ReasonNoStart             = "no_start"
	ReasonRepeatCompletion    = "repeat_completion"
	ReasonFullAge             = "full_age"
	ReasonStarted             = "started"
	ReasonCompleted           = "completed"
	ReasonTypeUnknown         = "type_unknown"
	ReasonTargetMissing       = "target_missing"
	ReasonImmature            = "immature"
	ReasonStatusUnmapped      = "status_unmapped"
	ReasonCollectionPartial   = "collection_incomplete"
	ReasonAsOfOnly            = "as_of_only"
	ReasonWatermarkMissing    = "watermark_missing"
	ReasonReconcileMissing    = "reconcile_missing"
	ReasonReconcileUnfinished = "reconcile_unfinished"
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
	Evidence   []byte
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
	EventKind string
	ActorKind PersonKind
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

// CalcRequest asks for the flow of a team over a window at the knowledge number KnowledgeSeq. Sources is the state
// of collection a chain fixed at its start; nil reads the current one.
type CalcRequest struct {
	OrganizationID ID
	TeamID         ID
	KnowledgeSeq   int64
	Window         Window
	AsOf           time.Time
	GroupBy        GroupBy
	Sources        *[]AreaCollection
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
	InputKindTarget     = "target"
	InputKindWatermark  = "source_watermark"
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
// part number is not in the uniqueness, so a series has one chain of a window at a time. Sources is the state of
// collection and Structure the version of the structure the start fixed for the parts of the chain.
type CalcSliceJob struct {
	OrganizationID ID                `json:"organization_id" river:"unique"`
	SeriesID       ID                `json:"series_id" river:"unique"`
	From           time.Time         `json:"from" river:"unique"`
	To             time.Time         `json:"to" river:"unique"`
	AsOf           time.Time         `json:"as_of"`
	RevisionID     ID                `json:"revision_id,omitzero"`
	PartNo         int               `json:"part_no,omitempty"`
	Sources        *[]AreaCollection `json:"sources,omitempty"`
	ActivityAreas  *[]ID             `json:"activity_areas,omitempty"`
	Structure      int64             `json:"structure,omitempty"`
	CorrelationID  ID                `json:"correlation_id"`
}

func (CalcSliceJob) Kind() string { return KindCalcSlice }

func (CalcSliceJob) UniqueWhileActive() {}

const (
	KindCalcRecount    = "calc.recount"
	KindCalcExtend     = "calc.extend"
	KindCalcExtendTick = "calc.extend.tick"
)

// CalcRecountJob asks calc for a new chain of the series the facts of an area or the settings of a team feed, or
// of every series of the organization when neither is set; normalize, process and org queue it without depending
// on calc (TZ 10.3, rule 6).
type CalcRecountJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	AreaID         ID `json:"area_id,omitzero" river:"unique"`
	TeamID         ID `json:"team_id,omitzero" river:"unique"`
	ConnectionID   ID `json:"connection_id,omitzero" river:"unique"`
	CodeReceiptID  ID `json:"code_receipt_id,omitzero" river:"unique"`
	DocsReceiptID  ID `json:"docs_receipt_id,omitzero" river:"unique"`
	CorrelationID  ID `json:"correlation_id"`
}

func (CalcRecountJob) Kind() string { return KindCalcRecount }

func (CalcRecountJob) UniqueWhileActive() {}

// CalcExtendJob queues the daily extension of the windows of the series of an organization due before the next
// tick (TZ 10.6).
type CalcExtendJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
}

func (CalcExtendJob) Kind() string { return KindCalcExtend }

func (CalcExtendJob) UniqueWhileActive() {}

// CalcExtendTickJob queues the extension of every organization every hour.
type CalcExtendTickJob struct{}

func (CalcExtendTickJob) Kind() string { return KindCalcExtendTick }

// SeriesRefresh is a manual refresh of a series queued in the transaction of its descriptor: a revision numbered
// after AfterRevisionNo answers it.
type SeriesRefresh struct {
	SeriesID        ID
	AfterRevisionNo int
}

// RefreshStatus is where a manual refresh is: the revision that answers it, once it exists.
type RefreshStatus struct {
	Status     OperationStatus
	RevisionID *ID
}

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

// PointSample is a contribution of a task to a point with the type and the size the task is counted by.
type PointSample struct {
	TaskID     ID
	Role       ContributionRole
	Value      *float64
	ReasonCode string
	TaskType   *string
	SizeLabel  *string
}

// TargetBaseline is the historical p85 of the cycle time of a group of tasks; a nil size is the group of tasks
// without a size.
type TargetBaseline struct {
	TaskType  string
	SizeLabel *string
	Unit      string
	P85       Point
}

// TargetBaselines are the baselines of a team over a period read from one revision of its weekly series.
type TargetBaselines struct {
	RevisionID ID
	Items      []TargetBaseline
}

// AreaCollection is the state of the collection of tasks of a source area by one of its connections, for the
// completeness of a calculation (T39, T40, T42). Completeness is complete only after a reconciliation without
// differences (BR-022).
type AreaCollection struct {
	AreaID              ID
	ConnectionID        ID
	DataKind            string
	LastErrorCode       string
	Completeness        string
	Watermark           *time.Time
	LastSuccessAt       *time.Time
	Reconcile           *ReconcileRun
	ConnectionReconcile *ReconcileRun
	// Phase is the phase of the collection of tasks; empty before the first page.
	Phase       SyncPhase
	HistoryFrom *time.Time
}
