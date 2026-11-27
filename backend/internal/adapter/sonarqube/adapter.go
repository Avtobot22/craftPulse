// Package sonarqube reads analysis reports scoped to a SonarQube project.
package sonarqube

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

// Adapter reads SonarQube reports over the SDK's guarded source client.
type Adapter struct {
	adaptersdk.Unimplemented
	client *adaptersdk.SourceClient
}

// New constructs a SonarQube adapter.
func New(client *adaptersdk.SourceClient) *Adapter { return &Adapter{client: client} }

var _ adaptersdk.Source = (*Adapter)(nil)

func (a *Adapter) bound(ctx context.Context, call adaptersdk.Call) (source, error) {
	binding, secret, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, err
	}
	token, err := tokenFrom(secret)
	return source{client: a.client, binding: binding, token: token}, err
}

func validScope(scope adaptersdk.ScopeRef) bool {
	return scope.Kind == adaptersdk.ScopeRefKindAnalysisProject &&
		scope.NativeScopeId != "" && strings.TrimSpace(scope.NativeScopeId) == scope.NativeScopeId &&
		textFits(scope.NativeScopeId, 255)
}

type systemStatus struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Edition string `json:"edition"`
}

func (s systemStatus) binding(base string) (adaptersdk.SourceBinding, error) {
	if s.Status != "UP" {
		return adaptersdk.SourceBinding{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube is not ready")
	}
	if s.Version == "" || !textFits(s.Version, 64) || len(s.ID) > 512 || !textFits(s.Edition, 64) {
		return adaptersdk.SourceBinding{}, adaptersdk.InvalidResponseError("invalid SonarQube server identity")
	}
	id := s.ID
	if id == "" {
		id = base
	}
	edition := s.Edition
	// The exact build used on the stand is recorded in deploy/sonarqube/instance.json.
	if edition == "" && s.Version == "26.11.0.134175" {
		edition = "Community Build"
	}
	return adaptersdk.SourceBinding{BaseURL: base, InstanceKey: id, Version: s.Version, Edition: edition}, nil
}

func (a *Adapter) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
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
	s := source{client: a.client, token: token, binding: adaptersdk.SourceBinding{BaseURL: base}}
	var status systemStatus
	if err := s.get(ctx, "/api/system/status", nil, &status); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	binding, err := status.binding(base)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	out := adaptersdk.ProbeResult{
		ContractVersions: []string{adaptersdk.CurrentContractVersion},
		Provider:         adaptersdk.ProbeResultProviderSonarqube, InstanceKey: binding.InstanceKey,
		Version: &binding.Version, Edition: optional(binding.Edition),
		AccessMode: new(adaptersdk.ProbeResultAccessModeUserToken),
		Scopes:     make([]adaptersdk.ScopeCheck, 0, len(in.Scopes)),
	}
	verified := 0
	for _, scope := range in.Scopes {
		check, err := s.probeScope(ctx, scope)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		out.Scopes = append(out.Scopes, check)
		if check.State == adaptersdk.ScopeCheckStateVerified {
			verified++
		}
	}
	now := time.Now().UTC()
	read := adaptersdk.Capability{Capability: adaptersdk.CapabilityCapabilityReadCurrent, State: adaptersdk.CapabilityStateVerified, CheckedAt: now}
	switch {
	case verified == 0:
		read.State, read.ReasonCode = adaptersdk.CapabilityStateMissing, optional("no_readable_project")
	case verified < len(in.Scopes):
		read.State, read.ReasonCode = adaptersdk.CapabilityStatePartial, optional("project_unavailable")
	}
	branchReason, mrReason := "edition_capability_unverified", "edition_capability_unverified"
	if binding.Edition == "Community Build" {
		branchReason, mrReason = "edition_does_not_support_branch_analysis", "edition_does_not_support_mr_analysis"
	}
	out.Capabilities = []adaptersdk.Capability{
		read,
		{Capability: adaptersdk.CapabilityCapabilityReadBranchAnalysis, State: adaptersdk.CapabilityStatePartial, ReasonCode: optional(branchReason), CheckedAt: now},
		{Capability: adaptersdk.CapabilityCapabilityReadMrAnalysis, State: adaptersdk.CapabilityStatePartial, ReasonCode: optional(mrReason), CheckedAt: now},
	}
	if err := call.BindSource(ctx, secret, binding); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	return out, nil
}

func (s source) probeScope(ctx context.Context, scope adaptersdk.ScopeRef) (adaptersdk.ScopeCheck, error) {
	check := adaptersdk.ScopeCheck{Kind: adaptersdk.ScopeCheckKind(scope.Kind), NativeScopeId: scope.NativeScopeId, State: adaptersdk.ScopeCheckStateMissing}
	if !validScope(scope) {
		check.ReasonCode = optional("invalid_analysis_project")
		return check, nil
	}
	project, err := s.readProject(ctx, scope.NativeScopeId)
	if err == nil {
		if project.Qualifier != "TRK" {
			check.ReasonCode = optional("project_unavailable")
			return check, nil
		}
		_, err = s.analysisPage(ctx, scope.NativeScopeId, 1, 1, -1)
	}
	if err == nil {
		_, err = s.issuePage(ctx, scope.NativeScopeId, 1, 1, -1, false)
	}
	if hasCode(err, adaptersdk.AdapterErrorCodePermissionMissing) || hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
		check.ReasonCode = optional("project_unavailable")
		return check, nil
	}
	if err != nil {
		return check, err
	}
	check.State = adaptersdk.ScopeCheckStateVerified
	return check, nil
}

type projectComponent struct {
	Key       string `json:"key"`
	Qualifier string `json:"qualifier"`
}

// readProject distinguishes a readable project from an empty issue search.
func (s source) readProject(ctx context.Context, project string) (projectComponent, error) {
	var result struct {
		Component projectComponent `json:"component"`
	}
	err := s.get(ctx, "/api/components/show", url.Values{"component": {project}}, &result)
	if err != nil {
		return projectComponent{}, err
	}
	if result.Component.Key != project {
		return projectComponent{}, adaptersdk.InvalidResponseError("SonarQube returned a different project")
	}
	return result.Component, nil
}
