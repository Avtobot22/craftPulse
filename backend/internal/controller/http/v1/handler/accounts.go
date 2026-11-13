package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const (
	_personCursor  = "person"
	_accountCursor = "source_account"
)

type accounts struct {
	uc          usecase.Accounts
	connections usecase.Connections
}

func (h *accounts) ListPersons(ctx context.Context, req openapi.ListPersonsRequestObject) (openapi.ListPersonsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.PersonFilter{Limit: limit}
	if req.Params.Kind != nil {
		f.Kind = entity.PersonKind(*req.Params.Kind)
	}
	if f.After, err = idCursor(req.Params.Cursor, _personCursor, string(f.Kind)); err != nil {
		return nil, err
	}
	page, err := h.uc.ListPersons(ctx, s, f)
	if err != nil {
		return nil, err
	}
	out := openapi.PersonPage{Data: make([]openapi.Person, 0, len(page.Items))}
	for _, p := range page.Items {
		out.Data = append(out.Data, openapi.Person{Id: uuid.UUID(p.ID), DisplayName: optional(p.DisplayName), Kind: openapi.ActorKind(p.Kind)})
	}
	out.NextCursor = nextIDCursor(page.Next, _personCursor, string(f.Kind))
	return openapi.ListPersons200JSONResponse(out), nil
}

func (h *accounts) ListSourceAccounts(ctx context.Context, req openapi.ListSourceAccountsRequestObject) (openapi.ListSourceAccountsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.SourceAccountFilter{Limit: limit}
	filter := ""
	if req.Params.Assignment != nil {
		f.Assigned = entity.AccountAssignedFilter(*req.Params.Assignment)
		filter = string(f.Assigned)
	}
	if req.Params.InstanceId != nil {
		id := entity.ID(*req.Params.InstanceId)
		f.InstanceID, filter = &id, filter+"/"+id.String()
	}
	if f.After, err = idCursor(req.Params.Cursor, _accountCursor, filter); err != nil {
		return nil, err
	}
	page, err := h.uc.ListSourceAccounts(ctx, s, f)
	if err != nil {
		return nil, err
	}
	providers := map[entity.ID]entity.Provider{}
	out := openapi.SourceAccountPage{Data: make([]openapi.SourceAccount, 0, len(page.Items))}
	for i := range page.Items {
		v := &page.Items[i]
		if _, ok := providers[v.InstanceID]; !ok {
			inst, err := h.connections.Instance(ctx, s.OrganizationID, v.InstanceID)
			if err != nil {
				return nil, err
			}
			providers[v.InstanceID] = inst.Provider
		}
		out.Data = append(out.Data, sourceAccountBody(v, providers[v.InstanceID]))
	}
	out.NextCursor = nextIDCursor(page.Next, _accountCursor, filter)
	return openapi.ListSourceAccounts200JSONResponse(out), nil
}

func sourceAccountBody(v *entity.AccountView, provider entity.Provider) openapi.SourceAccount {
	out := openapi.SourceAccount{
		Id: uuid.UUID(v.ID), InstanceId: uuid.UUID(v.InstanceID), Provider: openapi.Provider(provider), NativeAccountId: v.Account.NativeID,
		Login: optional(v.Account.Login), DisplayName: optional(v.Account.DisplayName), ProposedKind: openapi.ActorKind(v.Account.Kind),
	}
	out.Proposals = emptyOf(out.Proposals, len(v.Proposals))
	for _, p := range v.Proposals {
		item := elemOf(out.Proposals)
		item.Basis, item.PersonId = openapi.SourceAccountProposalsBasis(p.Basis), uuid.UUID(p.PersonID)
		out.Proposals = append(out.Proposals, *item)
	}
	if a := v.Assignment; a != nil {
		out.Assignment = alloc(out.Assignment)
		out.Assignment.Id, out.Assignment.ActorKind, out.Assignment.ConfirmedAt = uuid.UUID(a.ID), openapi.ActorKind(a.ActorKind), a.ConfirmedAt
		out.Assignment.PersonId = uuidPtr(a.PersonID)
	}
	return out
}

func (h *accounts) CreateAccountAssignments(ctx context.Context, req openapi.CreateAccountAssignmentsRequestObject) (openapi.CreateAccountAssignmentsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/items", Code: entity.FieldRequired}}}
	}
	items := make([]entity.AssignmentRequest, 0, len(req.Body.Items))
	for _, it := range req.Body.Items {
		r := entity.AssignmentRequest{
			SourceAccountID: entity.ID(it.SourceAccountId), ActorKind: entity.PersonKind(it.ActorKind),
			CreatePerson: it.CreatePerson != nil && *it.CreatePerson,
		}
		if it.PersonId != nil {
			id := entity.ID(*it.PersonId)
			r.PersonID = &id
		}
		if it.BaseAssignmentId != nil {
			id := entity.ID(*it.BaseAssignmentId)
			r.BaseAssignmentID = &id
		}
		items = append(items, r)
	}
	outcomes, err := h.uc.AssignAccounts(ctx, s, entity.ID(req.Params.IdempotencyKey), items)
	if err != nil {
		return nil, err
	}
	var out openapi.AccountAssignmentBatchResult
	out.Items = emptyOf(out.Items, len(outcomes))
	for _, o := range outcomes {
		item := elemOf(out.Items)
		item.SourceAccountId, item.Outcome = uuid.UUID(o.SourceAccountID), openapi.AccountAssignmentBatchResultItemsOutcome(o.Outcome)
		item.AssignmentId, item.ErrorCode = uuidPtr(o.AssignmentID), optional(o.ErrorCode)
		out.Items = append(out.Items, *item)
	}
	return openapi.CreateAccountAssignments200JSONResponse(out), nil
}

// elemOf makes an element of a slice of an anonymous type of the generated contract.
func elemOf[T any]([]T) *T { return new(T) }

// emptyOf makes an empty slice of that type, which encodes as [] rather than null.
func emptyOf[T any](_ []T, capacity int) []T { return make([]T, 0, capacity) }

// idCursor reads a cursor of a page ordered by ID; the filter it was made for must be the same.
func idCursor(raw *string, kind, filter string) (*entity.ID, error) {
	if raw == nil {
		return nil, nil
	}
	c, ok := decodeCursor(*raw)
	if !ok || c.Kind != kind || c.Name != filter || c.ID == uuid.Nil {
		return nil, errMalformedPage
	}
	id := entity.ID(c.ID)
	return &id, nil
}

func nextIDCursor(next *entity.ID, kind, filter string) *string {
	if next == nil {
		return nil
	}
	c := encodeCursor(cursorData{Kind: kind, Name: filter, ID: uuid.UUID(*next)})
	return &c
}
