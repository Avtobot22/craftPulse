package entity

import (
	"strings"
	"time"
)

const (
	ExceptionFalsePositive = "false_positive"
	ExceptionAcceptedRisk  = "accepted_risk"
	ExceptionNotApplicable = "not_applicable"

	ExceptionPending   = "pending"
	ExceptionApproved  = "approved"
	ExceptionRejected  = "rejected"
	ExceptionWithdrawn = "withdrawn"
	ExceptionActive    = "active"
	ExceptionExpired   = "expired"

	ExceptionApprove  = "approve"
	ExceptionReject   = "reject"
	ExceptionWithdraw = "withdraw"
)

// FindingAccess identifies the source area and visibility class of a finding without its contents.
type FindingAccess struct {
	AreaID    ID
	Kind      string
	WholeArea bool
}

// SensitiveFinding is the M0 visibility class for findings that require security_access.
func SensitiveFinding(kind string) bool {
	switch kind {
	case "security", "secret", "dependency", "iac":
		return true
	}
	return false
}

// ExceptionScope narrows an exception to a finding or rule and optional coordinates within one source area.
type ExceptionScope struct {
	AreaID      ID
	ComponentID ID
	FindingID   ID
	RuleID      string
	PathPrefix  string
	PackagePURL string
}

// ExceptionDraft describes one immutable version of an exception.
type ExceptionDraft struct {
	Type          string
	Reason        string
	Scope         ExceptionScope
	EffectiveFrom time.Time
	ExpiresAt     time.Time
}

// ExceptionVersion is a confirmed write of a draft; Status is projected from its transitions.
type ExceptionVersion struct {
	ExceptionDraft
	ID, DecisionID, AuthorID, Key ID
	Number, Base                  int
	Seq                           int64
	FindingKind                   string
	Status                        string
	ApprovalRequired              bool
	CreatedAt                     time.Time
}

// Access describes the rights needed to read the version or to apply it over its scope.
func (v *ExceptionVersion) Access(write bool) FindingAccess {
	return FindingAccess{AreaID: v.Scope.AreaID, Kind: v.FindingKind, WholeArea: write && v.Scope.ComponentID.IsZero()}
}

// ExceptionTransition records one change of status with its knowledge and event times.
type ExceptionTransition struct {
	VersionID, ActorID, Key ID
	Version                 int
	Seq                     int64
	Status                  string
	Comment                 string
	At                      time.Time
}

// ExceptionRecord holds the latest timer and the immutable history of a decision.
type ExceptionRecord struct {
	ID          ID
	CreatedAt   time.Time
	WakeAt      *time.Time
	Versions    []ExceptionVersion
	Transitions []ExceptionTransition
}

// ExceptionView is the current display version, with active state determined by the entire visible history.
type ExceptionView struct {
	ID          ID
	State       string
	Current     ExceptionVersion
	Versions    []ExceptionVersion
	Transitions []ExceptionTransition
}

// ExceptionWrite is a request to store a draft already resolved against change's canonical findings.
type ExceptionWrite struct {
	ID          ID
	Key         ID
	Base        int
	FindingKind string
	Draft       ExceptionDraft
}

// ExceptionDecision is a request for an audited transition of the latest version.
type ExceptionDecision struct {
	ID, Key ID
	Base    int
	Action  string
	Comment string
}

// ExceptionCursor keeps the sort boundary and the reproducible cut of a page of decisions.
type ExceptionCursor struct {
	At, SortAt time.Time
	Seq        int64
	ID         ID
}

// ExceptionQuery selects the latest visible versions at one cut.
type ExceptionQuery struct {
	State, Sort string
	ComponentID ID
	FindingID   ID
	Limit       int
	After       *ExceptionCursor
}

// ExceptionPage contains only visible decisions and marks whether access omitted some.
type ExceptionPage struct {
	Items []ExceptionView
	Next  *ExceptionCursor
	Notes []AccessNote
}

// FindingCoordinates are the confirmed occurrence coordinates matched by an exception.
type FindingCoordinates struct {
	FindingAccess
	ComponentID ID
	FindingID   ID
	RuleID      string
	Path        string
	PackagePURL string
}

// Matches checks the version's scope against an occurrence without classifying the finding as fixed.
func (v *ExceptionVersion) Matches(f FindingCoordinates) bool {
	s := v.Scope
	return s.AreaID == f.AreaID && v.FindingKind == f.Kind &&
		(s.ComponentID.IsZero() || s.ComponentID == f.ComponentID) &&
		(s.FindingID.IsZero() || s.FindingID == f.FindingID) &&
		(s.RuleID == "" || s.RuleID == f.RuleID) &&
		(s.PathPrefix == "" || UnderFindingPath(f.Path, s.PathPrefix)) &&
		(s.PackagePURL == "" || s.PackagePURL == f.PackagePURL)
}

// UnderFindingPath matches a path prefix on a segment boundary.
func UnderFindingPath(file, prefix string) bool {
	return file == prefix || strings.HasPrefix(file, prefix+"/")
}
