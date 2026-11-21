package gitlab

import (
	"context"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_reporter      = 20
	_accessTimeout = 2 * time.Second
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
	if err == nil && m.ID != userID {
		err = adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab membership returned a different account")
	}
	return m, err
}

func (m membership) reporter() bool {
	return m.State == "active" && (m.MembershipState == "" || m.MembershipState == "active") && m.AccessLevel >= _reporter
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
	s, err := a.bound(ctx, call)
	if err != nil {
		return adaptersdk.CheckAccessResult{}, err
	}
	now := time.Now().UTC()
	userID, _ := numericID(in.AccountNativeId)
	out := adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeServiceAccount, CheckedAt: now,
		Decisions: make([]adaptersdk.AccessDecision, len(in.Items)),
	}
	var workers sync.WaitGroup
	count := min(8, len(in.Items))
	for worker := range count {
		workers.Go(func() {
			for i := worker; i < len(in.Items); i += count {
				out.Decisions[i] = s.access(ctx, userID, in.Items[i], now)
			}
		})
	}
	workers.Wait()
	return out, nil
}

func (s source) access(ctx context.Context, userID int64, item adaptersdk.AccessItem, now time.Time) adaptersdk.AccessDecision {
	decision := func(state adaptersdk.AccessDecisionDecision, reason string) adaptersdk.AccessDecision {
		return adaptersdk.AccessDecision{Decision: state, ReasonCode: optional(reason), CheckedAt: now}
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
