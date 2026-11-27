package entity

const (
	CodeFormatLizard = "lizard_csv"
	CodeFormatJscpd  = "jscpd_json"
	MetricSetCode    = "code"

	ObjectCodeFile     ObjectKind = "code_file"
	ObjectCodeFunction ObjectKind = "code_function"
	ObjectCloneGroup   ObjectKind = "clone_group"
	ObjectCodeReport   ObjectKind = "code_report"
)

type CodeThresholds struct {
	CCN  int `json:"ccn_threshold"`
	NLOC int `json:"nloc_threshold"`
}

type CodeRange struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type CodeFunction struct {
	CodeRange
	Name string `json:"name"`
	CCN  int    `json:"ccn"`
	NLOC int    `json:"nloc"`
}

type CodeFile struct {
	Path string `json:"path"`
	NLOC int    `json:"nloc"`
	Test bool   `json:"test"`
}

type CloneGroup struct {
	Language string      `json:"language"`
	Members  []CodeRange `json:"members"`
}

// CodeFacts are normalized observations from one report. Nil inventory and denominator are unknown.
type CodeFacts struct {
	Functions       []CodeFunction  `json:"functions,omitempty"`
	Files           *[]CodeFile     `json:"files,omitempty"`
	Groups          []CloneGroup    `json:"groups,omitempty"`
	PhysicalLines   *int64          `json:"physical_lines,omitempty"`
	Thresholds      *CodeThresholds `json:"thresholds,omitempty"`
	ExcludedTargets []SkippedTarget `json:"excluded_targets,omitempty"`
}

// CodeScope identifies one report stream at a commit. A format determines which metrics that stream supplies.
type CodeScope struct {
	RepositoryID ID               `json:"repository_id"`
	ComponentID  ID               `json:"component_id"`
	SHA          string           `json:"sha"`
	Format       string           `json:"format"`
	Producer     string           `json:"producer"`
	ProfileID    string           `json:"profile_id"`
	Source       CodeReportSource `json:"source,omitzero"`
}

type CodeReport struct {
	ID           ID
	Completeness string
	SkippedCount int
	Skipped      []SkippedTarget
	Facts        CodeFacts
}

type ComponentCodeRevision struct {
	ID, ReportID                ID
	Format, Producer, ProfileID string
	Window                      Window
}

type CodeReportSource struct {
	Mode     string `json:"mode"`
	Language string `json:"language"`
}
