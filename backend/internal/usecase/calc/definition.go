package calc

import "slices"

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

// Inputs a definition reads; a change of any of them makes a revision of its series stale.
const (
	InputDefinition = "definition"
	InputMapping    = "mapping"
	InputStructure  = "structure"
)

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
}

const _tasksCatalog = "docs/product/metrics/tasks.md#"

var _flowInputs = []string{InputDefinition, InputMapping, InputStructure}

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
}

// Definitions is the registry of the definitions of the flow metrics in force.
func Definitions() []Definition {
	out := slices.Clone(_definitions)
	for i := range out {
		out[i].Estimators = slices.Clone(out[i].Estimators)
		out[i].Roles = slices.Clone(out[i].Roles)
		out[i].Inputs = slices.Clone(out[i].Inputs)
	}
	return out
}

func definition(id string) Definition {
	i := slices.IndexFunc(_definitions, func(d Definition) bool { return d.ID == id })
	return _definitions[i]
}
