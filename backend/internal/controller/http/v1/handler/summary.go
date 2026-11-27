package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var errMalformedSummarySelector = errors.New("invalid summary selector")

func (h *codeDetails) GetMergeRequestSummary(ctx context.Context, req openapi.GetMergeRequestSummaryRequestObject) (openapi.GetMergeRequestSummaryResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if no := req.Params.RevisionNo; no != nil && (*no < 1 || *no > math.MaxInt32) {
		return nil, errMalformedSummarySelector
	}
	if head := req.Params.HeadSha; head != nil && !entity.ValidGitSHA(*head) {
		return nil, errMalformedSummarySelector
	}
	summary, err := h.uc.MergeRequestSummary(ctx, s, scope, entity.ID(req.MergeRequestId), req.Params.RevisionNo, req.Params.HeadSha)
	if err != nil {
		return nil, err
	}
	body, err := summaryBody(&summary)
	if err != nil {
		return nil, err
	}
	return openapi.GetMergeRequestSummary200JSONResponse(body), nil
}

type summaryMetrics struct {
	ci                                                  *entity.SummaryCI
	coverage, added, removed, files, complexity, clones entity.Point
	passed, failed, errored, skipped                    entity.Point
	checks, reports                                     entity.Point
}

func summaryBody(s *entity.SummaryRead) (openapi.MergeRequestSummary, error) {
	out := openapi.MergeRequestSummary{
		Id: uuid.UUID(s.ID), MergeRequestId: uuid.UUID(s.MRID), RevisionNo: s.No,
		Status: openapi.MergeRequestSummaryStatus(s.Status), HeadSha: s.HeadSHA, BaseSha: optional(s.BaseSHA),
		PipelineState: openapi.MergeRequestSummaryPipelineState(s.PipelineState), BuiltAt: s.BuiltAt, StaleReason: optional(s.StaleReason),
		Items: make([]openapi.SummaryItem, 0, len(s.Items)), Redactions: s.Redactions,
		AccessLimited: len(s.Redactions) > 0, AccessNotes: []openapi.AccessNote{},
	}
	out.NotEvaluated = make([]struct {
		AnalysisKind openapi.AnalysisKind `json:"analysis_kind"`
		ReasonCode   string               `json:"reason_code"`
	}, 0)
	if out.AccessLimited {
		out.AccessNotes = accessNotes([]entity.AccessNote{{Code: entity.NoteRestricted}})
	}
	out.Publication.Status = openapi.MergeRequestSummaryPublicationStatus(s.Publication.Status)
	out.Publication.ExternalId = optional(s.Publication.ExternalID)
	out.Publication.ErrorCode = optional(s.Publication.ErrorCode)
	out.Publication.PublishedAt = s.Publication.PublishedAt
	if g := s.LastGood; g != nil {
		out.LastGood = &struct {
			BuiltAt    time.Time      `json:"built_at"`
			HeadSha    openapi.GitSha `json:"head_sha"`
			RevisionNo int            `json:"revision_no"`
		}{g.BuiltAt, g.HeadSHA, g.No}
	}
	var m summaryMetrics
	for i := range s.Items {
		item := &s.Items[i]
		out.Items = append(out.Items, summaryItemBody(item, i+1))
		if item.Kind == "not_evaluated" {
			out.NotEvaluated = append(out.NotEvaluated, struct {
				AnalysisKind openapi.AnalysisKind `json:"analysis_kind"`
				ReasonCode   string               `json:"reason_code"`
			}{openapi.AnalysisKind(item.AnalysisKind), item.ReasonCode})
		}
		if err := m.read(item, s.SecurityAllowed); err != nil {
			return out, fmt.Errorf("decode summary item %s: %w", item.Kind, err)
		}
	}
	if s.SourceRestricted {
		// The stored I09 point includes reports from separately authorized sources.
		m.reports = entity.Point{}
	}
	m.apply(&out)
	return out, nil
}

func summaryItemBody(i *entity.SummaryItem, no int) openapi.SummaryItem {
	out := openapi.SummaryItem{
		ItemNo: no, ItemKind: openapi.SummaryItemItemKind(i.Kind), FindingId: uuidPtr(i.FindingID),
		RuleId: optional(i.RuleID), Severity: optional(i.Severity), Rank: i.Rank,
		ComponentId: uuidPtr(i.ComponentID),
	}
	if kind := openapi.AnalysisKind(i.AnalysisKind); kind.Valid() {
		out.AnalysisKind = &kind
	}
	if mode := openapi.SummaryItemMode(i.Mode); mode.Valid() {
		out.Mode = &mode
	}
	if i.Path != "" {
		out.Location = &openapi.CodeLocation{Path: i.Path, LineStart: i.LineStart, LineEnd: i.LineEnd}
	}
	return out
}

func (m *summaryMetrics) read(i *entity.SummaryItem, security bool) error {
	if len(i.Value) == 0 {
		return nil
	}
	switch i.Kind {
	case "ci_attempts":
		var v struct {
			Display       *entity.SummaryCI
			Metrics       []entity.SeriesPoint
			PublicChecks  entity.SeriesPoint `json:"public_checks"`
			PublicReports entity.SeriesPoint `json:"public_reports"`
		}
		if err := json.Unmarshal(i.Value, &v); err != nil {
			return err
		}
		// Older snapshots have no public I08. The full point is only safe for security readers.
		m.ci, m.checks, m.reports = v.Display, v.PublicChecks.Point, v.PublicReports.Point
		for idx := range v.Metrics {
			p := &v.Metrics[idx]
			switch {
			case p.MetricID == "I08" && security:
				m.checks = p.Point
			case p.MetricID == "I09" && security:
				m.reports = p.Point
			}
		}
	case "tests":
		var v entity.TestMetric
		if err := json.Unmarshal(i.Value, &v); err != nil {
			return err
		}
		m.passed, m.failed = summaryCount(v.State, v.ReasonCode, 0), summaryCount(v.State, v.ReasonCode, 0)
		m.errored, m.skipped = m.passed, m.failed
		if v.Counts != nil {
			m.passed = summaryCount(v.State, v.ReasonCode, int64(v.Counts.Passed))
			m.failed = summaryCount(v.State, v.ReasonCode, int64(v.Counts.Failed))
			m.errored = summaryCount(v.State, v.ReasonCode, int64(v.Counts.Errors))
			m.skipped = summaryCount(v.State, v.ReasonCode, int64(v.Counts.Skipped))
		}
	case "coverage":
		return m.readStructure(i.Value)
	}
	return nil
}

func summaryCount(state entity.DataState, reason string, n int64) entity.Point {
	p := entity.Point{State: state, ReasonCode: reason}
	if state == entity.StateAssessed || state == entity.StateProvisional {
		p.Value = new(float64(n))
	}
	return p
}

func (m *summaryMetrics) readStructure(body json.RawMessage) error {
	var v struct {
		Patch entity.Point
		Size  struct {
			State      entity.DataState
			ReasonCode string
			Total      struct{ Files, Added, Deleted int64 }
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
	if err := json.Unmarshal(body, &v); err != nil {
		return err
	}
	m.coverage = v.Patch
	m.added = summaryCount(v.Size.State, v.Size.ReasonCode, v.Size.Total.Added)
	m.removed = summaryCount(v.Size.State, v.Size.ReasonCode, v.Size.Total.Deleted)
	m.files = summaryCount(v.Size.State, v.Size.ReasonCode, v.Size.Total.Files)
	for idx := range v.Code {
		stream := &v.Code[idx]
		var increased int64
		for _, p := range stream.Complexity.Deltas {
			if p.Delta != nil && *p.Delta > 0 {
				increased++
			}
		}
		m.complexity = addSummaryCounts(m.complexity, summaryCount(stream.Complexity.State, stream.Complexity.ReasonCode, increased))
		m.clones = addSummaryCounts(m.clones, stream.Clones)
	}
	for _, p := range []*entity.Point{&m.complexity, &m.clones} {
		if p.Value != nil && p.MissingCount > 0 {
			p.State, p.ReasonCode = entity.StateProvisional, "scan_incomplete"
		}
	}
	return nil
}

func addSummaryCounts(total, stream entity.Point) entity.Point {
	if stream.Value == nil {
		total.MissingCount++
		if total.State == "" {
			total.State, total.ReasonCode = stream.State, stream.ReasonCode
		}
		return total
	}
	if total.Value == nil {
		stream.MissingCount += total.MissingCount
		return stream
	}
	total.Value = new(*total.Value + *stream.Value)
	if total.Numerator != nil && stream.Numerator != nil {
		total.Numerator = new(*total.Numerator + *stream.Numerator)
	} else {
		total.Numerator = nil
	}
	total.ExcludedCount += stream.ExcludedCount
	total.MissingCount += stream.MissingCount
	if stream.State == entity.StateProvisional && total.State != entity.StateProvisional {
		total.State, total.ReasonCode = stream.State, stream.ReasonCode
	}
	return total
}

func summaryMetric(unit string, p entity.Point) openapi.MetricValue {
	if p.State == "" {
		p.State = entity.StateNotEvaluated
	}
	return metricValue(unit, &p)
}

func (m *summaryMetrics) apply(out *openapi.MergeRequestSummary) {
	out.CoverageChangedLines = summaryMetric("ratio", m.coverage)
	out.Change = &struct {
		ComplexityIncreasedFunctions openapi.MetricValue `json:"complexity_increased_functions"`
		FilesChanged                 openapi.MetricValue `json:"files_changed"`
		LinesAdded                   openapi.MetricValue `json:"lines_added"`
		LinesRemoved                 openapi.MetricValue `json:"lines_removed"`
		NewCloneGroups               openapi.MetricValue `json:"new_clone_groups"`
	}{
		ComplexityIncreasedFunctions: summaryMetric("count", m.complexity),
		FilesChanged:                 summaryMetric("count", m.files),
		LinesAdded:                   summaryMetric("count", m.added),
		LinesRemoved:                 summaryMetric("count", m.removed),
		NewCloneGroups:               summaryMetric("count", m.clones),
	}
	out.Tests = &struct {
		Errored openapi.MetricValue `json:"errored"`
		Failed  openapi.MetricValue `json:"failed"`
		Passed  openapi.MetricValue `json:"passed"`
		Skipped openapi.MetricValue `json:"skipped"`
	}{summaryMetric("count", m.errored), summaryMetric("count", m.failed), summaryMetric("count", m.passed), summaryMetric("count", m.skipped)}
	out.Expected = &struct {
		MissingChecks  openapi.MetricValue `json:"missing_checks"`
		MissingReports openapi.MetricValue `json:"missing_reports"`
	}{summaryMetric("ratio", m.checks), summaryMetric("ratio", m.reports)}
	if c := m.ci; c != nil {
		out.Ci = &struct {
			Attempts         int                                  `json:"attempts"`
			Outcome          openapi.MergeRequestSummaryCiOutcome `json:"outcome"`
			ParentPipelineId *uuid.UUID                           `json:"parent_pipeline_id,omitempty"` //nolint:revive,staticcheck // Match the generated OpenAPI type.
			PipelineId       uuid.UUID                            `json:"pipeline_id"`                  //nolint:revive,staticcheck // Match the generated OpenAPI type.
			RetriedJobs      int                                  `json:"retried_jobs"`
		}{c.Attempts, openapi.MergeRequestSummaryCiOutcome(c.Outcome), uuidPtr(c.ParentPipelineID), uuid.UUID(c.PipelineID), c.RetriedJobs}
	}
}
