package calc

var _testCoverageDefinitions = []Definition{
	{
		ID: "I19", Version: 1, Name: "Test outcomes", Unit: UnitCount, CatalogRef: _ciCatalog + "i19",
		Formula: "passed, failed, error, skipped, distinct test cases and executions; separately by suite, configuration and attempt",
		Roles:   _ciRoles, Inputs: _ciInputs, Parameters: map[string]any{"group_by": []string{"suite", "config_fingerprint", "attempt_id"}},
	},
	{
		ID: "I25", Version: 1, Name: "Line coverage", Unit: UnitRatio, CatalogRef: _ciCatalog + "i25",
		Formula: "covered executable lines / executable lines in the report scope",
		Roles:   _ciRoles, Inputs: _ciInputs, Parameters: map[string]any{"coverage_kind": "line"},
	},
	{
		ID: "I26", Version: 1, Name: "Branch coverage", Unit: UnitRatio, CatalogRef: _ciCatalog + "i26",
		Formula: "covered branches / reported branches, only where branch data was reported",
		Roles:   _ciRoles, Inputs: _ciInputs, Parameters: map[string]any{"coverage_kind": "branch"},
	},
}
