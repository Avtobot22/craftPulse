package handler

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const (
	_userCursor  = "user"
	_auditCursor = "audit"
)

type administration struct {
	users     usecase.Users
	audit     usecase.AuditLog
	protocols usecase.Protocols
}

func (h *administration) ListUsers(ctx context.Context, req openapi.ListUsersRequestObject) (openapi.ListUsersResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	q, err := userListRequest(req.Params)
	if err != nil {
		return nil, err
	}
	page, err := h.users.ListUsers(ctx, s, q)
	if err != nil {
		return nil, err
	}
	out := openapi.UserPage{Data: make([]openapi.User, len(page.Items))}
	for i, u := range page.Items {
		out.Data[i] = response.User(u)
	}
	if page.Next != nil {
		next := encodeCursor(cursorData{
			Kind: _userCursor, Sort: sortName(q.Sort), Desc: q.Desc,
			Name: page.Next.Name, At: page.Next.CreatedAt, ID: uuid.UUID(page.Next.ID),
		})
		out.NextCursor = &next
	}
	return openapi.ListUsers200JSONResponse(out), nil
}

func (h *administration) ListUserRoles(ctx context.Context, req openapi.ListUserRolesRequestObject) (openapi.ListUserRolesResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	set, err := h.users.UserRoles(ctx, s, entity.ID(req.UserId))
	if err != nil {
		return nil, err
	}
	tag := etag(set.Version)
	return openapi.ListUserRoles200JSONResponse{
		Body: response.Roles(set.Roles), Headers: openapi.ListUserRoles200ResponseHeaders{ETag: &tag},
	}, nil
}

func (h *administration) ReplaceUserRoles(ctx context.Context, req openapi.ReplaceUserRolesRequestObject) (openapi.ReplaceUserRolesResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		return nil, entity.ErrVersionConflict
	}
	if req.Body.Data == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/data", Code: entity.FieldRequired}}}
	}
	roles := make([]entity.RoleAssignment, len(req.Body.Data))
	for i, r := range req.Body.Data {
		roles[i] = entity.RoleAssignment{Role: entity.Role(r.Role), TeamID: idOf(r.TeamId), ProjectID: idOf(r.ProjectId), ServiceID: idOf(r.ServiceId)}
	}
	set, err := h.users.ReplaceUserRoles(ctx, s, entity.ID(req.UserId), base, roles)
	if err != nil {
		return nil, err
	}
	tag := etag(set.Version)
	return openapi.ReplaceUserRoles200JSONResponse{
		Body: response.Roles(set.Roles), Headers: openapi.ReplaceUserRoles200ResponseHeaders{ETag: &tag},
	}, nil
}

func (h *administration) BlockUser(ctx context.Context, req openapi.BlockUserRequestObject) (openapi.BlockUserResponseObject, error) {
	if err := h.change(ctx, entity.ID(req.UserId), h.users.BlockUser); err != nil {
		return nil, err
	}
	return openapi.BlockUser204Response{}, nil
}

func (h *administration) UnblockUser(ctx context.Context, req openapi.UnblockUserRequestObject) (openapi.UnblockUserResponseObject, error) {
	if err := h.change(ctx, entity.ID(req.UserId), h.users.UnblockUser); err != nil {
		return nil, err
	}
	return openapi.UnblockUser204Response{}, nil
}

func (h *administration) EndUserSessions(ctx context.Context, req openapi.EndUserSessionsRequestObject) (openapi.EndUserSessionsResponseObject, error) {
	if err := h.change(ctx, entity.ID(req.UserId), h.users.EndUserSessions); err != nil {
		return nil, err
	}
	return openapi.EndUserSessions204Response{}, nil
}

func (h *administration) ListAuditEvents(ctx context.Context, req openapi.ListAuditEventsRequestObject) (openapi.ListAuditEventsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	q, err := auditQuery(req.Params)
	if err != nil {
		return nil, err
	}
	page, err := h.audit.ListEvents(ctx, s, q)
	if err != nil {
		return nil, err
	}
	out := openapi.AuditEventPage{Data: make([]openapi.AuditEvent, len(page.Items))}
	for i := range page.Items {
		out.Data[i] = response.AuditEvent(&page.Items[i])
	}
	if page.Next != nil {
		next := encodeCursor(cursorData{Kind: _auditCursor, At: page.Next.OccurredAt, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &next
	}
	return openapi.ListAuditEvents200JSONResponse(out), nil
}

func (h *administration) GetLatestInstallationProtocol(ctx context.Context, _ openapi.GetLatestInstallationProtocolRequestObject) (openapi.GetLatestInstallationProtocolResponseObject, error) {
	p, err := h.protocols.LatestProtocol(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetLatestInstallationProtocol200JSONResponse(response.Protocol(p)), nil
}

func (*administration) change(ctx context.Context, userID entity.ID, fn func(context.Context, entity.Subject, entity.ID) error) error {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return entity.ErrAuthRequired
	}
	return fn(ctx, s, userID)
}

func userListRequest(p openapi.ListUsersParams) (entity.UserListRequest, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.UserListRequest{}, err
	}
	q := entity.UserListRequest{Limit: limit, Sort: entity.UserByName}
	if p.Status != nil {
		switch st := entity.UserStatus(*p.Status); st {
		case entity.UserActive, entity.UserBlocked:
			q.Status = st
		default:
			return q, errMalformedPage
		}
	}
	if p.Sort != nil {
		sort := string(*p.Sort)
		q.Desc = strings.HasPrefix(sort, "-")
		switch strings.TrimPrefix(sort, "-") {
		case "display_name":
		case "created_at":
			q.Sort = entity.UserByCreated
		default:
			return q, errMalformedPage
		}
	}
	if p.Cursor == nil {
		return q, nil
	}
	c, ok := decodeCursor(*p.Cursor)
	if !ok || c.Kind != _userCursor || c.Sort != sortName(q.Sort) || c.Desc != q.Desc {
		return q, errMalformedPage
	}
	q.After = &entity.UserKey{Name: c.Name, CreatedAt: c.At, ID: entity.ID(c.ID)}
	return q, nil
}

func auditQuery(p openapi.ListAuditEventsParams) (entity.AuditQuery, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.AuditQuery{}, err
	}
	q := entity.AuditQuery{Limit: limit, ActorID: idOf(p.ActorId)}
	if p.Outcome != nil {
		switch o := entity.AuditOutcome(*p.Outcome); o {
		case entity.AuditSucceeded, entity.AuditDenied, entity.AuditFailed:
			q.Outcome = o
		default:
			return q, errMalformedPage
		}
	}
	if p.OccurredAfter != nil {
		q.From = *p.OccurredAfter
	}
	if p.OccurredBefore != nil {
		q.To = *p.OccurredBefore
	}
	if p.Cursor == nil {
		return q, nil
	}
	c, ok := decodeCursor(*p.Cursor)
	if !ok || c.Kind != _auditCursor {
		return q, errMalformedPage
	}
	q.After = &entity.AuditKey{OccurredAt: c.At, ID: entity.ID(c.ID)}
	return q, nil
}

func sortName(s entity.UserSort) string {
	if s == entity.UserByCreated {
		return "created_at"
	}
	return "display_name"
}

func idOf(id *uuid.UUID) entity.ID {
	if id == nil {
		return entity.ID{}
	}
	return entity.ID(*id)
}
