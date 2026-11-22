package jiracloud

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_accessTimeout = 2 * time.Second
	_accessWorkers = 8
	_accessMaxAge  = 60

	_revocationUnverified = "jira_access_revocation_unverified"
	_accessTimeoutReason  = "jira_access_timeout"
)

var _cloudID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type accessConnection struct {
	organization [16]byte
	connection   [16]byte
}

type accessGate struct {
	slots chan struct{}
	calls int
}

func (a *Adapter) acquireAccessGate(call adaptersdk.Call) (gate *accessGate, release func()) {
	key := accessConnection{organization: call.Organization, connection: call.Connection}
	a.accessMu.Lock()
	gate = a.accessGates[key]
	if gate == nil {
		gate = &accessGate{slots: make(chan struct{}, _accessWorkers)}
		a.accessGates[key] = gate
	}
	gate.calls++
	a.accessMu.Unlock()
	return gate, func() {
		a.accessMu.Lock()
		defer a.accessMu.Unlock()
		gate.calls--
		if gate.calls == 0 {
			delete(a.accessGates, key)
		}
	}
}

func (a *Adapter) CheckAccess(ctx context.Context, call adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _accessTimeout)
	defer cancel()

	out := adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeUserToken, Decisions: make([]adaptersdk.AccessDecision, len(in.Items)),
	}
	gate, release := a.acquireAccessGate(call)
	defer release()
	user, reason := a.accessUser(ctx, call, gate, in)
	if reason != "" {
		for i := range out.Decisions {
			out.Decisions[i] = accessDecision(adaptersdk.AccessDecisionDecisionUnknown, reason)
		}
	} else {
		var workers sync.WaitGroup
		count := min(_accessWorkers, len(in.Items))
		for worker := range count {
			workers.Go(func() {
				for i := worker; i < len(in.Items); i += count {
					out.Decisions[i] = user.check(ctx, in.Items[i])
				}
			})
		}
		workers.Wait()
	}

	maxAge := _accessMaxAge
	for _, decision := range out.Decisions {
		if decision.Decision == adaptersdk.AccessDecisionDecisionUnknown {
			maxAge = 0
			break
		}
	}
	out.MaxAgeSeconds, out.CheckedAt = &maxAge, time.Now().UTC()
	return out, nil
}

type accessUser struct {
	client *adaptersdk.SourceClient
	gate   *accessGate
	base   string
	token  string
}

func (a *Adapter) accessUser(ctx context.Context, call adaptersdk.Call, gate *accessGate, in adaptersdk.CheckAccessInput) (user accessUser, reason string) {
	token, err := call.UserToken(ctx, in.UserId)
	switch {
	case errors.Is(err, adaptersdk.ErrNoCredential):
		return accessUser{}, "jira_user_token_missing"
	case err != nil:
		return accessUser{}, accessFailure(ctx, err, "jira_user_token_unavailable")
	case token.Access == "":
		return accessUser{}, "jira_user_token_missing"
	case token.ExpiresAt != nil && !time.Now().Before(*token.ExpiresAt):
		return accessUser{}, "token_expired"
	}
	binding, _, err := call.BoundSource(ctx)
	if err != nil {
		return accessUser{}, accessFailure(ctx, err, "jira_source_unverified")
	}
	if !_cloudID.MatchString(binding.InstanceKey) || binding.Edition != "cloud" {
		return accessUser{}, "jira_cloud_id_unverified"
	}
	user = accessUser{client: a.client, gate: gate, base: _oauthGateway + binding.InstanceKey, token: token.Access}
	var actor jiraAccount
	if err := user.get(ctx, "access_account", "myself", &actor); err != nil {
		return accessUser{}, accessFailure(ctx, err, "jira_account_unverified")
	}
	if actor.AccountID == "" || !actor.Active ||
		in.AccountNativeId != "" && in.AccountNativeId != actor.AccountID ||
		token.AccountNativeID != "" && token.AccountNativeID != actor.AccountID {
		return accessUser{}, "jira_account_unverified"
	}
	return user, ""
}

func (u accessUser) check(ctx context.Context, item adaptersdk.AccessItem) adaptersdk.AccessDecision {
	path, valid := accessPath(item)
	if !valid {
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, "jira_access_item_unsupported")
	}
	var obj struct {
		ID string `json:"id"`
	}
	err := u.get(ctx, "access_read", path, &obj)
	if ctx.Err() != nil {
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, _accessTimeoutReason)
	}
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		switch e.SourceStatusCode() {
		case http.StatusForbidden, http.StatusNotFound:
			return accessDecision(adaptersdk.AccessDecisionDecisionDenied, "jira_read_denied")
		}
	}
	if err != nil || obj.ID != item.NativeId {
		return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, accessFailure(ctx, err, "jira_read_unverified"))
	}
	// Jira can return 200 after revocation. An independent rights check is required.
	return accessDecision(adaptersdk.AccessDecisionDecisionUnknown, _revocationUnverified)
}

func accessPath(item adaptersdk.AccessItem) (string, bool) {
	if _, ok := numericID(item.NativeId); !ok || item.NativeNs != "" {
		return "", false
	}
	switch item.ItemKind {
	case adaptersdk.AccessItemItemKindArea:
		if item.AreaKind != nil && *item.AreaKind == adaptersdk.AccessItemAreaKindTrackerProject && item.ObjectKind == nil {
			return "project/" + item.NativeId, true
		}
	case adaptersdk.AccessItemItemKindObject:
		if item.ObjectKind != nil && *item.ObjectKind == adaptersdk.AccessItemObjectKindTask && item.AreaKind == nil {
			return "issue/" + item.NativeId + "?fields=id", true
		}
	}
	return "", false
}

func (u accessUser) get(ctx context.Context, operation, path string, out any) error {
	select {
	case u.gate.slots <- struct{}{}:
		defer func() { <-u.gate.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.base+_api+path, http.NoBody)
	if err != nil {
		return invalidRequest("cannot construct Jira access request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+u.token)
	resp, err := doRequest(ctx, u.client, operation, req) //nolint:bodyclose // DecodeJSON closes the body
	if err != nil {
		return err
	}
	return adaptersdk.DecodeJSON(resp, out)
}

func accessFailure(ctx context.Context, err error, fallback string) string {
	if ctx.Err() != nil || hasCode(err, adaptersdk.AdapterErrorCodeTimeout) {
		return _accessTimeoutReason
	}
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok && e.SourceStatusCode() == http.StatusUnauthorized {
		return "token_expired"
	}
	return fallback
}

func accessDecision(state adaptersdk.AccessDecisionDecision, reason string) adaptersdk.AccessDecision {
	return adaptersdk.AccessDecision{Decision: state, ReasonCode: &reason, CheckedAt: time.Now().UTC()}
}
