package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

const (
	_estimatorP95    = "p95"
	_historyFrom     = "history_from"
	_lastSuccessAt   = "last_success_at"
	_watermark       = "watermark"
	_lastReconcileAt = "last_reconcile_at"
	_ambiguous       = "ambiguous"
	_unambiguous     = "unambiguous"
)

var (
	_activityRoles     = []string{"numerator", "denominator", "excluded", "missing"}
	_activityInputs    = []string{InputDefinition, InputStructure, InputWatermark}
	_observationModes  = []entity.ObservedVia{entity.ViaWebhook, entity.ViaIncremental, entity.ViaBackfill, entity.ViaReconcile, entity.ViaImport}
	_activityDataKinds = []string{"accounts", "ci", "documents", "merge_requests", "permissions", "reports", "tasks"}
)

var _activityDefinitions = []Definition{
	{
		ID: "A23", Version: 1, Name: "Event delivery lag", Unit: UnitSeconds, CatalogRef: _activityCatalog + "a23",
		Estimators: deliveryEstimators(), Inputs: _activityInputs, Roles: _activityRoles,
		Formula: "nearest-rank p50 and p95 of observed_at - event_time in seconds by observation mode, with observed_at in [from,to)",
	},
	{
		ID: "A24", Version: 1, Name: "History completeness", Unit: UnitRatio, CatalogRef: _activityCatalog + "a24",
		Estimators: []string{"", _historyFrom}, Inputs: _activityInputs, Roles: _activityRoles,
		Formula: "objects with verified pagination and full history / objects checked against the latest completed source manifest, grouped by check time; history_from is the earliest proven timestamp in Unix seconds",
	},
	{
		ID: "A27", Version: 1, Name: "Collection freshness", Unit: UnitSeconds, CatalogRef: _activityCatalog + "a27",
		Estimators: freshnessEstimators(), Inputs: _activityInputs, Roles: _activityRoles,
		Formula: "last success, watermark and last reconciliation at the point end, by connection and data kind; Unix seconds; team points show the oldest known timestamp and retain each connection in contributions",
	},
	{
		ID: "A28", Version: 1, Name: "Actor kind coverage", Unit: UnitRatio, CatalogRef: _activityCatalog + "a28",
		Estimators: []string{"", ActorHuman, ActorBot, ActorSystem, ActorUnknown}, Inputs: []string{InputDefinition, InputStructure, InputAccounts}, Roles: _activityRoles,
		Formula: "events with confirmed human, bot or system accounts / events with an account, by event_time in [from,to); kind counts use the assignment at the revision knowledge cutoff",
	},
	{
		ID: "A30", Version: 1, Name: "Link quality", Unit: UnitCount, CatalogRef: _activityCatalog + "a30",
		Estimators: []string{"confirmed", "proposed", "rejected", _ambiguous, _unambiguous, "ambiguous_share"},
		Inputs:     []string{InputDefinition, InputStructure}, Roles: _activityRoles,
		Formula: "distinct task to MR and MR to CI run pairs at the revision knowledge cutoff by MR opening time; confirmed takes precedence over proposed and rejected; a proposed task pair is ambiguous if its MR has more than one possible task; ambiguous_share is ambiguous / proposed",
	},
}

func deliveryEstimators() []string {
	out := make([]string, 0, 2*len(_observationModes))
	for _, mode := range _observationModes {
		out = append(out, string(mode)+":"+EstimatorP50, string(mode)+":"+_estimatorP95)
	}
	return out
}

func freshnessEstimators() []string {
	out := make([]string, 0, 1+3*len(_activityDataKinds))
	out = append(out, _lastReconcileAt)
	for _, kind := range _activityDataKinds {
		for _, metric := range []string{_lastSuccessAt, _watermark, _lastReconcileAt} {
			out = append(out, kind+":"+metric)
		}
	}
	return out
}
