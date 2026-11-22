package calc

import (
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _activityCatalog = "docs/product/metrics/activity.md#"

// Estimators of A22: the kind of the participant of an event.
const (
	ActorHuman   = string(entity.PersonHuman)
	ActorBot     = string(entity.PersonBot)
	ActorSystem  = string(entity.PersonSystem)
	ActorUnknown = string(entity.PersonUnknown)
)

// _reviewInputs are the inputs of the review metrics: the confirmed kinds of accounts decide who responded.
var _reviewInputs = []string{InputDefinition, InputStructure, InputAccounts}

var _reviewDefinitions = []Definition{
	{
		ID: "A05", Version: 1, Name: "Ready MRs waiting", Unit: UnitCount, CatalogRef: _activityCatalog + "a05",
		Formula: "MRs open and out of draft at t = end of the point; the kind of the review wait of each is its reason",
		Roles:   []string{"numerator"}, Inputs: _reviewInputs,
	},
	{
		ID: "A06", Version: 1, Name: "First review wait age", Unit: UnitSeconds, CatalogRef: _activityCatalog + "a06",
		Estimators: []string{EstimatorP50, EstimatorP85},
		Formula:    "t - ready_at of the ready open MRs without a first human response at t = end of the point, from the opening when ready is unknown",
		Roles:      []string{"numerator", "missing"}, Inputs: _reviewInputs,
	},
	{
		ID: "A07", Version: 1, Name: "Time to first human review", Unit: UnitSeconds, CatalogRef: _activityCatalog + "a07",
		Estimators: []string{EstimatorP50, EstimatorP85},
		Formula: "first qualifying human review - ready_at of the MRs first responded in [from,to), nearest-rank percentile; " +
			"with comments only it is the time to the first external response",
		Roles: []string{"numerator"}, Inputs: _reviewInputs,
	},
	{
		ID: "A10", Version: 1, Name: "MRs without a reviewer", Unit: UnitRatio, CatalogRef: _activityCatalog + "a10",
		Formula: "ready open MRs without an assigned reviewer / ready open MRs with the field of reviewers available at t = end of the point; " +
			"MRs without the field are excluded",
		Roles: []string{"numerator", "denominator", "excluded"}, Inputs: _reviewInputs,
	},
}

var _eventDefinitions = []Definition{
	{
		ID: "A22", Version: 1, Name: "Event history", Unit: UnitCount, CatalogRef: _activityCatalog + "a22",
		Estimators: []string{ActorHuman, ActorBot, ActorSystem, ActorUnknown},
		Formula: "deduplicated events of tasks and MRs in [from,to) by the kind of their participant; the kind of each event is its " +
			"reason, and the series of kinds are not summed",
		Roles: []string{"numerator"}, Inputs: _reviewInputs,
	},
}

// definitionsOf are the definitions a set of metrics computes.
func definitionsOf(set string) []Definition {
	switch set {
	case entity.MetricSetFlow:
		return slices.Concat(_definitions, _reviewDefinitions, _eventDefinitions, _crossDefinitions)
	case entity.MetricSetReview:
		return _reviewDefinitions
	case entity.MetricSetEvents:
		return _eventDefinitions
	case entity.MetricSetLinks:
		return _linkDefinitions
	case entity.MetricSetCI:
		return _ciDefinitions
	case entity.MetricSetActivity:
		return _activityDefinitions
	case entity.MetricSetCode:
		return _codeDefinitions
	case entity.MetricSetDocumentation:
		return _documentationDefinitions
	}
	return _definitions
}

// SetDefinitions are the definitions in force a set of metrics computes.
func SetDefinitions(set string) []Definition {
	return slices.Clone(definitionsOf(set))
}

// registry is every definition in force.
func registry() []Definition {
	return slices.Concat(_definitions, _reviewDefinitions, _eventDefinitions, _linkDefinitions, _ciDefinitions, _activityDefinitions,
		_codeDefinitions, _testCoverageDefinitions, _crossDefinitions, _documentationDefinitions)
}
