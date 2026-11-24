package calc

var _securityDefinitions = []Definition{
	{
		ID: "S09", Version: 1, Name: "Open security occurrences", Unit: UnitCount, CatalogRef: _securityCatalog + "s09",
		Formula: "distinct actionable occurrences by analysis kind and native severity",
	},
	{
		ID: "S12", Version: 1, Name: "SAST findings", Unit: UnitCount, CatalogRef: _securityCatalog + "s12",
		Formula: "distinct SAST occurrences by CWE and rule with native confidence",
	},
	{
		ID: "S13", Version: 1, Name: "Affected package instances", Unit: UnitCount, CatalogRef: _securityCatalog + "s13",
		Formula: "distinct affected package versions in their dependency context",
	},
	{
		ID: "S14", Version: 1, Name: "Known advisories", Unit: UnitCount, CatalogRef: _securityCatalog + "s14",
		Formula: "distinct advisories joined by explicit aliases",
	},
	{
		ID: "S15", Version: 1, Name: "Affected package instances with a fix", Unit: UnitRatio, CatalogRef: _securityCatalog + "s15",
		Formula: "instances with a known fixed version / affected instances with known fix metadata",
	},
	{
		ID: "S16", Version: 1, Name: "New affected package instances", Unit: UnitCount, CatalogRef: _securityCatalog + "s16",
		Formula: "affected package instances added or changed in the lockfile diff",
	},
	{
		ID: "S18", Version: 1, Name: "IaC misconfigurations", Unit: UnitCount, CatalogRef: _securityCatalog + "s18",
		Formula: "distinct active rule and resource pairs in the checked IaC scope",
	},
	{
		ID: "S19", Version: 1, Name: "Potential secrets", Unit: UnitCount, CatalogRef: _securityCatalog + "s19",
		Formula: "distinct redacted detector occurrences, separately for head and diff",
	},
}
