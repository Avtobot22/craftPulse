package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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
	structure usecase.Structure
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
	page, err := h.list(ctx, entity.AreaTeam, entity.AreaFilter{}, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListTeams200JSONResponse(page), nil
}

func (h *organization) ListProjects(ctx context.Context, req openapi.ListProjectsRequestObject) (openapi.ListProjectsResponseObject, error) {
	var f entity.AreaFilter
	if req.Params.TeamId != nil {
		f.TeamID = entity.ID(*req.Params.TeamId)
	}
	page, err := h.list(ctx, entity.AreaProject, f, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListProjects200JSONResponse(openapi.ProjectPage(page)), nil
}

func (h *organization) ListServices(ctx context.Context, req openapi.ListServicesRequestObject) (openapi.ListServicesResponseObject, error) {
	page, err := h.list(ctx, entity.AreaService, entity.AreaFilter{}, req.Params.Limit, req.Params.Cursor, (*string)(req.Params.Sort))
	if err != nil {
		return nil, err
	}
	return openapi.ListServices200JSONResponse(openapi.ServicePage(page)), nil
}

// ListAreas has nothing to list yet: source areas come from connections, and this build has none.
func (*organization) ListAreas(_ context.Context, req openapi.ListAreasRequestObject) (openapi.ListAreasResponseObject, error) {
	if _, err := pageRequest(entity.AreaKind("source_area"), req.Params.Limit, req.Params.Cursor, nil); err != nil {
		return nil, err
	}
	return openapi.ListAreas200JSONResponse{Data: []openapi.AreaRef{}, NextCursor: nil}, nil
}

func (h *organization) list(ctx context.Context, kind entity.AreaKind, f entity.AreaFilter, limit *int, cursor, sort *string) (openapi.TeamPage, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return openapi.TeamPage{}, entity.ErrAuthRequired
	}
	scope, ok := middleware.ScopeFrom(ctx)
	if !ok {
		return openapi.TeamPage{}, entity.ErrForbidden
	}
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
	Desc bool      `json:"d"`
	Name string    `json:"n"`
	ID   uuid.UUID `json:"i"`
}

func pageRequest(kind entity.AreaKind, limit *int, cursor, sort *string) (entity.PageRequest, error) {
	p := entity.PageRequest{Limit: _defaultLimit}
	if limit != nil {
		if *limit < 1 || *limit > _maxLimit {
			return p, errMalformedPage
		}
		p.Limit = *limit
	}
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
		ProjectAreas: len(b.ProjectAreas),
		Components:   len(b.Components),
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
