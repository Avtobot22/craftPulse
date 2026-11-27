package normalize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestParseSonar_EvidenceAndFingerprint(t *testing.T) {
	t.Parallel()
	const input = `{"version":"1","issues_state":"complete","issues":[{"key":"k","type":"BUG","rule_id":"go:S1","path":"main.go",
		"line_start":2,"severity":"MAJOR","status":"RESOLVED","resolution":"WONTFIX","rule_description":"Check the result.",
		"rule":{"key":"go:S1","name":"Check result","language":"go","repository":"go"},"message":"a line of source code","snippet":"more code"}],"measures":{"coverage":"10.3"}}`
	parser, ok := parserOf(_sonarParserKey)
	require.True(t, ok)
	report, err := parser.read(strings.NewReader(input), parseInput{kind: entity.AnalysisSonar})
	require.NoError(t, err)
	require.Len(t, report.findings, 1)
	f := report.findings[0]
	assert.Equal(t, "MAJOR", f.NativeSeverity)
	assert.JSONEq(t, `{"status":"RESOLVED","resolution":"WONTFIX","rule_description":"Check the result.","rule":{"key":"go:S1","name":"Check result","language":"go","repository":"go"}}`, string(f.Evidence))
	assert.Equal(t, map[string]string{"coverage": "10.3"}, report.sonarMeasures)
	assert.Empty(t, f.ContentFingerprint)

	moved, err := parser.read(strings.NewReader(strings.Replace(input, `"line_start":2`, `"line_start":3`, 1)),
		parseInput{kind: entity.AnalysisSonar})
	require.NoError(t, err)
	require.Len(t, moved.findings, 1)
	assert.Equal(t, f.Fingerprint, moved.findings[0].Fingerprint)

	_, err = parser.read(strings.NewReader(strings.Replace(input, `"version":"1"`, `"version":"2"`, 1)),
		parseInput{kind: entity.AnalysisSonar})
	var rejected *errReport
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, entity.RecordReportVersionUnsupported, rejected.reason)
}

func TestParseSonar_RejectsInvalidMeasuresAndText(t *testing.T) {
	t.Parallel()
	const analysis = `{"version":"1","issues":[],"measures":%s}`
	const issue = `{"version":"1","measures":{},"issues":[{"key":"k","type":"BUG",
		"rule_id":"go:S1","severity":"MAJOR","status":"OPEN","rule_description":"Code\u0000example"}]}`
	parser, ok := parserOf(_sonarParserKey)
	require.True(t, ok)
	for name, input := range map[string]string{
		"null measure":      fmt.Sprintf(analysis, `{"coverage":null}`),
		"duplicate measure": fmt.Sprintf(analysis, `{"coverage":"10","coverage":"20"}`),
		"measure with NUL":  fmt.Sprintf(analysis, `{"coverage":"10\u0000"}`),
		"issue with NUL":    issue,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parser.read(strings.NewReader(input), parseInput{kind: entity.AnalysisSonar})
			var rejected *errReport
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, entity.RecordReportInvalid, rejected.reason)
		})
	}
}
