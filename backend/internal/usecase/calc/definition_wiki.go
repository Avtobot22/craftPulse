package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

var _wikiDefinition = Definition{
	ID: "D27", Version: 2, Name: "Wiki corpus collection", Unit: UnitRatio, CatalogRef: _docsCatalog + "d27",
	Formula:    "current pages read with known ACL / expected pages in the last source manifest of the wiki space",
	Estimators: []string{"last_success", "earliest_version"},
	Inputs:     []string{InputDefinition, _inputDocsRoot, entity.InputKindWatermark},
	Roles:      _analysisRoles,
}
