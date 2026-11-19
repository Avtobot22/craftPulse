package testadapter

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func (a *Adapter) ParseWebhook(ctx context.Context, call adaptersdk.Call, in adaptersdk.WebhookInput) (adaptersdk.WebhookResult, error) {
	_, fault := a.begin(ctx, call, OpParseWebhook, "", 1)
	if err := a.answer(ctx, fault); err != nil {
		return adaptersdk.WebhookResult{}, err
	}
	raw, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeCollect)
	if err != nil {
		return adaptersdk.WebhookResult{}, err
	}
	var credentials struct {
		Secret string `json:"webhook_secret"`
	}
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidSignature, "webhook secret is not configured")
	}
	token := http.Header{}
	for key, value := range in.Headers {
		token.Set(key, value)
	}
	if credentials.Secret == "" || subtle.ConstantTimeCompare([]byte(credentials.Secret), []byte(token.Get("X-Gitlab-Token"))) != 1 {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidSignature, "webhook signature does not match")
	}
	var body struct {
		EventKind string                 `json:"event_kind"`
		Keys      []adaptersdk.ObjectKey `json:"keys"`
	}
	if err := json.Unmarshal(in.BodyBase64, &body); err != nil {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid webhook payload")
	}
	if body.EventKind != "objects_changed" {
		return adaptersdk.WebhookResult{Ignored: true}, nil
	}
	for _, key := range body.Keys {
		if !key.ObjectKind.Valid() || key.NativeId == "" || len(key.NativeId) > 1024 || len(key.NativeNs) > 255 {
			return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid webhook object key")
		}
	}
	return adaptersdk.WebhookResult{EventKind: &body.EventKind, Keys: body.Keys}, nil
}
