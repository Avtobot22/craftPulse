package calc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCoverage_V40(t *testing.T) {
	t.Parallel()
	coverage := entity.ReportEvidence{ID: entity.ID{1}, Kind: entity.AnalysisCoverage, Completeness: "complete"}
	junit := entity.ReportEvidence{ID: entity.ID{2}, Kind: entity.AnalysisJUnit, Completeness: "complete"}
	malformed := entity.ReportEvidence{ID: entity.ID{3}, Kind: entity.AnalysisCoverage, RejectedReason: entity.RecordReportInvalid}
	file := entity.ReportCoverage{ID: entity.ID{4}, ReportID: coverage.ID, CoverageFact: entity.CoverageFact{
		Path: "src/math.go", Kind: entity.CoverageLine, Covered: 2, Executable: 4,
	}}
	zero := file
	zero.Covered, zero.Executable = 0, 0
	branch := file
	branch.ID, branch.Kind, branch.Covered, branch.Executable = entity.ID{5}, entity.CoverageBranch, 1, 4

	for _, tc := range []struct {
		name                   string
		reports                []entity.ReportEvidence
		files                  []entity.ReportCoverage
		lineState, branchState entity.DataState
		line, branch           *float64
	}{
		{"covered two of four", []entity.ReportEvidence{coverage}, []entity.ReportCoverage{file}, entity.StateAssessed, entity.StateNotEvaluated, new(0.5), nil},
		{"zero executable", []entity.ReportEvidence{coverage}, []entity.ReportCoverage{zero}, entity.StateNotApplicable, entity.StateNotEvaluated, nil, nil},
		{"no report", nil, nil, entity.StateNotEvaluated, entity.StateNotEvaluated, nil, nil},
		{"rejected report", []entity.ReportEvidence{malformed}, nil, entity.StateUnknown, entity.StateUnknown, nil, nil},
		{"JUnit alone", []entity.ReportEvidence{junit}, nil, entity.StateNotEvaluated, entity.StateNotEvaluated, nil, nil},
		{"separate kinds", []entity.ReportEvidence{coverage}, []entity.ReportCoverage{file, branch}, entity.StateAssessed, entity.StateAssessed, new(0.5), new(0.25)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := (&UseCase{}).CalculateTestCoverage(entity.ReportFacts{Reports: tc.reports, Coverage: tc.files})
			assert.Equal(t, tc.lineState, got.LineCoverage.State)
			assert.Equal(t, tc.line, got.LineCoverage.Value)
			assert.Equal(t, tc.branchState, got.BranchCoverage.State)
			assert.Equal(t, tc.branch, got.BranchCoverage.Value)
			if tc.line != nil {
				assert.Equal(t, new(2.0), got.LineCoverage.Numerator)
				assert.Equal(t, new(4.0), got.LineCoverage.Denominator)
				require.Len(t, got.LineCoverage.Contributions, 1)
				assert.Equal(t, file, got.LineCoverage.Contributions[0].File)
			}
		})
	}
}

func TestTestOutcomes_V40Matrix(t *testing.T) {
	t.Parallel()
	facts := entity.ReportFacts{Reports: []entity.ReportEvidence{{Kind: entity.AnalysisJUnit, Completeness: "complete"}}}
	for i, outcome := range []string{entity.TestPassed, entity.TestFailed} {
		facts.Executions = append(facts.Executions, entity.ReportExecution{
			CaseID: entity.ID{1}, TestExecution: entity.TestExecution{
				Suite: new("math"), AttemptID: new(entity.ID{10 + byte(i)}), ConfigFingerprint: []byte{byte(i)}, Outcome: outcome,
			},
		})
	}
	repeated := facts.Executions[1]
	repeated.Superseded = true
	facts.Executions = append(facts.Executions, repeated)
	got := (&UseCase{}).CalculateTestCoverage(facts).Tests
	assert.Equal(t, entity.StateAssessed, got.State)
	assert.Equal(t, &entity.TestCounts{Passed: 1, Failed: 1, DistinctCases: 1, Executions: 2}, got.Counts)
	require.Len(t, got.Groups, 2)
	assert.Equal(t, "00", got.Groups[0].ConfigFingerprint)
	assert.Equal(t, new("math"), got.Groups[0].Suite)
	assert.Equal(t, facts.Executions[1].AttemptID, got.Groups[1].AttemptID)
	assert.Equal(t, entity.TestCounts{Failed: 1, DistinctCases: 1, Executions: 1}, got.Groups[1].Counts)
	require.Len(t, got.Contributions, 3)
	assert.Equal(t, entity.RoleExcluded, got.Contributions[2].Role)
	assert.Equal(t, _reasonSuperseded, got.Contributions[2].ReasonCode)
}

func TestCoverage_OverlappingFilesKeepEvidence(t *testing.T) {
	t.Parallel()
	latest := entity.ReportEvidence{
		ID: entity.ID{1}, Kind: entity.AnalysisCoverage, KnownSeq: 2, Completeness: "complete",
		SkippedTargets: []entity.SkippedTarget{{Path: "/external/foo.go", Reason: entity.SkippedOutsideRepository}},
	}
	older := entity.ReportEvidence{ID: entity.ID{2}, Kind: entity.AnalysisCoverage, KnownSeq: 1, Completeness: "complete"}
	facts := entity.ReportFacts{Reports: []entity.ReportEvidence{older, latest}, Coverage: []entity.ReportCoverage{
		{ID: entity.ID{3}, ReportID: older.ID, CoverageFact: entity.CoverageFact{Path: "foo.go", Kind: entity.CoverageLine, Covered: 4, Executable: 4}},
		{ID: entity.ID{4}, ReportID: latest.ID, CoverageFact: entity.CoverageFact{Path: "foo.go", Kind: entity.CoverageLine, Covered: 2, Executable: 4}},
	}}
	got := (&UseCase{}).CalculateTestCoverage(facts).LineCoverage
	assert.Equal(t, new(0.5), got.Value)
	assert.Equal(t, 2, got.ExcludedCount)
	require.Len(t, got.Contributions, 3)
	assert.Equal(t, _reasonOverlap, got.Contributions[0].ReasonCode)
	assert.Equal(t, entity.SkippedOutsideRepository, got.Contributions[2].ReasonCode)
}
