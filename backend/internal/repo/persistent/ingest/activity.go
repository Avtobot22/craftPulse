package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func activityUUIDs(ids []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		out[i] = uuid.UUID(id)
	}
	return out
}

func (r *Repo) ActivityReceiptConnections(ctx context.Context, org entity.ID, receipts []entity.ID) (map[entity.ID]entity.ID, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityReceiptConnections(ctx, ActivityReceiptConnectionsParams{
		OrganizationID: uuid.UUID(org), ReceiptIds: activityUUIDs(receipts),
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: read activity receipt connections: %w", err)
	}
	out := make(map[entity.ID]entity.ID, len(rows))
	for _, row := range rows {
		out[entity.ID(row.ID)] = entity.ID(row.ConnectionID.UUID)
	}
	return out, nil
}

func (r *Repo) ActivityTruncations(ctx context.Context, org entity.ID, scopes []entity.ID, at time.Time) ([]entity.ActivityHistoryNotice, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityTruncationManifests(ctx, ActivityTruncationManifestsParams{
		OrganizationID: uuid.UUID(org), ScopeIds: activityUUIDs(scopes), Cutoff: at,
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: read history truncations: %w", err)
	}
	var out []entity.ActivityHistoryNotice
	for _, row := range rows {
		var m envelope.Manifest
		if err := json.Unmarshal(row.Manifest, &m); err != nil {
			return nil, fmt.Errorf("ingest: read truncations of receipt %s: %w", row.ID, err)
		}
		for _, mark := range m.Selection.Truncated {
			if mark.RecordKind == "task" || mark.RecordKind == "merge_request" {
				out = append(out, entity.ActivityHistoryNotice{
					ScopeID: entity.ID(row.ScopeID.UUID), ReceiptID: entity.ID(row.ID), ObservedAt: row.ReceivedAt, HistoryFrom: mark.HistoryFrom,
					Key: entity.ObjectKey{ObjectKind: string(mark.RecordKind), NativeNS: mark.NativeNs, NativeID: mark.NativeId},
				})
			}
		}
	}
	return out, nil
}

func (r *Repo) ActivityCompleteReceipts(ctx context.Context, org entity.ID, receipts []entity.ID) (map[entity.ID]bool, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).ActivityCompleteReceipts(ctx, ActivityCompleteReceiptsParams{
		OrganizationID: uuid.UUID(org), ReceiptIds: activityUUIDs(receipts),
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: validate history receipts: %w", err)
	}
	out := make(map[entity.ID]bool, len(rows))
	for _, id := range rows {
		out[entity.ID(id)] = true
	}
	return out, nil
}
