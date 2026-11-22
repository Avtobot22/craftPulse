package view

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type PipelineChecks interface {
	ExpectedPipelineReports(context.Context, entity.Scope, entity.ID, entity.ID, time.Time) ([]entity.ExpectedReport, error)
}

func (u *UseCase) TestExecutions(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, q entity.TestExecutionQuery) (entity.TestExecutionList, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return entity.TestExecutionList{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/limit", Code: entity.FieldOutOfRange}}}
	}
	now := u.d.Clock.Now()
	issued := now
	if q.IssuedAt != nil {
		issued = *q.IssuedAt
		if issued.After(now) || now.Sub(issued) > 24*time.Hour {
			return entity.TestExecutionList{}, entity.ErrInvalidCursor
		}
	}
	seq, err := u.d.Code.ReportKnowledge(ctx, s.OrganizationID, q.KnownSeq)
	if err != nil {
		return entity.TestExecutionList{}, err
	}
	detail, _, err := u.readPipeline(ctx, s, scope, id, "listTestExecutions", seq)
	if err != nil {
		return entity.TestExecutionList{}, err
	}
	out, err := u.d.Code.PipelineTestExecutions(ctx, s.OrganizationID, pipelineReportScope(&detail.Run, seq), q)
	out.IssuedAt = issued
	return out, err
}

func pipelineReportScope(run *entity.CIRun, seq int64) entity.ReportScope {
	root := &run.Run
	scope := entity.ReportScope{AreaID: root.AreaID, SHA: root.SHA, Pipeline: &root.RunID, KnownSeq: seq, RunIDs: []entity.ID{root.RunID}}
	runs := map[entity.ID]bool{root.RunID: true}
	for i := range run.Children {
		child := &run.Children[i]
		if child.AreaID == root.AreaID && child.SHA == root.SHA {
			runs[child.RunID] = true
			scope.RunIDs = append(scope.RunIDs, child.RunID)
		}
	}
	for i := range run.Attempts {
		attempt := &run.Attempts[i]
		if runs[attempt.RunID] {
			scope.AttemptIDs = append(scope.AttemptIDs, attempt.AttemptID)
		}
	}
	return scope
}

type reportKindState struct {
	expected []entity.ExpectedReport
	actual   []*entity.ReportEvidence
}

func pipelineReports(expected []entity.ExpectedReport, actual []entity.ReportEvidence) []entity.PipelineReport {
	kinds := map[string]reportKindState{}
	for _, want := range expected {
		state := kinds[want.Kind]
		state.expected = append(state.expected, want)
		kinds[want.Kind] = state
	}
	for i := range actual {
		got := &actual[i]
		state := kinds[got.Kind]
		state.actual = append(state.actual, got)
		kinds[got.Kind] = state
	}
	out := make([]entity.PipelineReport, 0, len(kinds))
	for _, kind := range slices.Sorted(maps.Keys(kinds)) {
		state := kinds[kind]
		out = append(out, entity.PipelineReport{Kind: kind, State: state.result()})
	}
	return out
}

func (s reportKindState) result() string {
	if len(s.actual) == 0 {
		return entity.PipelineReportMissing
	}
	usable := 0
	partial := false
	for _, r := range s.actual {
		if r.RejectedReason != "" {
			continue
		}
		usable++
		partial = partial || r.Partial()
	}
	if usable == 0 {
		return entity.PipelineReportMalformed
	}
	if partial || usable < len(s.actual) || s.missingExpected() {
		return entity.PipelineReportPartial
	}
	return entity.PipelineReportReceived
}

func (s reportKindState) missingExpected() bool {
	for _, want := range s.expected {
		matched := slices.ContainsFunc(s.actual, func(r *entity.ReportEvidence) bool {
			if r.RejectedReason != "" {
				return false
			}
			// A file from CI without a manifest may omit these dimensions. It can satisfy an
			// unambiguous expectation for this kind, but cannot cover an entire matrix.
			component := (r.ComponentID != nil && *r.ComponentID == want.ComponentID) || (r.ComponentID == nil && len(s.expected) == 1)
			language := want.Language == "" || r.Language == want.Language || (r.Language == "" && len(s.expected) == 1)
			return component && language
		})
		if !matched {
			return true
		}
	}
	return false
}
