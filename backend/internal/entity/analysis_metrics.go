package entity

import "time"

const (
	MetricSetAnalysis            = "analysis"
	ObjectAnalysisRun ObjectKind = "analysis_run"
	InputAnalysisRun             = "analysis_run"
)

type AnalysisScope struct {
	RepositoryID ID     `json:"repository_id"`
	ComponentID  ID     `json:"component_id"`
	SHA          string `json:"sha"`
}

type AnalysisRun struct {
	ID        ID
	ReceiptID ID
	KnownSeq  int64
	AnalysisScope
	ToolID    string
	RecordNos []int
	AnalysisRunMetadata
	Reports []AnalysisReport
}

type AnalysisRunMetadata struct {
	Kind          string       `json:"kind"`
	Producer      string       `json:"producer"`
	Profile       string       `json:"profile,omitempty"`
	Mode          string       `json:"mode,omitempty"`
	Language      string       `json:"language,omitempty"`
	ScopeHash     string       `json:"scope_hash,omitempty"`
	PathPrefix    string       `json:"path_prefix,omitempty"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	FinishedAt    *time.Time   `json:"finished_at,omitempty"`
	QueueSeconds  *float64     `json:"queue_seconds,omitempty"`
	ExitClass     string       `json:"exit_class"`
	Completeness  string       `json:"completeness"`
	ScannedFiles  *int         `json:"scanned_files,omitempty"`
	EligibleFiles *int         `json:"eligible_files,omitempty"`
	History       *ScanHistory `json:"history,omitempty"`
	DBVersion     string       `json:"db_version,omitempty"`
	DBPublished   *time.Time   `json:"db_published_at,omitempty"`
}

type DependencyInventory struct {
	Entries  int  `json:"entries"`
	Resolved int  `json:"resolved"`
	Partial  bool `json:"partial,omitempty"`
}

type AnalysisReport struct {
	ID           ID                   `json:"id"`
	Reason       string               `json:"reason,omitempty"`
	Dependencies *DependencyInventory `json:"dependencies,omitempty"`
}

type AnalysisDocumentVersion struct {
	ID         ID        `json:"id"`
	ReportID   ID        `json:"report_id"`
	Path       string    `json:"path"`
	VersionRef string    `json:"version_ref"`
	ObservedAt time.Time `json:"observed_at"`
}

type AnalysisFacts struct {
	Runs     []AnalysisRun
	Versions []AnalysisDocumentVersion
}
