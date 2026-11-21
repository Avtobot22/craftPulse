package jiracloud

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Adapter collects Jira Cloud facts using the adapter SDK.
type Adapter struct {
	adaptersdk.Unimplemented
	client *adaptersdk.SourceClient
}

// New creates a Jira Cloud adapter.
func New(client *adaptersdk.SourceClient) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) bound(ctx context.Context, call adaptersdk.Call) (source, error) {
	binding, secret, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, err
	}
	return newSource(a.client, binding, secret)
}

type serverInfo struct {
	DeploymentType string `json:"deploymentType"`
	Version        string `json:"version"`
	BuildNumber    int    `json:"buildNumber"`
}

func (s source) serverInfo(ctx context.Context) (serverInfo, error) {
	var info serverInfo
	err := s.get(ctx, "server_info", "serverInfo", nil, &info)
	if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		resp, fallbackErr := s.request(ctx, "server_info_v2", "/rest/api/2/serverInfo", nil, true) //nolint:bodyclose // DecodeJSON closes the body
		if fallbackErr != nil {
			return serverInfo{}, fallbackErr
		}
		err = adaptersdk.DecodeJSON(resp, &info)
	}
	if err != nil {
		return serverInfo{}, err
	}
	if info.DeploymentType == "" || info.Version == "" {
		return serverInfo{}, invalidResponse("Jira serverInfo has no edition or version")
	}
	if !strings.EqualFold(info.DeploymentType, "Cloud") {
		return serverInfo{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnsupportedEdition, "expected Jira Cloud")
	}
	return info, nil
}

func (s source) instanceKey(ctx context.Context) (string, error) {
	const gateway = "https://api.atlassian.com/ex/jira/"
	if cloudID, ok := strings.CutPrefix(s.binding.BaseURL, gateway); ok && cloudID != "" && !strings.Contains(cloudID, "/") {
		return cloudID, nil
	}
	var tenant struct {
		CloudID string `json:"cloudId"`
	}
	resp, err := s.request(ctx, "tenant_info", "/_edge/tenant_info", nil, false) //nolint:bodyclose // DecodeJSON closes the body
	if hidden(err) {
		return s.binding.BaseURL, nil
	}
	if err != nil {
		return "", err
	}
	if err := adaptersdk.DecodeJSON(resp, &tenant); err != nil {
		return "", err
	}
	if tenant.CloudID == "" {
		return s.binding.BaseURL, nil
	}
	return tenant.CloudID, nil
}

type project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (s source) project(ctx context.Context, key string) (project, error) {
	var p project
	if err := s.get(ctx, "project", "project/"+url.PathEscape(key), nil, &p); err != nil {
		return project{}, err
	}
	if p.ID != key || p.Key == "" {
		return project{}, invalidResponse("Jira returned a different project or omitted its ID")
	}
	return p, nil
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
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	s, err := newSource(a.client, adaptersdk.SourceBinding{BaseURL: base}, secret)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	out, err := s.probe(ctx, in.Scopes)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	binding := adaptersdk.SourceBinding{
		BaseURL: base, InstanceKey: out.InstanceKey, Edition: *out.Edition, Version: *out.Version,
	}
	if err := call.BindSource(ctx, secret, binding); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	return out, nil
}

var _capabilities = [...]adaptersdk.CapabilityCapability{
	adaptersdk.CapabilityCapabilityReadCurrent,
	adaptersdk.CapabilityCapabilityReadStatusHistory,
	adaptersdk.CapabilityCapabilityReadFieldHistory,
	adaptersdk.CapabilityCapabilityReadResolutionHistory,
	adaptersdk.CapabilityCapabilityReadLinks,
	adaptersdk.CapabilityCapabilityWriteTrackerIssue,
}

type probeEvidence [_capabilitiesCount]string

const _capabilitiesCount = len(_capabilities)

func allMissing(reason string) probeEvidence {
	var out probeEvidence
	for i := range out {
		out[i] = reason
	}
	return out
}

func (s source) probe(ctx context.Context, scopes []adaptersdk.ScopeRef) (adaptersdk.ProbeResult, error) {
	info, err := s.serverInfo(ctx)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	// serverInfo may be public. myself proves that the collection credentials are accepted.
	var actor jiraAccount
	if err := s.get(ctx, "myself", "myself", nil, &actor); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	if actor.AccountID == "" || !actor.Active {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "Jira collector account is missing or inactive")
	}
	instance, err := s.instanceKey(ctx)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	edition, version := "cloud", "REST v3 / "+info.Version
	if info.BuildNumber > 0 {
		version += " (" + strconv.Itoa(info.BuildNumber) + ")"
	}
	mode := adaptersdk.ProbeResultAccessModeUserToken
	out := adaptersdk.ProbeResult{
		Provider: adaptersdk.ProbeResultProviderJiraCloud, ContractVersions: []string{adaptersdk.CurrentContractVersion},
		InstanceKey: instance, Edition: &edition, Version: &version, AccessMode: &mode,
		Scopes: make([]adaptersdk.ScopeCheck, 0, len(scopes)),
	}
	evidence := make([]probeEvidence, 0, len(scopes))
	for _, scope := range scopes {
		check, reasons, err := s.probeScope(ctx, scope)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		out.Scopes = append(out.Scopes, check)
		evidence = append(evidence, reasons)
	}
	now := time.Now().UTC()
	for i, kind := range _capabilities {
		verified, reason := 0, ""
		for _, scope := range evidence {
			if scope[i] == "" {
				verified++
			} else if reason == "" {
				reason = scope[i]
			}
		}
		state := adaptersdk.CapabilityStateVerified
		switch {
		case verified == 0:
			state = adaptersdk.CapabilityStateMissing
			if reason == "" {
				reason = "no_project"
			}
		case verified != len(scopes):
			state = adaptersdk.CapabilityStatePartial
		}
		out.Capabilities = append(out.Capabilities, adaptersdk.Capability{
			Capability: kind, State: state, ReasonCode: optional(reason), CheckedAt: now,
		})
	}
	out.Capabilities = append(out.Capabilities, adaptersdk.Capability{
		Capability: adaptersdk.CapabilityCapabilityReadPermissions, State: adaptersdk.CapabilityStateMissing,
		ReasonCode: new("jira_access_revocation_unverified"), CheckedAt: now,
	})
	return out, nil
}

func (s source) probeScope(ctx context.Context, scope adaptersdk.ScopeRef) (adaptersdk.ScopeCheck, probeEvidence, error) {
	out := adaptersdk.ScopeCheck{Kind: adaptersdk.ScopeCheckKind(scope.Kind), NativeScopeId: scope.NativeScopeId, State: adaptersdk.ScopeCheckStateMissing}
	key, err := projectScope(scope)
	if err != nil {
		out.ReasonCode = new("invalid_project")
		return out, allMissing(*out.ReasonCode), nil //nolint:nilerr // invalid configured scope is reported in Probe
	}
	p, err := s.project(ctx, key)
	if hidden(err) {
		out.ReasonCode = new("project_not_visible")
		return out, allMissing(*out.ReasonCode), nil
	}
	if err != nil {
		return out, probeEvidence{}, err
	}
	var perms struct {
		Permissions map[string]struct {
			Allowed bool `json:"havePermission"`
		} `json:"permissions"`
	}
	err = s.get(ctx, "permissions", "mypermissions", url.Values{
		"projectId": {p.ID}, "permissions": {"BROWSE_PROJECTS,CREATE_ISSUES"},
	}, &perms)
	if hidden(err) || err == nil && !perms.Permissions["BROWSE_PROJECTS"].Allowed {
		out.ReasonCode = new("browse_projects_missing")
		return out, allMissing(*out.ReasonCode), nil
	}
	if err != nil {
		return out, probeEvidence{}, err
	}
	keys, err := s.search(ctx, "project = "+p.ID+" ORDER BY updated DESC, id DESC", "updated,project,issuelinks", "", 1)
	if hidden(err) {
		out.ReasonCode = new("issues_unavailable")
		return out, allMissing(*out.ReasonCode), nil
	}
	if err != nil {
		return out, probeEvidence{}, err
	}
	out.State, out.DisplayName = adaptersdk.ScopeCheckStateVerified, optional(p.Name)
	history, links := "no_issue_to_verify", "no_issue_to_verify"
	if len(keys.Issues.items) != 0 {
		issue := &keys.Issues.items[0]
		if issue.Fields.Project.ID != p.ID {
			return out, probeEvidence{}, invalidResponse("Jira probe returned an issue outside the project")
		}
		if _, err := issue.position(); err != nil {
			return out, probeEvidence{}, err
		}
		if issue.Fields.Links != nil {
			links = ""
		} else {
			links = "issue_links_unavailable"
		}
		_, err := s.changelog(ctx, issue.ID, 0, 1)
		switch {
		case hidden(err):
			history = "changelog_unavailable"
		case err != nil:
			return out, probeEvidence{}, err
		default:
			history = ""
		}
	}
	if history != "" {
		out.ReasonCode = &history
	}
	write := "create_issues_missing"
	if perms.Permissions["CREATE_ISSUES"].Allowed {
		write = ""
	}
	return out, probeEvidence{"", history, history, history, links, write}, nil
}

func (*Adapter) CheckAccess(_ context.Context, _ adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	now := time.Now().UTC()
	out := adaptersdk.CheckAccessResult{Mode: adaptersdk.CheckAccessResultModeUserToken, CheckedAt: now, Decisions: make([]adaptersdk.AccessDecision, len(in.Items))}
	for i := range out.Decisions {
		out.Decisions[i] = adaptersdk.AccessDecision{
			CheckedAt: now, Decision: adaptersdk.AccessDecisionDecisionUnknown,
			ReasonCode: new("jira_access_revocation_unverified"),
		}
	}
	return out, nil
}

func (s source) manifest(in adaptersdk.FetchInput) *envelope.Manifest {
	out := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(in.SchemaId), SchemaVersion: "1",
		Source: envelope.Source{
			Provider: envelope.SourceProviderJiraCloud, InstanceKey: s.binding.InstanceKey,
			Version: &s.binding.Version, Edition: &s.binding.Edition,
		},
		Area:      envelope.AreaRef{Kind: envelope.AreaRefKindTrackerProject, NativeId: in.Scope.NativeScopeId},
		Period:    envelope.Period{From: in.Period.From, To: in.Period.To},
		CreatedAt: time.Now().UTC(), Completeness: envelope.ManifestCompletenessComplete,
		Producer:  envelope.Producer{Name: "adapter-jiracloud", Version: "1"},
		Selection: envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
	}
	if in.Cursor != nil {
		out.Selection.Cursor = &envelope.Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeId}
	}
	return out
}
