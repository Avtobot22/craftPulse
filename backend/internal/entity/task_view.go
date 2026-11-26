package entity

import "time"

type TaskViewQuery struct {
	Project, Team *ID
	Meaning, Sort string
	OverTarget    *bool
	Limit         int
	After         *TaskViewCursor
}

type TaskViewCursor struct {
	AsOf      time.Time
	Knowledge int64
	ID        ID
	Age       *int
	UpdatedAt *time.Time
	Key       string
}

type TaskViewPage struct {
	Items []TaskSummary
	Next  *TaskViewCursor
	Notes []AccessNote
}

type TaskSummary struct {
	Version    TaskVersion
	ProjectID  *ID
	TeamID     *ID
	StatusName string
	SourceURL  *string
	TaskWorkflow
}

type TaskWorkflow struct {
	Meaning    string
	Type       *string
	Size       *string
	AgeSeconds *int
	Target     *TargetVersion
	OverTarget *bool
	Blocked    []TaskBlock
}

type TaskBlock struct {
	From time.Time
	To   *time.Time
}

type TaskDetail struct {
	TaskSummary
	Links        []TaskViewLink
	Observations []TaskObservation
	Redactions   []string
}

type TaskViewLink struct {
	Edge LinkEdge
	To   ObjectRef
}

type TaskEventQuery struct {
	Limit int
	After *TaskEventCursor
}

type TaskEventCursor struct {
	AsOf      time.Time
	Knowledge int64
	EventTime time.Time
	ID        ID
}

type TaskEventPage struct {
	Items       []TaskHistoryEvent
	HistoryFrom *time.Time
	Sources     []TimelineSource
	Notes       []AccessNote
	Next        *TaskEventCursor
}

type TaskHistoryEvent struct {
	Object                ObjectRef
	AreaID                ID
	Late                  bool
	ID                    ID
	Kind                  string
	Field, Before, After  *string
	AccountID             *ID
	ActorKind             PersonKind
	EventTime, ObservedAt time.Time
	EventOffsetS          int
	ObservedVia           ObservedVia
}

type ProjectTimelineQuery struct {
	TaskEventQuery
	From, To   time.Time
	RevisionID *ID
	ActorKind  PersonKind
	ObjectKind ObjectKind
}

type TimelineSource struct {
	ConnectionID ID
	Provider     Provider
	HistoryFrom  *time.Time
	Completeness string
}

// TimelineFact is an event projected from CI or document versions by normalize.
// For an attempt, RunID identifies the pipeline to which the event belongs.
type TimelineFact struct {
	ID, ObjectID, AreaID, InstanceID, RunID                  ID
	ObjectKind                                               ObjectKind
	Kind, NativeID, ParentNativeID, DisplayKey, Title, Value string
	ProjectPath                                              string
	EventTime, ObservedAt                                    time.Time
	EventOffsetS                                             int
	ObservedVia                                              ObservedVia
}

type TimelineFactQuery struct {
	From, To  time.Time
	Runs      []ID
	Kind      ObjectKind
	Knowledge int64
}

type TimelinePublication struct {
	TeamID            ID
	AsOf, PublishedAt time.Time
	Knowledge         int64
}
