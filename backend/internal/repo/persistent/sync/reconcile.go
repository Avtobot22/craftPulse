package sync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// States lists the states of every data kind of a connection, or of the organization when connectionID is zero.
func (r *Repo) States(ctx context.Context, orgID, connectionID entity.ID) ([]entity.SyncState, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ListStates(ctx, ListStatesParams{
		OrganizationID: uuid.UUID(orgID), OneConnection: !connectionID.IsZero(), ConnectionID: uuid.UUID(connectionID),
	})
	if err != nil {
		return nil, fmt.Errorf("sync: list states: %w", err)
	}
	out := make([]entity.SyncState, len(rows))
	for i := range rows {
		out[i] = syncState(&rows[i])
	}
	return out, nil
}

func (r *Repo) LeaseHeld(ctx context.Context, orgID, connectionID entity.ID, now time.Time) (bool, error) {
	held, err := New(postgres.Conn(ctx, r.pool)).LeaseHeld(ctx, LeaseHeldParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), Now: now})
	if err != nil {
		return false, fmt.Errorf("sync: lease held: %w", err)
	}
	return held, nil
}

// StartReconcile writes a running reconciliation; false when one of the data kind already runs.
func (r *Repo) StartReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string, at time.Time) (entity.ID, bool, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).StartReconcile(ctx, StartReconcileParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), DataKind: dataKind, At: at,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, false, nil
	}
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("sync: start reconcile: %w", err)
	}
	return entity.ID(id), true, nil
}

func (r *Repo) RunningReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.ReconcileRun, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).RunningReconcile(ctx, RunningReconcileParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), DataKind: dataKind,
	})
	return reconcileRun(row, err)
}

func (r *Repo) Reconcile(ctx context.Context, orgID, runID entity.ID) (entity.ReconcileRun, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetReconcile(ctx, GetReconcileParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(runID)})
	return reconcileRun(row, err)
}

// LastReconcile is the latest reconciliation of a connection, of one data kind when dataKind is set.
func (r *Repo) LastReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.ReconcileRun, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).LastReconcile(ctx, LastReconcileParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), OneKind: dataKind != "", DataKind: dataKind,
	})
	return reconcileRun(row, err)
}

func (r *Repo) FinishReconcile(ctx context.Context, orgID entity.ID, run entity.ReconcileRun) error {
	err := New(postgres.Conn(ctx, r.pool)).FinishReconcile(ctx, FinishReconcileParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(run.ID), AsOf: run.AsOf, StartedAt: run.StartedAt, FinishedAt: *run.FinishedAt,
		ExpectedCount: count(run.ExpectedCount),
		ObservedCount: count(run.ObservedCount), MissingCount: count(run.MissingCount), ChangedCount: count(run.ChangedCount),
		Outcome: run.Outcome, ReasonCode: text(run.ReasonCode),
	})
	if err != nil {
		return fmt.Errorf("sync: finish reconcile: %w", err)
	}
	return nil
}

func reconcileRun(row SyncReconcileRun, err error) (entity.ReconcileRun, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ReconcileRun{}, false, nil
	}
	if err != nil {
		return entity.ReconcileRun{}, false, fmt.Errorf("sync: read reconcile: %w", err)
	}
	return entity.ReconcileRun{
		ID: entity.ID(row.ID), ConnectionID: entity.ID(row.ConnectionID), DataKind: row.DataKind, AsOf: row.AsOf, StartedAt: row.StartedAt,
		FinishedAt: timePtr(row.FinishedAt), ExpectedCount: intPtr(row.ExpectedCount), ObservedCount: intPtr(row.ObservedCount),
		MissingCount: intPtr(row.MissingCount), ChangedCount: intPtr(row.ChangedCount), Outcome: row.Outcome, ReasonCode: row.ReasonCode.String,
	}, true, nil
}

// dirtyRef is native_ref of a key: the scope and the data kind it is read with and the object.
type dirtyRef struct {
	ScopeID  entity.ID `json:"scope_id"`
	DataKind string    `json:"data_kind"`
	NativeNS string    `json:"native_ns"`
	NativeID string    `json:"native_id"`
}

// AddDirtyKeys puts objects into the set; a key already there moves its added_at to at, so a key added
// while the set is read is not deleted with the read ones.
func (r *Repo) AddDirtyKeys(ctx context.Context, orgID, connectionID entity.ID, keys []entity.DirtyKey, at time.Time) error {
	if len(keys) == 0 {
		return nil
	}
	params := make([]AddDirtyKeyParams, len(keys))
	for i := range keys {
		k := &keys[i]
		ref, err := json.Marshal(dirtyRef{ScopeID: k.ScopeID, DataKind: k.DataKind, NativeNS: k.Object.NativeNS, NativeID: k.Object.NativeID})
		if err != nil {
			return fmt.Errorf("sync: encode dirty key: %w", err)
		}
		sum := sha256.Sum256(append([]byte(k.Object.ObjectKind+"\x00"), ref...))
		params[i] = AddDirtyKeyParams{
			OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), KeyHash: sum[:], ObjectKind: k.Object.ObjectKind,
			NativeRef: string(ref), AddedAt: at,
		}
	}
	var batchErr error
	New(postgres.Conn(ctx, r.pool)).AddDirtyKey(ctx, params).Exec(func(_ int, err error) {
		if batchErr == nil {
			batchErr = err
		}
	})
	if batchErr != nil {
		return fmt.Errorf("sync: add dirty keys: %w", batchErr)
	}
	return nil
}

func (r *Repo) DirtyKeys(ctx context.Context, orgID, connectionID entity.ID, limit int) ([]entity.DirtyKey, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).DirtyKeys(ctx, DirtyKeysParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), RowLimit: int32(min(limit, math.MaxInt32)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, fmt.Errorf("sync: dirty keys: %w", err)
	}
	out := make([]entity.DirtyKey, len(rows))
	for i, row := range rows {
		var ref dirtyRef
		if err := json.Unmarshal([]byte(row.NativeRef), &ref); err != nil {
			return nil, fmt.Errorf("sync: decode dirty key: %w", err)
		}
		out[i] = entity.DirtyKey{
			Hash: row.KeyHash, ScopeID: ref.ScopeID, DataKind: ref.DataKind, Added: row.AddedAt,
			Object: entity.ObjectKey{ObjectKind: row.ObjectKind, NativeNS: ref.NativeNS, NativeID: ref.NativeID},
		}
	}
	return out, nil
}

// DeleteDirtyKeys removes the keys read, except those added again after the read started.
func (r *Repo) DeleteDirtyKeys(ctx context.Context, orgID, connectionID entity.ID, hashes [][]byte, readStarted time.Time) error {
	_, err := New(postgres.Conn(ctx, r.pool)).DeleteDirtyKeys(ctx, DeleteDirtyKeysParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), KeyHashes: hashes, ReadStartedAt: readStarted,
	})
	if err != nil {
		return fmt.Errorf("sync: delete dirty keys: %w", err)
	}
	return nil
}

func (r *Repo) CountDirtyKeys(ctx context.Context, orgID, connectionID entity.ID) (int, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).CountDirtyKeys(ctx, CountDirtyKeysParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID)})
	if err != nil {
		return 0, fmt.Errorf("sync: count dirty keys: %w", err)
	}
	return int(n), nil
}

func count(n *int) pgtype.Int4 {
	if n == nil || *n > math.MaxInt32 || *n < 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}

func intPtr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}
