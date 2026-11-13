package adapterapi

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (c *Caller) PutCredentials(ctx context.Context, t entity.AdapterTarget, in entity.CredentialsInput) error {
	cl, err := c.prepare(ctx, t, entity.OpPutCredentials)
	if err != nil {
		return err
	}
	body := CredentialsInput{Purpose: CredentialsInputPurpose(in.Purpose), RedirectUri: optString(in.RedirectURI), UserId: optUUID(in.UserID)}
	if in.Secret != "" {
		v := in.Secret.Value()
		body.Secret = &v
	}
	if in.OAuthCode != "" {
		v := in.OAuthCode.Value()
		body.OauthCode = &v
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusNoContent, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.PutCredentials(ctx, cl.conn, &PutCredentialsParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, body, cl.authorize)
	})
	if err != nil {
		return err
	}
	discard(resp, cancel)
	return c.record(t, cl.op, nil)
}

func (c *Caller) DeleteCredentials(ctx context.Context, t entity.AdapterTarget, f entity.CredentialsFilter) error {
	cl, err := c.prepare(ctx, t, entity.OpDeleteCredentials)
	if err != nil {
		return err
	}
	params := &DeleteCredentialsParams{UserId: optUUID(f.UserID), XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}
	if f.Purpose != "" {
		p := DeleteCredentialsParamsPurpose(f.Purpose)
		params.Purpose = &p
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusNoContent, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.DeleteCredentials(ctx, cl.conn, params, cl.authorize)
	})
	if err != nil {
		return err
	}
	discard(resp, cancel)
	return c.record(t, cl.op, nil)
}

func (c *Caller) Probe(ctx context.Context, t entity.AdapterTarget, in entity.ProbeInput) (entity.ProbeResult, error) {
	cl, err := c.prepare(ctx, t, entity.OpProbe)
	if err != nil {
		return entity.ProbeResult{}, err
	}
	body := ProbeInput{BaseUrl: in.BaseURL, Scopes: make([]ScopeRef, 0, len(in.Scopes))}
	for _, s := range in.Scopes {
		body.Scopes = append(body.Scopes, ScopeRef{Kind: ScopeRefKind(s.Kind), NativeScopeId: s.NativeScopeID})
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.Probe(ctx, cl.conn, &ProbeParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, body, cl.authorize)
	})
	if err != nil {
		return entity.ProbeResult{}, err
	}
	defer func() {
		_ = resp.Body.Close()
		cancel()
	}()
	var res ProbeResult
	if err := decodeJSON(resp, &res); err != nil {
		return entity.ProbeResult{}, c.record(t, cl.op, err)
	}
	out, err := probeResult(res)
	return out, c.record(t, cl.op, err)
}

func probeResult(res ProbeResult) (entity.ProbeResult, error) {
	if len(res.ContractVersions) == 0 || res.InstanceKey == "" || !res.Provider.Valid() ||
		(res.AccessMode != nil && !res.AccessMode.Valid()) {
		return entity.ProbeResult{}, violation("probe result misses versions, instance key or a known provider")
	}
	out := entity.ProbeResult{
		ContractVersions: res.ContractVersions, Provider: entity.Provider(res.Provider), InstanceKey: res.InstanceKey,
		Edition: deref(res.Edition), Version: deref(res.Version),
		Capabilities: make([]entity.Capability, 0, len(res.Capabilities)), Scopes: make([]entity.ScopeCheck, 0, len(res.Scopes)),
	}
	if res.AccessMode != nil {
		out.AccessMode = entity.AccessMode(*res.AccessMode)
	}
	for _, cp := range res.Capabilities {
		if !cp.Capability.Valid() || !cp.State.Valid() {
			return entity.ProbeResult{}, violation("probe result has an unknown capability or state")
		}
		out.Capabilities = append(out.Capabilities, entity.Capability{
			Name: string(cp.Capability), State: entity.CapabilityState(cp.State), ReasonCode: deref(cp.ReasonCode),
			CheckedAt: cp.CheckedAt, SourceVersion: out.Version,
		})
	}
	for _, s := range res.Scopes {
		if !s.Kind.Valid() || !s.State.Valid() {
			return entity.ProbeResult{}, violation("probe result has an unknown scope kind or state")
		}
		out.Scopes = append(out.Scopes, entity.ScopeCheck{
			Kind: entity.ScopeKind(s.Kind), NativeScopeID: s.NativeScopeId, DisplayName: deref(s.DisplayName),
			Verified: s.State == ScopeCheckStateVerified, ReasonCode: deref(s.ReasonCode),
		})
	}
	return out, nil
}

func (c *Caller) Fetch(ctx context.Context, t entity.AdapterTarget, in entity.FetchInput) (io.ReadCloser, error) {
	cl, err := c.prepare(ctx, t, entity.OpFetch)
	if err != nil {
		return nil, err
	}
	body := FetchInput{
		SchemaId: FetchInputSchemaId(in.SchemaID), Scope: ScopeRef{Kind: ScopeRefKind(in.Scope.Kind), NativeScopeId: in.Scope.NativeScopeID},
		Period: Period{From: in.Period.From, To: in.Period.To}, Limit: in.Limit, ObjectKeys: make([]ObjectKey, 0, len(in.ObjectKeys)),
	}
	if in.Cursor != nil {
		body.Cursor = &Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeID}
	}
	for _, k := range in.ObjectKeys {
		body.ObjectKeys = append(body.ObjectKeys, ObjectKey{ObjectKind: ObjectKeyObjectKind(k.ObjectKind), NativeNs: k.NativeNS, NativeId: k.NativeID})
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.Fetch(ctx, cl.conn, &FetchParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, body, cl.authorize)
	})
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/gzip") {
		discard(resp, cancel)
		return nil, c.record(t, cl.op, violation("fetch answer is not application/gzip"))
	}
	return &stream{rc: resp.Body, cancel: cancel}, c.record(t, cl.op, nil)
}

// stream is the package body of Fetch; the deadline of the call holds until it is closed.
type stream struct {
	rc     io.ReadCloser
	cancel context.CancelFunc
}

func (s *stream) Read(p []byte) (int, error) { return s.rc.Read(p) }

func (s *stream) Close() error {
	err := s.rc.Close()
	s.cancel()
	return err
}

func (c *Caller) Manifest(ctx context.Context, t entity.AdapterTarget, in entity.ManifestInput) (entity.ManifestPage, error) {
	cl, err := c.prepare(ctx, t, entity.OpManifest)
	if err != nil {
		return entity.ManifestPage{}, err
	}
	body := ManifestInput{
		SchemaId: ManifestInputSchemaId(in.SchemaID), Scope: ScopeRef{Kind: ScopeRefKind(in.Scope.Kind), NativeScopeId: in.Scope.NativeScopeID},
		Period: Period{From: in.Period.From, To: in.Period.To}, Cursor: optString(in.Cursor), Limit: in.Limit,
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.Manifest(ctx, cl.conn, &ManifestParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, body, cl.authorize)
	})
	if err != nil {
		return entity.ManifestPage{}, err
	}
	defer func() {
		_ = resp.Body.Close()
		cancel()
	}()
	var page ManifestPage
	if err := decodeJSON(resp, &page); err != nil {
		return entity.ManifestPage{}, c.record(t, cl.op, err)
	}
	out := entity.ManifestPage{Items: make([]entity.ManifestItem, 0, len(page.Items)), NextCursor: deref(page.NextCursor), TotalCount: page.TotalCount}
	for _, it := range page.Items {
		if it.NativeId == "" || (it.Version == nil && it.UpdatedAt == nil) {
			return entity.ManifestPage{}, c.record(t, cl.op, violation("manifest item has no ID or neither version nor update time"))
		}
		out.Items = append(out.Items, entity.ManifestItem{RecordKind: it.RecordKind, NativeNS: it.NativeNs, NativeID: it.NativeId, Version: deref(it.Version), UpdatedAt: it.UpdatedAt})
	}
	return out, c.record(t, cl.op, nil)
}

func (c *Caller) CheckAccess(ctx context.Context, t entity.AdapterTarget, in entity.SourceAccessInput) (entity.SourceAccessResult, error) {
	cl, err := c.prepare(ctx, t, entity.OpCheckAccess)
	if err != nil {
		return entity.SourceAccessResult{}, err
	}
	body := CheckAccessInput{UserId: uuid.UUID(in.UserID), AccountNativeId: in.AccountNativeID, Items: make([]AccessItem, 0, len(in.Items))}
	for _, it := range in.Items {
		item := AccessItem{NativeNs: it.NativeNS, NativeId: it.NativeID}
		if it.AreaKind != "" {
			kind := AccessItemAreaKind(it.AreaKind)
			item.ItemKind, item.AreaKind = AccessItemItemKindArea, &kind
		} else {
			kind := AccessItemObjectKind(it.ObjectKind)
			item.ItemKind, item.ObjectKind = AccessItemItemKindObject, &kind
		}
		body.Items = append(body.Items, item)
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.CheckAccess(ctx, cl.conn, &CheckAccessParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, body, cl.authorize)
	})
	if err != nil {
		return entity.SourceAccessResult{}, err
	}
	defer func() {
		_ = resp.Body.Close()
		cancel()
	}()
	var res CheckAccessResult
	if err := decodeJSON(resp, &res); err != nil {
		return entity.SourceAccessResult{}, c.record(t, cl.op, err)
	}
	if !res.Mode.Valid() || len(res.Decisions) != len(in.Items) {
		return entity.SourceAccessResult{}, c.record(t, cl.op, violation("check access answer has an unknown mode or not one decision per item"))
	}
	out := entity.SourceAccessResult{
		Mode: entity.AccessMode(res.Mode), CheckedAt: res.CheckedAt, MaxAgeSeconds: res.MaxAgeSeconds,
		Decisions: make([]entity.SourceAccessDecision, 0, len(res.Decisions)),
	}
	for _, d := range res.Decisions {
		if !d.Decision.Valid() {
			return entity.SourceAccessResult{}, c.record(t, cl.op, violation("check access answer has an unknown decision"))
		}
		out.Decisions = append(out.Decisions, entity.SourceAccessDecision{Decision: string(d.Decision), CheckedAt: d.CheckedAt, ReasonCode: deref(d.ReasonCode)})
	}
	return out, c.record(t, cl.op, nil)
}

func discard(resp *http.Response, cancel context.CancelFunc) {
	_, _ = io.CopyN(io.Discard, resp.Body, _drainBytes)
	_ = resp.Body.Close()
	cancel()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optUUID(id *entity.ID) *uuid.UUID {
	if id == nil {
		return nil
	}
	u := uuid.UUID(*id)
	return &u
}
