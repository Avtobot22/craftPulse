package entity

import (
	"errors"
	"time"
)

// ErrIdempotencyKeyReused is a key of a confirmation sent again with another request (TZ 5.2).
var ErrIdempotencyKeyReused = errors.New("org: idempotency key was used with another request")

// MaxAssignmentBatch bounds the decisions of one confirmation (UI-15).
const MaxAssignmentBatch = 200

// Person is a human, a bot or a service participant accounts of sources are linked to.
type Person struct {
	ID          ID
	DisplayName string
	Kind        PersonKind
}

type PersonFilter struct {
	Kind  PersonKind
	Limit int
	After *ID
}

type PersonPage struct {
	Items []Person
	Next  *ID
}

// AccountAssignment is a confirmed link of an account to a person and the kind of the participant; a change
// is a new version of knowledge (FR-018).
type AccountAssignment struct {
	ID              ID
	SourceAccountID ID
	PersonID        *ID
	ActorKind       PersonKind
	Knowledge
	ConfirmedBy    ID
	ConfirmedAt    time.Time
	ConfirmationID ID
}

// Bases of a proposal of a link; a matching display name makes none.
const (
	ProposalLoginMatch = "login_match"
	ProposalEmailMatch = "email_match"
)

type AccountProposal struct {
	PersonID ID
	Basis    string
}

// AccountView is an account of a source with its current link and the proposals for it.
type AccountView struct {
	ID         ID
	InstanceID ID
	Account    SourceAccount
	Assignment *AccountAssignment
	Proposals  []AccountProposal
}

type AccountAssignedFilter string

const (
	AccountsAssigned   AccountAssignedFilter = "assigned"
	AccountsUnassigned AccountAssignedFilter = "unassigned"
)

type SourceAccountFilter struct {
	Assigned   AccountAssignedFilter
	InstanceID *ID
	Limit      int
	After      *ID
}

type SourceAccountPage struct {
	Items []AccountView
	Next  *ID
}

// AssignmentRequest is one decision of an administrator; BaseAssignmentID is the link the form showed.
type AssignmentRequest struct {
	SourceAccountID  ID
	PersonID         *ID
	CreatePerson     bool
	ActorKind        PersonKind
	BaseAssignmentID *ID
}

// Outcomes of a decision; each one of a batch has its own.
const (
	AssignmentSaved           = "saved"
	AssignmentUnchanged       = "unchanged"
	AssignmentVersionConflict = "version_conflict"
	AssignmentRejected        = "rejected"
)

// Reasons of a rejected decision.
const (
	AssignmentAccountNotFound = "not_found"
	AssignmentPersonRequired  = "person_required"
	AssignmentPersonNotFound  = "person_not_found"
)

type AssignmentOutcome struct {
	SourceAccountID ID
	Outcome         string
	AssignmentID    *ID
	ErrorCode       string
}

// UserAccount is the account of a user of craftPulse in an instance of a source, by a confirmed link.
type UserAccount struct {
	SourceAccountID ID
	NativeID        string
}
