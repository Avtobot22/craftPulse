package sync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCollectionStatus(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	before, after := since.Add(-time.Minute), since.Add(time.Minute)
	tests := []struct {
		name      string
		giveState entity.SyncState
		giveFound bool
		giveBusy  bool
		want      entity.OperationStatus
		wantCode  string
	}{
		{name: "no state yet", giveFound: false, want: entity.OperationQueued},
		{name: "last attempt before the request", giveState: entity.SyncState{LastAttemptAt: &before}, giveFound: true, giveBusy: true, want: entity.OperationQueued},
		{name: "round in progress", giveState: entity.SyncState{LastAttemptAt: &after}, giveFound: true, giveBusy: true, want: entity.OperationRunning},
		{name: "round ended", giveState: entity.SyncState{LastAttemptAt: &after}, giveFound: true, want: entity.OperationSucceeded},
		{
			name: "stopped by auth", giveState: entity.SyncState{LastAttemptAt: &after, Phase: entity.SyncStopped, LastErrorCode: entity.SourceAuth},
			giveFound: true, want: entity.OperationFailed, wantCode: entity.OperationAccessLost,
		},
		{
			name: "stopped by the adapter", giveState: entity.SyncState{LastAttemptAt: &after, Phase: entity.SyncStopped, LastErrorCode: "source:not_supported"},
			giveFound: true, want: entity.OperationFailed, wantCode: entity.OperationSourceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := collectionStatus(tt.giveState, tt.giveFound, tt.giveBusy, since)
			assert.Equal(t, tt.want, got.Status)
			assert.Equal(t, tt.wantCode, got.ErrorCode)
		})
	}
}

func TestReconcileStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		giveOutcome string
		giveReason  string
		want        entity.OperationStatus
		wantCode    string
	}{
		{name: "running", giveOutcome: entity.ReconcileRunning, want: entity.OperationRunning},
		{name: "complete", giveOutcome: entity.ReconcileComplete, want: entity.OperationSucceeded},
		{name: "differences found", giveOutcome: entity.ReconcileIncomplete, want: entity.OperationSucceeded},
		{name: "failed", giveOutcome: entity.ReconcileFailed, giveReason: "permission_missing", want: entity.OperationFailed, wantCode: entity.OperationAccessLost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := reconcileStatus(entity.ReconcileRun{Outcome: tt.giveOutcome, ReasonCode: tt.giveReason})
			assert.Equal(t, tt.want, got.Status)
			assert.Equal(t, tt.wantCode, got.ErrorCode)
		})
	}
}

func TestUnsupported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		giveS entity.SyncState
		want  bool
	}{
		{name: "not served", giveS: entity.SyncState{Phase: entity.SyncStopped, LastErrorCode: "source:not_supported"}, want: true},
		{name: "old contract", giveS: entity.SyncState{Phase: entity.SyncStopped, LastErrorCode: "contract_unsupported:version"}, want: true},
		{name: "stopped by auth", giveS: entity.SyncState{Phase: entity.SyncStopped, LastErrorCode: "auth"}, want: false},
		{name: "collecting", giveS: entity.SyncState{Phase: entity.SyncIncremental}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, unsupported(&tt.giveS))
		})
	}
}
