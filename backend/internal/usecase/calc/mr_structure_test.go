package calc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestMRStructure_Size(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, completeness string
		state              entity.DataState
	}{
		{"complete", entity.MRDiffComplete, entity.StateAssessed},
		{"partial", entity.MRDiffPartial, entity.StateProvisional},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := []entity.MRDiffFile{
				{OldPath: "old.go", NewPath: "app.go", RenamedFile: true, AddedLines: new(3), DeletedLines: new(2)},
				{OldPath: "gen.go", NewPath: "gen.go", DeletedFile: true, AddedLines: new(0), DeletedLines: new(6)},
				{OldPath: "vendor/raw.bin", NewPath: "vendor/raw.bin", Binary: true, AddedLines: new(0), DeletedLines: new(0)},
			}
			in := MRStructureInput{
				Diff:         entity.MRDiff{Completeness: tt.completeness, Files: files},
				BaseExcluded: []entity.SkippedTarget{{Path: "gen.go", Reason: "generated"}},
				HeadExcluded: []entity.SkippedTarget{{Path: "vendor", Reason: "vendor"}},
			}
			got := new(UseCase).CalculateMRStructure(&in).Size
			assert.Equal(t, tt.state, got.State)
			assert.Equal(t, MRSizeCounts{Files: 3, Added: 3, Deleted: 8}, got.Total)
			assert.Equal(t, MRSizeCounts{Files: 1, Added: 3, Deleted: 2}, got.Regular)
			assert.Equal(t, MRSizeCounts{Files: 1, Deleted: 6}, got.Generated)
			assert.Equal(t, MRSizeCounts{Files: 1}, got.Vendor)
			assert.Equal(t, files[0], got.Contributions[0].File)
			if tt.completeness == entity.MRDiffPartial {
				assert.Equal(t, _reasonMRDiffPartial, got.ReasonCode)
			}
		})
	}
	in := MRStructureInput{Diff: mrStructureDiff(entity.MRDiffFile{NewPath: "unknown.go"})}
	got := new(UseCase).CalculateMRStructure(&in).Size
	assert.Equal(t, entity.StateProvisional, got.State)
	require.Len(t, got.Contributions, 1)
	assert.Nil(t, got.Contributions[0].File.AddedLines)
	assert.Equal(t, entity.RoleMissing, got.Contributions[0].Role)
}

func TestMRStructure_Complexity(t *testing.T) {
	t.Parallel()
	fn := func(path, name string, line, cc int) entity.CodeFunction {
		return entity.CodeFunction{CodeRange: entity.CodeRange{Path: path, Start: line, End: line + 4}, Name: name, CCN: cc}
	}
	base := []entity.CodeFunction{fn("old.go", "Run", 3, 5), fn("moved.go", "Move", 3, 2), fn("old.go", "Split", 12, 4)}
	head := []entity.CodeFunction{
		fn("app.go", "Run", 3, 8), fn("app.go", "New", 8, 2), fn("app.go", "Move", 9, 4), fn("app.go", "Split", 13, 3), fn("app.go", "Split", 17, 2),
	}
	for _, tt := range []struct {
		name, producer, reason string
		comparable             bool
		state                  entity.DataState
	}{
		{"rename, moved and split", "lizard", _reasonAmbiguousFunction, true, entity.StateProvisional},
		{"incomparable base", "lizard", "profile_changed", false, entity.StateNotEvaluated},
		{"different producer", "other", _reasonCodeScopeMismatch, true, entity.StateNotEvaluated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pair := mrStructurePair(entity.CodeFormatLizard, "lizard", entity.CodeFacts{Functions: base}, entity.CodeFacts{Functions: head})
			pair.Comparable, pair.Head.Scope.Producer = tt.comparable, tt.producer
			if !tt.comparable {
				pair.ReasonCode = tt.reason
			}
			in := MRStructureInput{Functions: pair, Diff: mrStructureDiff(entity.MRDiffFile{
				OldPath: "old.go", NewPath: "app.go", RenamedFile: true, HeadRanges: []entity.MRDiffRange{{From: 4, To: 18}},
			})}
			got := new(UseCase).CalculateMRStructure(&in).Complexity
			assert.Equal(t, tt.state, got.State)
			assert.Equal(t, tt.reason, got.ReasonCode)
			if tt.state == entity.StateNotEvaluated {
				assert.Empty(t, got.Deltas)
				return
			}
			require.Len(t, got.Deltas, 1)
			assert.Equal(t, base[0], *got.Deltas[0].Base)
			assert.Equal(t, head[0], got.Deltas[0].Head)
			assert.Equal(t, new(3), got.Deltas[0].Delta)
			require.Len(t, got.NewFunctions, 1)
			assert.Equal(t, "New", got.NewFunctions[0].Head.Name)
			assert.Nil(t, got.NewFunctions[0].Delta)
			require.Len(t, got.Excluded, 3)
			assert.Equal(t, "Move", got.Excluded[0].Head.Name)
			assert.Nil(t, got.Excluded[0].Delta)
		})
	}
}

func TestMRStructure_Clones(t *testing.T) {
	t.Parallel()
	a := entity.CodeRange{Path: "old.go", Start: 3, End: 8}
	b := entity.CodeRange{Path: "peer.go", Start: 8, End: 13}
	c := entity.CodeRange{Path: "added.go", Start: 1, End: 6}
	renamed := entity.CodeRange{Path: "app.go", Start: 3, End: 8}
	file := entity.MRDiffFile{
		OldPath: "old.go", NewPath: "app.go", RenamedFile: true, AddedLines: new(1), DeletedLines: new(1),
		HeadRanges: []entity.MRDiffRange{{From: 4, To: 4}},
	}
	for _, tt := range []struct {
		name, reason string
		members      []entity.CodeRange
		baseMembers  []entity.CodeRange
		count        float64
	}{
		{"rename", _reasonKnownClone, []entity.CodeRange{renamed, b}, []entity.CodeRange{a, b}, 0},
		{"remove one member", _reasonKnownClone, []entity.CodeRange{renamed, b}, []entity.CodeRange{a, b, c}, 0},
		{"add a member", "", []entity.CodeRange{renamed, b, c}, []entity.CodeRange{a, b}, 1},
		{"shift after deletion", _reasonAmbiguousClone, []entity.CodeRange{{Path: "app.go", Start: 2, End: 7}, b}, []entity.CodeRange{a, b}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			group := func(members []entity.CodeRange) entity.CodeFacts {
				return entity.CodeFacts{Groups: []entity.CloneGroup{{Language: "go", Members: members}}}
			}
			in := MRStructureInput{Diff: mrStructureDiff(file, entity.MRDiffFile{
				NewPath: "added.go", NewFile: true, HeadRanges: []entity.MRDiffRange{{From: 1, To: 6}},
			}), Clones: mrStructurePair(entity.CodeFormatJscpd, "jscpd", group(tt.baseMembers), group(tt.members))}
			got := new(UseCase).CalculateMRStructure(&in).Clones
			assert.Equal(t, new(tt.count), got.Value)
			require.Len(t, got.Contributions, 1)
			assert.Equal(t, tt.reason, got.Contributions[0].ReasonCode)
			assert.Equal(t, tt.members, got.Contributions[0].Group.Members)
			if tt.reason == _reasonAmbiguousClone {
				assert.Equal(t, entity.StateProvisional, got.State)
			}
		})
	}
}

func TestMRStructure_PatchCoverage(t *testing.T) {
	t.Parallel()
	lineMap := MRCoverageMap{
		Path: "app.go", Kind: entity.CoverageLine, Executable: 5,
		Lines: []MRCoverageLine{{Line: 1, Hits: 8}, {Line: 3, Hits: 1}, {Line: 4}, {Line: 6, Hits: 2}, {Line: 7}},
	}
	complete := MRCoverageReport{SHA: "h1", Evidence: entity.ReportEvidence{Kind: entity.AnalysisCoverage, Completeness: "complete"}, Files: []MRCoverageMap{lineMap}}
	rejected := complete
	rejected.Evidence.RejectedReason = "invalid_report"
	old := complete
	old.SHA = "h0"
	mapMissing := complete
	mapMissing.Files = []MRCoverageMap{{Path: "app.go", Kind: entity.CoverageLine, Executable: 5, Lines: []MRCoverageLine{}}}
	branch := complete
	branch.Files = []MRCoverageMap{{Path: "app.go", Kind: entity.CoverageBranch, Executable: 4}}
	junit := complete
	junit.Evidence.Kind = entity.AnalysisJUnit
	for _, tt := range []struct {
		name, reason string
		reports      []MRCoverageReport
		from, to     int
		state        entity.DataState
		value, n, d  *float64
	}{
		{"V-40", "", []MRCoverageReport{complete, old, branch}, 3, 7, entity.StateAssessed, new(0.5), new(2.0), new(4.0)},
		{"no changed executable lines", _reasonNoExecutableUnits, []MRCoverageReport{complete}, 5, 5, entity.StateNotApplicable, nil, new(0.0), new(0.0)},
		{"no report", _reasonReportMissing, nil, 3, 7, entity.StateNotEvaluated, nil, nil, nil},
		{"stale report", _reasonCoverageSHAMismatch, []MRCoverageReport{old}, 3, 7, entity.StateNotEvaluated, nil, nil, nil},
		{"rejected report", _reasonReportRejected, []MRCoverageReport{rejected}, 3, 7, entity.StateUnknown, nil, nil, nil},
		{"aggregate without complete map", _reasonCoverageMapMissing, []MRCoverageReport{mapMissing}, 3, 7, entity.StateUnknown, nil, nil, nil},
		{"branch only", _reasonCoverageKindMissing, []MRCoverageReport{branch}, 3, 7, entity.StateNotEvaluated, nil, nil, nil},
		{"JUnit only", _reasonReportMissing, []MRCoverageReport{junit}, 3, 7, entity.StateNotEvaluated, nil, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := MRStructureInput{Coverage: tt.reports, Diff: mrStructureDiff(entity.MRDiffFile{
				NewPath: "app.go", HeadRanges: []entity.MRDiffRange{{From: tt.from, To: tt.to}},
			})}
			got := new(UseCase).CalculateMRStructure(&in).PatchCoverage
			assert.Equal(t, tt.state, got.State)
			assert.Equal(t, tt.reason, got.ReasonCode)
			assert.Equal(t, tt.value, got.Value)
			assert.Equal(t, tt.n, got.Numerator)
			assert.Equal(t, tt.d, got.Denominator)
			if tt.name == "V-40" {
				require.Len(t, got.Contributions, 5)
				assert.Equal(t, entity.RoleNumerator, got.Contributions[0].Role)
				assert.Equal(t, entity.RoleDenominator, got.Contributions[1].Role)
				assert.Equal(t, MRPatchContribution{
					Path: "app.go", Range: entity.MRDiffRange{From: 5, To: 5}, Role: entity.RoleExcluded, ReasonCode: _reasonNoExecutableUnits,
				}, got.Contributions[2])
			}
		})
	}
}

func TestMRStructure_PatchCoverageAlias(t *testing.T) {
	t.Parallel()
	ids := []string{"C15", "C16", "C17", "C28"}
	got := new(UseCase).CalculateMRStructure(&MRStructureInput{})
	require.Len(t, got.Definitions, len(ids))
	for i, id := range ids {
		assert.Equal(t, entity.RevisionInput{Kind: InputDefinition, ID: id, Version: "1"}, got.Definitions[i])
	}
	assert.Equal(t, definition("C28"), definition("I27"))
	for _, def := range Definitions() {
		assert.NotEqual(t, "I27", def.ID)
	}
}

func mrStructureDiff(files ...entity.MRDiffFile) entity.MRDiff {
	return entity.MRDiff{BaseSHA: "h0", HeadSHA: "h1", Completeness: entity.MRDiffComplete, Files: files}
}

func mrStructurePair(format, producer string, base, head entity.CodeFacts) MRCodeComparison {
	return MRCodeComparison{
		Comparable: true,
		Base:       MRCodeSnapshot{Scope: entity.CodeScope{SHA: "h0", Format: format, Producer: producer}, Report: &entity.CodeReport{Completeness: "complete", Facts: base}},
		Head:       MRCodeSnapshot{Scope: entity.CodeScope{SHA: "h1", Format: format, Producer: producer}, Report: &entity.CodeReport{Completeness: "complete", Facts: head}},
	}
}
