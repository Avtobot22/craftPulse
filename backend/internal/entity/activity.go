package entity

import "time"

const MetricSetActivity = "activity_completeness"

type ActivityScope struct {
	TeamID       ID
	ConnectionID ID
}

type ActivityHistory struct {
	ObjectID         ID
	ConnectionID     ID
	Kind             ObjectKind
	OpenedAt         time.Time
	CheckedAt        time.Time
	Checked          bool
	Matches          bool
	Complete         bool
	Truncated        bool
	HistoryFrom      *time.Time
	ReconcileID      ID
	HistoryReceiptID ID
}

type ActivityHistoryCheck struct {
	ScopeID ID
	Key     ObjectKey
	Matches bool
}

type ActivityCheckResult struct {
	ActivityHistoryCheck
	RunID             ID
	CheckedAt         time.Time
	PaginationReceipt ID
}

type ActivityHistoryNotice struct {
	ScopeID     ID
	Key         ObjectKey
	ReceiptID   ID
	ObservedAt  time.Time
	HistoryFrom *time.Time
}

type ActivityHistoryEvidence struct {
	Source   AreaSource
	Check    *ActivityCheckResult
	Cut      *ActivityHistoryNotice
	Recovery *ActivityHistoryNotice
	Key      ObjectKey
}
