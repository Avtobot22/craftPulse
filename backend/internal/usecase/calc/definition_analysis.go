package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

const _analysisCodeCatalog = "docs/product/metrics/code.md#"

var _analysisRoles = []string{"numerator", "denominator", "missing", "excluded"}

var _analysisDefinitions = []Definition{
	analysisDefinition("C02", "Eligible file coverage", UnitRatio, _analysisCodeCatalog+"c02",
		"analyzed eligible files / eligible files reported by each language and tool"),
	analysisDefinition("C03", "Analysis scope completion", UnitRatio, _analysisCodeCatalog+"c03",
		"complete requested runs / runs in the latest scan manifest and the latest report of each Sonar project"),
	analysisDefinition("C35", "Analysis snapshot age", UnitSeconds, _analysisCodeCatalog+"c35",
		"as_of - latest completed snapshot of the profile at this commit"),
	analysisDefinition("C36", "Analyzer duration", UnitSeconds, _analysisCodeCatalog+"c36",
		"finish - start per run, nearest-rank p95 by profile; observed queue time is a separate series"),
	analysisDefinition("S01", "Last completed security run", UnitSeconds, _securityCatalog+"s01",
		"Unix time of the latest complete run by security kind and profile at this commit"),
	analysisDefinition("S02", "SAST eligible file coverage", UnitRatio, _securityCatalog+"s02",
		"successfully analyzed eligible SAST files / eligible files of the language when known by the producer"),
	analysisDefinition("S03", "Dependency resolution", UnitRatio, _securityCatalog+"s03",
		"dependency entries with an exact resolved version / detected dependency entries in the SCA inventory"),
	analysisDefinition("S04", "Checked secrets history", UnitCount, _securityCatalog+"s04",
		"checked commit and time boundaries in the contribution; the value indicates truncation (1 or 0)"),
	analysisDefinition("S05", "Vulnerability database age", UnitSeconds, _securityCatalog+"s05",
		"as_of - publication time of the vulnerability database used by the scan"),
	analysisDefinition("S07", "Security run success", UnitRatio, _securityCatalog+"s07",
		"successfully completed security runs / requested security runs in the latest manifest with security checks"),
	analysisDefinition("S08", "Security scan age", UnitSeconds, _securityCatalog+"s08",
		"as_of - latest complete security scan of the same scope"),
	analysisDefinition("D27", "Git corpus collection", UnitRatio, _docsCatalog+"d27",
		"complete / requested documentation-profile runs, with last successful check and earliest observed version as Unix times"),
}

func analysisDefinition(id, name, unit, catalog, formula string) Definition {
	version := 1
	if id == "C03" {
		version = 2
	}
	return Definition{
		ID: id, Version: version, Name: name, Unit: unit, CatalogRef: catalog, Formula: formula,
		Inputs: []string{InputDefinition, entity.InputAnalysisRun}, Roles: _analysisRoles,
	}
}
