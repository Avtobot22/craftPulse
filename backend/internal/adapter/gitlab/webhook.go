package gitlab

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const _gitLabTokenHeader = "X-Gitlab-Token" //nolint:gosec // header name only

func (*Adapter) ParseWebhook(ctx context.Context, call adaptersdk.Call, in adaptersdk.WebhookInput) (adaptersdk.WebhookResult, error) {
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeWebhookSecret)
	if err != nil && !hasCode(err, adaptersdk.AdapterErrorCodeAuth) {
		return adaptersdk.WebhookResult{}, err
	}
	if !validWebhookToken(in.Headers, secret) {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidSignature, "GitLab webhook signature does not match")
	}

	var event struct {
		Kind string `json:"object_kind"`
	}
	if err := json.Unmarshal(in.BodyBase64, &event); err != nil || event.Kind == "" {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab webhook payload")
	}
	switch event.Kind {
	case "merge_request", "note", "pipeline", "build":
		var payload webhookPayload
		if err := json.Unmarshal(in.BodyBase64, &payload); err != nil {
			return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab webhook payload")
		}
		return payload.result(event.Kind)
	default:
		return ignoredWebhook(), nil
	}
}

func validWebhookToken(headers map[string]string, secret string) bool {
	var token string
	count := 0
	for name, value := range headers {
		if strings.EqualFold(name, _gitLabTokenHeader) {
			token = value
			count++
		}
	}
	if count != 1 || token == "" || secret == "" {
		return false
	}
	provided, expected := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(provided[:], expected[:]) == 1
}

type webhookPayload struct {
	PipelineID int64 `json:"pipeline_id"`
	Attributes struct {
		ID         int64  `json:"id"`
		TargetType string `json:"noteable_type"` //nolint:misspell // GitLab field name
		TargetID   int64  `json:"noteable_id"`   //nolint:misspell // GitLab field name
	} `json:"object_attributes"`
	MergeRequest struct {
		ID int64 `json:"id"`
	} `json:"merge_request"`
}

func (p webhookPayload) result(kind string) (adaptersdk.WebhookResult, error) {
	var object adaptersdk.ObjectKeyObjectKind
	var event string
	var nativeID int64
	switch kind {
	case "merge_request":
		event, object, nativeID = "merge_request", adaptersdk.ObjectKeyObjectKindMergeRequest, p.Attributes.ID
	case "note":
		if p.Attributes.TargetType == "MergeRequest" {
			event, object, nativeID = "merge_request", adaptersdk.ObjectKeyObjectKindMergeRequest, p.MergeRequest.ID
			if nativeID == 0 {
				nativeID = p.Attributes.TargetID
			}
		}
	case "pipeline":
		event, object, nativeID = "pipeline", adaptersdk.ObjectKeyObjectKindCiRun, p.Attributes.ID
	case "build":
		event, object, nativeID = "pipeline", adaptersdk.ObjectKeyObjectKindCiRun, p.PipelineID
	}
	if event == "" {
		return ignoredWebhook(), nil
	}
	if nativeID <= 0 {
		return adaptersdk.WebhookResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "GitLab webhook is missing the object ID")
	}
	keys := []adaptersdk.ObjectKey{{ObjectKind: object, NativeId: id(nativeID)}}
	if object == adaptersdk.ObjectKeyObjectKindCiRun && p.MergeRequest.ID > 0 {
		keys = append(keys, adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: id(p.MergeRequest.ID)})
	}
	return adaptersdk.WebhookResult{EventKind: &event, Keys: keys}, nil
}

func ignoredWebhook() adaptersdk.WebhookResult {
	return adaptersdk.WebhookResult{Ignored: true, Keys: []adaptersdk.ObjectKey{}}
}
