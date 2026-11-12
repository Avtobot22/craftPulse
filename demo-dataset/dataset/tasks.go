package dataset

// Task model of the demo set: projects, tasks and their history, independent of the source they are loaded into.

// Workflow of every task project. Category is the API category of the tracker: an intake status may be
// reported as active by the source (V-14), the mapping decides what it means.
const (
	StatusBacklog    = "Backlog"
	StatusTriage     = "Triage"
	StatusInProgress = "In Progress"
	StatusInReview   = "In Review"
	StatusWaiting    = "Waiting"
	StatusBlocked    = "Blocked"
	StatusLegacyQA   = "Legacy QA"
	StatusDone       = "Done"
	StatusCancelled  = "Cancelled"
	StatusDuplicate  = "Duplicate"

	ResolutionDone      = "Done"
	ResolutionWontDo    = "Won't Do"
	ResolutionDuplicate = "Duplicate"

	FieldStatus     = "status"
	FieldResolution = "resolution"
	FieldType       = "type"
	FieldSize       = "size"
	FieldTeam       = "team"
	FieldFlagged    = "flagged"

	FlagImpediment = "Impediment"

	EventCreated     = "created"
	EventFieldChange = "field_change"
)

// UnknownStatus has no mapping rule on purpose: it gives neither start nor completion (FR-017 A3).
const UnknownStatus = StatusLegacyQA

type TaskFile struct {
	SchemaVersion string           `json:"schema_version"`
	Seed          uint64           `json:"seed"`
	Period        Window           `json:"period"`
	AsOf          string           `json:"as_of"`
	Projects      []TaskProject    `json:"projects"`
	Tasks         []Task           `json:"tasks"`
	Variants      []VariantObjects `json:"variants"`
}

type TaskProject struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	InstanceID     string       `json:"instance_id"`
	Key            string       `json:"key"`
	Name           string       `json:"name"`
	Restricted     bool         `json:"restricted"`
	Statuses       []TaskStatus `json:"statuses"`
	Types          []string     `json:"types"`
	Sizes          []string     `json:"sizes"`
	Teams          []string     `json:"teams"`
	Resolutions    []string     `json:"resolutions"`
}

type TaskStatus struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

type Task struct {
	ID             string      `json:"id"`
	OrganizationID string      `json:"organization_id"`
	InstanceID     string      `json:"instance_id"`
	ProjectID      string      `json:"project_id"`
	Key            string      `json:"key"`
	Summary        string      `json:"summary"`
	Events         []TaskEvent `json:"events"`
}

// TaskFields are the values a task is created with; an empty Size means the task has no size.
type TaskFields struct {
	Status string `json:"status"`
	Type   string `json:"type"`
	Size   string `json:"size"`
	Team   string `json:"team"`
}

// TaskEvent is one history entry. A created event carries Initial and no field; a field change carries
// Field, From and To. ObservedTime is set only when the source delivers the event later than it happened.
type TaskEvent struct {
	ID           string      `json:"id"`
	Kind         string      `json:"kind"`
	EventTime    SourceTime  `json:"event_time"`
	ObservedTime *SourceTime `json:"observed_time"`
	AuthorID     string      `json:"author_id"`
	Field        string      `json:"field"`
	From         string      `json:"from"`
	To           string      `json:"to"`
	Initial      *TaskFields `json:"initial"`
}

// Objects is the per-kind object file that the manifest command reads.
func (f *TaskFile) Objects() ObjectFile {
	objects := make([]ModelObject, len(f.Tasks))
	for i, task := range f.Tasks {
		objects[i] = ModelObject{ID: task.ID, OrganizationID: task.OrganizationID, Kind: "task"}
	}
	return ObjectFile{SchemaVersion: "1", Objects: objects, Variants: f.Variants}
}

func workflow() []TaskStatus {
	return []TaskStatus{
		{Name: StatusBacklog, Category: "new"},
		{Name: StatusTriage, Category: "indeterminate"},
		{Name: StatusInProgress, Category: "indeterminate"},
		{Name: StatusInReview, Category: "indeterminate"},
		{Name: StatusWaiting, Category: "indeterminate"},
		{Name: StatusBlocked, Category: "indeterminate"},
		{Name: StatusLegacyQA, Category: "indeterminate"},
		{Name: StatusDone, Category: "done"},
		{Name: StatusCancelled, Category: "done"},
		{Name: StatusDuplicate, Category: "done"},
	}
}

func taskProjects() []TaskProject {
	project := func(id, org, instance, key, name string, restricted bool) TaskProject {
		return TaskProject{
			ID: id, OrganizationID: org, InstanceID: instance, Key: key, Name: name, Restricted: restricted,
			Statuses: workflow(), Types: []string{"Story", "Bug", "Task"}, Sizes: []string{"S", "M", "L"},
			Teams: []string{"Platform", "Payments"}, Resolutions: []string{ResolutionDone, ResolutionWontDo, ResolutionDuplicate},
		}
	}
	return []TaskProject{
		project("jira-project-a", "org-1", "jira-dc-1", "PA", "Demo project A", false),
		project("jira-project-b", "org-1", "jira-dc-1", "PB", "Demo project B", false),
		project("import-project", "org-1", "universal-1", "IMP", "Demo import project", false),
		project("private-project", "org-2", "jira-dc-1", "SEC", "Other organization project", true),
	}
}
