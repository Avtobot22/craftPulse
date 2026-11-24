package normalize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestCodeReportParsers_ProfileFixtures(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		language, branchFn, longFn, testPath string
		cc, nloc, testVolume, cloneLines     int
	}{
		{"go", "StatusText", "Accumulate", "service_test.go", 13, 58, 9, 62},
		{"python", "status_text", "accumulate", "test_service.py", 14, 59, 4, 50},
		{"js", "statusText", "accumulate", "service.test.ts", 14, 60, 5, 50},
	} {
		t.Run(tt.language, func(t *testing.T) {
			functions := readCodeFixture(t, tt.language+"-functions.csv", entity.CodeFormatLizard)
			require.Len(t, functions.Functions, 3)
			assert.Equal(t, tt.branchFn, functions.Functions[0].Name)
			assert.Equal(t, tt.cc, functions.Functions[0].CCN)
			assert.Equal(t, tt.longFn, functions.Functions[1].Name)
			assert.Equal(t, tt.nloc, functions.Functions[1].NLOC)

			scope := readCodeFixture(t, tt.language+"-scope.csv", entity.CodeFormatLizard)
			manifest := &envelope.ScanToolRun{TestTargets: new([]string{tt.testPath})}
			scoped := (&parsedReport{code: scope}).scopedCodeFacts(manifest)
			assert.Contains(t, *scoped.Files, entity.CodeFile{Path: tt.testPath, NLOC: tt.testVolume, Test: true})
			assert.Contains(t, *scope.Files, entity.CodeFile{Path: tt.testPath, NLOC: tt.testVolume})
			assert.Len(t, *scope.Files, 3)

			clones := readCodeFixture(t, tt.language+"-clones.json", entity.CodeFormatJscpd)
			require.Len(t, clones.Groups, 1)
			assert.Len(t, clones.Groups[0].Members, 2)
			require.NotNil(t, clones.PhysicalLines)
			assert.Equal(t, int64(tt.cloneLines), *clones.PhysicalLines)
		})
	}
}

func readCodeFixture(t *testing.T, name, format string) *entity.CodeFacts {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "code", name))
	require.NoError(t, err)
	defer f.Close()
	p, ok := parserOf(format)
	require.True(t, ok)
	parsed, err := p.read(f, parseInput{kind: entity.AnalysisCode})
	require.NoError(t, err)
	return parsed.code
}

func TestCodeReportParsers_LegacyAndInvalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, format, data string
		valid              bool
	}{
		{"legacy", entity.CodeFormatLizard, "2,1,7,1,2,run@4-5@a.go,a.go,run,run,4,5\n", true},
		{"empty inventory", entity.CodeFormatLizard, "craftpulse-lizard,2\n", true},
		{"reversed lines", entity.CodeFormatLizard, "2,1,7,1,2,run@5-4@a.go,a.go,run,run,5,4\n", false},
		{"outside repo", entity.CodeFormatLizard, "craftpulse-lizard,2\nfile,../x,12\n", false},
		{"missing denominator", entity.CodeFormatJscpd, `{"duplicates":[]}`, false},
		{"tail", entity.CodeFormatJscpd, `{"duplicates":[],"statistics":{"total":{"lines":0}}}{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := parserOf(tt.format)
			out, err := p.read(strings.NewReader(tt.data), parseInput{kind: entity.AnalysisCode})
			if !tt.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.name == "legacy" {
				assert.Nil(t, out.code.Files)
				require.Len(t, out.code.Functions, 1)
			} else {
				require.NotNil(t, out.code.Files)
				assert.Empty(t, *out.code.Files)
			}
		})
	}
}

func TestLizard_ManifestExcludesFilesAndDirectories(t *testing.T) {
	t.Parallel()
	p, _ := parserOf(entity.CodeFormatLizard)
	out, err := p.read(strings.NewReader("craftpulse-lizard,2\nfile,src/app.go,20\nfile,src/app_test.go,10\n"+
		"file,src/gen.go,30\nfile,vendor/lib/a.go,40\nfile,build/out.go,50\n"+
		"7,3,20,1,8,Gen@3-10@src/gen.go,src/gen.go,Gen,Gen,3,10\n"), parseInput{kind: entity.AnalysisCode})
	require.NoError(t, err)
	facts := out.scopedCodeFacts(&envelope.ScanToolRun{
		TestTargets: new([]string{"src/app_test.go"}),
		ExcludedTargets: new([]envelope.ScanExcludedTarget{
			{Path: "src/gen.go", Reason: envelope.ScanExcludedTargetReasonGenerated},
			{Path: "vendor", Reason: envelope.ScanExcludedTargetReasonVendor},
			{Path: "build", Reason: envelope.ScanExcludedTargetReasonBuild},
		}),
	})
	assert.Empty(t, facts.Functions)
	assert.Equal(t, []entity.SkippedTarget{
		{Path: "src/gen.go", Reason: "generated"}, {Path: "vendor", Reason: "vendor"}, {Path: "build", Reason: "build"},
	}, facts.ExcludedTargets)
	assert.Equal(t, []entity.CodeFile{{Path: "src/app.go", NLOC: 20}, {Path: "src/app_test.go", NLOC: 10, Test: true}}, *facts.Files)
}

func TestJscpd_ConnectedPairs(t *testing.T) {
	t.Parallel()
	p, _ := parserOf(entity.CodeFormatJscpd)
	raw := `{
		"duplicates": [
			{"firstFile":{"name":"src/a.go","start":1,"end":6},"secondFile":{"name":"./src/b.go","start":1,"end":6},"format":"go","fragment":"discard"},
			{"firstFile":{"name":"src/b.go","start":1,"end":6},"secondFile":{"name":"src/c.go","start":1,"end":6},"format":"go"},
			{"firstFile":{"name":"src/c.go","start":1,"end":6},"secondFile":{"name":"src/a.go","start":1,"end":6},"format":"go"},
			{"firstFile":{"name":"src/a.go","start":4,"end":9},"secondFile":{"name":"src/b.go","start":4,"end":9},"format":"go"}
		], "statistics":{"total":{"lines":40}}
	}`
	out, err := p.read(strings.NewReader(raw), parseInput{kind: entity.AnalysisCode})
	require.NoError(t, err)
	require.Len(t, out.code.Groups, 2)
	assert.Equal(t, []entity.CodeRange{
		{Path: "src/a.go", Start: 1, End: 6},
		{Path: "src/b.go", Start: 1, End: 6},
		{Path: "src/c.go", Start: 1, End: 6},
	}, out.code.Groups[0].Members)
}

func TestJscpd_ManifestScope(t *testing.T) {
	t.Parallel()
	p, _ := parserOf(entity.CodeFormatJscpd)
	const raw = `{
		"duplicates":[
			{"firstFile":{"name":"src/a.go","start":1,"end":6},"secondFile":{"name":"src/b.go","start":1,"end":6},"format":"go"}
		],"statistics":{"total":{"lines":40}}`
	for _, tt := range []struct {
		name, scope, exclusion string
		unlistedSkips          bool
		trustedTotal           bool
		groups                 int
	}{
		{"native without exclusions", "", "", false, true, 1},
		{"native with excluded directory", "", "vendor", false, false, 1},
		{"declared file already excluded", `,"scope_excluded_paths":["vendor"]`, "vendor/lib.go", false, true, 1},
		{"declared directory present in pair", `,"scope_excluded_paths":["src"]`, "src", false, false, 0},
		{"unlisted skipped files", `,"scope_excluded_paths":[]`, "", true, false, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := p.read(strings.NewReader(raw+tt.scope+"}"), parseInput{kind: entity.AnalysisCode})
			require.NoError(t, err)
			tool := &envelope.ScanToolRun{Mode: envelope.ScanToolRunModeImport}
			if tt.exclusion != "" {
				tool.ExcludedTargets = new([]envelope.ScanExcludedTarget{{Path: tt.exclusion, Reason: envelope.ScanExcludedTargetReasonVendor}})
			}
			if tt.unlistedSkips {
				tool.SkippedCount = new(1)
			}
			got := parsed.scopedCodeFacts(tool)
			assert.Len(t, got.Groups, tt.groups)
			if tt.trustedTotal {
				require.NotNil(t, got.PhysicalLines)
				assert.Equal(t, int64(40), *got.PhysicalLines)
			} else {
				assert.Nil(t, got.PhysicalLines)
			}
			assert.Len(t, parsed.code.Groups, 1)
			assert.NotNil(t, parsed.code.PhysicalLines)
		})
	}
}
