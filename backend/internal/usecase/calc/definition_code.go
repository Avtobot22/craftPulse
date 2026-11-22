package calc

import (
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_codeCatalog       = "docs/product/metrics/code.md#"
	_estProduction     = "production"
	_estTest           = "test"
	_estP90            = "p90"
	_estMax            = "max"
	_estAboveThreshold = "above_threshold"
	_inputCodeReport   = "code_report"
	_inputThreshold    = "profile_threshold"
)

var (
	_codeInputs         = []string{InputDefinition, _inputCodeReport}
	_thresholdInputs    = []string{InputDefinition, _inputCodeReport, _inputThreshold}
	_distributionParams = map[string]any{"estimators": []string{EstimatorP50, _estP90, _estMax}, "percentiles": "nearest_rank"}
	_lizardDefinitions  = []Definition{
		{
			ID: "C01", Version: 1, Name: "Analyzed code volume", Unit: UnitCount, CatalogRef: _codeCatalog + "c01",
			Estimators: []string{_estProduction, _estTest}, Inputs: _codeInputs, Roles: []string{"numerator", "missing"},
			Formula:    "sum of file NLOC in the report scope, by production and test code",
			Parameters: map[string]any{"estimators": []string{_estProduction, _estTest}},
		},
		{
			ID: "C04", Version: 1, Name: "Cyclomatic complexity", Unit: UnitCount, CatalogRef: _codeCatalog + "c04",
			Estimators: []string{EstimatorP50, _estP90, _estMax}, Inputs: _codeInputs, Roles: []string{"numerator", "missing"},
			Formula:    "nearest-rank p50 and p90 and maximum of the CC values of the functions",
			Parameters: _distributionParams,
		},
		{
			ID: "C05", Version: 1, Name: "Functions above complexity threshold", Unit: UnitRatio, CatalogRef: _codeCatalog + "c05",
			Inputs: _thresholdInputs, Roles: _completeRoles, Formula: "functions with CC strictly above the profile threshold / analyzed functions",
			Parameters: map[string]any{"threshold": "scan_manifest.function_thresholds.ccn_threshold", "operator": ">"},
		},
		{
			ID: "C06", Version: 1, Name: "Function length", Unit: UnitCount, CatalogRef: _codeCatalog + "c06",
			Estimators: []string{EstimatorP50, _estP90, _estMax, _estAboveThreshold}, Inputs: _thresholdInputs, Roles: []string{"numerator", "excluded", "missing"},
			Formula: "nearest-rank p50 and p90 and maximum of function NLOC, and count strictly above the profile threshold",
			Parameters: map[string]any{
				"estimators": []string{EstimatorP50, _estP90, _estMax, _estAboveThreshold}, "percentiles": "nearest_rank",
				"threshold": "scan_manifest.function_thresholds.nloc_threshold", "operator": ">",
			},
		},
	}
	_cloneDefinitions = []Definition{
		{
			ID: "C08", Version: 1, Name: "Duplicate lines", Unit: UnitRatio, CatalogRef: _codeCatalog + "c08",
			Inputs: _codeInputs, Roles: _completeRoles, Formula: "union of inclusive clone ranges by file / analyzed physical lines",
			Parameters: map[string]any{"denominator": "analyzed_physical_lines", "ranges": "inclusive_union_by_file"},
		},
		{
			ID: "C09", Version: 1, Name: "Clone groups", Unit: UnitCount, CatalogRef: _codeCatalog + "c09",
			Inputs: _codeInputs, Roles: []string{"numerator", "missing"}, Formula: "count of connected groups of clone occurrences",
			Parameters: map[string]any{"group": "connected_clone_pairs"},
		},
	}
	_codeDefinitions = slices.Concat(_lizardDefinitions, _cloneDefinitions)
)

func codeDefinitions(format string) []Definition {
	if format == entity.CodeFormatLizard {
		return _lizardDefinitions
	}
	return _cloneDefinitions
}
