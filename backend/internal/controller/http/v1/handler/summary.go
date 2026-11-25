package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var errMalformedSummaryRevision = errors.New("invalid summary revision number")

func (h *codeDetails) GetMergeRequestSummary(ctx context.Context, req openapi.GetMergeRequestSummaryRequestObject) (openapi.GetMergeRequestSummaryResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if no := req.Params.RevisionNo; no != nil && (*no < 1 || *no > 2147483647) {
		return nil, errMalformedSummaryRevision
	}
	v, hidden, err := h.uc.MergeRequestSummary(ctx, s, scope, entity.ID(req.MergeRequestId), req.Params.RevisionNo)
	if err != nil {
		return nil, err
	}
	out, err := summaryBody(&v, hidden)
	if err != nil {
		return nil, err
	}
	return openapi.GetMergeRequestSummary200JSONResponse(out), nil
}

func summaryBody(v *entity.SummaryRevision, hidden bool) (openapi.MergeRequestSummary, error) {
	out := openapi.MergeRequestSummary{
		Id: uuid.UUID(v.ID), MergeRequestId: uuid.UUID(v.MRID), RevisionNo: v.No,
		Status: openapi.MergeRequestSummaryStatus(v.Status), HeadSha: v.HeadSHA, BaseSha: optional(v.BaseSHA),
		PipelineState: openapi.MergeRequestSummaryPipelineState(v.PipelineState), StaleReason: optional(v.StaleReason),
		Items: make([]openapi.SummaryItem, 0, len(v.Items)), Redactions: []string{}, AccessLimited: hidden,
		AccessNotes: []openapi.AccessNote{}, CoverageChangedLines: summaryPoint("ratio", entity.Point{}),
	}
	out.NotEvaluated = make([]struct {
		AnalysisKind openapi.AnalysisKind `json:"analysis_kind"`
		ReasonCode   string               `json:"reason_code"`
	}, 0)
	if !v.BuiltAt.IsZero() {
		out.BuiltAt = &v.BuiltAt
	}
	if hidden {
		out.AccessNotes = accessNotes([]entity.AccessNote{{Code: entity.NoteRestricted}})
	}
	out.Publication.Status = openapi.MergeRequestSummaryPublicationStatus(v.Publication.Status)
	out.Publication.ExternalId, out.Publication.ErrorCode = optional(v.Publication.ExternalID), optional(v.Publication.ErrorCode)
	out.Publication.PublishedAt = v.Publication.PublishedAt
	for i := range v.Items {
		item := &v.Items[i]
		row := openapi.SummaryItem{
			ItemNo: i + 1, ItemKind: openapi.SummaryItemItemKind(item.Kind), FindingId: uuidPtr(item.FindingID),
			Severity: optional(item.Severity), Rank: item.Rank,
		}
		if kind := openapi.AnalysisKind(item.AnalysisKind); kind.Valid() {
			row.AnalysisKind = &kind
			if item.Kind == "not_evaluated" {
				out.NotEvaluated = append(out.NotEvaluated, struct {
					AnalysisKind openapi.AnalysisKind `json:"analysis_kind"`
					ReasonCode   string               `json:"reason_code"`
				}{kind, item.ReasonCode})
			}
		}
		if err := summaryItemValue(&out, &row, item); err != nil {
			return openapi.MergeRequestSummary{}, err
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

func summaryItemValue(out *openapi.MergeRequestSummary, row *openapi.SummaryItem, item *entity.SummaryItem) error {
	if len(item.Value) == 0 {
		return nil
	}
	switch item.Kind {
	case "coverage":
		var data struct {
			Patch entity.Point
			Size  struct {
				State      entity.DataState
				ReasonCode string
				Total      struct{ Added, Deleted, Files int64 }
			}
			Code []struct {
				Complexity struct {
					State      entity.DataState
					ReasonCode string
					Deltas     []struct{ Delta *int }
				}
				Clones entity.Point
			}
		}
		if err := json.Unmarshal(item.Value, &data); err != nil {
			return err
		}
		out.CoverageChangedLines = summaryPoint("ratio", data.Patch)
		size := entity.Point{State: data.Size.State, ReasonCode: data.Size.ReasonCode}
		var complexity, clones entity.Point
		if len(data.Code) == 1 {
			code := &data.Code[0]
			complexity.State, complexity.ReasonCode = code.Complexity.State, code.Complexity.ReasonCode
			var increased int64
			for _, f := range code.Complexity.Deltas {
				if f.Delta != nil && *f.Delta > 0 {
					increased++
				}
			}
			complexity = summaryCount(complexity, increased)
			clones = code.Clones
		}
		out.Change = &struct {
			ComplexityIncreasedFunctions openapi.MetricValue `json:"complexity_increased_functions"`
			FilesChanged                 openapi.MetricValue `json:"files_changed"`
			LinesAdded                   openapi.MetricValue `json:"lines_added"`
			LinesRemoved                 openapi.MetricValue `json:"lines_removed"`
			NewCloneGroups               openapi.MetricValue `json:"new_clone_groups"`
		}{
			ComplexityIncreasedFunctions: summaryPoint("count", complexity),
			FilesChanged:                 summaryPoint("count", summaryCount(size, data.Size.Total.Files)),
			LinesAdded:                   summaryPoint("count", summaryCount(size, data.Size.Total.Added)),
			LinesRemoved:                 summaryPoint("count", summaryCount(size, data.Size.Total.Deleted)),
			NewCloneGroups:               summaryPoint("count", clones),
		}
	case "tests":
		var data entity.TestMetric
		if err := json.Unmarshal(item.Value, &data); err != nil {
			return err
		}
		out.Tests = &struct {
			Errored openapi.MetricValue `json:"errored"`
			Failed  openapi.MetricValue `json:"failed"`
			Passed  openapi.MetricValue `json:"passed"`
			Skipped openapi.MetricValue `json:"skipped"`
		}{}
		point := entity.Point{State: data.State, ReasonCode: data.ReasonCode}
		missing := summaryPoint("count", point)
		out.Tests.Passed, out.Tests.Failed, out.Tests.Errored, out.Tests.Skipped = missing, missing, missing, missing
		if c := data.Counts; c != nil {
			out.Tests.Passed = summaryPoint("count", summaryCount(point, int64(c.Passed)))
			out.Tests.Failed = summaryPoint("count", summaryCount(point, int64(c.Failed)))
			out.Tests.Errored = summaryPoint("count", summaryCount(point, int64(c.Errors)))
			out.Tests.Skipped = summaryPoint("count", summaryCount(point, int64(c.Skipped)))
		}
	case "ci_attempts":
		return summaryCI(out, item.Value)
	default:
		var loc struct {
			entity.SummarySignalLocation
			LineEnd *int `json:"line_end"`
		}
		if err := json.Unmarshal(item.Value, &loc); err != nil {
			return err
		}
		row.RuleId = optional(loc.Rule)
		if loc.Path != "" {
			row.Location = &openapi.CodeLocation{Path: loc.Path, LineStart: loc.Line, LineEnd: loc.LineEnd}
		}
	}
	return nil
}

func summaryPoint(unit string, p entity.Point) openapi.MetricValue {
	if p.State == "" {
		p.State = entity.StateNotEvaluated
	}
	return metricValue(unit, &p)
}

func summaryCount(p entity.Point, count int64) entity.Point {
	if p.State == entity.StateAssessed || p.State == entity.StateProvisional {
		p.Value = new(float64(count))
	}
	return p
}

func summaryCI(out *openapi.MergeRequestSummary, raw json.RawMessage) error {
	var data struct {
		PipelineID       uuid.UUID                            `json:"pipeline_id"`
		Outcome          openapi.MergeRequestSummaryCiOutcome `json:"outcome"`
		ParentPipelineID *uuid.UUID                           `json:"parent_pipeline_id"`
		AttemptVersions  []uuid.UUID                          `json:"attempt_versions"`
		RetriedJobs      int                                  `json:"retried_jobs"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.PipelineID != uuid.Nil {
		if data.Outcome == "" {
			data.Outcome = "unknown"
		}
		out.Ci = alloc(out.Ci)
		out.Ci.Attempts = len(data.AttemptVersions)
		out.Ci.Outcome = data.Outcome
		out.Ci.ParentPipelineId = data.ParentPipelineID
		out.Ci.PipelineId = data.PipelineID
		out.Ci.RetriedJobs = data.RetriedJobs
	}
	return nil
}
