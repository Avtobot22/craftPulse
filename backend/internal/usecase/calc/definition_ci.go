package calc

const (
	_ciCatalog = "docs/product/metrics/ci-cd.md#"
	_ciCohort  = "; run completion cohort: finished_at in [from,to)"
)

var (
	_ciInputs = []string{InputDefinition, InputStructure, InputWatermark}
	_ciRoles  = []string{"numerator", "denominator", "excluded", "missing"}
)

// _ciNativeClasses is part of definition version 1, independent of normalization's coarser outcomes. A native status
// is matched without case: Jenkins gives UNSTABLE where GitLab gives failed.
var _ciNativeClasses = map[string]string{
	"success": "success", "failed": "failure", "failure": "failure", "timeout": "timeout",
	"timed_out": "timeout", "canceled": "canceled", "cancelled": "canceled", "canceling": "canceled",
	"skipped": "skipped", "manual": "manual", "unstable": "unstable",
}

var _ciClasses = []string{"success", "failure", "timeout", "canceled", "skipped", "manual", "unstable", "unknown"}

var _ciDefinitions = []Definition{
	{
		ID: "I01", Version: 1, Name: "Final CI success", Unit: UnitRatio, CatalogRef: _ciCatalog + "i01",
		Estimators: []string{"runs", "jobs"}, Formula: "terminal success / (success + failure + timeout + unstable), separately for runs and final logical jobs; native status retained" + _ciCohort,
		Roles: _ciRoles, Inputs: _ciInputs, Parameters: map[string]any{"native_status_classes": _ciNativeClasses},
	},
	{
		ID: "I02", Version: 1, Name: "First-pass CI success", Unit: UnitRatio, CatalogRef: _ciCatalog + "i02",
		Formula: "logical jobs successful on attempt 1 / logical jobs with a known first outcome; incomplete attempts never give an exact ratio" + _ciCohort,
		Roles:   _ciRoles, Inputs: _ciInputs,
	},
	{
		ID: "I03", Version: 1, Name: "Observed retry outcome flip", Unit: UnitRatio, CatalogRef: _ciCatalog + "i03",
		Formula: "comparable logical jobs with a failure followed by success / comparable jobs with complete attempts; an observed flip is not a confirmed flaky test" + _ciCohort,
		Roles:   _ciRoles, Inputs: _ciInputs,
	},
	{
		ID: "I04", Version: 1, Name: "CI retry overhead", Unit: UnitSeconds, CatalogRef: _ciCatalog + "i04",
		Estimators: []string{"attempts", "seconds", "total_seconds"},
		Formula:    "observed attempts numbered above 1 and their job execution seconds; total_seconds includes all attempts; incomplete history or duration yields a lower bound" + _ciCohort,
		Roles:      _ciRoles, Inputs: _ciInputs,
	},
	{
		ID: "I05", Version: 1, Name: "Success with allowed failures", Unit: UnitRatio, CatalogRef: _ciCatalog + "i05",
		Formula: "successful runs with a finally failed allow_failure job / successful complete runs of a source with allow_failure semantics; a retried success is not an allowed failure" + _ciCohort,
		Roles:   _ciRoles, Inputs: _ciInputs,
	},
	{
		ID: "I06", Version: 1, Name: "CI outcomes", Unit: UnitRatio, CatalogRef: _ciCatalog + "i06",
		Estimators: ciOutcomeEstimators(), Formula: "count and share of every outcome class, separately for runs and final logical jobs; running and unmapped statuses are unknown" + _ciCohort,
		Roles: _ciRoles, Inputs: _ciInputs, Parameters: map[string]any{"native_status_classes": _ciNativeClasses},
	},
	{
		ID: "I07", Version: 1, Name: "Source failure reasons", Unit: UnitCount, CatalogRef: _ciCatalog + "i07",
		Formula: "failed attempts grouped by the unmodified source failure reason, with unknown as a separate group; no inferred code or test cause" + _ciCohort,
		Roles:   _ciRoles, Inputs: _ciInputs,
	},
	{
		ID: "I10", Version: 1, Name: "CI history completeness", Unit: UnitRatio, CatalogRef: _ciCatalog + "i10",
		Estimators: []string{"", "loaded"}, Formula: "complete runs / known runs when CI reconciliation and watermark cover the point; otherwise loaded is a lower-bound count and no percentage is produced" + _ciCohort,
		Roles: _ciRoles, Inputs: _ciInputs,
	},
}

func ciOutcomeEstimators() []string {
	out := make([]string, 0, 2*len(_ciClasses))
	for _, kind := range []string{"runs", "jobs"} {
		for _, status := range _ciClasses {
			out = append(out, kind+":"+status)
		}
	}
	return out
}
