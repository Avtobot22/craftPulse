package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

const (
	_inputMRDiff         = "mr_diff"
	_inputMRComparison   = "mr_code_comparison"
	_inputLineMap        = "coverage_line_map"
	_inputMRFileScope    = "mr_file_scope"
	_metricPatchCoverage = "C28"
	_aliasPatchCoverage  = "I27"
)

var _mrStructureDefinitions = []Definition{
	{
		ID: "C15", Version: 1, Name: "Merge request size", Unit: UnitCount, CatalogRef: _codeCatalog + "c15",
		Formula: "added lines, deleted lines and changed files from the final diff, separately for regular, generated and vendor files",
		Inputs:  []string{InputDefinition, _inputMRDiff, _inputMRFileScope}, Roles: []string{"numerator", "missing"},
	},
	{
		ID: "C16", Version: 1, Name: "Function complexity change", Unit: UnitCount, CatalogRef: _codeCatalog + "c16",
		Formula: "head CC minus base CC per uniquely matched file and function name, using diff renames; newly observed functions separately",
		Inputs:  []string{InputDefinition, _inputMRDiff, _inputCodeReport, _inputMRComparison}, Roles: []string{"numerator", "excluded"},
	},
	{
		ID: "C17", Version: 1, Name: "New clone groups in the change", Unit: UnitCount, CatalogRef: _codeCatalog + "c17",
		Formula: "head clone groups with a new or changed member in changed lines that cannot be accounted for by a base group",
		Inputs:  []string{InputDefinition, _inputMRDiff, _inputCodeReport, _inputMRComparison}, Roles: []string{"numerator", "excluded"},
	},
	{
		ID: _metricPatchCoverage, Version: 1, Name: "Patch line coverage", Unit: UnitRatio, CatalogRef: _codeCatalog + "c28",
		Formula:    "covered changed executable lines / changed executable lines with a line map at the diff head SHA",
		Inputs:     []string{InputDefinition, _inputMRDiff, _inputLineMap},
		Roles:      []string{string(entity.RoleNumerator), string(entity.RoleDenominator), string(entity.RoleExcluded)},
		Parameters: map[string]any{"alias": _aliasPatchCoverage, "coverage_kind": "line"},
	},
}
