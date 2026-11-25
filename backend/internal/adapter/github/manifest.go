package github

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	if in.SchemaId != adaptersdk.ManifestInputSchemaIdMergeRequests {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitHub manifest serves merge_requests")
	}
	if in.Limit < 1 || in.Limit > 10000 {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub Manifest limit")
	}
	var cursor *adaptersdk.Cursor
	if in.Cursor != nil {
		raw, err := base64.RawURLEncoding.DecodeString(*in.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor == nil || cursor.NativeId == "" || cursor.ModifiedAt.IsZero() {
			return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub manifest cursor")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	repo, err := s.repository(ctx, in.Scope)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	items, err := s.selectPulls(ctx, repo, in.Period, cursor, nil, in.Limit+1)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	out := adaptersdk.ManifestPage{Items: make([]adaptersdk.ManifestItem, 0, min(len(items), in.Limit))}
	for _, p := range items[:min(len(items), in.Limit)] {
		out.Items = append(out.Items, adaptersdk.ManifestItem{RecordKind: "merge_request", NativeId: id(p.ID), UpdatedAt: &p.UpdatedAt})
	}
	if len(items) > in.Limit {
		last := items[in.Limit-1]
		raw, err := json.Marshal(adaptersdk.Cursor{NativeId: id(last.ID), ModifiedAt: last.UpdatedAt})
		if err != nil {
			return adaptersdk.ManifestPage{}, err
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &next
	}
	return out, nil
}
