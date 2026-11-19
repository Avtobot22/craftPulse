package calc

import (
	"maps"
	"slices"
	"time"
)

// Units of definitions.
const (
	UnitCount   = "count"
	UnitSeconds = "seconds"
	UnitRatio   = "ratio"
)

// Estimators of a distribution: nearest-rank percentiles sorted[ceil(p*n)-1] (BR-006).
const (
	EstimatorP50 = "p50"
	EstimatorP85 = "p85"
)

// Series of the metrics of completeness: the field of T38, the kind of freshness of T40, the unit of coverage of
// T41 and the kind of difference of T42.
const (
	FieldSize          = "size"
	FieldType          = "type"
	FieldTeam          = "team"
	FieldStart         = "start"
	FreshnessLag       = "lag"
	FreshnessSuccess   = "last_success_age"
	CoverageTasks      = "tasks"
	CoverageTransition = "transitions"
	DifferenceMissing  = "missing"
	DifferenceChanged  = "changed"
)

// Inputs a definition reads; a change of any of them makes a revision of its series stale.
const (
	InputDefinition = "definition"
	InputMapping    = "mapping"
	InputStructure  = "structure"
	InputTarget     = "target"
	InputWatermark  = "source_watermark"
)

// _followUpDays is the window H of T23 in version 1 of its definition (BR-008); another window is a new version.
const _followUpDays = 30

var _followUp = _followUpDays * 24 * time.Hour

// _lowSample is the n below which a point is marked as a small sample; it is a product guard, not a rule of
// statistics (TZ 4.4, BR-006).
const _lowSample = 20

// Definition is a version of a metric of the catalog. Rows of calc.definition repeat ID, Version, Unit and
// CatalogRef; the release migration adds them and the application does not change them.
type Definition struct {
	ID         string
	Version    int
	Name       string
	Unit       string
	CatalogRef string
	// Estimators are the series of a distribution; a metric without them has one series.
	Estimators []string
	// Formula is what the value of a point is, as the passport shows it.
	Formula string
	// Roles are the roles its contributions take.
	Roles  []string
	Inputs []string
	// Parameters are the fixed choices of the version, as calc.definition.parameters keeps them.
	Parameters map[string]any
}

const _tasksCatalog = "docs/product/metrics/tasks.md#"

var (
	_flowInputs     = []string{InputDefinition, InputMapping, InputStructure}
	_targetInputs   = []string{InputDefinition, InputMapping, InputStructure, InputTarget}
	_sourceInputs   = []string{InputDefinition, InputMapping, InputStructure, InputWatermark}
	_completeRoles  = []string{"numerator", "denominator", "missing"}
	_collectedRoles = []string{"numerator"}
)

var _definitions = []Definition{
	{
		ID: "T04", Version: 1, Name: "Started", Unit: UnitCount, CatalogRef: _tasksCatalog + "t04",
		Formula: "unique tasks whose first valid start falls in [from,to)",
		Roles:   []string{"numerator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T05", Version: 1, Name: "Completed", Unit: UnitCount, CatalogRef: _tasksCatalog + "t05",
		Formula: "unique tasks whose first accepted completion falls in [from,to); repeated completions and cancellations are excluded",
		Roles:   []string{"numerator", "excluded", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T07", Version: 1, Name: "Cancelled and duplicates", Unit: UnitCount, CatalogRef: _tasksCatalog + "t07",
		Formula: "unique tasks with a terminal outcome without an accepted result in [from,to), by reason",
		Roles:   []string{"numerator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T08", Version: 1, Name: "Cycle time", Unit: UnitSeconds, CatalogRef: _tasksCatalog + "t08",
		Estimators: []string{EstimatorP50, EstimatorP85},
		Formula:    "first accepted completion - first start of the tasks first completed in [from,to), nearest-rank percentile",
		Roles:      []string{"numerator", "excluded", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T12", Version: 1, Name: "Work in progress", Unit: UnitCount, CatalogRef: _tasksCatalog + "t12",
		Formula: "tasks with an open episode episode_started <= t < completed/cancelled at t = end of the point, blocked included",
		Roles:   []string{"numerator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T13", Version: 1, Name: "Blocked work in progress", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t13",
		Formula: "blocked tasks / tasks in WIP at t = end of the point; the count is the numerator",
		Roles:   []string{"numerator", "denominator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T14", Version: 1, Name: "Work in progress age", Unit: UnitSeconds, CatalogRef: _tasksCatalog + "t14",
		Estimators: []string{EstimatorP50, EstimatorP85},
		Formula:    "t - start of the current episode of the tasks in WIP at t = end of the point; the age from the first start is a separate contribution",
		Roles:      []string{"numerator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T15", Version: 1, Name: "Blocked time", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t15",
		Formula: "union of blocked intervals / time in open episodes within [from,to) bounded by as_of; the blocked seconds are the numerator",
		Roles:   []string{"numerator", "denominator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T18", Version: 1, Name: "Flow balance", Unit: UnitCount, CatalogRef: _tasksCatalog + "t18",
		Formula: "tasks first started - tasks first completed in [from,to), next to the WIP of T12",
		Roles:   []string{"numerator", "missing"}, Inputs: _flowInputs,
	},
	{
		ID: "T10", Version: 1, Name: "Within target", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t10",
		Formula: "tasks first completed in [from,to) with cycle <= the target of their type and size in force at the knowledge number / " +
			"those with a known start, size and target; the others are missing with the reason",
		Roles: []string{"numerator", "denominator", "missing"}, Inputs: _targetInputs,
	},
	{
		ID: "T11", Version: 1, Name: "Aging over target", Unit: UnitCount, CatalogRef: _tasksCatalog + "t11",
		Formula: "tasks in WIP at t = end of the point whose current episode is older than their target, among those with a target; " +
			"the ratio age/target of each is its contribution",
		Roles: []string{"numerator", "denominator", "missing"}, Inputs: _targetInputs,
	},
	{
		ID: "T23", Version: 1, Name: "Reopened", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t23",
		Formula: "tasks first completed in [from,to) reopened in [first_completed, first_completed + H) / those whose window ended by as_of; " +
			"immature tasks are excluded and listed",
		Roles: []string{"numerator", "denominator", "excluded", "missing"}, Inputs: _flowInputs,
		Parameters: map[string]any{"follow_up_days": _followUpDays},
	},
	{
		ID: "T38", Version: 1, Name: "Field completeness", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t38",
		Estimators: []string{FieldSize, FieldType, FieldTeam, FieldStart},
		Formula: "tasks with the field known / eligible tasks at t = end of the point, field by field; the size counts the tasks started, " +
			"as it is taken at the start, and the start the tasks started or completed",
		Roles: _completeRoles, Inputs: _flowInputs,
	},
	{
		ID: "T39", Version: 1, Name: "History completeness", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t39",
		Formula: "tasks with a confirmed full history / tasks created by t = end of the point; a history is confirmed when it has no " +
			"boundary or conflict and every connection of its area reconciled without differences",
		Roles: _completeRoles, Inputs: _sourceInputs,
	},
	{
		ID: "T40", Version: 1, Name: "Freshness", Unit: UnitSeconds, CatalogRef: _tasksCatalog + "t40",
		Estimators: []string{FreshnessLag, FreshnessSuccess},
		Formula: "as_of - the watermark and as_of - the last success of the stalest connection of the team, at the last point; " +
			"each connection is a contribution",
		Roles: _collectedRoles, Inputs: _sourceInputs,
	},
	{
		ID: "T41", Version: 1, Name: "Mapping coverage", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t41",
		Estimators: []string{CoverageTasks, CoverageTransition},
		Formula: "tasks all of whose statuses entered by t = end of the point have a confirmed meaning / tasks created by t; " +
			"and statuses entered with a confirmed meaning / statuses entered",
		Roles: _completeRoles, Inputs: _flowInputs,
	},
	{
		ID: "T42", Version: 1, Name: "Reconciliation variance", Unit: UnitCount, CatalogRef: _tasksCatalog + "t42",
		Estimators: []string{DifferenceMissing, DifferenceChanged},
		Formula: "tasks missing from craftPulse and tasks with another version than the source in the last reconciliation of each " +
			"connection of the team, at the last point; the expected number is the denominator",
		Roles: _collectedRoles, Inputs: _sourceInputs,
	},
}

// Definitions is the registry of the definitions of the flow metrics in force.
func Definitions() []Definition {
	out := slices.Clone(_definitions)
	for i := range out {
		out[i].Estimators = slices.Clone(out[i].Estimators)
		out[i].Roles = slices.Clone(out[i].Roles)
		out[i].Inputs = slices.Clone(out[i].Inputs)
		out[i].Parameters = maps.Clone(out[i].Parameters)
	}
	return out
}

// lookup finds a definition in force; a retired one is absent.
func lookup(id string) (Definition, bool) {
	i := slices.IndexFunc(_definitions, func(d Definition) bool { return d.ID == id })
	if i < 0 {
		return Definition{}, false
	}
	return _definitions[i], true
}

func definition(id string) Definition {
	i := slices.IndexFunc(_definitions, func(d Definition) bool { return d.ID == id })
	return _definitions[i]
}
