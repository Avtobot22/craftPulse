package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _targetCursor names the cursor of listTargetVersions; its Sort binds the team and the filter it came from.
const _targetCursor = "target_version"

func (h *process) GetTargetBaselines(ctx context.Context, req openapi.GetTargetBaselinesRequestObject) (openapi.GetTargetBaselinesResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	b, err := h.targets.TargetBaselines(ctx, s, entity.ID(req.TeamId), entity.Window{From: req.Params.From.UTC(), To: req.Params.To.UTC()})
	if err != nil {
		return nil, err
	}
	out := openapi.TargetBaselineList{RevisionId: uuid.UUID(b.RevisionID), Data: emptyOf(openapi.TargetBaselineList{}.Data, len(b.Items))}
	for i := range b.Items {
		it := &b.Items[i]
		item := elemOf(out.Data)
		item.TaskType, item.SizeLabel, item.P85 = it.TaskType, it.SizeLabel, metricValue(it.Unit, &it.P85)
		out.Data = append(out.Data, *item)
	}
	return openapi.GetTargetBaselines200JSONResponse(out), nil
}

// ListTargetVersions gives the ETag of the group only when the filter names one, as the contract says.
func (h *process) ListTargetVersions(ctx context.Context, req openapi.ListTargetVersionsRequestObject) (openapi.ListTargetVersionsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	team := entity.ID(req.TeamId)
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.TargetVersionFilter{TaskType: req.Params.TaskType, SizeLabel: req.Params.SizeLabel, Limit: limit}
	bound := targetFilter(team, &f)
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != _targetCursor || c.Sort != bound {
			return nil, errMalformedPage
		}
		before, err := strconv.ParseInt(c.Name, 10, 64)
		if err != nil || before < 1 {
			return nil, errMalformedPage
		}
		f.Before = &before
	}
	page, err := h.uc.ListTargetVersions(ctx, s, team, f)
	if err != nil {
		return nil, err
	}
	body := openapi.TargetVersionPage{Data: make([]openapi.TargetVersion, len(page.Items))}
	for i := range page.Items {
		body.Data[i] = targetVersionBody(&page.Items[i])
	}
	if page.Next != nil {
		next := encodeCursor(cursorData{Kind: _targetCursor, Sort: bound, Name: strconv.FormatInt(*page.Next, 10)})
		body.NextCursor = &next
	}
	out := openapi.ListTargetVersions200JSONResponse{Body: body}
	if page.Latest != nil {
		tag := etag(int64(*page.Latest))
		out.Headers.ETag = &tag
	}
	return out, nil
}

func (h *process) CreateTargetVersion(ctx context.Context, req openapi.CreateTargetVersionRequestObject) (openapi.CreateTargetVersionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/task_type", Code: entity.FieldRequired}}}
	}
	// A version that is not ours cannot match the current one; the usecase still answers a repeated key first.
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		base = -1
	}
	v, created, err := h.targets.CreateTargetVersion(ctx, s, entity.ID(req.TeamId), int(base), entity.ID(req.Params.IdempotencyKey),
		targetDraft(req.Body))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.VersionNo))
	if !created {
		return openapi.CreateTargetVersion200JSONResponse{Body: targetVersionBody(&v), Headers: openapi.CreateTargetVersion200ResponseHeaders{ETag: &tag}}, nil
	}
	return openapi.CreateTargetVersion201JSONResponse{Body: targetVersionBody(&v), Headers: openapi.CreateTargetVersion201ResponseHeaders{ETag: &tag}}, nil
}

// targetFilter binds a cursor to the team and the filter; a missing filter differs from an empty one.
func targetFilter(team entity.ID, f *entity.TargetVersionFilter) string {
	part := func(s *string) string {
		if s == nil {
			return "-"
		}
		return strconv.Quote(*s)
	}
	return team.String() + "/" + part(f.TaskType) + "/" + part(f.SizeLabel)
}

func targetDraft(b *openapi.TargetDraft) entity.TargetDraft {
	d := entity.TargetDraft{
		TargetGroup: entity.TargetGroup{TaskType: b.TaskType, SizeLabel: b.SizeLabel}, DurationSeconds: int64(b.DurationSeconds),
		Basis: string(b.Basis),
	}
	if b.BasisRevisionId != nil {
		id := entity.ID(*b.BasisRevisionId)
		d.BasisRevisionID = &id
	}
	if p := b.BasisPeriod; p != nil {
		d.BasisPeriod = &entity.Window{From: p.From.UTC(), To: p.To.UTC()}
	}
	return d
}

func targetVersionBody(v *entity.TargetVersion) openapi.TargetVersion {
	out := openapi.TargetVersion{
		TaskType: v.TaskType, SizeLabel: v.SizeLabel, VersionNo: v.VersionNo, BaseVersionNo: v.BaseVersionNo,
		DurationSeconds: int(v.DurationSeconds), Basis: openapi.TargetVersionBasis(v.Basis), ConfirmedBy: uuid.UUID(v.ConfirmedBy),
		ConfirmedAt: v.ConfirmedAt,
	}
	if b := v.BasisOf; b != nil {
		id, value, n := openapi_types.UUID(b.RevisionID), int(b.ValueSeconds), b.N
		out.BasisRevisionId, out.BasisValueSeconds, out.BasisN = &id, &value, &n
		out.BasisPeriod = &struct {
			From time.Time `json:"from"`
			To   time.Time `json:"to"`
		}{From: b.Window.From, To: b.Window.To}
	}
	return out
}
