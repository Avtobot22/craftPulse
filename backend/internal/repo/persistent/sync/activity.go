package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func activityUUIDs(ids []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		out[i] = uuid.UUID(id)
	}
	return out
}

func (r *Repo) SaveActivityChecks(ctx context.Context, org, run entity.ID, checks []entity.ActivityHistoryCheck) error {
	const batchSize = 500
	for start := 0; start < len(checks); start += batchSize {
		batch := checks[start:min(start+batchSize, len(checks))]
		args := make([]SaveActivityCheckParams, len(batch))
		for i, check := range batch {
			args[i] = SaveActivityCheckParams{
				OrganizationID: uuid.UUID(org), RunID: uuid.UUID(run), ScopeID: uuid.UUID(check.ScopeID),
				ObjectKind: check.Key.ObjectKind, NativeNs: check.Key.NativeNS, NativeID: check.Key.NativeID, Matches: check.Matches,
			}
		}
		var batchErr error
		New(postgres.Conn(ctx, r.pool)).SaveActivityCheck(ctx, args).Exec(func(_ int, err error) {
			if batchErr == nil {
				batchErr = err
			}
		})
		if batchErr != nil {
			return fmt.Errorf("sync: save activity checks: %w", batchErr)
		}
	}
	return nil
}

func (r *Repo) SaveActivityPagination(ctx context.Context, org, scope, receipt entity.ID, kind string, at time.Time, complete bool) error {
	if err := New(postgres.Conn(ctx, r.pool)).SaveActivityPagination(ctx, SaveActivityPaginationParams{
		OrganizationID: uuid.UUID(org), ScopeID: uuid.UUID(scope), ReceiptID: uuid.UUID(receipt), DataKind: kind, ObservedAt: at, Complete: complete,
	}); err != nil {
		return fmt.Errorf("sync: save activity pagination: %w", err)
	}
	return nil
}

func (r *Repo) SaveActivityRecovery(ctx context.Context, org, scope, receipt entity.ID, object entity.ObjectKey, at time.Time) error {
	if err := New(postgres.Conn(ctx, r.pool)).SaveActivityRecovery(ctx, SaveActivityRecoveryParams{
		OrganizationID: uuid.UUID(org), ScopeID: uuid.UUID(scope), ReceiptID: uuid.UUID(receipt), ObjectKind: object.ObjectKind,
		NativeNs: object.NativeNS, NativeID: object.NativeID, ObservedAt: at,
	}); err != nil {
		return fmt.Errorf("sync: save recovered activity history: %w", err)
	}
	return nil
}

func (r *Repo) ActivityReconciliations(ctx context.Context, org entity.ID, connections []entity.ID, from, at time.Time) ([]entity.ReconcileRun, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityReconciliations(ctx, ActivityReconciliationsParams{
		OrganizationID: uuid.UUID(org), ConnectionIds: activityUUIDs(connections), Cutoff: at, WindowFrom: from,
	})
	if err != nil {
		return nil, fmt.Errorf("sync: read activity reconciliations: %w", err)
	}
	out := make([]entity.ReconcileRun, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i], _, _ = reconcileRun(*row, nil)
	}
	return out, nil
}

func (r *Repo) ActivityChecks(ctx context.Context, org entity.ID, scopes []entity.AreaSource, at time.Time) ([]entity.ActivityCheckResult, error) {
	scopesIDs := make([]uuid.UUID, 0, len(scopes))
	connections := make([]uuid.UUID, 0, len(scopes))
	seen := make(map[entity.ID]bool)
	for i := range scopes {
		scope := &scopes[i]
		scopesIDs = append(scopesIDs, uuid.UUID(scope.ScopeID))
		if !seen[scope.ConnectionID] {
			connections = append(connections, uuid.UUID(scope.ConnectionID))
			seen[scope.ConnectionID] = true
		}
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityChecks(ctx, ActivityChecksParams{
		OrganizationID: uuid.UUID(org), ConnectionIds: connections, ScopeIds: scopesIDs, Cutoff: at,
	})
	if err != nil {
		return nil, fmt.Errorf("sync: read activity checks: %w", err)
	}
	out := make([]entity.ActivityCheckResult, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.ActivityCheckResult{
			ActivityHistoryCheck: entity.ActivityHistoryCheck{
				ScopeID: entity.ID(row.ScopeID), Key: entity.ObjectKey{ObjectKind: row.ObjectKind, NativeNS: row.NativeNs, NativeID: row.NativeID},
				Matches: row.Matches,
			},
			RunID: entity.ID(row.RunID), CheckedAt: row.FinishedAt.Time, PaginationReceipt: entity.ID(row.PaginationReceipt),
		}
	}
	return out, nil
}

func (r *Repo) ActivityRecoveries(ctx context.Context, org entity.ID, scopes []entity.ID, at time.Time) ([]entity.ActivityHistoryNotice, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityRecoveries(ctx, ActivityRecoveriesParams{
		OrganizationID: uuid.UUID(org), ScopeIds: activityUUIDs(scopes), Cutoff: at,
	})
	if err != nil {
		return nil, fmt.Errorf("sync: read activity recoveries: %w", err)
	}
	out := make([]entity.ActivityHistoryNotice, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.ActivityHistoryNotice{
			ScopeID: entity.ID(row.ScopeID), ReceiptID: entity.ID(row.ReceiptID), ObservedAt: row.ObservedAt,
			Key: entity.ObjectKey{ObjectKind: row.ObjectKind, NativeNS: row.NativeNs, NativeID: row.NativeID},
		}
	}
	return out, nil
}
