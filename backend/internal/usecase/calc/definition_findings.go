package calc

const (
	_securityCatalog    = "docs/product/metrics/security.md#"
	_estFindingNew      = "new"
	_estFindingKept     = "persisting"
	_estFindingObserved = "newly_observed"
	_estActive          = "active"
	_estExpired         = "expired"
)

var (
	_findingInputs    = []string{InputDefinition, "finding_report", "decision"}
	_comparisonInputs = []string{InputDefinition, "finding_report", "finding_comparison", "decision"}
	_findingRoles     = []string{"numerator", "excluded", "missing"}
	_deltaEstimators  = []string{_estFindingNew, _estFindingKept, _estFindingObserved}
	_exceptionStates  = []string{_estActive, _estExpired}
)

var _findingDefinitions = []Definition{
	{
		ID: "C11", Version: 1, Name: "Applicable code findings", Unit: UnitCount, CatalogRef: _codeCatalog + "c11",
		Formula: "distinct applicable code identities on the current head; evidence preserves class, rule, native severity and location",
		Roles:   _findingRoles, Inputs: _findingInputs,
	},
	{
		ID: "C12", Version: 1, Name: "New and persisting code findings", Unit: UnitCount, CatalogRef: _codeCatalog + "c12",
		Formula:    "new and persisting applicable identities on comparable head and base; without comparison the head is newly observed",
		Estimators: _deltaEstimators, Roles: _findingRoles, Inputs: _comparisonInputs,
	},
	{
		ID: "C13", Version: 1, Name: "Resolved code findings", Unit: UnitCount, CatalogRef: _codeCatalog + "c13",
		Formula: "applicable identities found on the comparable base and confirmed absent by the complete matching head scan",
		Roles:   _findingRoles, Inputs: _comparisonInputs,
	},
	{
		ID: "C26", Version: 1, Name: "Code exceptions", Unit: UnitCount, CatalogRef: _codeCatalog + "c26",
		Formula:    "distinct active code exceptions affecting the scope and expired approvals in a separate estimate",
		Estimators: _exceptionStates, Roles: _collectedRoles, Inputs: []string{InputDefinition, "decision"},
	},
	{
		ID: "S10", Version: 1, Name: "New and persisting security findings", Unit: UnitCount, CatalogRef: _securityCatalog + "s10",
		Formula:    "new and persisting applicable security identities on comparable head and base; without comparison the head is newly observed",
		Estimators: _deltaEstimators, Roles: _findingRoles, Inputs: _comparisonInputs,
	},
	{
		ID: "S11", Version: 1, Name: "Resolved security findings", Unit: UnitCount, CatalogRef: _securityCatalog + "s11",
		Formula: "applicable security identities found on the comparable base and confirmed absent by the complete matching head scan",
		Roles:   _findingRoles, Inputs: _comparisonInputs,
	},
	{
		ID: "S35", Version: 1, Name: "Security exceptions", Unit: UnitCount, CatalogRef: _securityCatalog + "s35",
		Formula:    "distinct active security exceptions affecting the scope and expired approvals in a separate estimate",
		Estimators: _exceptionStates, Roles: _collectedRoles, Inputs: []string{InputDefinition, "decision"},
	},
	{
		ID: "S38", Version: 1, Name: "Findings from new analysis inputs", Unit: UnitCount, CatalogRef: _securityCatalog + "s38",
		Formula: "distinct applicable security identities first detected on unchanged code due to a change of database, rules or analysis scope",
		Roles:   _findingRoles, Inputs: _comparisonInputs,
	},
	{
		ID: "I08", Version: 1, Name: "Missing expected checks", Unit: UnitRatio, CatalogRef: _ciCatalog + "i08",
		Formula: "distinct expected logical checks without a terminal outcome / expected checks",
		Roles:   []string{"numerator", "denominator"}, Inputs: []string{InputDefinition, "expected_checks"},
	},
	{
		ID: "I09", Version: 1, Name: "Missing or incomplete expected reports", Unit: UnitRatio, CatalogRef: _ciCatalog + "i09",
		Formula: "expected report scopes without a complete valid report on the requested head / expected report scopes",
		Roles:   []string{"numerator", "denominator"}, Inputs: []string{InputDefinition, "expected_reports", "finding_report"},
	},
}
