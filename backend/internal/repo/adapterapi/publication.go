package adapterapi

import (
	"context"
	"net/http"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func publicationObject(key entity.ObjectKey) ObjectKey {
	return ObjectKey{ObjectKind: ObjectKeyObjectKind(key.ObjectKind), NativeNs: key.NativeNS, NativeId: key.NativeID}
}

func (c *Caller) FindPublished(ctx context.Context, t entity.AdapterTarget, in entity.PublicationTarget) (string, error) {
	cl, err := c.prepare(ctx, t, entity.OpFindPublished)
	if err != nil {
		return "", err
	}
	body := FindPublishedInput{Action: FindPublishedInputAction(in.Kind), Target: publicationObject(in.Object), Marker: in.Marker}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.FindPublished(ctx, cl.conn, &FindPublishedParams{
			XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID,
		}, body, cl.authorize)
	})
	if err != nil {
		return "", err
	}
	defer discard(resp, cancel)
	var result FindPublishedResult
	if err := decodeJSON(resp, &result); err != nil {
		return "", c.record(t, cl.op, err)
	}
	id := deref(result.ExternalId)
	if result.Found != (id != "") {
		return "", c.record(t, cl.op, violation("inconsistent publication lookup result"))
	}
	return id, c.record(t, cl.op, nil)
}

func (c *Caller) Publish(ctx context.Context, t entity.AdapterTarget, in entity.PublishInput) (entity.Publication, error) {
	cl, err := c.prepare(ctx, t, entity.OpPublish)
	if err != nil {
		return entity.Publication{}, err
	}
	body := PublishInput{
		Action: PublishInputAction(in.Kind), Target: publicationObject(in.Object), Marker: in.Marker,
		BodyMarkdown: in.Markdown, ExternalId: optString(in.ExternalID), ExpectedHeadSha: optString(in.HeadSHA),
	}
	resp, cancel, err := c.send(ctx, cl, http.StatusOK, func(ctx context.Context) (*http.Response, error) {
		return cl.gen.Publish(ctx, cl.conn, &PublishParams{
			XCraftpulseOrganization: cl.org, XCraftpulseContractVersion: cl.ver, XCorrelationId: cl.corrID,
		}, body, cl.authorize)
	})
	if err != nil {
		return entity.Publication{}, err
	}
	defer discard(resp, cancel)
	var result PublishResult
	if err := decodeJSON(resp, &result); err != nil {
		return entity.Publication{}, c.record(t, cl.op, err)
	}
	if !result.Outcome.Valid() || result.Outcome == PublishResultOutcomeSucceeded && deref(result.ExternalId) == "" {
		return entity.Publication{}, c.record(t, cl.op, violation("invalid publication result"))
	}
	return entity.Publication{
		Status: string(result.Outcome), ExternalID: deref(result.ExternalId), ErrorCode: deref(result.ReasonCode),
	}, c.record(t, cl.op, nil)
}
