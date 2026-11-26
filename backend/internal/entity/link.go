package entity

import (
	"encoding/json"
	"fmt"
	"time"
)

type LinkType string

const (
	LinkTaskMR             LinkType = "task_mr"
	LinkMRRun              LinkType = "mr_run"
	LinkRunReport          LinkType = "run_report"
	LinkTaskParent         LinkType = "task_parent"
	LinkFindingTask        LinkType = "finding_task"
	LinkDocumentService    LinkType = "document_service"
	LinkTaskTask           LinkType = "task_task"
	LinkDeploymentIncident LinkType = "deployment_incident"
	LinkIncidentDeployment LinkType = "incident_deployment"
)

type LinkMethod string

const (
	LinkSourceExplicit LinkMethod = "source_explicit"
	LinkUserConfirmed  LinkMethod = "user_confirmed"
	LinkBranchKey      LinkMethod = "branch_key"
	LinkTitleKey       LinkMethod = "title_key"
	LinkTimeWindow     LinkMethod = "time_window"
	LinkPathMatch      LinkMethod = "path_match"
)

// Proposed tells whether the method only proposes a candidate (BR-010).
func (m LinkMethod) Proposed() bool {
	switch m {
	case LinkBranchKey, LinkTitleKey, LinkTimeWindow, LinkPathMatch:
		return true
	case LinkSourceExplicit, LinkUserConfirmed:
		return false
	}
	return false
}

type LinkStatus string

const (
	LinkCandidate LinkStatus = "candidate"
	LinkConfirmed LinkStatus = "confirmed"
	LinkRejected  LinkStatus = "rejected"
)

type LinkDecision string

const (
	LinkConfirm LinkDecision = "confirm"
	LinkReject  LinkDecision = "reject"
)

// ErrLinkConfirmedBySource refuses to reject a link the source or a user has already confirmed.
var ErrLinkConfirmedBySource = fmt.Errorf("link: a confirmed link cannot be rejected: %w", ErrStateConflict)

// LinkDirection reads the links of an object as their start or as their end.
type LinkDirection int

const (
	LinksFrom LinkDirection = iota + 1
	LinksTo
)

// LinkEdge is a version of a link between two objects of normalize with its method and status (TZ 12.5.6).
type LinkEdge struct {
	ID     ID
	Type   LinkType
	From   ID
	To     ID
	Method LinkMethod
	Status LinkStatus
	Knowledge
	Record         *RecordRef
	Evidence       json.RawMessage
	ActorUserID    *ID
	ConfirmationID *ID
	DecidedAt      *time.Time
	Validity       *LinkValidity
}

type LinkValidity struct {
	From, To                          *time.Time
	FromExclusive, ToInclusive, Empty bool
}

func (v *LinkValidity) Contains(at time.Time) bool {
	return v == nil || !v.Empty &&
		(v.From == nil || at.After(*v.From) || !v.FromExclusive && at.Equal(*v.From)) &&
		(v.To == nil || at.Before(*v.To) || v.ToInclusive && at.Equal(*v.To))
}

// SameEnds tells whether both links connect the same objects with the same type.
func (e *LinkEdge) SameEnds(o *LinkEdge) bool {
	return e.Type == o.Type && e.From == o.From && e.To == o.To
}

// LinkProposal is an explicit link of a source or a candidate found by a method of normalize.
type LinkProposal struct {
	Type     LinkType
	From     ID
	To       ID
	Method   LinkMethod
	Record   *RecordRef
	Evidence json.RawMessage
}

// LinkEnd is an object at an end of a link, with what access decides it by and what a reference shows.
type LinkEnd struct {
	ID         ID
	Kind       ObjectKind
	InstanceID ID
	AreaID     ID
	Restricted bool
	NativeNS   string
	NativeID   string
	DisplayKey *string
	Title      *string
}

// LinkResult is a link after a decision with its end for the answer.
type LinkResult struct {
	Edge LinkEdge
	To   LinkEnd
}

type LinkMRFilter struct {
	Areas    []ID
	MRIDs    []ID
	TaskKeys []string
}
