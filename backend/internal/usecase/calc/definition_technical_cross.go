package calc

var _technicalCrossDefinitions = []Definition{
	{
		ID: "X03", Version: 1, Name: "Tasks with current technical warnings", Unit: UnitRatio, CatalogRef: _crossCatalog,
		Estimators: []string{"warning", "new_code", "new_security", "docs_violation", "code_exception", "security_exception"},
		Formula:    "WIP tasks with a confirmed current-head warning or active exception, counted once across MRs; the denominator is the cohort with known state for the signal",
		Roles:      _completeRoles, Inputs: _targetInputs,
	},
	{
		ID: "X04", Version: 1, Name: "Successful pipelines with caveats", Unit: UnitRatio, CatalogRef: _crossCatalog,
		Estimators: []string{"caveat", "new_code", "new_security", "docs_violation", "missing_report", "fail_to_pass", "allowed_failure", "code_exception", "security_exception"},
		Formula:    "unique successful current-head runs with the selected signal among runs with known state for the signal; exceptions are reported separately",
		Roles:      _completeRoles, Inputs: _crossInputs,
	},
	{
		ID: "X05", Version: 1, Name: "Task-to-check chain completeness", Unit: UnitRatio, CatalogRef: _crossCatalog,
		Estimators: []string{"confirmed_mr", "current_head_ci", "complete_reports", "checked_code", "checked_security", "checked_tests", "checked_coverage", "checked_docs"},
		Formula:    "tasks of code-required types in progress at point end or completed in the point, reaching the selected stage through one MR or having that scope checked",
		Roles:      _completeRoles, Inputs: _crossInputs,
	},
	{
		ID: "X06", Version: 1, Name: "Verified MR fixes", Unit: UnitCount, CatalogRef: _crossCatalog,
		Estimators: technicalFixEstimators(),
		Formula:    "counts of distinct current-head findings per MR and analysis kind by comparison lifecycle, with accepted exceptions and unverified scopes separate",
		Roles:      _completeRoles, Inputs: _crossInputs,
	},
}

func technicalFixEstimators() []string {
	kinds := []string{"code", "sast", "sca", "secrets", "iac", "image", "docs"}
	dispositions := []string{"new", "persisting", "resolved", "accepted", "unverified"}
	out := make([]string, 0, len(kinds)*len(dispositions))
	for _, kind := range kinds {
		for _, disposition := range dispositions {
			out = append(out, kind+":"+disposition)
		}
	}
	return out
}

func isTechnicalCross(metric, estimator string) bool {
	switch metric {
	case "X02":
		return estimator == _ciMissingReport
	case "X03", "X04", "X05", "X06":
		return true
	default:
		return false
	}
}
