package entity

import "time"

// RuleKind is what a rule of a mapping version maps (TZ 12.5.6).
type RuleKind string

const (
	RuleStatus            RuleKind = "status"
	RuleResolution        RuleKind = "resolution"
	RuleIssueType         RuleKind = "issue_type"
	RuleCodeRequiredType  RuleKind = "code_required_type"
	RuleSizeField         RuleKind = "size_field"
	RuleSizeValue         RuleKind = "size_value"
	RuleTeamField         RuleKind = "team_field"
	RuleTeamValue         RuleKind = "team_value"
	RuleBlockedFlag       RuleKind = "blocked_flag"
	RuleAcceptanceGate    RuleKind = "acceptance_gate"
	RuleEpicField         RuleKind = "epic_field"
	RuleBusinessEpicValue RuleKind = "business_epic_value"
)

// Meanings of statuses and resolutions; a status without a rule gives neither a start nor an end (FR-017 A3).
const (
	MeaningIntake       = "intake"
	MeaningActive       = "active"
	MeaningWait         = "wait"
	MeaningBlocked      = "blocked"
	MeaningDone         = "done"
	MeaningCancelled    = "cancelled"
	MeaningDuplicate    = "duplicate"
	MeaningIgnored      = "ignored"
	MeaningAccepted     = "accepted"
	MeaningCodeRequired = "code_required"
	MeaningAcceptance   = "acceptance"
	MeaningParentEpic   = "parent_epic"
	MeaningBusinessEpic = "business_epic"
)

// MaxMappingRules bounds the rules of one draft, as the contract does.
const MaxMappingRules = 2000

// MappingRule maps a native status, resolution, type, field or field value of an instance; NativeValue is
// empty for rules without a value.
type MappingRule struct {
	Kind        RuleKind
	InstanceID  ID
	NativeRef   string
	NativeValue string
	Meaning     string
	Rank        *int
}

// MappingDraft is the form of a process owner before it is confirmed; previewMapping takes it as well.
type MappingDraft struct {
	Timezone          string
	CalendarVersionNo *int
	Rules             []MappingRule
}

// Confirmation is who confirmed a version of a setting, when, and under which idempotency key.
type Confirmation struct {
	ConfirmedBy    ID
	ConfirmedAt    time.Time
	ConfirmationID ID
}

// MappingVersion is an immutable confirmed mapping of a team; Rules are ordered by their key.
type MappingVersion struct {
	ID                ID
	TeamID            ID
	VersionNo         int
	BaseVersionNo     *int
	ContentHash       []byte
	Timezone          string
	CalendarVersionID *ID
	CalendarVersionNo *int
	KnownSeq          int64
	Confirmation
	Rules []MappingRule
}

// Workdays is a mask of working days of a week with Monday in bit 0 and Sunday in bit 6.
type Workdays uint8

// AllWorkdays is the mask of the whole week, the bound of the column.
const AllWorkdays Workdays = 1<<7 - 1

// Has tells whether the day is working.
func (w Workdays) Has(d time.Weekday) bool { return w&dayBit(d) != 0 }

// With adds the day to the mask.
func (w Workdays) With(d time.Weekday) Workdays { return w | dayBit(d) }

func dayBit(d time.Weekday) Workdays { return 1 << ((int(d) + 6) % 7) }

// TimeOfDay is minutes after midnight in the time zone of the calendar.
type TimeOfDay int

// CalendarDraft is the form of a working calendar; Holidays are dates at midnight UTC in ascending order.
type CalendarDraft struct {
	Timezone  string
	Workdays  Workdays
	WorkStart *TimeOfDay
	WorkEnd   *TimeOfDay
	Holidays  []time.Time
}

// CalendarVersion is an immutable confirmed calendar of a team.
type CalendarVersion struct {
	ID            ID
	TeamID        ID
	VersionNo     int
	BaseVersionNo *int
	CalendarDraft
	KnownSeq int64
	Confirmation
}

// VersionPageRequest reads versions newest first; Before is the number the previous page ended on.
type VersionPageRequest struct {
	Limit  int
	Before *int
}

// MappingVersionPage is a page of versions; Latest is the number of the last version of the team, 0 without one.
type MappingVersionPage struct {
	Items  []MappingVersion
	Next   *int
	Latest int
}

type CalendarVersionPage struct {
	Items  []CalendarVersion
	Next   *int
	Latest int
}

// DictionaryItem is a status, a resolution, a type or a field of a source; Category is the status category of
// the source and ValueType the type of a field, empty when the source does not name them.
type DictionaryItem struct {
	Kind      DictionaryKind
	NativeID  string
	Name      string
	Category  string
	ValueType string
}

// AreaDictionary is what normalize knows of the dictionaries of an area of an instance.
type AreaDictionary struct {
	AreaID     ID
	InstanceID ID
	Items      []DictionaryItem
}

// CatalogInstance is the dictionaries of one instance over the areas of a team, each value once.
type CatalogInstance struct {
	InstanceID  ID
	Statuses    []DictionaryItem
	Resolutions []DictionaryItem
	Types       []DictionaryItem
	Fields      []DictionaryItem
}

// NativeStatusRef is a status of an instance.
type NativeStatusRef struct {
	InstanceID ID
	NativeID   string
}

// WorkflowCatalog is what the form of a mapping offers; Unmapped are the statuses without a rule in the current
// version (FR-017).
type WorkflowCatalog struct {
	Instances []CatalogInstance
	Unmapped  []NativeStatusRef
}

// Bases of a target duration (FR-019): set by hand or taken from the historical p85 of a revision.
const (
	TargetManual        = "manual"
	TargetHistoricalP85 = "historical_p85"
)

// ReasonSizeUnknown marks the baseline of tasks without a size, which no target applies to (BR-007).
const ReasonSizeUnknown = "size_unknown"

// TargetGroup is the type and the size a target applies to.
type TargetGroup struct {
	TaskType  string
	SizeLabel string
}

// TargetDraft is the form of a target of a group; a historical p85 names the revision and the period of its
// baseline.
type TargetDraft struct {
	TargetGroup
	DurationSeconds int64
	Basis           string
	BasisRevisionID *ID
	BasisPeriod     *Window
}

// TargetBasis is the historical p85 of a group read from a revision, kept with the version it founded.
type TargetBasis struct {
	RevisionID   ID
	ValueSeconds int64
	N            int
	Window       Window
}

// TargetVersion is an immutable confirmed target of a group of a team; it holds from its confirmation until the
// next version of the group, and earlier results keep their version (FR-019).
type TargetVersion struct {
	ID     ID
	TeamID ID
	TargetGroup
	VersionNo       int
	BaseVersionNo   *int
	DurationSeconds int64
	Basis           string
	BasisOf         *TargetBasis
	KnownSeq        int64
	Confirmation
}

// TargetVersionFilter narrows the versions of a team to a type or to one group; Before is the knowledge number
// the previous page ended on.
type TargetVersionFilter struct {
	TaskType  *string
	SizeLabel *string
	Limit     int
	Before    *int64
}

// TargetVersionPage is a page of versions newest first; Latest is the last version number of the group when the
// filter names one, 0 without versions.
type TargetVersionPage struct {
	Items  []TargetVersion
	Next   *int64
	Latest *int
}

// SaveResult tells how a confirmation of a version ended.
type SaveResult int

const (
	SaveInserted SaveResult = iota + 1
	// SaveRepeated is a repeat of the idempotency key: the version it saved before.
	SaveRepeated
	// SaveUnchanged is a form equal to the version in force, which stays.
	SaveUnchanged
)
