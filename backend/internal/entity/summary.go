package entity

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"time"
)

// ChangeSummaryJob coalesces changes to the facts of one MR.
type ChangeSummaryJob struct {
	OrganizationID ID `json:"organization_id" river:"unique"`
	MRID           ID `json:"mr_id" river:"unique"`
}

func (ChangeSummaryJob) Kind() string       { return "change.summary" }
func (ChangeSummaryJob) UniqueWhileActive() {}

// OutboundPublishJob coalesces publication of ready revisions for one MR.
type OutboundPublishJob struct {
	OrganizationID    ID `json:"organization_id" river:"unique"`
	MRID              ID `json:"mr_id" river:"unique"`
	SummaryRevisionID ID `json:"summary_revision_id"`
}

func (OutboundPublishJob) Kind() string       { return "outbound.publish" }
func (OutboundPublishJob) UniqueWhileActive() {}

// SummaryMR is the normalized source identity and knowledge cut of an MR.
type SummaryMR struct {
	MRVersion
	Source                 ObjectKey
	InstanceID             ID
	FactsSeq, KnowledgeSeq int64
}

// SummaryInput identifies evidence used to build a revision.
type SummaryInput struct {
	Kind string
	ID   ID
}

// SummaryItem is one visible row whose required right also governs its value.
type SummaryItem struct {
	Kind          string
	FindingID     *ID
	OccurrenceID  *ID
	RequiredRight string
	Rank          *int
	ComponentID   *ID
	AnalysisKind  string
	ReasonCode    string
	Severity      string
	Value         json.RawMessage
}

// SummaryCodePair identifies the exact streams used by the MR structure definitions.
type SummaryCodePair struct {
	ComponentID ID
	Head, Base  *TechnicalReport
	Comparable  bool
}

// SummaryRevision is the persisted lifecycle and content of one MR summary.
type SummaryRevision struct {
	ExpectedReports       []ExpectedReport
	CompleteReports       []SummaryReportEvidence
	ID                    ID
	MRID                  ID
	No                    int
	HeadSHA, BaseSHA      string
	ProfileVersionID      ID
	Status, PipelineState string
	InputHash             []byte
	KnowledgeSeq          int64
	BuiltAt               time.Time
	ReportsReadyAt        time.Time
	Publication           Publication
	StaleReason           string
	Inputs                []SummaryInput
	Items                 []SummaryItem
	CodePairs             []SummaryCodePair
	FindingValues         []SummaryFindingValues
	Definitions           []RevisionInput
}

// SummaryFindingComparison is the input to the finding definitions for one stream.
type SummaryFindingComparison struct {
	AreaID, ComponentID ID
	AnalysisKind        string
	Head, Base          *TechnicalReport
	ReasonCode          string
	Findings            []SummaryFinding
	AsOf                time.Time
}

// SummaryFinding carries one classified occurrence into the finding definitions.
type SummaryFinding struct {
	FindingCoordinates
	OccurrenceID, ReportID ID
	Lifecycle, Severity    string
	LineStart              *int
}

// SummaryFindingValues contains scalar outputs scoped by analysis and right.
type SummaryFindingValues struct {
	ComponentID                 ID
	AnalysisKind, RequiredRight string
	Points                      []SeriesPoint
}

// SummaryReportEvidence identifies the selected complete report for one expectation.
type SummaryReportEvidence struct {
	ExpectedReport
	ReportID ID
}

func severityOrder(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	case "info":
		return 4
	default:
		return 5
	}
}

// CompareSummarySignals orders findings by severity and stable identity.
func CompareSummarySignals(a, b *SummaryItem) int {
	return cmp.Or(cmp.Compare(severityOrder(a.Severity), severityOrder(b.Severity)),
		cmp.Compare(b.RequiredRight, a.RequiredRight), cmp.Compare(a.AnalysisKind, b.AnalysisKind),
		bytes.Compare(a.FindingID[:], b.FindingID[:]), bytes.Compare(a.OccurrenceID[:], b.OccurrenceID[:]))
}

// RankSummary assigns the top five ranks for the supplied audience.
func RankSummary(items []SummaryItem) {
	var candidates []int
	for i := range items {
		items[i].Rank = nil
		if items[i].Kind == FindingNew && items[i].FindingID != nil && items[i].OccurrenceID != nil {
			candidates = append(candidates, i)
		}
	}
	slices.SortStableFunc(candidates, func(a, b int) int { return CompareSummarySignals(&items[a], &items[b]) })
	for rank, index := range candidates[:min(5, len(candidates))] {
		items[index].Rank = new(rank + 1)
	}
}
