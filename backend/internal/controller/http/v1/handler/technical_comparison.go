package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

var errMalformedTechnicalComparison = errors.New("invalid technical comparison coordinates")

type comparisons struct{ uc usecase.TechnicalComparisons }

func (h *comparisons) CompareTechnicalResults(ctx context.Context, req openapi.CompareTechnicalResultsRequestObject) (openapi.CompareTechnicalResultsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	if !p.AnalysisKind.Valid() || !entity.ValidGitSHA(p.BaseSha) || !entity.ValidGitSHA(p.HeadSha) {
		return nil, errMalformedTechnicalComparison
	}
	result, err := h.uc.CompareTechnicalResults(ctx, s, scope, entity.TechnicalComparisonQuery{
		ComponentID: entity.ID(p.ComponentId), BaseSHA: p.BaseSha, HeadSHA: p.HeadSha, Kind: string(p.AnalysisKind),
	})
	if err != nil {
		return nil, err
	}
	out := openapi.TechnicalComparison{
		Comparable: result.Comparable, AccessLimited: len(result.Notes) > 0, AccessNotes: accessNotes(result.Notes),
		IncomparableReasons: make([]openapi.TechnicalComparisonIncomparableReasons, len(result.IncomparableReasons)),
	}
	out.Base.Sha, out.Base.ReportId = result.Base.SHA, uuidPtr(result.Base.ReportID)
	out.Head.Sha, out.Head.ReportId = result.Head.SHA, uuidPtr(result.Head.ReportID)
	for i, reason := range result.IncomparableReasons {
		out.IncomparableReasons[i] = openapi.TechnicalComparisonIncomparableReasons(reason)
	}
	out.Factors = emptyOf(out.Factors, len(result.Factors))
	for i := range result.Factors {
		f := &result.Factors[i]
		item := elemOf(out.Factors)
		item.Factor = openapi.TechnicalComparisonFactorsFactor(f.Factor)
		item.FindingIds = make([]uuid.UUID, len(f.FindingIDs))
		for j, id := range f.FindingIDs {
			item.FindingIds[j] = uuid.UUID(id)
		}
		if result.Comparable {
			item.NewCount, item.ResolvedCount = new(f.NewCount), new(f.ResolvedCount)
		}
		out.Factors = append(out.Factors, *item)
	}
	return openapi.CompareTechnicalResults200JSONResponse(out), nil
}
