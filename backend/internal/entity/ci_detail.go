package entity

import (
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("invalid or expired list cursor")

// GitLabCISource locates a run or job from a selected CI version.
type GitLabCISource struct {
	ObjectID       ID
	Kind           ObjectKind
	NativeID       string
	InstanceURL    string
	RepositoryURL  string
	RepositoryPath string
	ManifestPath   string
}

type MRDetail struct {
	ReviewWait *ReviewWait
	Version    MRVersion
	Repository RepositoryVersion
	AuthorKind PersonKind
	Links      []LinkResult
	Notes      []AccessNote
	SourceURL  *string
}

type RunSummary struct {
	AttemptsTotal   int
	RetriedJobs     int
	AllowedFailures int
	UsageSeconds    Point
}

const (
	PipelineReportReceived  = "received"
	PipelineReportMissing   = "missing"
	PipelineReportMalformed = "malformed"
	PipelineReportPartial   = "partial"
)

type PipelineReport struct {
	Kind  string
	State string
}

type PipelineDetail struct {
	SourceURL *string
	Reports   []PipelineReport
	Run       CIRun
	Summary   RunSummary
	Metrics   TestCoverageRevision
}

type AttemptQuery struct {
	IssuedAt *time.Time
	Limit    int
	Outcome  CIOutcome
	After    *ID
}

type AttemptList struct {
	SourceURLs   map[ID]string // keyed by attempt ID
	IssuedAt     time.Time
	Items        []CIAttemptVersion
	Next         *ID
	UsageSeconds Point
}
