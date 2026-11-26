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
	Notes       []AccessNote
	Next        *TaskEventCursor
}

type TaskHistoryEvent struct {
	Object                ObjectRef
	ID                    ID
	Kind                  string
	Field, Before, After  *string
	AccountID             *ID
	ActorKind             PersonKind
	EventTime, ObservedAt time.Time
	EventOffsetS          int
	ObservedVia           ObservedVia
}
