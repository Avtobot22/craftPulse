package entity

import "regexp"

var _gitSHA = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

func ValidGitSHA(value string) bool { return _gitSHA.MatchString(value) }

const (
	FindingNew           = "new"
	FindingUnchanged     = "unchanged"
	FindingResolved      = "resolved"
	FindingNewlyObserved = "newly_observed"
	FindingOpen          = "open"
	FindingUncertain     = "uncertain"
)

type TechnicalComparisonQuery struct {
	ComponentID ID
	BaseSHA     string
	HeadSHA     string
	Kind        string
}

type TechnicalReportScope struct {
	AreaID      ID
	ComponentID ID
	Kind        string
	SHAs        []string
	ReportIDs   []ID
	Latest      bool
	Language    *string
	ProfileID   *string
}

type TechnicalReport struct {
	ID                ID
	SHA               string
	Language          string
	ProfileID         string
	Producer          string
	ProducerVersion   string
	Mode              string
	RulesHash         []byte
	ConfigHash        []byte
	ScopeHash         []byte
	DBVersion         string
	Completeness      string
	ExitClass         string
	Interrupted       bool
	Rejected          bool
	DependencyDelta   string
	DependencyBaseSHA string
	Occurrences       []ReportOccurrence
}

func (r *TechnicalReport) Complete() bool {
	return r != nil && r.Completeness == "complete" && !r.Interrupted && !r.Rejected &&
		(r.ExitClass == "success" || r.ExitClass == "findings")
}

type TechnicalRevision struct {
	SHA      string
	ReportID *ID
}

type TechnicalFinding struct {
	ID        ID
	Kind      string
	Lifecycle string
}

type TechnicalFactor struct {
	Factor        string
	NewCount      int
	ResolvedCount int
	FindingIDs    []ID
}

type TechnicalComparison struct {
	Comparable          bool
	IncomparableReasons []string
	Base                TechnicalRevision
	Head                TechnicalRevision
	Factors             []TechnicalFactor
	Findings            []TechnicalFinding
	Notes               []AccessNote
}
