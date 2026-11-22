package normalize

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "reports", name))
	require.NoError(t, err)
	return data
}

func parse(t *testing.T, format, kind string, data []byte) (*parsedReport, error) {
	t.Helper()
	p, ok := parserOf(format)
	require.True(t, ok)
	return p.read(bytes.NewReader(data), parseInput{kind: kind})
}

type row struct {
	rule, path       string
	start, end       int
	severity, native string
}

func rows(findings []entity.FindingOccurrence) []row {
	out := make([]row, len(findings))
	for i := range findings {
		f := &findings[i]
		out[i] = row{rule: f.RuleID, path: f.Path, severity: f.NativeSeverity, native: f.NativeID}
		if f.LineStart != nil {
			out[i].start = *f.LineStart
		}
		if f.LineEnd != nil {
			out[i].end = *f.LineEnd
		}
	}
	return out
}

func evidenceOf(t *testing.T, f entity.FindingOccurrence) evidence {
	t.Helper()
	var ev evidence
	require.NoError(t, json.Unmarshal(f.Evidence, &ev))
	return ev
}

func TestParseSARIF_Tools(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file, kind string
		want       []row
		tool       toolRef
		finding    string
	}{
		{"golangci.sarif", entity.AnalysisCode, []row{
			{rule: "errcheck", path: "app/main.go", start: 14, severity: "error"},
			{rule: "errcheck", path: "app/main.go", start: 15, severity: "error"},
			{rule: "errcheck", path: "app/main.go", start: 16, severity: "error"},
			{rule: "gosec", path: "app/main.go", start: 13, severity: "error"},
		}, toolRef{Name: "golangci-lint"}, entity.FindingCode},
		{"opengrep.sarif", entity.AnalysisSAST, []row{
			{rule: "rules.go-sql-format-string", path: "app/main.go", start: 13, end: 13, severity: "error"},
		}, toolRef{Name: "Opengrep OSS", Version: "1.33.1"}, entity.FindingSecurity},
		{"gitleaks.sarif", entity.AnalysisSecrets, []row{
			{rule: "aws-access-token", path: "config.env", start: 3, end: 3, severity: "warning"},
		}, toolRef{Name: "gitleaks", Version: "v8.0.0"}, entity.FindingSecret},
	}
	for _, tt := range cases {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			data := fixture(t, tt.file)
			got, err := parse(t, "sarif", tt.kind, data)
			require.NoError(t, err)
			assert.Equal(t, "2.1.0", got.formatVersion)
			assert.Equal(t, tt.want, rows(got.findings))
			for _, f := range got.findings {
				assert.Equal(t, tt.finding, f.Kind)
				assert.Empty(t, f.NativeConfidence, "SARIF of these tools names no confidence")
				assert.Equal(t, tt.tool, *evidenceOf(t, f).Tool)
				assert.Len(t, f.Fingerprint, 32)
			}
			again, err := parse(t, "sarif", tt.kind, data)
			require.NoError(t, err)
			assert.Equal(t, got, again, "the same file gives the same findings")
		})
	}
}

func TestParseSARIF_RulePropertiesAfterResults(t *testing.T) {
	t.Parallel()
	got, err := parse(t, "sarif", entity.AnalysisSAST, fixture(t, "opengrep.sarif"))
	require.NoError(t, err)
	require.Len(t, got.findings, 1)
	ev := evidenceOf(t, got.findings[0])
	assert.Equal(t, []string{"CWE-89"}, ev.CWE)
	assert.Equal(t, "8.8", ev.SecuritySeverity)
	assert.Equal(t, "rules.go-sql-format-string", ev.RuleName)
	assert.Contains(t, ev.RuleDescription, "Opengrep Finding")
}

func TestParseSARIF_NoCodeOrSecretInEvidence(t *testing.T) {
	t.Parallel()
	for file, kind := range map[string]string{"gitleaks.sarif": entity.AnalysisSecrets, "opengrep.sarif": entity.AnalysisSAST} {
		got, err := parse(t, "sarif", kind, fixture(t, file))
		require.NoError(t, err)
		for _, f := range got.findings {
			text := string(f.Evidence)
			for _, banned := range []string{"demo-secret-snippet", "SELECT * FROM users", "has detected secret", "pass the values as query"} {
				assert.NotContains(t, text, banned, file)
			}
		}
	}
}

func TestParseSARIF_Rejections(t *testing.T) {
	t.Parallel()
	golangci := fixture(t, "golangci.sarif")
	v20 := strings.Replace(string(golangci), `"version":"2.1.0"`, `"version":"2.0.0"`, 1)
	require.NotEqual(t, string(golangci), v20)
	cases := []struct {
		name, kind, reason string
		data               []byte
	}{
		{"truncated", entity.AnalysisCode, entity.RecordReportInvalid, golangci[:len(golangci)/2]},
		{"not json", entity.AnalysisCode, entity.RecordReportInvalid, []byte("<sarif/>")},
		{"trailing data", entity.AnalysisCode, entity.RecordReportInvalid, append(bytes.Clone(golangci), []byte("{}")...)},
		{"no version", entity.AnalysisCode, entity.RecordReportInvalid, []byte(`{"runs":[]}`)},
		{"version 2.0", entity.AnalysisCode, entity.RecordReportVersionUnsupported, []byte(v20)},
		{"result without rule", entity.AnalysisCode, entity.RecordReportInvalid, []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x"}},"results":[{"level":"error"}]}]}`)},
		{"kind without SARIF", entity.AnalysisJUnit, entity.RecordReportFormatUnsupported, golangci},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse(t, "sarif", tt.kind, tt.data)
			var rejected *errReport
			require.True(t, errors.As(err, &rejected), "%v", err)
			assert.Equal(t, tt.reason, rejected.reason)
		})
	}
}

func TestParseSARIF_PathsAndLevels(t *testing.T) {
	t.Parallel()
	data := []byte(`{"version":"2.1.0","runs":[{
		"originalUriBaseIds":{"SRCROOT":{"uri":"file:///builds/group/app/"}},
		"results":[
			{"ruleId":"r1","ruleIndex":0,"locations":[{"physicalLocation":{"artifactLocation":{"uri":"file:///builds/group/app/pkg/a.go"},"region":{"startLine":7,"endLine":5}}}]},
			{"ruleId":"r1","level":"note","locations":[{"physicalLocation":{"artifactLocation":{"uri":"../outside.go"},"region":{"startLine":2}}}]},
			{"ruleId":"r2","locations":[{"physicalLocation":{"artifactLocation":{"uri":"file:///etc/passwd"}}}],"partialFingerprints":{"primary":"abc"}},
			{"ruleId":"r2","kind":"pass","locations":[]},
			{"ruleId":"r2","guid":"f81d4fae-7dec-11d0-a765-00a0c91e6bf6","locations":[{"physicalLocation":{"artifactLocation":{"uri":"dir%20x/b.go"},"region":{"startLine":3}}}],"properties":{"confidence":"high"}}
		],
		"tool":{"driver":{"name":"t","rules":[{"id":"r1","defaultConfiguration":{"level":"error"},"properties":{"tags":["external/cwe/cwe-079"]}}]}}
	}]}`)
	got, err := parse(t, "sarif", entity.AnalysisSAST, data)
	require.NoError(t, err)
	assert.Equal(t, []row{
		{rule: "r1", path: "pkg/a.go", start: 7, severity: "error"},
		{rule: "r1", start: 2, severity: "note"},
		{rule: "r2", severity: "warning"},
		{rule: "r2", path: "dir x/b.go", start: 3, severity: "warning", native: "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"},
	}, rows(got.findings))
	assert.Equal(t, []string{"CWE-79"}, evidenceOf(t, got.findings[0]).CWE)
	assert.Equal(t, "high", got.findings[3].NativeConfidence)
}

func TestSARIFFingerprint_PartialSurvivesMovedLines(t *testing.T) {
	t.Parallel()
	report := func(line string, partial string) []byte {
		return []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"results":[{"ruleId":"r","locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},"region":{"startLine":` +
			line + `}}}]` + partial + `}]}]}`)
	}
	fp := func(data []byte) []byte {
		got, err := parse(t, "sarif", entity.AnalysisCode, data)
		require.NoError(t, err)
		return got.findings[0].Fingerprint
	}
	withPartial := `,"partialFingerprints":{"primaryLocationLineHash":"9f2c"}`
	assert.Equal(t, fp(report("3", withPartial)), fp(report("9", withPartial)))
	assert.NotEqual(t, fp(report("3", "")), fp(report("9", "")))
	assert.Equal(t, fp(report("3", `,"partialFingerprints":{"commitSha":""}`)), fp(report("3", "")), "an empty fingerprint is no fingerprint")
}

func TestParseGitLabSecurity_Kinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file, kind, finding string
		want                []row
	}{
		{"gl-sast-report.json", entity.AnalysisSAST, entity.FindingSecurity, []row{
			{rule: "go_sql_rule-concat-sqli", path: "app/main.go", start: 13, end: 13, severity: "High", native: "5b9c1f0d6c0e4f7a8f2d3b1a0e9c8d7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d"},
			{rule: "go_errors_rule-unhandled-error", path: "app/main.go", start: 15, severity: "Low", native: "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"},
		}},
		{"gl-secret-detection-report.json", entity.AnalysisSecrets, entity.FindingSecret, []row{
			{rule: "AWS", path: "config.env", start: 3, end: 3, severity: "Critical", native: "3a1d9f6b2c4e5d7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f"},
		}},
		{"gl-dependency-scanning-report.json", entity.AnalysisSCA, entity.FindingDependency, []row{
			{rule: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d", path: "go.sum", severity: "Medium", native: "8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d8c7b"},
		}},
	}
	for _, tt := range cases {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			got, err := parse(t, "gitlab_security", tt.kind, fixture(t, tt.file))
			require.NoError(t, err)
			assert.Equal(t, "15.2.1", got.formatVersion)
			assert.Equal(t, tt.want, rows(got.findings))
			for _, f := range got.findings {
				assert.Equal(t, tt.finding, f.Kind)
				assert.Empty(t, f.NativeConfidence, "schema 15 has no confidence")
				for _, banned := range []string{"demo-secret-extract", "demo-commit-message", "demo-author"} {
					assert.NotContains(t, string(f.Evidence), banned)
				}
			}
		})
	}
	sast, err := parse(t, "gitlab_security", entity.AnalysisSAST, fixture(t, "gl-sast-report.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"CWE-89"}, evidenceOf(t, sast.findings[0]).CWE)
	deps, err := parse(t, "gitlab_security", entity.AnalysisSCA, fixture(t, "gl-dependency-scanning-report.json"))
	require.NoError(t, err)
	assert.Equal(t, &packageRef{Name: "golang.org/x/net", Version: "v0.30.0"}, evidenceOf(t, deps.findings[0]).Package)
	assert.Len(t, deps.findings[0].ContentFingerprint, 32, "a package without its lockfile is the content of a dependency finding")
	assert.Nil(t, sast.findings[0].ContentFingerprint, "schema 15 says nothing of the content of a SAST finding")
}

func TestParseSARIF_ContentFingerprint(t *testing.T) {
	t.Parallel()
	opengrep, err := parse(t, "sarif", entity.AnalysisSAST, fixture(t, "opengrep.sarif"))
	require.NoError(t, err)
	moved := bytes.Replace(fixture(t, "opengrep.sarif"), []byte(`"startLine":13`), []byte(`"startLine":31`), 1)
	again, err := parse(t, "sarif", entity.AnalysisSAST, moved)
	require.NoError(t, err)
	assert.Len(t, opengrep.findings[0].ContentFingerprint, 32)
	assert.Equal(t, opengrep.findings[0].ContentFingerprint, again.findings[0].ContentFingerprint, "the content fingerprint leaves the lines out")
	assert.NotEqual(t, opengrep.findings[0].Fingerprint, again.findings[0].Fingerprint, "without partial fingerprints the lines identify a result")
	golangci, err := parse(t, "sarif", entity.AnalysisCode, fixture(t, "golangci.sarif"))
	require.NoError(t, err)
	assert.Nil(t, golangci.findings[0].ContentFingerprint, "a tool without fingerprints says nothing of the content")
}

func TestParseGitLabSecurity_Rejections(t *testing.T) {
	t.Parallel()
	sast := fixture(t, "gl-sast-report.json")
	cases := []struct {
		name, kind, reason string
		data               []byte
	}{
		{"schema 14", entity.AnalysisSAST, entity.RecordReportVersionUnsupported, fixture(t, "gl-sast-report-v14.json")},
		{"schema 16", entity.AnalysisSAST, entity.RecordReportVersionUnsupported, []byte(strings.Replace(string(sast), `"15.2.1"`, `"16.0.0"`, 1))},
		{"other scan type", entity.AnalysisSecrets, entity.RecordReportInvalid, sast},
		{"truncated", entity.AnalysisSAST, entity.RecordReportInvalid, sast[:len(sast)-40]},
		{"no scan", entity.AnalysisSAST, entity.RecordReportInvalid, []byte(`{"version":"15.0.0","vulnerabilities":[]}`)},
		{"container scanning", entity.AnalysisImage, entity.RecordReportFormatUnsupported, sast},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse(t, "gitlab_security", tt.kind, tt.data)
			var rejected *errReport
			require.True(t, errors.As(err, &rejected), "%v", err)
			assert.Equal(t, tt.reason, rejected.reason)
		})
	}
}

func TestParseSARIF_VersionAfterRuns(t *testing.T) {
	t.Parallel()
	data := []byte(`{"runs":[{"results":[{"ruleId":"r","locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},"region":{"startLine":4}}}]}],` +
		`"tool":{"driver":{"name":"t","rules":[{"id":"r","defaultConfiguration":{"level":"note"}}]}}}],"$schema":"x","version":"2.1.0"}`)
	got, err := parse(t, "sarif", entity.AnalysisCode, data)
	require.NoError(t, err)
	assert.Equal(t, []row{{rule: "r", path: "a.go", start: 4, severity: "note"}}, rows(got.findings))
	_, err = parse(t, "sarif", entity.AnalysisCode, bytes.Replace(data, []byte(`"2.1.0"`), []byte(`"2.0.0"`), 1))
	var rejected *errReport
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, entity.RecordReportVersionUnsupported, rejected.reason)
}

func TestFormatsWithoutParser(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"trivy_json", "gitleaks_json"} {
		_, ok := parserOf(format)
		assert.False(t, ok, format)
	}
}
