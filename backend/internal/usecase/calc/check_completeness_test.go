package calc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCheckCompleteness_ChecksAndReportMatrix(t *testing.T) {
	t.Parallel()
	component := entity.ID{11}
	in := CheckCompletenessInput{
		AreaID: entity.ID{10}, AsOf: _findingAt, HeadSHA: "current", KnownSeq: 12,
		Checks: []ExpectedCheck{
			{Key: "opengrep:go", Outcome: entity.CIRunning},
			{Key: "opengrep:go", Outcome: entity.CIFailed, JobID: entity.ID{12}},
			{Key: "test:go", Outcome: entity.CISuccess, JobID: entity.ID{13}},
			{Key: "sast:go", Outcome: entity.CIUnknown},
		},
		Expected: []entity.ExpectedReport{
			{Kind: entity.AnalysisCode, ComponentID: component, Language: "go"},
			{Kind: entity.AnalysisSAST, ComponentID: component, Language: "go"},
			{Kind: entity.AnalysisSAST, ComponentID: component, Language: "go"},
		},
	}
	code := completeScan(in.HeadSHA, entity.ID{14}, entity.AnalysisCode)
	code.ExitClass, code.Language, code.ComponentID = "success", "go", &component
	stale := completeScan("previous", entity.ID{15}, entity.AnalysisSAST)
	stale.Language, stale.ComponentID = "go", &component
	in.Received = []FindingScan{*code, *stale}
	all := New(Deps{}).CalculateCheckCompleteness(in)
	checks := measured(t, all, "I08", "")
	assertReading(t, checks, entity.StateAssessed, num(1.0/3))
	assert.Equal(t, num(1), checks.Numerator)
	assert.Equal(t, num(3), checks.Denominator)

	reports := measured(t, all, "I09", "")
	assertReading(t, reports, entity.StateAssessed, num(0.5))
	require.Len(t, reports.Contributions, 2)
	assert.Equal(t, entity.RoleDenominator, reports.Contributions[0].Role, "an empty complete report satisfies the expectation")
	missing := reports.Contributions[1]
	assert.Equal(t, component, missing.ObjectID)
	assert.Equal(t, entity.RoleNumerator, missing.Role)
	assert.Equal(t, _reasonReportMissing, missing.ReasonCode)
	assert.JSONEq(t, `{"kind":"sast","component_id":"0b000000-0000-0000-0000-000000000000","language":"go"}`, string(missing.Evidence))
}

func TestCheckCompleteness_RequiredReportIsNotSatisfiedByIncompleteEvidence(t *testing.T) {
	t.Parallel()
	compA, compB := entity.ID{1}, entity.ID{2}
	in := CheckCompletenessInput{
		AreaID: entity.ID{3}, AsOf: _findingAt, HeadSHA: "head", KnownSeq: 10,
		Expected: []entity.ExpectedReport{
			{Kind: entity.AnalysisCode, ComponentID: compA, Language: "go"},
			{Kind: entity.AnalysisCode, ComponentID: compB, Language: "go"},
		},
	}
	unscoped := completeScan(in.HeadSHA, entity.ID{4}, entity.AnalysisCode)
	partial := completeScan(in.HeadSHA, entity.ID{5}, entity.AnalysisCode)
	partial.Completeness, partial.Language, partial.ComponentID = "partial", "go", &compA
	rejected := completeScan(in.HeadSHA, entity.ID{6}, entity.AnalysisCode)
	rejected.RejectedReason, rejected.Language, rejected.ComponentID = "malformed", "go", &compB
	in.Received = []FindingScan{*unscoped, *partial, *rejected}
	res := measured(t, New(Deps{}).CalculateCheckCompleteness(in), "I09", "")
	assertReading(t, res, entity.StateAssessed, num(1))
	require.Len(t, res.Contributions, 2)
	assert.Equal(t, _reasonReportPartial, res.Contributions[0].ReasonCode)
	assert.Equal(t, _reasonReportRejected, res.Contributions[1].ReasonCode)
}

func TestCheckCompleteness_NoConfiguredExpectations(t *testing.T) {
	t.Parallel()
	for _, p := range New(Deps{}).CalculateCheckCompleteness(CheckCompletenessInput{AsOf: _findingAt}) {
		assertReading(t, p.Point, entity.StateNotApplicable, nil)
		assert.Nil(t, p.Denominator)
		assert.Empty(t, p.Contributions)
	}
}
