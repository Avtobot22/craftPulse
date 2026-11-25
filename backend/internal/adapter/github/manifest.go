package github

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	items, err := s.reconcilePulls(ctx, repo, in, cursor)
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

// An update can move an unread PR to a page we already visited. Compare the
// entire scanned prefix, including items outside the requested upper bound,
// before publishing a page or advancing its cursor.
func (s source) reconcilePulls(ctx context.Context, repo repository, in adaptersdk.ManifestInput, cursor *adaptersdk.Cursor) ([]coordinate, error) {
	var previous []byte
	for range 4 {
		fingerprint := sha256.New()
		items, err := s.selectPulls(ctx, repo, in.Period, cursor, nil, in.Limit+1, fingerprint)
		if err != nil {
			return nil, err
		}
		current := fingerprint.Sum(nil)
		if bytes.Equal(previous, current) {
			return items, nil
		}
		previous = current
	}
	return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "GitHub pull listing changed during manifest reconciliation")
}
