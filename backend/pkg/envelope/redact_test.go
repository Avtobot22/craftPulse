package envelope_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type appendReportLine struct{}

func (appendReportLine) Discover(string, []byte) error { return nil }
func (appendReportLine) Redact(name string, data []byte) (sanitized []byte, replacements int64, err error) {
	if strings.HasSuffix(name, ".xml") {
		return append(bytes.Clone(data), '\n'), 1, nil
	}
	return data, 0, nil
}

func TestCleanWithRedactor_RewritesLegacyJenkinsReferences(t *testing.T) {
	input := ciArchive(t, "jenkins-retry", func(files map[string][]byte) {
		var manifest envelope.JenkinsExecutionManifestV1
		require.NoError(t, json.Unmarshal(files[envelope.ExecutionManifestName], &manifest))
		for i := range manifest.Steps {
			for j := range manifest.Steps[i].Reports {
				report := &manifest.Steps[i].Reports[j]
				report.Path = strings.TrimPrefix(report.Path, envelope.FilesDir)
			}
		}
		encoded, err := json.Marshal(&manifest)
		require.NoError(t, err)
		files[envelope.ExecutionManifestName] = encoded
	})
	var out bytes.Buffer
	result, err := envelope.CleanWithRedactor(bytes.NewReader(input), envelope.DefaultLimits, &out, appendReportLine{})
	require.NoError(t, err)
	require.Nil(t, result.Envelope)
	require.Zero(t, result.Rejected())
	assert.EqualValues(t, 3, result.Redactions)
	checked := envelope.Check(bytes.NewReader(out.Bytes()), envelope.DefaultLimits)
	require.Nil(t, checked.Envelope)
	require.Zero(t, checked.Rejected())
	require.NotNil(t, checked.ExecutionManifest)
	for _, step := range checked.ExecutionManifest.Steps {
		for _, report := range step.Reports {
			assert.False(t, strings.HasPrefix(report.Path, envelope.FilesDir))
		}
	}
}
