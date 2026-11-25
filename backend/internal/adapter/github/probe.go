package github

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func (a *Adapter) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	u, err := url.Parse(strings.TrimSpace(in.BaseUrl))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitHub base URL must be HTTPS without credentials, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "api.github.com" || strings.Trim(u.EscapedPath(), "/") != "" || u.Port() != "" && u.Port() != "443" {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnsupportedEdition, "GitHub.com required; GitHub Enterprise Server is unsupported")
	}
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeCollect)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	token, err := readToken(secret)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	s := source{client: a.client, token: token}
	out := adaptersdk.ProbeResult{
		Provider: adaptersdk.ProbeResultProviderGithub, Edition: optional(_edition), Version: optional(_apiVersion), InstanceKey: _baseURL,
		ContractVersions: []string{adaptersdk.CurrentContractVersion}, Scopes: []adaptersdk.ScopeCheck{},
	}
	kinds := [...]adaptersdk.CapabilityCapability{adaptersdk.CapabilityCapabilityReadCurrent, adaptersdk.CapabilityCapabilityReadLinks, adaptersdk.CapabilityCapabilityReadPermissions}
	var results [len(kinds)]probeOutcome
	for _, scope := range in.Scopes {
		check, reasons, err := s.probeScope(ctx, scope)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		out.Scopes = append(out.Scopes, check)
		for i, reason := range reasons {
			results[i].add(reason)
		}
	}
	now := time.Now().UTC()
	for i, kind := range kinds {
		out.Capabilities = append(out.Capabilities, results[i].capability(kind, now))
	}
	if err := call.BindSource(ctx, secret, githubBinding); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	return out, nil
}

type probeOutcome struct {
	success, failure int
	reason           string
}

func (o *probeOutcome) add(reason string) {
	if reason == "" {
		o.success++
		return
	}
	o.failure++
	if o.reason == "" {
		o.reason = reason
	}
}

func (o probeOutcome) capability(kind adaptersdk.CapabilityCapability, now time.Time) adaptersdk.Capability {
	check := adaptersdk.Capability{Capability: kind, State: adaptersdk.CapabilityStateVerified, CheckedAt: now}
	if o.success == 0 {
		check.State = adaptersdk.CapabilityStateMissing
		if o.reason == "" {
			o.reason = "no_repository"
		}
	} else if o.failure != 0 {
		check.State = adaptersdk.CapabilityStatePartial
	}
	check.ReasonCode = optional(o.reason)
	return check
}

func (s source) probeScope(ctx context.Context, scope adaptersdk.ScopeRef) (adaptersdk.ScopeCheck, [3]string, error) {
	check := adaptersdk.ScopeCheck{Kind: adaptersdk.ScopeCheckKind(scope.Kind), NativeScopeId: scope.NativeScopeId, State: adaptersdk.ScopeCheckStateMissing}
	repo, err := s.repository(ctx, scope)
	if unavailableScope(err) || hasCode(err, adaptersdk.AdapterErrorCodeInvalidRequest) {
		reason := "repository_unavailable"
		check.ReasonCode = &reason
		return check, [3]string{reason, reason, reason}, nil
	}
	if err != nil {
		return check, [3]string{}, err
	}
	check.DisplayName = &repo.FullName
	query := url.Values{"state": {"all"}, "per_page": {"1"}}
	var sample pull
	err = walk(ctx, s, "pulls", repo.path()+"/pulls", query, true, func(p pull) error {
		sample = p
		return p.validate(repo.ID)
	})
	if unavailableScope(err) {
		reason := "pull_requests_unavailable"
		check.ReasonCode = &reason
		return check, [3]string{reason, reason, reason}, nil
	}
	if err != nil {
		return check, [3]string{}, err
	}
	links, err := s.probeHistory(ctx, repo, sample)
	if err != nil {
		return check, [3]string{}, err
	}
	permission, err := s.probePermission(ctx, repo, sample.User)
	if err != nil {
		return check, [3]string{}, err
	}
	check.State = adaptersdk.ScopeCheckStateVerified
	if links != "" || permission != "" {
		check.ReasonCode = optional(links)
		if check.ReasonCode == nil {
			check.ReasonCode = &permission
		}
	}
	return check, [3]string{"", links, permission}, nil
}

func (s source) probeHistory(ctx context.Context, repo repository, p pull) (string, error) {
	if p.Number == 0 {
		return "no_pull_request_to_verify", nil
	}
	for _, endpoint := range historyEndpoints(repo, p) {
		err := walk(ctx, s, endpoint.name, endpoint.path, url.Values{"per_page": {"1"}}, true, func(json.RawMessage) error { return nil })
		if unavailableScope(err) {
			return "pull_request_history_unavailable", nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func (s source) probePermission(ctx context.Context, repo repository, subject account) (string, error) {
	if subject.Login == "" {
		subject = repo.Owner
	}
	if subject.Login == "" {
		return "repository_owner_unavailable", nil
	}
	var reply struct {
		Permission string  `json:"permission"`
		User       account `json:"user"`
	}
	err := s.get(ctx, "permission", repo.path()+"/collaborators/"+url.PathEscape(subject.Login)+"/permission", &reply)
	if unavailableScope(err) {
		return "repository_permissions_unavailable", nil
	}
	if err != nil {
		return "", err
	}
	switch reply.Permission {
	case "admin", "write", "read", "none", "maintain", "triage":
		if strings.EqualFold(reply.User.Login, subject.Login) {
			return "", nil
		}
	}
	return "repository_permissions_unavailable", nil
}
