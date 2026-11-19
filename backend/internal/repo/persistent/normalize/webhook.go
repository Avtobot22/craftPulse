package normalize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) WebhookTargets(ctx context.Context, org, instance entity.ID, keys []entity.ObjectKey) (map[entity.ObjectKey]entity.WebhookMRTarget, error) {
	type lookupKey struct {
		ObjectKind string `json:"object_kind"`
		NativeNS   string `json:"native_ns"`
		NativeID   string `json:"native_id"`
	}
	input := make([]lookupKey, len(keys))
	for i, k := range keys {
		input[i] = lookupKey{k.ObjectKind, k.NativeNS, k.NativeID}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("normalize: encode webhook keys: %w", err)
	}
	p := WebhookTargetsParams{OrganizationID: uuid.UUID(org), InstanceID: uuid.UUID(instance), Keys: raw}
	rows, err := r.q(ctx).WebhookTargets(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("normalize: webhook targets: %w", err)
	}
	out := make(map[entity.ObjectKey]entity.WebhookMRTarget, len(rows))
	for _, row := range rows {
		key := entity.ObjectKey{ObjectKind: row.ObjectKind, NativeNS: row.InputNs, NativeID: row.InputID}
		target := entity.WebhookMRTarget{Key: key, ScopeNativeID: row.ScopeNativeID, Open: row.State == "open" || row.State == "" && key.ObjectKind == "merge_request"}
		if target.Open {
			target.Key = entity.ObjectKey{ObjectKind: "merge_request", NativeNS: row.NativeNs, NativeID: row.NativeID}
		}
		out[key] = target
	}
	return out, nil
}

func (r *Repo) CurrentSyncVersions(ctx context.Context, org, area entity.ID, nativeIDs []string) (map[entity.ObjectKey]entity.SyncObjectVersion, error) {
	rows, err := r.q(ctx).CurrentSyncVersions(ctx, CurrentSyncVersionsParams{OrganizationID: uuid.UUID(org), AreaID: uuid.UUID(area), Column3: nativeIDs})
	if err != nil {
		return nil, fmt.Errorf("normalize: current sync versions: %w", err)
	}
	out := make(map[entity.ObjectKey]entity.SyncObjectVersion, len(rows))
	for _, row := range rows {
		out[entity.ObjectKey{ObjectKind: row.ObjectKind, NativeNS: row.NativeNs, NativeID: row.NativeID}] = entity.SyncObjectVersion{Revision: row.SourceRevision, UpdatedAt: timePtr(row.SourceUpdatedAt)}
	}
	return out, nil
}
