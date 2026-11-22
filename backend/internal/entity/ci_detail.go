package entity

import (
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("invalid or expired list cursor")

type MRDetail struct {
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

// PipelineReport summarizes normalized reports of one analysis kind.
type PipelineReport struct {
	Kind                            string
	Partial                         bool
	Complete                        bool
	Passed, Failed, Errors, Skipped int64
}

type PipelineDetail struct {
	Reports []PipelineReport
	Run     CIRun
	Summary RunSummary
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
