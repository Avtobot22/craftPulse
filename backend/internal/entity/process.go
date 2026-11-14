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
