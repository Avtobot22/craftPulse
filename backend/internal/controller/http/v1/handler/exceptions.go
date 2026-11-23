package handler

import (
	"context"
	"math"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const _exceptionCursor = "exception"

type exceptions struct {
	uc     usecase.Exceptions
	drafts usecase.ExceptionDrafts
}

func (h *exceptions) CreateException(ctx context.Context, req openapi.CreateExceptionRequestObject) (openapi.CreateExceptionResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	d, err := exceptionDraft(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.drafts.CreateException(ctx, s, scope, entity.ExceptionWrite{Key: entity.ID(req.Params.IdempotencyKey), Draft: d})
	if err != nil {
		return nil, err
	}
	tag, location := etag(int64(v.Current.Number)), "/api/v1/exceptions/"+v.ID.String()
	return openapi.CreateException201JSONResponse{
		Body:    exceptionDetail(&v),
		Headers: openapi.CreateException201ResponseHeaders{Location: &location, ETag: &tag},
	}, nil
}

func (h *exceptions) CreateExceptionVersion(ctx context.Context, req openapi.CreateExceptionVersionRequestObject,
) (openapi.CreateExceptionVersionResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	d, err := exceptionDraft(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.drafts.CreateException(ctx, s, scope, entity.ExceptionWrite{
		ID: entity.ID(req.ExceptionId), Base: exceptionBase(req.Params.IfMatch), Key: entity.ID(req.Params.IdempotencyKey), Draft: d,
	})
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.Current.Number))
	return openapi.CreateExceptionVersion201JSONResponse{
		Body:    exceptionDetail(&v),
		Headers: openapi.CreateExceptionVersion201ResponseHeaders{ETag: &tag},
	}, nil
}

func (h *exceptions) DecideException(ctx context.Context, req openapi.DecideExceptionRequestObject) (openapi.DecideExceptionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, requiredExceptionField("/decision")
	}
	v, err := h.uc.Decide(ctx, s, entity.ExceptionDecision{
		ID: entity.ID(req.ExceptionId), Key: entity.ID(req.Params.IdempotencyKey), Base: exceptionBase(req.Params.IfMatch),
		Action: string(req.Body.Decision), Comment: exceptionText(req.Body.Comment),
	})
	if err != nil {
		return nil, err
	}
	return openapi.DecideException200JSONResponse(exceptionDetail(&v)), nil
}

func (h *exceptions) GetException(ctx context.Context, req openapi.GetExceptionRequestObject) (openapi.GetExceptionResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.uc.Get(ctx, s, scope, entity.ID(req.ExceptionId))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.Current.Number))
	return openapi.GetException200JSONResponse{Body: exceptionDetail(&v), Headers: openapi.GetException200ResponseHeaders{ETag: &tag}}, nil
}

func (h *exceptions) ListExceptions(ctx context.Context, req openapi.ListExceptionsRequestObject) (openapi.ListExceptionsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	q, bound, err := exceptionQuery(&req.Params, s.OrganizationID)
	if err != nil {
		return nil, err
	}
	page, err := h.uc.List(ctx, s, scope, q)
	if err != nil {
		return nil, err
	}
	out := openapi.ExceptionPage{
		Data: make([]openapi.ExceptionSummary, len(page.Items)), AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes),
	}
	for i := range page.Items {
		v := &page.Items[i]
		out.Data[i] = openapi.ExceptionSummary{
			Id: uuid.UUID(v.ID), State: openapi.ExceptionSummaryState(v.State), CurrentVersion: exceptionVersion(&v.Current),
		}
	}
	if c := page.Next; c != nil {
		cursor := encodeCursor(cursorData{Kind: _exceptionCursor, Sort: bound, ID: uuid.UUID(c.ID), At: c.At, OpenedAt: c.SortAt, Seq: &c.Seq})
		out.NextCursor = &cursor
	}
	return openapi.ListExceptions200JSONResponse(out), nil
}

func exceptionQuery(p *openapi.ListExceptionsParams, org entity.ID) (entity.ExceptionQuery, string, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.ExceptionQuery{}, "", err
	}
	q := entity.ExceptionQuery{Limit: limit, Sort: "expires_at", ComponentID: exceptionID(p.ComponentId), FindingID: exceptionID(p.FindingId)}
	if p.State != nil {
		q.State = string(*p.State)
	}
	if p.Sort != nil {
		q.Sort = string(*p.Sort)
	}
	switch q.State {
	case "", entity.ExceptionPending, entity.ExceptionActive, entity.ExceptionRejected, entity.ExceptionWithdrawn, entity.ExceptionExpired:
	default:
		return q, "", errMalformedPage
	}
	switch q.Sort {
	case "expires_at", "-expires_at", "created_at", "-created_at":
	default:
		return q, "", errMalformedPage
	}
	bound := org.String() + "/" + q.Sort + "/" + q.State + "/" + q.ComponentID.String() + "/" + q.FindingID.String()
	if p.Cursor != nil {
		c, ok := decodeCursor(*p.Cursor)
		if !ok || c.Kind != _exceptionCursor || c.Sort != bound || c.Seq == nil || c.At.IsZero() || c.OpenedAt.IsZero() || c.ID == uuid.Nil {
			return q, "", errMalformedPage
		}
		q.After = &entity.ExceptionCursor{ID: entity.ID(c.ID), At: c.At, SortAt: c.OpenedAt, Seq: *c.Seq}
	}
	return q, bound, nil
}

func exceptionDraft(b *openapi.ExceptionDraft) (entity.ExceptionDraft, error) {
	if b == nil {
		return entity.ExceptionDraft{}, requiredExceptionField("/reason")
	}
	s := &b.Scope
	return entity.ExceptionDraft{
		Type: string(b.DecisionType), Reason: b.Reason, EffectiveFrom: b.EffectiveFrom, ExpiresAt: b.ExpiresAt,
		Scope: entity.ExceptionScope{
			AreaID: entity.ID(s.AreaId), ComponentID: exceptionID(s.ComponentId), FindingID: exceptionID(s.FindingId),
			RuleID: exceptionText(s.RuleId), PathPrefix: exceptionText(s.PathPrefix), PackagePURL: exceptionText(s.PackagePurl),
		},
	}, nil
}

func exceptionDetail(v *entity.ExceptionView) openapi.ExceptionDetail {
	out := openapi.ExceptionDetail{
		Id: uuid.UUID(v.ID), State: openapi.ExceptionDetailState(v.State), CurrentVersion: exceptionVersion(&v.Current),
		Versions: make([]openapi.ExceptionVersion, len(v.Versions)), Transitions: make([]openapi.ExceptionTransition, len(v.Transitions)),
	}
	for i := range v.Versions {
		out.Versions[i] = exceptionVersion(&v.Versions[i])
	}
	for i := range v.Transitions {
		t := &v.Transitions[i]
		out.Transitions[i] = openapi.ExceptionTransition{
			VersionNo:   t.Version,
			ActorUserId: uuid.UUID(t.ActorID), OccurredAt: t.At, Status: openapi.ExceptionTransitionStatus(t.Status),
		}
	}
	return out
}

func exceptionVersion(v *entity.ExceptionVersion) openapi.ExceptionVersion {
	s := &v.Scope
	return openapi.ExceptionVersion{
		VersionNo: v.Number, ApprovalRequired: v.ApprovalRequired, AuthorUserId: uuid.UUID(v.AuthorID), CreatedAt: v.CreatedAt,
		DecisionType: openapi.ExceptionVersionDecisionType(v.Type), Reason: v.Reason, Status: openapi.ExceptionVersionStatus(v.Status),
		EffectiveFrom: v.EffectiveFrom, ExpiresAt: v.ExpiresAt,
		Scope: openapi.ExceptionScope{
			AreaId: uuid.UUID(s.AreaID), ComponentId: exceptionUUID(s.ComponentID), FindingId: exceptionUUID(s.FindingID),
			RuleId: optional(s.RuleID), PathPrefix: optional(s.PathPrefix), PackagePurl: optional(s.PackagePURL),
		},
	}
}

func exceptionBase(tag string) int {
	base, ok := parseETag(tag)
	if !ok || base > math.MaxInt32 {
		return -1
	}
	return int(base)
}

func exceptionID(v *uuid.UUID) entity.ID {
	if v != nil {
		return entity.ID(*v)
	}
	return entity.ID{}
}

func requiredExceptionField(field string) error {
	return &entity.ValidationError{Fields: []entity.FieldError{{Field: field, Code: entity.FieldRequired}}}
}

func exceptionText(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func exceptionUUID(id entity.ID) *uuid.UUID {
	if id.IsZero() {
		return nil
	}
	return uuidPtr(&id)
}
