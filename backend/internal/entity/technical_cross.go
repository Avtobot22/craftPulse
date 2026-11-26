package entity

import (
	"context"
	"time"
)

// TechnicalSummary is the immutable analysis evidence captured with an MR summary.
type TechnicalSummary struct {
	EffectiveAt time.Time         `json:"effective_at,omitzero"`
	Streams     []TechnicalStream `json:"streams"`
}

// TechnicalStream describes one expected report and its head/base comparison.
type TechnicalStream struct {
	ExpectedReport
	Complete         bool             `json:"complete"`
	DocumentationKey []byte           `json:"documentation_key,omitempty"`
	Comparable       bool             `json:"comparable"`
	Findings         []SummaryFinding `json:"findings,omitempty"`
}

type TechnicalSummaryRevision struct {
	MRID, ProjectID     ID
	HeadSHA             string
	BuiltAt             time.Time
	PipelineID          ID
	MissingChecks       bool
	PublicMissingChecks bool
	TechnicalSummary
}

// AppliesAt tests the effective date of the evidence, independently of when the summary was built.
func (r *TechnicalSummaryRevision) AppliesAt(at time.Time) bool {
	if r.EffectiveAt.IsZero() {
		return !r.BuiltAt.After(at)
	}
	return !r.EffectiveAt.After(at)
}

// TechnicalHead identifies an MR head at one report point.
type TechnicalHead struct {
	MRVersion
	PipelineID      ID
	PipelineStarted *time.Time
}

// TechnicalScope carries only aggregates; security data stays in its own scopes until read access is applied.
type TechnicalScope struct {
	Kind       string `json:"kind"`
	Expected   int    `json:"expected"`
	Complete   int    `json:"complete"`
	Comparable int    `json:"comparable"`
	Current    int    `json:"current,omitempty"`
	New        int    `json:"new,omitempty"`
	Persisting int    `json:"persisting,omitempty"`
	Resolved   int    `json:"resolved,omitempty"`
	Accepted   int    `json:"accepted,omitempty"`
}

type TechnicalCross struct {
	ProjectID           ID               `json:"project_id"`
	Scopes              []TechnicalScope `json:"scopes,omitempty"`
	MissingChecks       bool             `json:"missing_checks,omitempty"`
	PublicMissingChecks bool             `json:"public_missing_checks,omitempty"`
}

// TechnicalCrossSnapshot evaluates evidence at the date of a point on a fixed knowledge cut.
type TechnicalCrossSnapshot func(context.Context, time.Time, []TechnicalHead) (map[ID]TechnicalCross, error)
