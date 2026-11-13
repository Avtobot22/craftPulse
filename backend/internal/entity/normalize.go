package entity

import "time"

type ObjectKind string

const (
	ObjectTrackerProject ObjectKind = "tracker_project"
	ObjectTask           ObjectKind = "task"
)

// IdentityQuality tells whether an identity may enter an exact count (BR-001): uncertain may not.
type IdentityQuality string

const (
	IdentityStable    IdentityQuality = "stable"
	IdentityDerived   IdentityQuality = "derived"
	IdentityUncertain IdentityQuality = "uncertain"
)

// ObservedVia is how craftPulse learned of an event.
type ObservedVia string

const (
	ViaWebhook     ObservedVia = "webhook"
	ViaIncremental ObservedVia = "incremental"
	ViaBackfill    ObservedVia = "backfill"
	ViaReconcile   ObservedVia = "reconcile"
	ViaImport      ObservedVia = "import"
)

// SourceKey is the identity of an external object (TZ 12.4): an empty NativeNS means the ID is unique in
// the instance.
type SourceKey struct {
	InstanceID ID
	Kind       ObjectKind
	NativeNS   string
	NativeID   string
}

// SourceObject is an external object with the area it belongs to now; access decides by them.
type SourceObject struct {
	ID            ID
	Key           SourceKey
	Quality       IdentityQuality
	FirstKnownSeq int64
	CurrentAreaID *ID
	Restricted    bool
	DeletedAt     *time.Time
}

// ObjectPlacement is what an object's current version says of its area and its own restriction.
type ObjectPlacement struct {
	ObjectID   ID
	AreaID     ID
	Restricted bool
}

// Knowledge places a version in slices (TZ 12.6): it holds for a slice K when KnownSeq <= K and it was
// not superseded at or before K.
type Knowledge struct {
	KnownSeq      int64
	SupersededSeq *int64
	SupersededAt  *time.Time
}

// RecordRef points to the record of the intake journal a fact came from.
type RecordRef struct {
	ReceiptID ID
	RecordNo  int
}

// Revision is a revision of the source with the hash of its content; a version is one per both.
type Revision struct {
	Source string
	Hash   []byte
}

type TaskVersion struct {
	ID     ID
	TaskID ID
	AreaID ID
	Knowledge
	Record   RecordRef
	Revision Revision
	// ObservedAt is when craftPulse received the state; with SourceUpdatedAt it orders states of a task.
	ObservedAt        time.Time
	SourceUpdatedAt   *time.Time
	TaskKey           string
	Title             string
	TypeNativeID      string
	StatusNativeID    string
	ResolutionNative  string
	CreatedSrcAt      time.Time
	SizeNative        string
	TeamNative        string
	AssigneeAccountID *ID
	ParentTaskID      *ID
	RestrictionNative string
	RawFields         []byte
}

type TaskEventKind string

const (
	TaskCreated     TaskEventKind = "created"
	TaskFieldChange TaskEventKind = "field_change"
	TaskComment     TaskEventKind = "comment"
	TaskLinkChange  TaskEventKind = "link_change"
	TaskMoved       TaskEventKind = "moved"
)

// TaskEvent is an event of the history of a task (DR-002): EventTime places it in history, KnownSeq in a slice.
type TaskEvent struct {
	ID       ID
	TaskID   ID
	AreaID   ID
	EventKey string
	Knowledge
	Record          RecordRef
	Revision        Revision
	Kind            TaskEventKind
	FieldNativeID   string
	BeforeValue     *string
	AfterValue      *string
	BeforeAccountID *ID
	AfterAccountID  *ID
	ActorAccountID  *ID
	EventTime       time.Time
	EventOffsetS    int
	ObservedAt      time.Time
	ObservedVia     ObservedVia
}

// EventKey identifies an event of a task; its versions are corrections of the source.
type EventKey struct {
	TaskID ID
	Key    string
}

type DictionaryKind string

const (
	DictionaryStatus     DictionaryKind = "task_status"
	DictionaryResolution DictionaryKind = "task_resolution"
	DictionaryType       DictionaryKind = "task_type"
	DictionaryField      DictionaryKind = "task_field"
)

// DictionaryEntry is a status, a resolution, a type or a field of an area as the source describes it;
// Attributes is the rest of its record as JSON.
type DictionaryEntry struct {
	ID       ID
	AreaID   ID
	Kind     DictionaryKind
	NativeID string
	Knowledge
	Record     RecordRef
	Revision   Revision
	Name       string
	Attributes []byte
}

// DictionaryKey identifies an entry of a dictionary of an area.
type DictionaryKey struct {
	AreaID   ID
	Kind     DictionaryKind
	NativeID string
}

// AreaChange tells calc which area changed at a knowledge number and from which event time to recount.
type AreaChange struct {
	AreaID            ID
	KnownSeq          int64
	EarliestEventTime *time.Time
	FactKinds         []string
}

// SourceAccount is an account of a source instance; only its native ID identifies it there.
type SourceAccount struct {
	NativeID    string
	Login       string
	DisplayName string
	Email       string
	Kind        PersonKind
	// Described is set when the account comes with its own record; a bare reference does not change one.
	Described bool
}

type PersonKind string

const (
	PersonHuman   PersonKind = "human"
	PersonBot     PersonKind = "bot"
	PersonSystem  PersonKind = "system"
	PersonUnknown PersonKind = "unknown"
)
