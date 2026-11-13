package entity

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrVersionConflict = errors.New("org: structure version has changed")
	ErrIntervalOverlap = errors.New("org: intervals of one pair overlap")
)

// Limits of one structure in the contract (OrganizationStructureInput).
const (
	MaxTeams    = 500
	MaxProjects = 2000
	MaxServices = 2000
	MaxLinks    = 5000
	MaxAreaName = 200
)

type LinkKind string

const (
	LinkTeamProject LinkKind = "team_project"
	LinkTeamMember  LinkKind = "team_member"
)

// Link is a current interval [ValidFrom, ValidTo) of a pair; a zero ValidTo is open.
type Link struct {
	RowID     ID
	From      ID
	To        ID
	ValidFrom time.Time
	ValidTo   time.Time
}

type Structure struct {
	Version      int64
	Teams        []Area
	Projects     []Area
	Services     []Area
	TeamProjects []Link
	TeamMembers  []Link
}

// AreaInput is a team, project or service of a confirmed structure; a zero ID is a new record.
type AreaInput struct {
	ID     ID
	Name   string
	Status string
}

// LinkInput carries the dates the client sent: on M0 both are nil and the interval starts on saving.
type LinkInput struct {
	From      ID
	To        ID
	ValidFrom *time.Time
	ValidTo   *time.Time
}

type StructureInput struct {
	Teams        []AreaInput
	Projects     []AreaInput
	Services     []AreaInput
	TeamProjects []LinkInput
	TeamMembers  []LinkInput
	// ProjectAreas and Components count the entries that reference source areas.
	ProjectAreas int
	Components   int
}

// Field error codes of the contract (FieldError).
const (
	FieldRequired         = "required"
	FieldOutOfRange       = "out_of_range"
	FieldTooLong          = "too_long"
	FieldUnknownField     = "unknown_field"
	FieldNotAllowedValue  = "not_allowed_value"
	FieldDuplicate        = "duplicate"
	FieldUnknownReference = "unknown_reference"
	FieldOutsideScope     = "outside_scope"
	FieldInvalidFormat    = "invalid_format"
)

type FieldError struct {
	Field  string
	Code   string
	Detail string
}

// ValidationError lists every field error of one request (contract ValidationFailed).
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Code
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

func (e *ValidationError) Add(field, code, detail string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Code: code, Detail: detail})
}

// Scope is the set of areas the subject may read; only access builds it (TZ 13.4).
type Scope struct {
	Whole    bool
	Teams    []ID
	Projects []ID
	Services []ID
}

type PageKey struct {
	Name string
	ID   ID
}

type PageRequest struct {
	Limit int
	Desc  bool
	After *PageKey
}

type Page struct {
	Items []Area
	Next  *PageKey
}

// AreaFilter narrows a list; TeamID keeps the projects the team works on now.
type AreaFilter struct {
	TeamID ID
}
