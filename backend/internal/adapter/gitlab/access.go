package gitlab

import (
	"context"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_planner         = 15
	_reporter        = 20
	_accessTimeout   = 2 * time.Second
	_accessFreshness = 60
	_accessParallel  = 8
)

type account struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	PublicEmail    string `json:"public_email"`
	State          string `json:"state"`
	Bot            bool   `json:"bot"`
	UserType       string `json:"user_type"`
	ServiceAccount bool   `json:"service_account"`
}

type membership struct {
	account
	AccessLevel     int    `json:"access_level"`
	MembershipState string `json:"membership_state"`
}

func (s source) collector(ctx context.Context) (account, error) {
	var u account
	err := s.get(ctx, "current_user", "user", &u)
	if err == nil && (u.ID <= 0 || u.State != "active") {
		err = adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab collector is not active")
	}
	return u, err
}

func (s source) member(ctx context.Context, projectID, userID int64) (membership, error) {
	var m membership
	err := s.get(ctx, "member", projectPath(projectID)+"/members/all/"+id(userID), &m)
	if err == nil && (m.ID != userID || m.State == "") {
		err = adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab membership account identity or state is invalid")
	}
	return m, err
}

func (m membership) reporter() bool {
	return m.active() && m.AccessLevel >= _reporter
}

func (m membership) active() bool {
	return m.State == "active" && (m.MembershipState == "" || m.MembershipState == "active")
}

func (s source) requireReporter(ctx context.Context, projectID, userID int64) error {
	m, err := s.member(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if !m.reporter() {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitLab collector requires an active Reporter membership")
	}
	return nil
}

func (a *Adapter) CheckAccess(ctx context.Context, call adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _accessTimeout)
	defer cancel()

	now := time.Now().UTC()
	freshness := _accessFreshness
	out := adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeServiceAccount, CheckedAt: now, MaxAgeSeconds: &freshness,
		Decisions: make([]adaptersdk.AccessDecision, len(in.Items)),
	}
	s, err := a.bound(ctx, call)
	if err != nil {
		reason := "source_not_verified"
		if ctx.Err() != nil {
			reason = "access_timeout"
		}
		for i := range out.Decisions {
			out.Decisions[i] = accessDecision(now, adaptersdk.AccessDecisionDecisionUnknown, reason)
		}
		return out, nil
	}

	userID, valid := numericID(in.AccountNativeId)
	if !valid && in.AccountNativeId != "" {
		for i := range out.Decisions {
			out.Decisions[i] = accessDecision(now, adaptersdk.AccessDecisionDecisionUnknown, "invalid_account_id")
		}
		return out, nil
	}

	limit, release := a.accessLimits.share(call.Organization.String()+"/"+call.Connection.String(), _accessParallel)
	defer release()
	var workers sync.WaitGroup
	count := min(_accessParallel, len(in.Items))
	for worker := range count {
		workers.Go(func() {
			for i := worker; i < len(in.Items); i += count {
				out.Decisions[i] = s.limitedAccess(ctx, limit, userID, in.Items[i], now)
			}
		})
	}
	workers.Wait()
	return out, nil
}

func accessDecision(at time.Time, state adaptersdk.AccessDecisionDecision, reason string) adaptersdk.AccessDecision {
	return adaptersdk.AccessDecision{Decision: state, ReasonCode: optional(reason), CheckedAt: at}
}

func (s source) limitedAccess(ctx context.Context, limit *requestLimit, userID int64, item adaptersdk.AccessItem, now time.Time) adaptersdk.AccessDecision {
	expired := accessDecision(now, adaptersdk.AccessDecisionDecisionUnknown, "access_timeout")
	if err := limit.acquire(ctx); err != nil {
		return expired
	}
	defer limit.release()
	out := s.access(ctx, userID, item, now)
	if ctx.Err() != nil {
		return expired
	}
	return out
}

func (s source) access(ctx context.Context, userID int64, item adaptersdk.AccessItem, now time.Time) adaptersdk.AccessDecision {
	decision := func(state adaptersdk.AccessDecisionDecision, reason string) adaptersdk.AccessDecision {
		return accessDecision(now, state, reason)
	}

	projectID, ok := numericID(item.NativeId)
	if !ok || item.ItemKind != adaptersdk.AccessItemItemKindArea || item.AreaKind == nil ||
		*item.AreaKind != adaptersdk.AccessItemAreaKindRepository {
		return decision(adaptersdk.AccessDecisionDecisionUnknown, "unsupported_access_item")
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return decision(adaptersdk.AccessDecisionDecisionUnknown, "project_not_verified")
	}
	if p.Visibility == "public" {
		if userID != 0 {
			var user account
			if err := s.get(ctx, "user", "users/"+id(userID), &user); err != nil || user.ID != userID {
				return decision(adaptersdk.AccessDecisionDecisionUnknown, "account_not_verified")
			}
			if user.State == "" {
				return decision(adaptersdk.AccessDecisionDecisionUnknown, "account_not_verified")
			}
			if user.State != "active" {
				return decision(adaptersdk.AccessDecisionDecisionDenied, "inactive_account")
			}
		}
		return decision(adaptersdk.AccessDecisionDecisionAllowed, "")
	}
	if p.Visibility != "private" && p.Visibility != "internal" {
		return decision(adaptersdk.AccessDecisionDecisionUnknown, "visibility_not_verified")
	}
	if userID == 0 {
		return decision(adaptersdk.AccessDecisionDecisionDenied, "account_required")
	}
	m, err := s.member(ctx, projectID, userID)
	switch {
	case hasCode(err, adaptersdk.AdapterErrorCodeNotFound):
		return decision(adaptersdk.AccessDecisionDecisionDenied, "not_a_member")
	case err != nil:
		return decision(adaptersdk.AccessDecisionDecisionUnknown, "membership_not_verified")
	case !m.reporter():
		return decision(adaptersdk.AccessDecisionDecisionDenied, "active_reporter_required")
	default:
		return decision(adaptersdk.AccessDecisionDecisionAllowed, "")
	}
}
