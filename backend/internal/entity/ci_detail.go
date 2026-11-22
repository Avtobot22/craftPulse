package entity

import (
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("invalid or expired list cursor")

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
	Reports []PipelineReport
	Run     CIRun
	Summary RunSummary
	Metrics TestCoverageRevision
}

type AttemptQuery struct {
	IssuedAt *time.Time
	Limit    int
	Outcome  CIOutcome
	After    *ID
}

type AttemptList struct {
	IssuedAt     time.Time
	Items        []CIAttemptVersion
	Next         *ID
	UsageSeconds Point
}
