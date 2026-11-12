package entity

import (
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("access: object or area is not visible to the subject")
	ErrForbidden = errors.New("access: the role of the subject does not grant the action")
)

type Role string

const (
	RoleAdministrator  Role = "administrator"
	RoleDevLead        Role = "dev_lead"
	RoleProjectManager Role = "project_manager"
	RoleTechLead       Role = "tech_lead"
	RoleDeveloper      Role = "developer"
	RoleProcessOwner   Role = "process_owner"
	RoleServiceOwner   Role = "service_owner"
	RoleSecurityAccess Role = "security_access"
	RoleWikiAccess     Role = "wiki_access"
)

// RoleAssignment has at most one of the area IDs; all zero means the whole organization.
type RoleAssignment struct {
	ID         ID
	Role       Role
	TeamID     ID
	ProjectID  ID
	ServiceID  ID
	AssignedAt time.Time
}

type AreaKind string

const (
	AreaTeam    AreaKind = "team"
	AreaProject AreaKind = "project"
	AreaService AreaKind = "service"
)

// RoleSet is the active roles of a user; Version grows with every assignment and revocation.
type RoleSet struct {
	Version int64
	Roles   []RoleAssignment
}

// MaxRoles is the limit of one role set in the contract (RoleAssignmentListInput).
const MaxRoles = 200

// AreaRef is the area a request addresses; the zero value is the organization itself.
type AreaRef struct {
	Kind AreaKind
	ID   ID
}

type Area struct {
	ID     ID
	Name   string
	Status string
}

type Areas struct {
	Teams    []Area
	Projects []Area
	Services []Area
}

// Profile is what the subject may see, read on every request (TZ 7.2): roles and areas from roles and teams.
// Whole marks a role over the whole organization, which sees every area.
type Profile struct {
	User         User
	Organization Organization
	Roles        []RoleAssignment
	Areas        Areas
	Whole        bool
}

type AccessRequest struct {
	Operation string
	Action    string
	Area      AreaRef
}

type CurrentUser struct {
	Profile Profile
	Slug    string
	Actions []string
	Session Subject
}
