package sync

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _mrDelay = 5 * time.Second

func webhookAAD(org, conn, delivery entity.ID) []byte {
	return []byte("sync.webhook:" + org.String() + ":" + conn.String() + ":" + delivery.String())
}

func (u *UseCase) WebhookRoute(ctx context.Context, id entity.ID) (entity.WebhookRoute, error) {
	return u.d.Connect.WebhookRoute(ctx, id)
}

func (u *UseCase) ReceiveWebhook(ctx context.Context, route entity.WebhookRoute, in entity.WebhookInput, corr entity.ID) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return u.d.Tx.InOrgTx(ctx, route.OrganizationID, func(ctx context.Context) error {
		if _, err := u.d.Connect.WebhookRoute(ctx, route.ID); err != nil {
			return err
		}
		id, err := u.d.Repo.ReceiveWebhook(ctx, route.OrganizationID, route.ConnectionID, u.d.Clock.Now())
		if err != nil {
			return err
		}
		blob, version, err := u.d.Keyring.SealEnvelope(raw, webhookAAD(route.OrganizationID, route.ConnectionID, id))
		if err != nil {
			return err
		}
		return u.d.Queue.Enqueue(ctx, entity.SyncWebhookJob{
			OrganizationID: route.OrganizationID, ConnectionID: route.ConnectionID,
			DeliveryID: id, PayloadEnc: blob, KeyVersion: version, CorrelationID: corr,
		}, entity.QueueInteractive)
	})
}

func (u *UseCase) RunWebhook(ctx context.Context, job entity.SyncWebhookJob, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	cs, err := u.d.Connect.State(ctx, org, conn)
	if errors.Is(err, entity.ErrNotFound) || err == nil && cs.Connection.Status == entity.ConnectionDeleted {
		return u.completeWebhook(ctx, org, complete)
	}
	if err != nil {
		return err
	}
	raw, err := u.d.Keyring.OpenEnvelope(job.PayloadEnc, job.KeyVersion, webhookAAD(org, conn, job.DeliveryID))
	if err != nil {
		return err
	}
	var in entity.WebhookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	res, err := u.d.Connect.ParseWebhook(ctx, org, conn, in)
	if err != nil {
		return u.failWebhook(ctx, job, err, complete)
	}
	if res.Ignored || len(res.Keys) == 0 {
		return u.completeWebhook(ctx, org, complete)
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	targets, err := u.d.Normalize.WebhookTargets(ctx, org, cs.Instance.ID, res.Keys)
	if err != nil {
		return err
	}
	var dirty []entity.DirtyKey
	var mrs []entity.SyncMRJob
	for _, key := range res.Keys {
		mr := targets[key]
		if key.ObjectKind == "merge_request" || key.ObjectKind == "ci_run" {
			// An explicit MR key in the parsed event also relates a pipeline not collected yet.
			if !mr.Open && key.ObjectKind == "ci_run" {
				for _, candidate := range res.Keys {
					if candidate.ObjectKind == "merge_request" && candidate.NativeNS == key.NativeNS {
						mr = targets[candidate]
						if mr.Open {
							break
						}
					}
				}
			}
		}
		scopeIDs := webhookScopes(scopes, mr.ScopeNativeID, key.ObjectKind)
		if len(scopeIDs) == 0 {
			continue
		}
		if mr.Open {
			mrs = append(mrs, entity.SyncMRJob{OrganizationID: org, ConnectionID: conn, ScopeID: scopeIDs[0], ScopeIDs: scopeIDs, NativeNS: mr.Key.NativeNS, NativeID: mr.Key.NativeID})
			continue
		}
		if kind := webhookDataKind(key.ObjectKind); kind != "" {
			for _, id := range scopeIDs {
				dirty = append(dirty, entity.DirtyKey{ScopeID: id, DataKind: kind, Object: key})
			}
		}
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		for i := range mrs {
			if err := u.d.Queue.EnqueueAt(ctx, mrs[i], entity.QueueInteractive, u.d.Clock.Now().Add(_mrDelay)); err != nil {
				return err
			}
		}
		if len(dirty) > 0 {
			if err := u.d.Repo.AddDirtyKeys(ctx, org, conn, dirty, u.d.Clock.Now()); err != nil {
				return err
			}
			if err := u.d.Queue.EnqueueAt(ctx, entity.SyncDirtyJob{OrganizationID: org, ConnectionID: conn}, entity.QueueBulk, u.d.Clock.Now().Add(_mrDelay)); err != nil {
				return err
			}
		}
		return finishWebhook(ctx, complete)
	})
}

func webhookScopes(scopes []entity.ConnectionScope, nativeScope, objectKind string) []entity.ID {
	var candidates []entity.ID
	for i := range scopes {
		s := &scopes[i]
		if !s.Active {
			continue
		}
		if s.NativeScopeID == nativeScope {
			return []entity.ID{s.ID}
		}
		if nativeScope != "" {
			continue
		}
		eligible := objectKind == "task" && s.Kind == entity.ScopeTrackerProject ||
			(objectKind == "merge_request" || objectKind == "ci_run" || objectKind == "ci_attempt") && s.Kind == entity.ScopeRepository ||
			objectKind == "document" && (s.Kind == entity.ScopeRepository || s.Kind == entity.ScopeWikiSpace) ||
			objectKind == "finding" && (s.Kind == entity.ScopeRepository || s.Kind == entity.ScopeAnalysisProject)
		if eligible {
			candidates = append(candidates, s.ID)
		}
	}
	return candidates
}

func webhookDataKind(kind string) string {
	switch kind {
	case "task":
		return "tasks"
	case "merge_request":
		return "merge_requests"
	case "ci_run", "ci_attempt":
		return "ci"
	case "document":
		return "documents"
	case "finding":
		return "reports"
	}
	return ""
}

func (u *UseCase) failWebhook(ctx context.Context, job entity.SyncWebhookJob, err error, complete Complete) error {
	ae, ok := errors.AsType[*entity.AdapterError](err)
	if !ok {
		return err
	}
	reason := ""
	if ae.Class == entity.CallSource {
		switch ae.Code {
		case "invalid_signature":
			reason = "signature_invalid"
		case "invalid_request":
			reason = "payload_invalid"
		}
	}
	if reason != "" {
		var changed bool
		err := u.d.Tx.InOrgTx(ctx, job.OrganizationID, func(ctx context.Context) error {
			var err error
			changed, err = u.d.Repo.RejectWebhook(ctx, job.OrganizationID, job.DeliveryID, u.d.Clock.Now(), reason)
			if err != nil {
				return err
			}
			return finishWebhook(ctx, complete)
		})
		if err == nil && changed {
			u.d.Metrics.WebhookRejected(job.ConnectionID.String(), reason)
		}
		return err
	}
	f, _ := u.classify(err)
	if f.stop {
		return u.completeWebhook(ctx, job.OrganizationID, complete)
	}
	at := f.at
	if at.IsZero() {
		at = u.d.Clock.Now().Add(Pause(job.Retry, u.d.Rand()))
	}
	retry := job
	retry.Retry++
	u.d.Metrics.CollectionFailed(f.class)
	return u.d.Tx.InOrgTx(ctx, job.OrganizationID, func(ctx context.Context) error {
		if err := finishWebhook(ctx, complete); err != nil {
			return err
		}
		return u.d.Queue.EnqueueAt(ctx, retry, entity.QueueInteractive, at)
	})
}

func finishWebhook(ctx context.Context, complete Complete) error {
	if complete != nil {
		return complete(ctx)
	}
	return nil
}

func (u *UseCase) completeWebhook(ctx context.Context, org entity.ID, complete Complete) error {
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error { return finishWebhook(ctx, complete) })
}
