package calc

const (
	_docsCatalog     = "docs/product/metrics/documentation.md#"
	_inputDocsRoot   = "documentation_root"
	_inputDocsReport = "documentation_report"
	_estDocRoots     = "roots"
	_estDocRead      = "documents"
	_estDocCoverage  = "coverage"
	_estRootOwner    = "root_owner"
	_estDocOwners    = "document_owners"
	_estSpectral     = "spectral_findings"
)

var (
	_docsInputs               = []string{InputDefinition, _inputDocsRoot, _inputDocsReport}
	_documentationDefinitions = []Definition{
		{
			ID: "D01", Version: 1, Name: "Documentation coverage", Unit: UnitCount, CatalogRef: _docsCatalog + "d01",
			Estimators: []string{_estDocRoots, _estDocRead, _estDocCoverage}, Inputs: _docsInputs, Roles: _completeRoles,
			Formula:    "confirmed roots and read documents; read / eligible documents when the inventory is complete",
			Parameters: map[string]any{"estimators": []string{_estDocRoots, _estDocRead, _estDocCoverage}, "ratio_estimator": _estDocCoverage},
		},
		{
			ID: "D03", Version: 1, Name: "Documentation owners", Unit: UnitRatio, CatalogRef: _docsCatalog + "d03",
			Estimators: []string{_estRootOwner, _estDocOwners}, Inputs: _docsInputs, Roles: _completeRoles,
			Formula:    "explicit root owner and Markdown documents with an owner / eligible Markdown documents under the configured front matter rule",
			Parameters: map[string]any{"estimators": []string{_estRootOwner, _estDocOwners}, "document_format": "markdown"},
		},
		{
			ID: "D10", Version: 1, Name: "Broken local links", Unit: UnitRatio, CatalogRef: _docsCatalog + "d10",
			Inputs: _docsInputs, Roles: _completeRoles, Formula: "confirmed missing files and anchors / checked local links",
		},
		{
			ID: "D24", Version: 1, Name: "OpenAPI validity", Unit: UnitRatio, CatalogRef: _docsCatalog + "d24",
			Estimators: []string{"", _estSpectral}, Inputs: _docsInputs, Roles: _completeRoles,
			Formula:    "valid / checked specifications, with Spectral findings counted separately",
			Parameters: map[string]any{"count_estimator": _estSpectral},
		},
		{
			ID: "D30", Version: 1, Name: "Open documentation findings", Unit: UnitCount, CatalogRef: _docsCatalog + "d30",
			Inputs: _docsInputs, Roles: _completeRoles, Formula: "open findings of enabled checks; age since the earliest report of the same fingerprint in the repository",
			Parameters: map[string]any{"age_unit": "seconds"},
		},
	}
)
