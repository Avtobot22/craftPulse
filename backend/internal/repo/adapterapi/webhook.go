package adapterapi

import (
	"context"
	"net/http"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (c *Caller) ParseWebhook(ctx context.Context, t entity.AdapterTarget, in entity.WebhookInput) (entity.WebhookResult, error) {
	cl, err := c.prepare(ctx, t, entity.OpParseWebhook)
	if err != nil {
		return entity.WebhookResult{}, err
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.ParseWebhook(ctx, cl.conn, &ParseWebhookParams{XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID}, WebhookInput{Headers: in.Headers, BodyBase64: in.Body}, cl.authorize)
	})
	if err != nil {
		return entity.WebhookResult{}, err
	}
	defer func() { _ = resp.Body.Close(); cancel() }()
	var res WebhookResult
	if err := decodeJSON(resp, &res); err != nil {
		return entity.WebhookResult{}, c.record(t, cl.op, err)
	}
	out := entity.WebhookResult{Ignored: res.Ignored, Keys: make([]entity.ObjectKey, 0, len(res.Keys))}
	for _, key := range res.Keys {
		if !key.ObjectKind.Valid() || key.NativeId == "" || len(key.NativeId) > 1024 || len(key.NativeNs) > 255 {
			return entity.WebhookResult{}, c.record(t, cl.op, violation("webhook result has an invalid object key"))
		}
		out.Keys = append(out.Keys, entity.ObjectKey{ObjectKind: string(key.ObjectKind), NativeNS: key.NativeNs, NativeID: key.NativeId})
	}
	return out, c.record(t, cl.op, nil)
}
