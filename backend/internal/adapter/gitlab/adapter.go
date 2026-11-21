package gitlab

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Adapter implements the read operations for a GitLab self-managed instance.
type Adapter struct {
	adaptersdk.Unimplemented
	client *adaptersdk.SourceClient
}

// New constructs the GitLab adapter around the SDK source client.
func New(client *adaptersdk.SourceClient) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) bound(ctx context.Context, call adaptersdk.Call) (source, error) {
	binding, secret, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, err
	}
	token, err := tokenFrom(secret)
	return source{client: a.client, binding: binding, token: token}, err
}

type serverVersion struct {
	Version    string          `json:"version"`
	Enterprise *bool           `json:"enterprise"`
	InstanceID json.RawMessage `json:"instance_id"`
}

func (v serverVersion) binding(base string) (adaptersdk.SourceBinding, error) {
	edition := ""
	switch {
	case v.Enterprise != nil && *v.Enterprise, strings.HasSuffix(v.Version, "-ee"):
		edition = "EE"
	case v.Enterprise != nil, strings.HasSuffix(v.Version, "-ce"):
		edition = "CE"
	}
	if edition == "" || v.Version == "" {
		return adaptersdk.SourceBinding{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab version or edition is missing")
	}
	instance := base
	var nativeID string
	if json.Unmarshal(v.InstanceID, &nativeID) == nil && strings.TrimSpace(nativeID) != "" {
		instance = nativeID
	}
	return adaptersdk.SourceBinding{BaseURL: base, InstanceKey: instance, Edition: edition, Version: v.Version}, nil
}

type project struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Path          string     `json:"path_with_namespace"`
	DefaultBranch string     `json:"default_branch"`
	Visibility    string     `json:"visibility"`
	WebURL        string     `json:"web_url"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

func (s source) project(ctx context.Context, projectID int64) (project, error) {
	var p project
	err := s.get(ctx, "project", projectPath(projectID), &p)
	if err == nil && (p.ID != projectID || p.Path == "") {
		err = adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab project identity is missing or does not match")
	}
	return p, err
}

func (a *Adapter) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	base, err := normalizeBase(in.BaseUrl)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeCollect)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	token, err := tokenFrom(secret)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s := source{client: a.client, token: token, binding: adaptersdk.SourceBinding{BaseURL: base}}
	var version serverVersion
	if err := s.get(ctx, "version", "version", &version); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	binding, err := version.binding(base)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	collector, err := s.collector(ctx)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	mode := adaptersdk.ProbeResultAccessModeServiceAccount
	out := adaptersdk.ProbeResult{
		Provider: adaptersdk.ProbeResultProviderGitlab, ContractVersions: []string{adaptersdk.CurrentContractVersion},
		InstanceKey: binding.InstanceKey, Edition: &binding.Edition, Version: &binding.Version, AccessMode: &mode,
		Scopes: make([]adaptersdk.ScopeCheck, 0, len(in.Scopes)),
	}
	var outcomes [3]probeOutcome
	for _, scope := range in.Scopes {
		check, states, err := s.probeScope(ctx, collector.ID, scope)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		out.Scopes = append(out.Scopes, check)
		for i, reason := range states {
			outcomes[i].add(reason)
		}
	}
	now := time.Now().UTC()
	out.Capabilities = []adaptersdk.Capability{
		outcomes[0].capability(adaptersdk.CapabilityCapabilityReadCurrent, now),
		outcomes[1].capability(adaptersdk.CapabilityCapabilityReadLinks, now),
		outcomes[2].capability(adaptersdk.CapabilityCapabilityReadPermissions, now),
	}
	if err := call.BindSource(ctx, secret, binding); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	return out, nil
}

type probeOutcome struct {
	verified   int
	unverified int
	reason     string
}

func (o *probeOutcome) add(reason string) {
	if reason == "" {
		o.verified++
		return
	}
	o.unverified++
	if o.reason == "" {
		o.reason = reason
	}
}

func (o probeOutcome) capability(kind adaptersdk.CapabilityCapability, at time.Time) adaptersdk.Capability {
	out := adaptersdk.Capability{Capability: kind, State: adaptersdk.CapabilityStateVerified, CheckedAt: at}
	switch {
	case o.verified == 0:
		out.State = adaptersdk.CapabilityStateMissing
		if o.reason == "" {
			o.reason = "no_repository"
		}
	case o.unverified != 0:
		out.State = adaptersdk.CapabilityStatePartial
	}
	out.ReasonCode = optional(o.reason)
	return out
}

func probeReason(err error, fallback string) (string, error) {
	if err == nil {
		return "", nil
	}
	if hasCode(err, adaptersdk.AdapterErrorCodeAuth) || hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		return fallback, nil
	}
	return "", err
}

func (s source) probeScope(ctx context.Context, collectorID int64, scope adaptersdk.ScopeRef) (adaptersdk.ScopeCheck, [3]string, error) {
	out := adaptersdk.ScopeCheck{
		Kind: adaptersdk.ScopeCheckKind(scope.Kind), NativeScopeId: scope.NativeScopeId,
		State: adaptersdk.ScopeCheckStateMissing,
	}
	projectID, ok := numericID(scope.NativeScopeId)
	if !ok || scope.Kind != adaptersdk.ScopeRefKindRepository {
		out.ReasonCode = optional("invalid_repository")
		return out, [3]string{"invalid_repository", "invalid_repository", "invalid_repository"}, nil
	}
	p, err := s.project(ctx, projectID)
	if reason, err := probeReason(err, "project_not_visible"); err != nil || reason != "" {
		out.ReasonCode = optional(reason)
		return out, [3]string{reason, reason, reason}, err
	}

	permissions, err := probeReason(s.requireReporter(ctx, projectID, collectorID), "project_permissions_unavailable")
	if err != nil {
		return out, [3]string{}, err
	}
	if permissions != "" && p.Visibility != "public" {
		out.ReasonCode = &permissions
		return out, [3]string{permissions, permissions, permissions}, nil
	}
	var mr listedMR
	_, err = readPage(ctx, s, "merge_requests", projectPath(projectID)+"/merge_requests",
		url.Values{"scope": {"all"}, "state": {"all"}}, 1, 1, func(item listedMR) error {
			mr = item
			return nil
		})
	current, err := probeReason(err, "merge_requests_unavailable")
	if err != nil || current != "" {
		out.ReasonCode = optional(current)
		return out, [3]string{current, current, permissions}, err
	}
	out.State, out.DisplayName = adaptersdk.ScopeCheckStateVerified, &p.Path
	links := "no_merge_request_to_verify"
	if mr.IID > 0 {
		links, err = s.probeHistory(ctx, projectID, mr.IID)
	}
	return out, [3]string{"", links, permissions}, err
}

func (s source) probeHistory(ctx context.Context, projectID, iid int64) (string, error) {
	root := projectPath(projectID) + "/merge_requests/" + id(iid)
	for _, collection := range []string{"discussions", "resource_state_events", "versions", "closes_issues"} {
		_, err := readPage(ctx, s, collection, root+"/"+collection, url.Values{}, 1, 1, func(json.RawMessage) error { return nil })
		if reason, err := probeReason(err, "merge_request_history_unavailable"); reason != "" || err != nil {
			return reason, err
		}
	}
	return "", nil
}

func (s source) manifest(in adaptersdk.FetchInput, now time.Time) *envelope.Manifest {
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(in.SchemaId), SchemaVersion: "1",
		Source: envelope.Source{
			Provider: envelope.SourceProviderGitlab, InstanceKey: s.binding.InstanceKey,
			Version: &s.binding.Version, Edition: &s.binding.Edition,
		},
		Area:      envelope.AreaRef{Kind: envelope.AreaRefKindRepository, NativeId: in.Scope.NativeScopeId},
		Period:    envelope.Period{From: in.Period.From, To: in.Period.To},
		CreatedAt: now, Completeness: envelope.ManifestCompletenessComplete,
		Producer:  envelope.Producer{Name: "adapter-gitlab", Version: "1"},
		Selection: envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
	}
	if in.Cursor != nil {
		m.Selection.Cursor = &envelope.Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeId}
	}
	return m
}

func addRecord(page *adaptersdk.Page, kind, schema, class, namespace, nativeID string, revision *string, eventTime *time.Time, observed time.Time, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return page.AddRecord(&envelope.Record{
		RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: schema, RecordClass: envelope.RecordRecordClass(class),
		NativeId: nativeID, NativeNs: namespace, SourceRevision: revision, EventTime: eventTime, ObservedTime: &observed, Payload: payload,
	})
}

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	if in.Limit < 1 || in.Limit > 1000 {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid Fetch limit")
	}
	if in.SchemaId != adaptersdk.FetchInputSchemaIdMergeRequests && in.SchemaId != adaptersdk.FetchInputSchemaIdAccounts {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitLab serves merge_requests and accounts")
	}
	projectID, err := scopeID(in.Scope)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := a.bound(ctx, call)
	if err != nil {
		return nil, err
	}
	m := s.manifest(in, time.Now().UTC())
	switch in.SchemaId {
	case adaptersdk.FetchInputSchemaIdMergeRequests:
		err = s.fetchMRs(ctx, projectID, in, m, page)
	case adaptersdk.FetchInputSchemaIdAccounts:
		err = s.fetchAccounts(ctx, projectID, in, m, page)
	default:
		panic("validated GitLab schema")
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}
