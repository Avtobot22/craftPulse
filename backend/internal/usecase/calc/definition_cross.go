package calc

const (
	_reviewNoReviewer = "no_reviewer"
	_reviewNoResponse = "no_first_response"
	_ciFailed         = "failed"
	_ciRetried        = "passed_after_retry"
	_ciMissingReport  = "missing_report"
	_crossCatalog     = "docs/product/metrics/mvp.md#7-кросс-метрики-и-совместный-анализ"
)

var _crossDefinitions = []Definition{
	{
		ID: "X01", Version: 1, Name: "Tasks in progress waiting for review", Unit: UnitRatio, CatalogRef: _crossCatalog,
		Estimators: []string{_reviewNoReviewer, _reviewNoResponse},
		Formula: "distinct WIP tasks with a ready confirmed MR waiting for a reviewer, among tasks with a confirmed MR and known reviewer assignment; " +
			"the first-response slice counts tasks with a ready confirmed MR waiting for a human response, among tasks whose wait is known",
		Roles: _completeRoles, Inputs: _crossInputs,
	},
	{
		ID: "X02", Version: 1, Name: "Tasks in progress with CI caveats", Unit: UnitRatio, CatalogRef: _crossCatalog,
		Estimators: []string{_ciFailed, _ciRetried, _ciMissingReport},
		Formula: "distinct WIP tasks with a failed or timed out current pipeline, among tasks with a known terminal pipeline on the current linked head; " +
			"the retry slice counts successful pipelines with evidence of retries among assessable successful pipelines; expected reports require a profile",
		Roles: _completeRoles, Inputs: _crossInputs,
	},
}

var _crossInputs = []string{InputDefinition, InputMapping, InputStructure, InputAccounts, InputWatermark}
