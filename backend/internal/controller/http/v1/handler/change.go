package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type checks struct {
	uc usecase.CheckProfiles
}

func (h *checks) GetCheckProfile(ctx context.Context, req openapi.GetCheckProfileRequestObject) (openapi.GetCheckProfileResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	v, err := h.uc.CheckProfile(ctx, s, entity.ID(req.ProjectId))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.VersionNo))
	return openapi.GetCheckProfile200JSONResponse{Body: checkProfileBody(&v), Headers: openapi.GetCheckProfile200ResponseHeaders{ETag: &tag}}, nil
}

func (h *checks) CreateCheckProfileVersion(ctx context.Context, req openapi.CreateCheckProfileVersionRequestObject) (openapi.CreateCheckProfileVersionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil || req.Body.Entries == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/entries", Code: entity.FieldRequired}}}
	}
	// A version that is not ours cannot match the current one; the usecase still answers a repeated key first.
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		base = -1
	}
	d := entity.CheckProfileDraft{Entries: make([]entity.ProfileEntry, len(req.Body.Entries))}
	for i, e := range req.Body.Entries {
		d.Entries[i] = entity.ProfileEntry{
			ComponentID: entity.ID(e.ComponentId), Language: e.Language, AnalysisKind: string(e.AnalysisKind), Mode: string(e.Mode), ProfileID: e.ProfileId,
		}
	}
	v, created, err := h.uc.CreateCheckProfileVersion(ctx, s, entity.ID(req.ProjectId), int(base), entity.ID(req.Params.IdempotencyKey), d)
	if err != nil {
		return nil, err
	}
	tag := etag(int64(v.VersionNo))
	if !created {
		return openapi.CreateCheckProfileVersion200JSONResponse{Body: checkProfileBody(&v), Headers: openapi.CreateCheckProfileVersion200ResponseHeaders{ETag: &tag}}, nil
	}
	return openapi.CreateCheckProfileVersion201JSONResponse{Body: checkProfileBody(&v), Headers: openapi.CreateCheckProfileVersion201ResponseHeaders{ETag: &tag}}, nil
}

func (h *checks) GetCiCheckProfile(ctx context.Context, _ openapi.GetCiCheckProfileRequestObject) (openapi.GetCiCheckProfileResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok || s.UploadTokenID.IsZero() {
		return nil, entity.ErrAuthRequired
	}
	p, err := h.uc.CICheckProfile(ctx, s)
	if err != nil {
		return nil, err
	}
	out := openapi.CiCheckProfile{
		ProjectId: uuid.UUID(p.ProjectID), ProfileVersionNo: p.VersionNo, ExpectedReports: make([]openapi.AnalysisKind, len(p.ExpectedReports)),
	}
	out.Limits.MaxPackageBytes, out.Limits.MaxUnpackedBytes, out.Limits.MaxFiles = int(p.Limits.MaxPackageBytes), int(p.Limits.MaxUnpackedBytes), p.Limits.MaxFiles
	for i, k := range p.ExpectedReports {
		out.ExpectedReports[i] = openapi.AnalysisKind(k)
	}
	out.Entries = emptyOf(out.Entries, len(p.Entries))
	for i := range p.Entries {
		e := &p.Entries[i]
		item := elemOf(out.Entries)
		item.ComponentId, item.PathPrefix, item.Language, item.ProfileId = uuid.UUID(e.ComponentID), e.PathPrefix, e.Language, e.ProfileID
		item.AnalysisKind, item.Mode, item.ImageDigest = openapi.AnalysisKind(e.AnalysisKind), openapi.CiCheckProfileEntriesMode(e.Mode), optional(e.ImageDigest)
		out.Entries = append(out.Entries, *item)
	}
	out.DocumentationRoots = emptyOf(out.DocumentationRoots, len(p.DocumentationRoots))
	for _, r := range p.DocumentationRoots {
		item := elemOf(out.DocumentationRoots)
		item.RootId, item.Kind, item.PathPrefix = uuid.UUID(r.RootID), openapi.CiCheckProfileDocumentationRootsKind(r.Kind), r.PathPrefix
		out.DocumentationRoots = append(out.DocumentationRoots, *item)
	}
	return openapi.GetCiCheckProfile200JSONResponse(out), nil
}

func (h *checks) GetCiConfiguration(ctx context.Context, req openapi.GetCiConfigurationRequestObject) (openapi.GetCiConfigurationResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	c, err := h.uc.CIConfiguration(ctx, s, entity.ID(req.ProjectId), string(req.Params.CiSystem))
	if err != nil {
		return nil, err
	}
	out := openapi.CiConfiguration{CiSystem: openapi.CiConfigurationCiSystem(c.System), ProfileVersionNo: c.VersionNo, Snippet: c.Snippet}
	out.Variables = emptyOf(out.Variables, len(c.Variables))
	for _, v := range c.Variables {
		item := elemOf(out.Variables)
		item.Name, item.Masked = v.Name, v.Masked
		out.Variables = append(out.Variables, *item)
	}
	return openapi.GetCiConfiguration200JSONResponse(out), nil
}

func checkProfileBody(v *entity.CheckProfileVersion) openapi.CheckProfileVersion {
	out := openapi.CheckProfileVersion{
		VersionNo: v.VersionNo, BaseVersionNo: v.BaseVersionNo, ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt.UTC(),
	}
	out.Entries = emptyOf(out.Entries, len(v.Entries))
	for i := range v.Entries {
		e := &v.Entries[i]
		item := elemOf(out.Entries)
		item.ComponentId, item.Language, item.ProfileId, item.ToolVersions = uuid.UUID(e.ComponentID), e.Language, e.ProfileID, e.ToolVersions
		item.AnalysisKind, item.Mode = openapi.AnalysisKind(e.AnalysisKind), openapi.CheckProfileVersionEntriesMode(e.Mode)
		if item.ToolVersions == nil {
			item.ToolVersions = map[string]string{}
		}
		out.Entries = append(out.Entries, *item)
	}
	out.Uncovered = emptyOf(out.Uncovered, len(v.Uncovered))
	for _, u := range v.Uncovered {
		item := elemOf(out.Uncovered)
		item.ComponentId, item.Language, item.ReasonCode = uuid.UUID(u.ComponentID), u.Language, u.ReasonCode
		out.Uncovered = append(out.Uncovered, *item)
	}
	return out
}
