package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const (
	_defaultLimit = 50
	_maxLimit     = 200
	_sortDesc     = "-name"
)

var errMalformedPage = errors.New("handler: limit, sort or cursor is invalid")

type organization struct {
	structure   usecase.Structure
	access      usecase.Access
	connections usecase.Connections
}

func (h *organization) GetOrganizationStructure(ctx context.Context, _ openapi.GetOrganizationStructureRequestObject) (openapi.GetOrganizationStructureResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	st, err := h.structure.Structure(ctx, s)
	if err != nil {
		return nil, err
	}
	tag := etag(st.Version)
	return openapi.GetOrganizationStructure200JSONResponse{
		Body: response.Structure(st), Headers: openapi.GetOrganizationStructure200ResponseHeaders{ETag: &tag},
	}, nil
}

func (h *organization) ReplaceOrganizationStructure(ctx context.Context, req openapi.ReplaceOrganizationStructureRequestObject) (openapi.ReplaceOrganizationStructureResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	// A version that is not ours cannot match the current one.
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		return nil, entity.ErrVersionConflict
	}
	in, err := structureInput(req.Body)
	if err != nil {
		return nil, err
	}
	st, err := h.structure.ReplaceStructure(ctx, s, base, in)
	if err != nil {
		return nil, err
	}
	tag := etag(st.Version)
	return openapi.ReplaceOrganizationStructure200JSONResponse{
		Body: response.Structure(st), Headers: openapi.ReplaceOrganizationStructure200ResponseHeaders{ETag: &tag},
	}, nil
}

func (h *organization) ListTeams(ctx context.Context, req openapi.ListTeamsRequestObject) (openapi.ListTeamsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.list(ctx, s, scope, entity.AreaTeam, entity.AreaFilter{}, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListTeams200JSONResponse(page), nil
}

// ListProjects keeps the projects open in at least one of their sources for the user now (TZ 7.7).
func (h *organization) ListProjects(ctx context.Context, req openapi.ListProjectsRequestObject) (openapi.ListProjectsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	var f entity.AreaFilter
	if req.Params.TeamId != nil {
		f.TeamID = entity.ID(*req.Params.TeamId)
	}
	if _, err := pageRequest(entity.AreaProject, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort)); err != nil {
		return nil, err
	}
	scope, err = h.access.Decide(ctx, s, scope, entity.SourceQuery{Operation: "listProjects", Projects: true})
	if err != nil {
		return nil, err
	}
	page, err := h.list(ctx, s, scope, entity.AreaProject, f, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListProjects200JSONResponse(openapi.ProjectPage{
		Data: page.Data, NextCursor: page.NextCursor, AccessLimited: scope.Limited(), AccessNotes: response.AccessNotes(scope.Notes()),
	}), nil
}

func (h *organization) ListServices(ctx context.Context, req openapi.ListServicesRequestObject) (openapi.ListServicesResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.list(ctx, s, scope, entity.AreaService, entity.AreaFilter{}, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListServices200JSONResponse(openapi.ServicePage(page)), nil
}

// _sourceAreaCursor names the cursor of listAreas; its Sort binds the filters the cursor came from.
const _sourceAreaCursor = "source_area"

// ListAreas pages the source areas of active connections of the organization.
func (h *organization) ListAreas(ctx context.Context, req openapi.ListAreasRequestObject) (openapi.ListAreasResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.SourceAreaFilter{Limit: limit}
	if req.Params.Kind != nil {
		f.Kind = entity.ScopeKind(*req.Params.Kind)
	}
	if req.Params.ConnectionId != nil {
		f.ConnectionID = entity.ID(*req.Params.ConnectionId)
	}
	filter := string(f.Kind) + "/" + f.ConnectionID.String()
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != _sourceAreaCursor || c.Sort != filter || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		f.After = entity.ID(c.ID)
	}
	page, err := h.connections.ListAreas(ctx, s, f)
	if err != nil {
		return nil, err
	}
	out := openapi.ListAreas200JSONResponse{Data: response.SourceAreas(page.Items)}
	if page.Next != nil {
		next := encodeCursor(cursorData{Kind: _sourceAreaCursor, Sort: filter, ID: uuid.UUID(*page.Next)})
		out.NextCursor = &next
	}
	return out, nil
}

func subjectScope(ctx context.Context) (entity.Subject, entity.Scope, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return entity.Subject{}, entity.Scope{}, entity.ErrAuthRequired
	}
	scope, ok := middleware.ScopeFrom(ctx)
	if !ok {
		return entity.Subject{}, entity.Scope{}, entity.ErrForbidden
	}
	return s, scope, nil
}

func (h *organization) list(ctx context.Context, s entity.Subject, scope entity.Scope, kind entity.AreaKind, f entity.AreaFilter,
	limit *int, cursor, sort *string,
) (openapi.TeamPage, error) {
	p, err := pageRequest(kind, limit, cursor, sort)
	if err != nil {
		return openapi.TeamPage{}, err
	}
	page, err := h.structure.ListAreas(ctx, s, scope, kind, f, p)
	if err != nil {
		return openapi.TeamPage{}, err
	}
	out := openapi.TeamPage{Data: response.Areas(page.Items)}
	if page.Next != nil {
		next := encodeCursor(cursorData{Kind: string(kind), Desc: p.Desc, Name: page.Next.Name, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &next
	}
	return out, nil
}

// cursorData is opaque to the client; it binds the position to the list and the order it came from.
type cursorData struct {
	Kind string    `json:"k"`
	Sort string    `json:"s,omitempty"`
	Desc bool      `json:"d"`
	Name string    `json:"n,omitempty"`
	At   time.Time `json:"t,omitzero"`
	ID   uuid.UUID `json:"i"`
}

func pageLimit(limit *int) (int, error) {
	if limit == nil {
		return _defaultLimit, nil
	}
	if *limit < 1 || *limit > _maxLimit {
		return 0, errMalformedPage
	}
	return *limit, nil
}

func pageRequest(kind entity.AreaKind, limit *int, cursor, sort *string) (entity.PageRequest, error) {
	n, err := pageLimit(limit)
	if err != nil {
		return entity.PageRequest{}, err
	}
	p := entity.PageRequest{Limit: n}
	if sort != nil {
		switch *sort {
		case "name":
		case _sortDesc:
			p.Desc = true
		default:
			return p, errMalformedPage
		}
	}
	if cursor == nil {
		return p, nil
	}
	c, ok := decodeCursor(*cursor)
	if !ok || c.Kind != string(kind) || c.Desc != p.Desc {
		return p, errMalformedPage
	}
	p.After = &entity.PageKey{Name: c.Name, ID: entity.ID(c.ID)}
	return p, nil
}

func encodeCursor(c cursorData) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string) (cursorData, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursorData{}, false
	}
	var c cursorData
	if err := json.Unmarshal(b, &c); err != nil {
		return cursorData{}, false
	}
	return c, true
}

func etag(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

func parseETag(raw string) (int64, bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "W/")
	v, err := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
	return v, err == nil && v >= 0
}

// structureInput maps the body; a missing collection is reported like the other field errors.
func structureInput(b *openapi.OrganizationStructureInput) (entity.StructureInput, error) {
	verr := &entity.ValidationError{}
	for _, c := range []struct {
		field   string
		missing bool
	}{
		{"/teams", b.Teams == nil},
		{"/projects", b.Projects == nil},
		{"/services", b.Services == nil},
		{"/components", b.Components == nil},
		{"/team_projects", b.TeamProjects == nil},
		{"/project_areas", b.ProjectAreas == nil},
		{"/team_members", b.TeamMembers == nil},
	} {
		if c.missing {
			verr.Add(c.field, entity.FieldRequired, "collection is required, send [] when empty")
		}
	}
	if len(verr.Fields) > 0 {
		return entity.StructureInput{}, verr
	}
	in := entity.StructureInput{
		Teams:        areaInputs(b.Teams),
		Projects:     areaInputs(b.Projects),
		Services:     areaInputs(b.Services),
		TeamProjects: make([]entity.LinkInput, len(b.TeamProjects)),
		TeamMembers:  make([]entity.LinkInput, len(b.TeamMembers)),
		ProjectAreas: make([]entity.LinkInput, len(b.ProjectAreas)),
		Components:   len(b.Components),
	}
	for i, l := range b.ProjectAreas {
		in.ProjectAreas[i] = entity.LinkInput{From: entity.ID(l.ProjectId), To: entity.ID(l.AreaId), ValidFrom: l.ValidFrom, ValidTo: l.ValidTo}
	}
	for i, l := range b.TeamProjects {
		in.TeamProjects[i] = entity.LinkInput{From: entity.ID(l.TeamId), To: entity.ID(l.ProjectId), ValidFrom: l.ValidFrom, ValidTo: l.ValidTo}
	}
	for i, l := range b.TeamMembers {
		in.TeamMembers[i] = entity.LinkInput{From: entity.ID(l.TeamId), To: entity.ID(l.PersonId), ValidFrom: l.ValidFrom, ValidTo: l.ValidTo}
	}
	return in, nil
}

func areaInputs(in []openapi.NamedEntityInput) []entity.AreaInput {
	out := make([]entity.AreaInput, len(in))
	for i, a := range in {
		out[i] = entity.AreaInput{Name: a.Name, Status: string(a.Status)}
		if a.Id != nil {
			out[i].ID = entity.ID(*a.Id)
		}
	}
	return out
}
