package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_baseAdjust = `function adjust(value) {
    let result = value;
    if (value > 1) result += 1;
    return result;
}
`
	_headAdjust = `function adjust(value) {
    let result = value;
    if (value > 20) result += 1;
    if (value > 30) result += 2;
    if (value > 40) result += 3;
    if (value > 50) result += 4;
    return result;
}
`
	_mirror = `function mirror(value) {
    const formatted = value.trim();
    const prefix = formatted.slice(0, 2);
    const suffix = formatted.slice(-3);
    const prefixSize = prefix.length;
    const suffixSize = suffix.length;
    const first = { label: prefix, size: prefixSize };
    const last = { label: suffix, size: suffixSize };
    const joined = [first.label, last.label].join(':');
    const lengths = [first.size, last.size].join(',');
    const text = joined + ':' + lengths;
    return text.toUpperCase();
}
`
)

func TestCalcMRStructure_ReportsAndGitDiff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gitDiffCommand(t, dir, "init", "--quiet")
	diffCommit(t, dir, "app.js", _baseAdjust)
	diffCommit(t, dir, "original.js", _mirror)
	diffCommit(t, dir, "peer.js", _mirror)
	diffCommit(t, dir, "generated.js", "// @generated\nconst built = 1;\n")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "vendor"), 0o700))
	base := diffCommit(t, dir, "vendor/library.js", "const library = 1;\n")
	diffCommit(t, dir, "app.js", _headAdjust)
	gitDiffCommand(t, dir, "rm", "--quiet", "generated.js", "vendor/library.js")
	head := diffCommit(t, dir, "extra.js", _mirror)
	span := base + "..." + head
	numstat := gitDiffCommand(t, dir, "diff", "--numstat", span)
	patch := gitDiffCommand(t, dir, "diff", "--unified=0", span)
	assert.Contains(t, patch, "@@ -3 +3,4 @@")
	cloneLines := strings.Count(_mirror, "\n")
	assert.Equal(t, fmt.Sprintf("4\t1\tapp.js\n%d\t0\textra.js\n0\t2\tgenerated.js\n0\t1\tvendor/library.js\n", cloneLines), numstat)

	e := newNormalizeEnv(t)
	baseReceipt := e.imported(t, e.admin, mrStructureReport(t, "base", base))
	headReceipt := e.imported(t, e.admin, mrStructureReport(t, "head", head))
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, baseReceipt.ID).Status)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, headReceipt.ID).Status)
	area := mrArea(t, e, headReceipt, "42")
	snapshot := func(sha, producer, format string) calc.MRCodeSnapshot {
		t.Helper()
		scope := entity.CodeScope{RepositoryID: area, SHA: sha, Format: format, Producer: producer, ProfileID: "js-default"}
		r, err := e.norm.CodeReportAt(t.Context(), e.orgID, scope, math.MaxInt64)
		require.NoError(t, err)
		require.NotNil(t, r, "%s at %s", producer, sha)
		return calc.MRCodeSnapshot{Scope: scope, Report: r}
	}
	pair := func(producer, format string) calc.MRCodeComparison {
		t.Helper()
		return calc.MRCodeComparison{Comparable: true, Base: snapshot(base, producer, format), Head: snapshot(head, producer, format)}
	}
	in := calc.MRStructureInput{
		Diff: entity.MRDiff{BaseSHA: base, HeadSHA: head, Completeness: entity.MRDiffComplete, Files: []entity.MRDiffFile{
			{OldPath: "app.js", NewPath: "app.js", AddedLines: new(4), DeletedLines: new(1), HeadRanges: []entity.MRDiffRange{{From: 3, To: 6}}},
			{NewPath: "extra.js", NewFile: true, AddedLines: &cloneLines, DeletedLines: new(0), HeadRanges: []entity.MRDiffRange{{From: 1, To: cloneLines}}},
			{OldPath: "generated.js", NewPath: "generated.js", DeletedFile: true, AddedLines: new(0), DeletedLines: new(2)},
			{OldPath: "vendor/library.js", NewPath: "vendor/library.js", DeletedFile: true, AddedLines: new(0), DeletedLines: new(1)},
		}},
		Functions: pair("lizard", entity.CodeFormatLizard),
		Clones:    pair("jscpd", entity.CodeFormatJscpd),
		Coverage:  mrStructureCoverage(t, e, area, head),
	}
	got := new(calc.UseCase).CalculateMRStructure(&in)
	assert.Equal(t, calc.MRSizeCounts{Files: 4, Added: int64(4 + cloneLines), Deleted: 4}, got.Size.Total)
	assert.Equal(t, calc.MRSizeCounts{Files: 2, Added: int64(4 + cloneLines), Deleted: 1}, got.Size.Regular)
	assert.Equal(t, calc.MRSizeCounts{Files: 1, Deleted: 2}, got.Size.Generated)
	assert.Equal(t, calc.MRSizeCounts{Files: 1, Deleted: 1}, got.Size.Vendor)
	require.Len(t, got.Complexity.Deltas, 1)
	assert.Equal(t, "adjust", got.Complexity.Deltas[0].Head.Name)
	assert.Equal(t, new(3), got.Complexity.Deltas[0].Delta)
	require.Len(t, got.Complexity.NewFunctions, 1)
	assert.Equal(t, "extra.js", got.Complexity.NewFunctions[0].Head.Path)
	assert.Equal(t, new(1.0), got.Clones.Value)
	require.Len(t, got.Clones.Contributions, 1)
	assert.Len(t, got.Clones.Contributions[0].Group.Members, 3)
	assert.Equal(t, new(0.5), got.PatchCoverage.Value)
	assert.Equal(t, new(2.0), got.PatchCoverage.Numerator)
	assert.Equal(t, new(4.0), got.PatchCoverage.Denominator)
	assert.Equal(t, entity.StateProvisional, got.PatchCoverage.State, "the clone file has no coverage map")
	assert.Equal(t, cloneLines, got.PatchCoverage.ExcludedCount)
	require.Len(t, got.PatchCoverage.Contributions, 5)
	assert.Equal(t, "extra.js", got.PatchCoverage.Contributions[4].Path)
}

func mrStructureReport(t *testing.T, rev, sha string) []byte {
	t.Helper()
	fixture := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", "mr-structure", rev+"-"+name))
		require.NoError(t, err)
		return data
	}
	specs := []reportSpec{
		{
			name: "functions.csv", kind: entity.AnalysisCode, format: entity.CodeFormatLizard, producer: "lizard", version: "1.26.0", formatVersion: "2",
			profileID: "js-default", language: "javascript", exit: "success", completeness: "complete", content: fixture("functions.csv"),
		},
		{
			name: "clones.json", kind: entity.AnalysisCode, format: entity.CodeFormatJscpd, producer: "jscpd", version: "5.4.2",
			profileID: "js-default", language: "javascript", exit: "success", completeness: "complete", content: fixture("clones.json"),
		},
	}
	if rev == "base" {
		specs[0].excludedTargets = new([]envelope.ScanExcludedTarget{
			{Path: "generated.js", Reason: envelope.ScanExcludedTargetReasonGenerated},
			{Path: "vendor", Reason: envelope.ScanExcludedTargetReasonVendor},
		})
	} else {
		specs = append(specs, reportSpec{
			name: "coverage.info", kind: entity.AnalysisCoverage, format: "lcov", producer: "istanbul", version: "6.0.3",
			profileID: "js-default", language: "javascript", exit: "success", completeness: "complete", content: fixture("coverage.info"),
		})
	}
	return reportPackage(t, sha, specs...)
}

func mrStructureCoverage(t *testing.T, e *normalizeEnv, area entity.ID, sha string) []calc.MRCoverageReport {
	t.Helper()
	facts, err := e.norm.TestCoverageFacts(t.Context(), e.orgID, entity.ReportScope{AreaID: area, SHA: sha, KnownSeq: math.MaxInt64})
	require.NoError(t, err)
	var out []calc.MRCoverageReport
	for i := range facts.Reports {
		report := &facts.Reports[i]
		if report.Kind != entity.AnalysisCoverage {
			continue
		}
		r := calc.MRCoverageReport{SHA: sha, Evidence: *report}
		for _, f := range facts.Coverage {
			if f.ReportID != report.ID || f.Kind != entity.CoverageLine {
				continue
			}
			body, err := e.store.Get(t.Context(), f.LineMapKey)
			require.NoError(t, err)
			for _, raw := range lineMaps(t, body) {
				var p struct {
					Path   string    `json:"path"`
					Kind   string    `json:"kind"`
					Points [][]int64 `json:"points"`
				}
				require.NoError(t, json.Unmarshal([]byte(raw), &p))
				if p.Path != f.Path || p.Kind != f.Kind {
					continue
				}
				m := calc.MRCoverageMap{Path: f.Path, Kind: f.Kind, Executable: f.Executable, Lines: make([]calc.MRCoverageLine, 0, len(p.Points))}
				for _, pair := range p.Points {
					require.Len(t, pair, 2)
					m.Lines = append(m.Lines, calc.MRCoverageLine{Line: int(pair[0]), Hits: pair[1]})
				}
				r.Files = append(r.Files, m)
			}
		}
		out = append(out, r)
	}
	return out
}
