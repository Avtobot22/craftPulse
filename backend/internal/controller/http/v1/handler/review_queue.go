package handler

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (h *codeDetails) ListMergeRequests(ctx context.Context, req openapi.ListMergeRequestsRequestObject) (openapi.ListMergeRequestsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	q, err := queueQuery(req.Params)
	if err != nil {
		return nil, err
	}
	const kind = "review_queue"
	filter := strings.Join([]string{s.OrganizationID.String(), s.UserID.String(), queueID(q.Project), queueID(q.Area), q.State, q.Sort, string(q.Wait)}, ":")
	if raw := req.Params.Cursor; raw != nil {
		c, ok := decodeCursor(*raw)
		if !ok || c.Kind != kind || c.Name != filter || c.ID == uuid.Nil || c.OpenedAt.IsZero() || c.WaitSeconds < -1 || c.Seq == nil || *c.Seq < 0 {
			return nil, errMalformedPage
		}
		q.After = &entity.MRQueueCursor{AsOf: c.At, Seq: *c.Seq, WaitSeconds: c.WaitSeconds, OpenedAt: c.OpenedAt, ID: entity.ID(c.ID)}
	}
	page, err := h.uc.ListMergeRequests(ctx, s, scope, q)
	if err != nil {
		return nil, err
	}
	out := openapi.MergeRequestPage{Data: make([]openapi.MergeRequest, len(page.Items)), AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes)}
	for i := range page.Items {
		out.Data[i] = mergeRequestBody(&page.Items[i])
	}
	if n := page.Next; n != nil {
		cursor := encodeCursor(cursorData{Kind: kind, Name: filter, At: n.AsOf, Seq: &n.Seq, OpenedAt: n.OpenedAt, WaitSeconds: n.WaitSeconds, ID: uuid.UUID(n.ID)})
		out.NextCursor = &cursor
	}
	return openapi.ListMergeRequests200JSONResponse(out), nil
}

func queueQuery(p openapi.ListMergeRequestsParams) (entity.MRQueueQuery, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.MRQueueQuery{}, err
	}
	q := entity.MRQueueQuery{Project: (*entity.ID)(p.ProjectId), Area: (*entity.ID)(p.AreaId), State: "open", Sort: "-wait", Limit: limit}
	if p.State != nil {
		if !p.State.Valid() {
			return q, errMalformedPage
		}
		q.State = string(*p.State)
	}
	if p.Sort != nil {
		if !p.Sort.Valid() {
			return q, errMalformedPage
		}
		q.Sort = string(*p.Sort)
	}
	if p.ReviewWait != nil {
		q.Wait = entity.ReviewWaitKind(*p.ReviewWait)
		if q.Wait != entity.ReviewNoReviewer && q.Wait != entity.ReviewAwaitingFirst {
			return q, errMalformedPage
		}
	}
	return q, nil
}

func queueID(id *entity.ID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
