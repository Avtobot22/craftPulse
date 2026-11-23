package calc

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var _findingAt = time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)

func findingInput(kind string) FindingMetricsInput {
	in := FindingMetricsInput{
		OrganizationID: entity.ID{1}, AreaID: entity.ID{2}, ComponentID: entity.ID{3}, Kind: kind, HeadSHA: "head", BaseSHA: "base",
		KnownSeq: 11, AsOf: _findingAt,
		Head: completeScan(entity.ID{2}, "head", entity.ID{4}, entity.AnalysisCode),
		Base: completeScan(entity.ID{2}, "base", entity.ID{5}, entity.AnalysisCode),
	}
	if kind != entity.FindingCode {
		in.Head.Kind, in.Base.Kind = entity.AnalysisSAST, entity.AnalysisSAST
	}
	return in
}

func completeScan(area entity.ID, sha string, id entity.ID, kind string) *FindingScan {
	return &FindingScan{AreaID: area, SHA: sha, ReportEvidence: entity.ReportEvidence{ID: id, Kind: kind, ExitClass: "findings", Completeness: "complete"}}
}

func classified(in *FindingMetricsInput, id entity.ID, disposition FindingDisposition) ClassifiedFinding {
	f := ClassifiedFinding{
		FindingCoordinates: entity.FindingCoordinates{
			FindingAccess: entity.FindingAccess{AreaID: in.AreaID, Kind: in.Kind}, ComponentID: in.ComponentID,
			FindingID: id, RuleID: "scanner-rule", Path: "src/pkg.go",
		},
		ReportID: in.Head.ID, OccurrenceID: entity.ID{22}, SourceID: entity.ID{23}, Class: "style",
		Disposition: disposition, NativeSeverity: "warning", LineStart: new(12),
	}
	if disposition == FindingResolved {
		f.ReportID = in.Base.ID
	}
	return f
}

func measured(t *testing.T, all []entity.SeriesPoint, id, estimator string) entity.Point {
	t.Helper()
	i := slices.IndexFunc(all, func(p entity.SeriesPoint) bool { return p.MetricID == id && p.Estimator == estimator })
	require.GreaterOrEqual(t, i, 0, [2]string{id, estimator})
	return all[i].Point
}

func assertReading(t *testing.T, p entity.Point, state entity.DataState, value *float64) {
	t.Helper()
	assert.Equal(t, state, p.State)
	assert.Equal(t, value, p.Value)
}

type findingDecisionStub struct {
	call func(context.Context, entity.ID, entity.ID, int64, time.Time) ([]entity.ExceptionVersion, error)
}

func (d findingDecisionStub) ActiveAndExpiredAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error) {
	return d.call(ctx, org, area, seq, at)
}

func TestFindingMetrics_ExceptionsRespectScopeAndTime(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		status    string
		otherArea bool
		atExpiry  bool
		open      float64
		active    float64
		expired   float64
	}{
		{"approved in scope A", entity.ExceptionApproved, false, false, 0, 1, 0},
		{"same rule in scope B", entity.ExceptionApproved, true, false, 1, 0, 0},
		{"pending approval", entity.ExceptionPending, false, false, 1, 0, 0},
		{"rejected approval", entity.ExceptionRejected, false, false, 1, 0, 0},
		{"exact expiration", entity.ExceptionExpired, false, true, 1, 0, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := findingInput(entity.FindingCode)
			f := classified(&in, entity.ID{6}, FindingNew)
			in.Findings = []ClassifiedFinding{f, f}
			v := entity.ExceptionVersion{
				ID: entity.ID{7}, DecisionID: entity.ID{8}, AuthorID: entity.ID{9}, FindingKind: in.Kind, Status: tt.status,
				ExceptionDraft: entity.ExceptionDraft{
					Type: entity.ExceptionAcceptedRisk, Reason: "reviewed scope", Scope: entity.ExceptionScope{AreaID: in.AreaID, RuleID: f.RuleID, PathPrefix: "src"},
					EffectiveFrom: in.AsOf.Add(-time.Hour), ExpiresAt: in.AsOf.Add(time.Hour),
				},
			}
			if tt.otherArea {
				v.Scope.AreaID = entity.ID{10}
			}
			if tt.atExpiry {
				in.AsOf = v.ExpiresAt
			}
			u := New(Deps{Decisions: findingDecisionStub{call: func(_ context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error) {
				assert.Equal(t, in.OrganizationID, org)
				assert.Equal(t, in.AreaID, area)
				assert.Equal(t, in.KnownSeq, seq)
				assert.Equal(t, in.AsOf, at)
				return []entity.ExceptionVersion{v}, nil
			}}})
			all, err := u.FindingMetricsAt(t.Context(), in)
			require.NoError(t, err)
			open := measured(t, all.Points, "C11", "")
			assertReading(t, open, entity.StateAssessed, num(tt.open))
			require.Len(t, open.Contributions, 1, "one contribution per identity")
			var basis findingEvidence
			require.NoError(t, json.Unmarshal(open.Contributions[0].Evidence, &basis))
			assert.Equal(t, f.Path, basis.Path)
			assert.Equal(t, f.RuleID, basis.Rule)
			assert.Equal(t, f.NativeSeverity, basis.NativeSeverity)
			assert.Equal(t, f.Class, basis.Class)
			assert.Equal(t, f.LineStart, basis.Line)
			assert.Equal(t, f.SourceID, basis.SourceID)
			active, expired := measured(t, all.Points, "C26", _estActive), measured(t, all.Points, "C26", _estExpired)
			assertReading(t, active, entity.StateAssessed, num(tt.active))
			assertReading(t, expired, entity.StateAssessed, num(tt.expired))
			if tt.active == 1 {
				assert.Equal(t, entity.RoleExcluded, open.Contributions[0].Role)
				require.Len(t, active.Contributions, 1)
				var decision exceptionEvidence
				require.NoError(t, json.Unmarshal(active.Contributions[0].Evidence, &decision))
				assert.Equal(t, v.AuthorID, decision.AuthorID)
				assert.Equal(t, in.AreaID, decision.Scope.AreaID)
				assert.Equal(t, v.Reason, decision.Reason)
				assert.Equal(t, v.ExpiresAt.Format(time.RFC3339Nano), decision.ExpiresAt)
			}
		})
	}
}

func TestFindingMetrics_RequiresComparableCompleteScansForDelta(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		change    func(*FindingMetricsInput)
		openState entity.DataState
		openValue *float64
		delta     entity.DataState
		reason    string
		observed  *float64
	}{
		{"full comparable scans", func(*FindingMetricsInput) {}, entity.StateAssessed, num(2), entity.StateAssessed, "", nil},
		{"baseline absent", func(in *FindingMetricsInput) { in.Base = nil }, entity.StateAssessed, num(2), entity.StateNotEvaluated, "comparable_baseline_missing", num(2)},
		{"scope changed", func(in *FindingMetricsInput) { in.IncomparableReason = "scope_changed" }, entity.StateAssessed, num(2), entity.StateNotEvaluated, "scope_changed", num(2)},
		{"head missing", func(in *FindingMetricsInput) { in.Head = nil }, entity.StateNotEvaluated, nil, entity.StateNotEvaluated, _reasonReportMissing, nil},
		{"previous head report", func(in *FindingMetricsInput) { in.Head.SHA = "old" }, entity.StateNotEvaluated, nil, entity.StateNotEvaluated, _reasonReportMissing, nil},
		{"same head from another repository", func(in *FindingMetricsInput) { in.Head.AreaID = entity.ID{19} }, entity.StateNotEvaluated, nil, entity.StateNotEvaluated, _reasonReportMissing, nil},
		{"baseline for another analysis kind", func(in *FindingMetricsInput) { in.Base.Kind = entity.AnalysisSAST }, entity.StateAssessed, num(2), entity.StateNotEvaluated, "report_scope_mismatch", num(2)},
		{"baseline for another scope", func(in *FindingMetricsInput) {
			in.Head.ScopeHash, in.Base.ScopeHash = "scope-a", "scope-b"
		}, entity.StateAssessed, num(2), entity.StateNotEvaluated, "report_scope_mismatch", num(2)},
		{"head rejected", func(in *FindingMetricsInput) { in.Head.RejectedReason = "malformed" }, entity.StateUnknown, nil, entity.StateUnknown, _reasonReportRejected, nil},
		{"interrupted head", func(in *FindingMetricsInput) { in.Head.ExitClass = "timeout" }, entity.StateProvisional, num(2), entity.StateProvisional, _reasonReportPartial, num(2)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := findingInput(entity.FindingCode)
			in.Findings = []ClassifiedFinding{
				classified(&in, entity.ID{6}, FindingNew), classified(&in, entity.ID{7}, FindingPersisting), classified(&in, entity.ID{8}, FindingResolved),
			}
			tt.change(&in)
			all := findingMetrics(&in, nil)
			assertReading(t, measured(t, all.Points, "C11", ""), tt.openState, tt.openValue)
			assert.Equal(t, tt.observed, measured(t, all.Points, "C12", _estFindingObserved).Value)
			for _, p := range []entity.Point{measured(t, all.Points, "C12", _estFindingNew), measured(t, all.Points, "C12", _estFindingKept), measured(t, all.Points, "C13", "")} {
				var expected *float64
				if tt.delta == entity.StateAssessed {
					expected = num(1)
				}
				assertReading(t, p, tt.delta, expected)
				assert.Equal(t, tt.reason, p.ReasonCode)
			}
		})
	}
}

func TestFindingMetrics_CodeGroupsCountApplicableIdentities(t *testing.T) {
	t.Parallel()
	in := findingInput(entity.FindingCode)
	excepted := classified(&in, entity.ID{6}, FindingNew)
	excepted.Path = "src/internal/a.go"
	applicable := excepted
	applicable.Path = "src/other.go"
	sameGroup := classified(&in, entity.ID{7}, FindingPersisting)
	otherGroup := classified(&in, entity.ID{8}, FindingNew)
	otherGroup.NativeSeverity = "error"
	in.Findings = []ClassifiedFinding{excepted, applicable, sameGroup, otherGroup, sameGroup}
	v := entity.ExceptionVersion{
		ID: entity.ID{9}, DecisionID: entity.ID{10}, FindingKind: in.Kind, Status: entity.ExceptionApproved,
		ExceptionDraft: entity.ExceptionDraft{
			Scope:         entity.ExceptionScope{AreaID: in.AreaID, RuleID: excepted.RuleID, PathPrefix: "src/internal"},
			EffectiveFrom: in.AsOf.Add(-time.Hour), ExpiresAt: in.AsOf.Add(time.Hour),
		},
	}
	all := findingMetrics(&in, []entity.ExceptionVersion{v})
	assertReading(t, measured(t, all.Points, "C11", ""), entity.StateAssessed, num(3))
	assert.Equal(t, []CodeFindingGroup{
		{CodeFindingClass: CodeFindingClass{Class: "style", RuleID: "scanner-rule", NativeSeverity: "error"}, Count: 1},
		{CodeFindingClass: CodeFindingClass{Class: "style", RuleID: "scanner-rule", NativeSeverity: "warning"}, Count: 2},
	}, all.CodeGroups)
}

func TestFindingMetrics_CompleteZeroAndPartialAbsence(t *testing.T) {
	t.Parallel()
	in := findingInput(entity.FindingCode)
	assertReading(t, measured(t, findingMetrics(&in, nil).Points, "C11", ""), entity.StateAssessed, num(0))
	in.Head.SkippedTargets = []entity.SkippedTarget{{Path: "src", Reason: "scope incomplete"}}
	assertReading(t, measured(t, findingMetrics(&in, nil).Points, "C11", ""), entity.StateProvisional, nil)
}

func TestFindingMetrics_SecurityFindingsFromNewData(t *testing.T) {
	t.Parallel()
	for _, factor := range []FindingFactor{FindingFromDatabase, FindingFromRules, FindingFromScope} {
		t.Run(string(factor), func(t *testing.T) {
			in := findingInput(entity.FindingDependency)
			in.IncomparableReason, in.SameArtifact = string(factor), true
			f := classified(&in, entity.ID{6}, FindingNew)
			f.Factor = factor
			in.Findings = []ClassifiedFinding{f}
			all := findingMetrics(&in, nil)
			assertReading(t, measured(t, all.Points, "S10", _estFindingNew), entity.StateNotEvaluated, nil)
			source := measured(t, all.Points, "S38", "")
			assertReading(t, source, entity.StateAssessed, num(1))
			require.Len(t, source.Contributions, 1)
			var basis findingEvidence
			require.NoError(t, json.Unmarshal(source.Contributions[0].Evidence, &basis))
			assert.Equal(t, factor, basis.Factor)
		})
	}
}

func TestFindingMetrics_AcceptedRiskIsNotAResolution(t *testing.T) {
	t.Parallel()
	in := findingInput(entity.FindingSecurity)
	f := classified(&in, entity.ID{6}, FindingResolved)
	in.Findings = []ClassifiedFinding{f}
	v := entity.ExceptionVersion{
		ID: entity.ID{7}, DecisionID: entity.ID{8}, FindingKind: f.Kind, Status: entity.ExceptionApproved,
		ExceptionDraft: entity.ExceptionDraft{
			Type: entity.ExceptionAcceptedRisk, Scope: entity.ExceptionScope{AreaID: f.AreaID, FindingID: f.FindingID},
			EffectiveFrom: in.AsOf.Add(-time.Hour), ExpiresAt: in.AsOf.Add(time.Hour),
		},
	}
	all := findingMetrics(&in, []entity.ExceptionVersion{v})
	fixed := measured(t, all.Points, "S11", "")
	assertReading(t, fixed, entity.StateAssessed, num(0))
	require.Len(t, fixed.Contributions, 1)
	assert.Equal(t, entity.RoleExcluded, fixed.Contributions[0].Role)
	assertReading(t, measured(t, all.Points, "S35", _estActive), entity.StateAssessed, num(1))
}
