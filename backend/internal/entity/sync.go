package entity

import "time"

type SyncPhase string

const (
	SyncBackfill    SyncPhase = "backfill"
	SyncIncremental SyncPhase = "incremental"
	SyncPaused      SyncPhase = "paused"
	SyncStopped     SyncPhase = "stopped"
)

// SyncState is the state of collection of a data kind of a connection; Cursor is opaque to everyone but sync.
type SyncState struct {
	ConnectionID  ID
	DataKind      string
	Phase         SyncPhase
	Cursor        string
	CheckpointAt  *time.Time
	FailedCursor  string
	LastAttemptAt *time.Time
	LastOutcome   string
	LastErrorCode string
	LastSuccessAt *time.Time
	Watermark     *time.Time
	HistoryFrom   *time.Time
	Completeness  string
}
