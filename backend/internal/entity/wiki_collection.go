package entity

import "time"

// WikiReconcileScope counts a complete enumeration of one source manifest.
type WikiReconcileScope struct {
	ScopeID  ID
	Failed   bool
	Expected int
	Read     int
}

// WikiCollection is the evidence for D27 at a knowledge and time cutoff.
type WikiCollection struct {
	RunID           ID
	Failed          bool
	ConnectionID    ID
	KnownSeq        int64
	Expected        int
	Read            int
	LastSuccess     *time.Time
	EarliestVersion *time.Time
}
