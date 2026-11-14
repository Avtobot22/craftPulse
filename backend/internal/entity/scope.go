package entity

import (
	"slices"
	"time"
)

// Scope is what a subject may read (TZ 13.4): the areas of its roles and, after a check of the sources, the
// source areas and the objects with their own restriction the sources allow. Only access builds it with
// GrantScope, which forbidigo refuses elsewhere; the zero value allows nothing.
type Scope struct {
	whole    bool
	teams    []ID
	projects []ID
	services []ID
	areas    []ID
	objects  []ID
	notes    []AccessNote
}

// ScopeGrant is the decision access turns into a scope.
type ScopeGrant struct {
	Whole    bool
	Teams    []ID
	Projects []ID
	Services []ID
	// Areas are the source areas the sources allow the user now.
	Areas []ID
	// Objects are the objects with their own restriction the sources allow one by one.
	Objects []ID
	Notes   []AccessNote
}

func GrantScope(g ScopeGrant) Scope {
	return Scope{
		whole: g.Whole, teams: slices.Clone(g.Teams), projects: slices.Clone(g.Projects), services: slices.Clone(g.Services),
		areas: slices.Clone(g.Areas), objects: slices.Clone(g.Objects), notes: slices.Clone(g.Notes),
	}
}

// Whole marks a role over the whole organization; the area lists still hold every area of it.
func (s Scope) Whole() bool { return s.whole }

func (s Scope) Teams() []ID { return slices.Clone(s.teams) }

func (s Scope) Projects() []ID { return slices.Clone(s.projects) }

func (s Scope) Services() []ID { return slices.Clone(s.services) }

// Area reports whether the sources allow the source area.
func (s Scope) Area(id ID) bool { return slices.Contains(s.areas, id) }

func (s Scope) Areas() []ID { return slices.Clone(s.areas) }

// Objects are the objects with their own restriction that may be shown and counted.
func (s Scope) Objects() []ID { return slices.Clone(s.objects) }

// Limited marks an answer some objects of which are hidden; it never tells how many (FR-001).
func (s Scope) Limited() bool { return len(s.notes) > 0 }

func (s Scope) Notes() []AccessNote { return slices.Clone(s.notes) }

type AccessNoteCode string

const (
	NoteRestricted       AccessNoteCode = "restricted_by_access"
	NoteCheckUnavailable AccessNoteCode = "access_check_unavailable"
	NoteAccessLost       AccessNoteCode = "source_access_lost"
)

// AccessNote marks a restricted answer without the number and names of hidden objects (FR-001).
type AccessNote struct {
	Code         AccessNoteCode
	ConnectionID ID
}

// SourceQuery is what a read needs the sources to allow beyond the areas of the roles.
type SourceQuery struct {
	// Operation names the read in the audit of a refusal.
	Operation string
	// Projects keeps the projects of the scope that are open in at least one of their sources (TZ 7.7).
	Projects bool
	// Areas are the source areas the read takes data from.
	Areas []ID
	// Objects are the objects with their own restriction among the data of the areas (TZ 13.5).
	Objects []RestrictedObject
}

// RestrictedObject is an object with its own restriction in the source, such as a task with a security level.
type RestrictedObject struct {
	ID       ID
	AreaID   ID
	Kind     ObjectKind
	NativeNS string
	NativeID string
}

// AreaSource is a scope of a connection that collected a source area; an area without one came only from
// an import.
type AreaSource struct {
	AreaID           ID
	ConnectionID     ID
	InstanceID       ID
	Kind             ScopeKind
	NativeScopeID    string
	Active           bool
	ConnectionStatus ConnectionStatus
	PausedUntil      *time.Time
}

// SourceArea is an area of a source in listAreas.
type SourceArea struct {
	ID           ID
	Kind         ScopeKind
	DisplayName  string
	ConnectionID ID
}

type SourceAreaFilter struct {
	Kind         ScopeKind
	ConnectionID ID
	Limit        int
	After        ID
}

type SourceAreaPage struct {
	Items []SourceArea
	Next  *ID
}

// Decisions of CheckAccess; unknown and a failed check close the item (FR-001).
const (
	SourceAllowed = "allowed"
	SourceDenied  = "denied"
	SourceUnknown = "unknown"
)
