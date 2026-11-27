package sonarqube

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_accessTimeout       = 2 * time.Second
	_accessMaxAge        = 60
	_accessTimeoutReason = "sonarqube_access_timeout"
)

func (a *Adapter) CheckAccess(ctx context.Context, call adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _accessTimeout)
	defer cancel()

	s, reason := a.accessSource(ctx, call, in)
	decisions := make([]adaptersdk.AccessDecision, len(in.Items))
	if reason == "" {
		for i, item := range in.Items {
			if ctx.Err() != nil {
				break
			}
			decision := s.projectAccess(ctx, item)
			if decision.Decision == adaptersdk.AccessDecisionDecisionAllowed {
				// Enable positive decisions only after the stand verifies revocation.
				decision = accessDecision(adaptersdk.AccessDecisionDecisionUnknown, "sonarqube_access_revocation_unverified")
			}
			decisions[i] = decision
		}
	}
	if ctx.Err() != nil {
		reason = _accessTimeoutReason
	}
	if reason != "" {
		for i := range decisions {
			decisions[i] = accessDecision(adaptersdk.AccessDecisionDecisionUnknown, reason)
		}
	}
	return adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeUserToken, CheckedAt: time.Now().UTC(),
		MaxAgeSeconds: new(_accessMaxAge), Decisions: decisions,
	}, nil
}

func (a *Adapter) accessSource(ctx context.Context, call adaptersdk.Call, in adaptersdk.CheckAccessInput) (src source, reason string) {
	token, err := call.UserToken(ctx, in.UserId)
	switch {
	case errors.Is(err, adaptersdk.ErrNoCredential):
		return source{}, "sonarqube_user_token_missing"
	case err != nil:
		return source{}, "sonarqube_user_token_unavailable"
	case token.Access == "" || strings.ContainsAny(token.Access, "\r\n"):
		return source{}, "sonarqube_user_token_unavailable"
	case token.ExpiresAt != nil && !time.Now().Before(*token.ExpiresAt):
		return source{}, "token_expired"
	}
	binding, _, err := call.BoundSource(ctx)
	if err != nil {
		return source{}, "sonarqube_source_unverified"
	}
	s := source{client: a.client, binding: binding, token: token.Access}
	if reason := s.confirmAccount(ctx, in.AccountNativeId, token.AccountNativeID); reason != "" {
		return source{}, reason
	}
	return s, ""
}

func (s source) confirmAccount(ctx context.Context, expected, stored string) string {
	//nolint:tagliatelle // SonarQube Web API field.
	var actor struct {
		Login      string `json:"login"`
		IsLoggedIn bool   `json:"isLoggedIn"`
	}
	err := s.get(ctx, "/api/users/current", nil, &actor)
	if hasCode(err, adaptersdk.AdapterErrorCodeAuth) {
		return "token_expired"
	}
	if hasCode(err, adaptersdk.AdapterErrorCodeTimeout) {
		return _accessTimeoutReason
	}
	if err != nil || !actor.IsLoggedIn || actor.Login == "" ||
		strings.TrimSpace(actor.Login) != actor.Login || !textFits(actor.Login, 255) ||
		expected != "" && actor.Login != expected || stored != "" && actor.Login != stored {
		return "sonarqube_account_unverified"
	}
	return ""
}

func (s source) projectAccess(ctx context.Context, item adaptersdk.AccessItem) adaptersdk.AccessDecision {
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindAnalysisProject, NativeScopeId: item.NativeId}
	if item.ItemKind != adaptersdk.AccessItemItemKindArea || item.AreaKind == nil ||
		*item.AreaKind != adaptersdk.AccessItemAreaKindAnalysisProject ||
		item.ObjectKind != nil || item.NativeNs != "" || !validScope(scope) {
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, "sonarqube_access_item_unsupported")
	}

	project, err := s.readProject(ctx, item.NativeId)
	switch {
	case hasCode(err, adaptersdk.AdapterErrorCodeTimeout):
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, _accessTimeoutReason)
	case hasCode(err, adaptersdk.AdapterErrorCodeAuth):
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, "token_expired")
	case hasCode(err, adaptersdk.AdapterErrorCodePermissionMissing):
		return accessDecision(adaptersdk.AccessDecisionDecisionDenied, "sonarqube_browse_denied")
	case err != nil || project.Qualifier != "TRK":
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, "sonarqube_project_unverified")
	default:
		return accessDecision(adaptersdk.AccessDecisionDecisionAllowed, "")
	}
}

func accessDecision(decision adaptersdk.AccessDecisionDecision, reason string) adaptersdk.AccessDecision {
	return adaptersdk.AccessDecision{Decision: decision, ReasonCode: optional(reason), CheckedAt: time.Now().UTC()}
}
