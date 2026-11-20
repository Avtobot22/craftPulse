package entity

// MetricSetLinks is the set of the coverage of the links of tasks and MRs of a team (A18, A20, T28).
const MetricSetLinks = "task_mr_links"

// Reasons of the points and contributions of the coverage of links.
const (
	// ReasonCodePolicyMissing marks A20 and T28 of a team whose mapping has no code_required_type rules.
	ReasonCodePolicyMissing = "code_required_policy_missing"
	// ReasonNotCodeRequired marks a task whose type needs no MR: T28 is NOT_APPLICABLE for it.
	ReasonNotCodeRequired = "not_code_required"
	// ReasonMRUnknown marks a task whose confirmed MR is outside the repositories of the team: its merge is unknown.
	ReasonMRUnknown = "mr_unknown"
)
