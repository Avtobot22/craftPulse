package calc

import (
	"bytes"
	"context"
	"encoding/hex"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonReportMissing       = "report_missing"
	_reasonReportRejected      = "report_rejected"
	_reasonReportPartial       = "report_partial"
	_reasonCoverageKindMissing = "coverage_kind_missing"
	_reasonNoExecutableUnits   = "no_executable_units"
	_reasonSuperseded          = "superseded"
	_reasonOverlap             = "overlapping_report_scope"
)

type TestCoverageFacts interface {
	TestCoverageFacts(context.Context, entity.ID, entity.ReportScope) (entity.ReportFacts, error)
}

// TestCoverageAt reads a revision for an authorized pipeline or for a repository and SHA in a system calculation.
func (u *UseCase) TestCoverageAt(ctx context.Context, orgID entity.ID, scope entity.ReportScope) (entity.TestCoverageRevision, error) {
	facts, err := u.d.TestCoverage.TestCoverageFacts(ctx, orgID, scope)
	if err != nil {
		return entity.TestCoverageRevision{}, err
	}
	return u.CalculateTestCoverage(facts), nil
}

// CalculateTestCoverage applies the definitions to one immutable snapshot of reports.
func (*UseCase) CalculateTestCoverage(facts entity.ReportFacts) entity.TestCoverageRevision {
	inputs := make([]entity.RevisionInput, 0, len(_testCoverageDefinitions))
	for i := range _testCoverageDefinitions {
		d := &_testCoverageDefinitions[i]
		inputs = append(inputs, entity.RevisionInput{Kind: InputDefinition, ID: d.ID, Version: strconv.Itoa(d.Version)})
	}
	return entity.TestCoverageRevision{
		Scope: facts.Scope, Definitions: inputs, Reports: facts.Reports,
		Tests: testOutcomes(facts), LineCoverage: coverageOf(facts, entity.CoverageLine), BranchCoverage: coverageOf(facts, entity.CoverageBranch),
	}
}

func reportState(reports []entity.ReportEvidence, kind string) (state entity.DataState, reason string) {
	var accepted, rejected, partial bool
	for i := range reports {
		r := &reports[i]
		if r.Kind != kind {
			continue
		}
		if r.RejectedReason != "" {
			rejected = true
			continue
		}
		accepted = true
		partial = partial || r.Completeness != "complete" || r.Partial()
	}
	switch {
	case accepted && (rejected || partial):
		return entity.StateProvisional, _reasonReportPartial
	case accepted:
		return entity.StateAssessed, ""
	case rejected:
		return entity.StateUnknown, _reasonReportRejected
	default:
		return entity.StateNotEvaluated, _reasonReportMissing
	}
}

type testGroupKey struct {
	suite      string
	knownSuite bool
	config     string
	attempt    entity.ID
}

type testTally struct {
	cases  map[entity.ID]struct{}
	counts entity.TestCounts
}

func (t *testTally) add(x *entity.ReportExecution) {
	if t.cases == nil {
		t.cases = make(map[entity.ID]struct{})
	}
	t.cases[x.CaseID] = struct{}{}
	t.counts.DistinctCases = len(t.cases)
	t.counts.Executions++
	switch x.Outcome {
	case entity.TestPassed:
		t.counts.Passed++
	case entity.TestFailed:
		t.counts.Failed++
	case entity.TestError:
		t.counts.Errors++
	case entity.TestSkipped:
		t.counts.Skipped++
	}
}

func testOutcomes(facts entity.ReportFacts) entity.TestMetric {
	state, reason := reportState(facts.Reports, entity.AnalysisJUnit)
	out := entity.TestMetric{State: state, ReasonCode: reason}
	if state != entity.StateAssessed && state != entity.StateProvisional {
		return out
	}
	var total testTally
	groups := map[testGroupKey]int{}
	var tallies []testTally
	for i := range facts.Executions {
		x := &facts.Executions[i]
		c := entity.TestContribution{Execution: *x, Role: entity.RoleNumerator}
		if x.Superseded {
			c.Role, c.ReasonCode = entity.RoleExcluded, _reasonSuperseded
		}
		out.Contributions = append(out.Contributions, c)
		if x.Superseded {
			continue
		}
		total.add(x)
		key := testGroupKey{config: hex.EncodeToString(x.ConfigFingerprint)}
		if x.Suite != nil {
			key.suite, key.knownSuite = *x.Suite, true
		}
		if x.AttemptID != nil {
			key.attempt = *x.AttemptID
		}
		j, found := groups[key]
		if !found {
			j = len(out.Groups)
			groups[key] = j
			out.Groups = append(out.Groups, entity.TestGroup{Suite: x.Suite, ConfigFingerprint: key.config, AttemptID: x.AttemptID})
			tallies = append(tallies, testTally{})
		}
		tallies[j].add(x)
	}
	for i := range out.Groups {
		out.Groups[i].Counts = tallies[i].counts
	}
	out.Counts = &total.counts
	return out
}

func coverageOf(facts entity.ReportFacts, kind string) entity.CoverageMetric {
	state, reason := reportState(facts.Reports, entity.AnalysisCoverage)
	out := entity.CoverageMetric{Point: entity.Point{State: state, ReasonCode: reason}}
	rank := make(map[entity.ID]int64, len(facts.Reports))
	for i := range facts.Reports {
		rank[facts.Reports[i].ID] = facts.Reports[i].KnownSeq
	}
	// A file can be measured by several reports. Use its most recent observation for this kind and keep
	// the other observations as evidence. Summing their counts would count the same units repeatedly.
	latest := map[string]entity.ReportCoverage{}
	for _, f := range facts.Coverage {
		if f.Kind != kind {
			continue
		}
		prev, found := latest[f.Path]
		seq, prevSeq := rank[f.ReportID], rank[prev.ReportID]
		if !found || seq > prevSeq || (seq == prevSeq && bytes.Compare(f.ReportID[:], prev.ReportID[:]) > 0) {
			latest[f.Path] = f
		}
	}
	var covered, executable int64
	var fileCount int
	for _, f := range facts.Coverage {
		if f.Kind != kind {
			continue
		}
		c := entity.CoverageContribution{File: f, Role: entity.RoleDenominator}
		switch {
		case latest[f.Path].ID != f.ID:
			c.Role, c.ReasonCode = entity.RoleExcluded, _reasonOverlap
		case f.Executable == 0:
			c.Role, c.ReasonCode = entity.RoleExcluded, _reasonNoExecutableUnits
		default:
			covered += int64(f.Covered)
			executable += int64(f.Executable)
			fileCount++
		}
		addCoverageContribution(&out, c)
	}
	for i := range facts.Reports {
		r := &facts.Reports[i]
		if r.Kind != entity.AnalysisCoverage || r.RejectedReason != "" {
			continue
		}
		for _, s := range r.SkippedTargets {
			if s.Reason == entity.SkippedOutsideRepository {
				file := entity.ReportCoverage{ReportID: r.ID, CoverageFact: entity.CoverageFact{Path: s.Path, Kind: kind}}
				addCoverageContribution(&out, entity.CoverageContribution{File: file, Role: entity.RoleExcluded, ReasonCode: s.Reason})
			}
		}
	}
	switch {
	case len(latest) == 0 && state == entity.StateProvisional && hasRejectedCoverage(facts.Reports):
		out.State, out.ReasonCode = entity.StateUnknown, _reasonReportRejected
	case len(latest) == 0 && (state == entity.StateAssessed || state == entity.StateProvisional):
		out.State, out.ReasonCode = entity.StateNotEvaluated, _reasonCoverageKindMissing
	case len(latest) > 0 && executable == 0:
		out.State, out.ReasonCode = entity.StateNotApplicable, _reasonNoExecutableUnits
		out.Numerator, out.Denominator = new(0.0), new(0.0)
	case executable > 0:
		value := float64(covered) / float64(executable)
		out.Value, out.Numerator, out.Denominator = &value, new(float64(covered)), new(float64(executable))
		out.N = &fileCount
	}
	return out
}

func addCoverageContribution(m *entity.CoverageMetric, c entity.CoverageContribution) {
	m.Contributions = append(m.Contributions, c)
	if c.Role == entity.RoleExcluded {
		m.ExcludedCount++
	}
}

func hasRejectedCoverage(reports []entity.ReportEvidence) bool {
	for i := range reports {
		r := &reports[i]
		if r.Kind == entity.AnalysisCoverage && r.RejectedReason != "" {
			return true
		}
	}
	return false
}
