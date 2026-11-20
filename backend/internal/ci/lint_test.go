package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestReadRuff_FindingsAndSyntaxErrorsAsSkipped(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "ruff.json")
	writeFiles(t, filepath.Dir(out), map[string]string{"ruff.json": `[
		{"code":"F401","message":"` + "`os`" + ` imported but unused","filename":"` + filepath.Join(root, "app", "a.py") + `",
		 "location":{"row":1,"column":8},"end_location":{"row":1,"column":10},
		 "fix":{"edits":[{"content":"FIXTURE_CODE_FRAGMENT"}]}},
		{"code":"invalid-syntax","message":"Expected ')'","filename":"` + filepath.Join(root, "app", "broken.py") + `",
		 "location":{"row":3,"column":12},"end_location":{"row":3,"column":13}},
		{"code":"F821","message":"Undefined name","filename":"` + filepath.Join(root, "app", "broken.py") + `",
		 "location":{"row":4,"column":1},"end_location":{"row":4,"column":2}}
	]`})

	results, skipped, err := readRuff([]string{out}, root)

	require.NoError(t, err)
	assert.Equal(t, []lintResult{{
		rule: "F401", level: "error", message: "`os` imported but unused", file: "app/a.py", line: 1, column: 8, endLine: 1, endColumn: 10,
	}}, results)
	assert.Equal(t, []envelope.ScanSkippedTarget{{Path: "app/broken.py", Reason: envelope.ScanSkippedTargetReasonUnsupported}}, skipped)

	writeFiles(t, filepath.Dir(out), map[string]string{"ruff.json": `[{"code":"F401","filename":"/elsewhere/a.py"}]`})
	_, _, err = readRuff([]string{out}, root)
	require.ErrorContains(t, err, "outside the repository")
}

func TestReadESLint_FatalParsingErrorIsSkippedAndSourceIsDropped(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "eslint.json")
	writeFiles(t, filepath.Dir(out), map[string]string{"eslint.json": `[
		{"filePath":"` + filepath.Join(root, "web", "a.ts") + `","source":"FIXTURE_CODE_FRAGMENT","messages":[
			{"ruleId":"@typescript-eslint/no-unused-vars","severity":2,"message":"'x' is assigned a value but never used.",
			 "line":2,"column":9,"endLine":2,"endColumn":10,"suggestions":[{"fix":{"text":"FIXTURE_CODE_FRAGMENT"}}]},
			{"ruleId":"no-self-assign","severity":1,"message":"'a' is assigned to itself.","line":5,"column":3}]},
		{"filePath":"` + filepath.Join(root, "web", "broken.ts") + `","messages":[
			{"ruleId":null,"fatal":true,"severity":2,"message":"Parsing error: Expression expected.","line":1,"column":10}]},
		{"filePath":"` + filepath.Join(root, "web", "clean.js") + `","messages":[]}
	]`})

	results, skipped, err := readESLint([]string{out}, root)
	require.NoError(t, err)
	report := filepath.Join(t.TempDir(), "lint.sarif")
	require.NoError(t, writeLintSARIF(report, ToolESLint, "10.14.0", "https://eslint.org/", results))

	assert.Equal(t, []envelope.ScanSkippedTarget{{Path: "web/broken.ts", Reason: envelope.ScanSkippedTargetReasonUnsupported}}, skipped)
	data, err := os.ReadFile(report)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "FIXTURE_CODE_FRAGMENT")
	assert.Contains(t, string(data), `"version":"2.1.0"`)
	assert.Contains(t, string(data), `"driver":{"name":"eslint","version":"10.14.0","informationUri":"https://eslint.org/","rules":[{"id":"@typescript-eslint/no-unused-vars"},{"id":"no-self-assign"}]}`)
	assert.Contains(t, string(data), `"level":"warning"`)
	got, err := SARIFFindings(data, root)
	require.NoError(t, err)
	assert.Equal(t, []Finding{
		{Rule: "@typescript-eslint/no-unused-vars", File: "web/a.ts", Line: 2},
		{Rule: "no-self-assign", File: "web/a.ts", Line: 5},
	}, got)
}

func TestLintChunks_SplitsFilesAndPrefixesPaths(t *testing.T) {
	files := make([]string, _lintChunk+1)
	for i := range files {
		files[i] = "-f" + string(rune('a'+i%26)) + ".py"
	}

	commands, outputs := lintChunks("/repo", &Scope{Files: files}, "/work", func(output string) []string {
		return []string{ToolRuff, "check", "--output-file", output}
	})

	require.Len(t, commands, 2)
	assert.Equal(t, []string{"/work/lint-1.json", "/work/lint-2.json"}, outputs)
	assert.Len(t, commands[0].Command, 4+_lintChunk)
	assert.Equal(t, []string{ToolRuff, "check", "--output-file", "/work/lint-2.json", "./" + files[_lintChunk]}, commands[1].Command)
	assert.Equal(t, "/repo", commands[1].Dir)
	assert.Equal(t, []int{1}, commands[1].FindingsCodes)

	commands, outputs = lintChunks("/repo", &Scope{}, "/work", func(string) []string { return nil })
	assert.Empty(t, commands)
	assert.Empty(t, outputs)
}
