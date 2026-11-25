package github

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

// CheckAccess keeps every item closed until the candidate service_account mode
// is verified on GitHub.com, including revocation and public repository behavior.
func (*Adapter) CheckAccess(_ context.Context, _ adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	now := time.Now().UTC()
	out := adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeServiceAccount, CheckedAt: now,
		Decisions: make([]adaptersdk.AccessDecision, len(in.Items)),
	}
	for i := range out.Decisions {
		out.Decisions[i] = adaptersdk.AccessDecision{
			Decision: adaptersdk.AccessDecisionDecisionUnknown, CheckedAt: now,
			ReasonCode: optional("github_access_mode_unverified"),
		}
	}
	return out, nil
}
