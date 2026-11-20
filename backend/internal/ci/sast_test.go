package ci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _sastProfile = "profiles/opengrep-default"

// copyProfile copies the SAST profile of the image into a temporary directory the test may change.
func copyProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{CodeProfileFile, SASTSourcesFile, "rules/go.yaml", "rules/python.yaml", "rules/javascript.yaml"} {
		data, err := os.ReadFile(filepath.Join(_sastProfile, name))
		require.NoError(t, err)
		writeFiles(t, dir, map[string]string{name: string(data)})
	}
	return dir
}

func TestLoadSASTProfile_EveryRuleHasADistributableSource(t *testing.T) {
	p, err := LoadSASTProfile(_sastProfile)

	require.NoError(t, err)
	assert.Equal(t, "opengrep-default", p.ID)
	assert.Equal(t, "1.33.1", p.Tools[ToolSAST])
	assert.Len(t, p.Sources(), 9, "three rules for each of Go, Python and JS/TS")
	for _, s := range p.Sources() {
		assert.Equal(t, "LicenseRef-craftPulse", s.License, s.ID)
	}
}

func TestLoadSASTProfile_RejectsRulesWithoutSourceOrLicense(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   string
	}{
		{"rule without source", func(t *testing.T, dir string) {
			appendRule(t, dir, "  - id: go-extra\n    languages: [go]\n    severity: INFO\n    message: m\n    pattern: panic(...)\n")
		}, "rule go-extra has no source"},
		{"source without rule", func(t *testing.T, dir string) {
			editSources(t, dir, func(rules []map[string]any) []map[string]any {
				return append(rules, map[string]any{"id": "go-gone", "file": "rules/go.yaml", "source": "craftPulse", "source_version": "1", "license": "MIT"})
			})
		}, "rule go-gone is not in rules/go.yaml"},
		{"license without distribution", func(t *testing.T, dir string) {
			editSources(t, dir, func(rules []map[string]any) []map[string]any {
				rules[0]["license"] = "LicenseRef-Semgrep-Rules-1.0"
				return rules
			})
		}, `license "LicenseRef-Semgrep-Rules-1.0" that does not allow distribution`},
		{"rule id with a dot", func(t *testing.T, dir string) {
			appendRule(t, dir, "  - id: go.extra\n    languages: [go]\n    severity: INFO\n    message: m\n    pattern: panic(...)\n")
		}, `rule id "go.extra" must have no dots`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyProfile(t)
			tt.mutate(t, dir)
			_, err := LoadSASTProfile(dir)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func appendRule(t *testing.T, dir, rule string) {
	t.Helper()
	name := filepath.Join(dir, "rules", "go.yaml")
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(name, append(data, rule...), 0o600))
}

func editSources(t *testing.T, dir string, edit func([]map[string]any) []map[string]any) {
	t.Helper()
	name := filepath.Join(dir, SASTSourcesFile)
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	var f struct {
		Rules []map[string]any `json:"rules"`
	}
	require.NoError(t, json.Unmarshal(data, &f))
	f.Rules = edit(f.Rules)
	data, err = json.Marshal(f)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(name, data, 0o600))
}

func TestLoadSASTProfile_RulesHashFollowsTheRuleFiles(t *testing.T) {
	dir := copyProfile(t)
	before, err := LoadSASTProfile(dir)
	require.NoError(t, err)
	name := filepath.Join(dir, "rules", "python.yaml")
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(name, []byte(strings.Replace(string(data), "security-severity: \"8.1\"", "security-severity: \"9.0\"", 1)), 0o600))

	after, err := LoadSASTProfile(dir)

	require.NoError(t, err)
	assert.NotEqual(t, before.RulesHash(), after.RulesHash())
}

func TestCleanSARIF_BareRuleIDsWithoutCode(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"raw.sarif": `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Opengrep OSS",
		"rules":[{"id":"opt.craftpulse.profiles.opengrep-default.rules.go-tls-skip-verify","name":"opt.rules.go-tls-skip-verify",
		"properties":{"security-severity":"7.4","tags":["CWE-295: Improper Certificate Validation"]}}]}},
		"results":[{"ruleId":"opt.craftpulse.profiles.opengrep-default.rules.go-tls-skip-verify","locations":[{"physicalLocation":
		{"artifactLocation":{"uri":"svc/tls.go"},"region":{"startLine":6,"snippet":{"text":"InsecureSkipVerify: true, // SECRET_LINE"}}}}]}]}]}`})
	out := filepath.Join(dir, "clean.sarif")

	require.NoError(t, cleanSARIF(filepath.Join(dir, "raw.sarif"), out))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "SECRET_LINE")
	assert.Contains(t, string(data), `"security-severity":"7.4"`)
	assert.Contains(t, string(data), "CWE-295")
	got, err := SARIFFindings(data, dir)
	require.NoError(t, err)
	assert.Equal(t, []Finding{{Rule: "go-tls-skip-verify", File: "svc/tls.go", Line: 6}}, got)

	writeFiles(t, dir, map[string]string{"old.sarif": `{"version":"2.0.0","runs":[]}`})
	require.ErrorContains(t, cleanSARIF(filepath.Join(dir, "old.sarif"), out), "SARIF version 2.0.0")
}

func TestReadDetails_UnparsedAndTimedOutFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"details.json": `{"errors":[
		{"type":["PartialParsing",[{"path":"web/broken.js"}]],"level":"warn","path":"web/broken.js","message":"Syntax error"},
		{"type":"Timeout","level":"warn","path":"./svc/huge.go","message":"timeout"},
		{"type":"Timeout","level":"warn","path":"svc/huge.go","message":"timeout of another rule"},
		{"type":"Rule parse error","level":"error","message":"no path"}],
		"paths":{"scanned":["svc/a.go","svc/huge.go","web/broken.js"]}}`})

	scanned, skipped, err := readDetails(filepath.Join(dir, "details.json"))

	require.NoError(t, err)
	assert.Equal(t, 3, scanned)
	assert.Equal(t, []envelope.ScanSkippedTarget{
		{Path: "svc/huge.go", Reason: envelope.ScanSkippedTargetReasonTimeout},
		{Path: "web/broken.js", Reason: envelope.ScanSkippedTargetReasonUnsupported},
	}, skipped)
}
