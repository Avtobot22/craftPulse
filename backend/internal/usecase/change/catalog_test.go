package change

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestParseCatalog_ReadsTheTestCatalog(t *testing.T) {
	t.Parallel()
	c, err := ReadCatalog("testdata/catalog.yaml")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/avtobot22/craftpulse-ci", c.StepImage)
	require.Len(t, c.Profiles, 7)
	assert.Equal(t, entity.CheckModeBuiltin, c.Profiles[0].Mode)
	assert.Equal(t, map[string]string{"golangci-lint": "2.15.0", "lizard": "1.25.0", "jscpd": "5.4.2"}, c.Profiles[0].ToolVersions)
	assert.Len(t, c.Profiles[0].RulesHash, 32)
	assert.Equal(t, map[string]string{}, c.Profiles[6].ToolVersions, "an import profile has no tools")
}

func TestParseCatalog_WikiInventoryHasNoCIChecks(t *testing.T) {
	t.Parallel()
	valid, err := os.ReadFile("testdata/catalog.yaml")
	require.NoError(t, err)
	const wiki = "  - id: wiki-inventory\n    mode: import\n    analysis_kinds: "
	c, err := ParseCatalog(append(valid, []byte(wiki+"[]\n")...))
	require.NoError(t, err)
	require.Len(t, c.Profiles, 7)
	_, err = ParseCatalog(append(valid, []byte(wiki+"[docs]\n")...))
	require.ErrorContains(t, err, "wiki inventory only accepts import mode without checks")
}

func TestParseCatalog_RejectsInvalidCatalogs(t *testing.T) {
	t.Parallel()
	valid, err := os.ReadFile("testdata/catalog.yaml")
	require.NoError(t, err)
	cases := []struct {
		name, old, new, want string
	}{
		{"unknown version", "version: 1", "version: 2", "version 2"},
		{"step without digest", "  digest: sha256:1111111111111111111111111111111111111111111111111111111111111111\n", "", "step"},
		{"repeated profile", "id: trivy-default", "id: go-default", "repeated"},
		{"unknown analysis kind", "analysis_kinds: [sca, iac]", "analysis_kinds: [sca, lint]", "lint"},
		{"builtin without script", "    script:\n      - craftpulse-ci run --profile-id docs-default", "    tags:\n      - craftpulse-ci run --profile-id docs-default", "field tags"},
		{"import with an image", "    report_format: sarif\n  - id: sca-import", "    report_format: sarif\n    image: x\n  - id: sca-import", "no image"},
		{"bad mode", "mode: import\n    analysis_kinds: [sca]", "mode: plugin\n    analysis_kinds: [sca]", "plugin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := strings.Replace(string(valid), tc.old, tc.new, 1)
			require.NotEqual(t, string(valid), raw)
			_, err := ParseCatalog([]byte(raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
