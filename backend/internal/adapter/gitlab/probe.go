package gitlab

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func (a *Adapter) probeBot(ctx context.Context, call adaptersdk.Call, binding adaptersdk.SourceBinding, collector account, collectToken string, scopes []adaptersdk.ScopeCheck, now time.Time) adaptersdk.Capability {
	out := probeOutcome{}
	s, err := a.bindBot(ctx, call, binding, collectToken)
	if err != nil {
		out.add("write_bot_token_unavailable")
		return out.capability(adaptersdk.CapabilityCapabilityWriteMrComment, now)
	}
	bot, err := s.botAccount(ctx)
	if err != nil || bot.ID == collector.ID || !s.hasWriteScope(ctx) {
		out.add(_writePermissionMissing)
		return out.capability(adaptersdk.CapabilityCapabilityWriteMrComment, now)
	}

	for _, scope := range scopes {
		reason := _writePermissionMissing
		if scope.State == adaptersdk.ScopeCheckStateVerified {
			projectID, _ := numericID(scope.NativeScopeId)
			p, err := s.project(ctx, projectID)
			if err == nil && !p.Archived && p.MergeRequestsAccess != "disabled" && s.requireCommentMember(ctx, projectID, bot.ID) == nil {
				reason = ""
			}
		}
		out.add(reason)
	}
	return out.capability(adaptersdk.CapabilityCapabilityWriteMrComment, now)
}

func probeWebhook(ctx context.Context, call adaptersdk.Call, now time.Time) adaptersdk.Capability {
	out := probeOutcome{}
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeWebhookSecret)
	if err != nil || secret == "" {
		out.add("webhook_secret_missing_nightly_reconciliation")
	} else {
		out.add("")
	}
	return out.capability(adaptersdk.CapabilityCapabilityWebhook, now)
}
