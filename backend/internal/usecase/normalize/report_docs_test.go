package normalize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestNormalizeDocs_InventoryBounds(t *testing.T) {
	t.Parallel()
	sha, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for _, tt := range []struct {
		name, prefix, p, version, hash, acl string
		ok                                  bool
	}{
		{"confirmed Git version", "docs", "docs/guide.md", sha, hash, "known", true},
		{"single file root", "docs/guide.md", "docs/guide.md", sha, hash, "known", true},
		{"sibling prefix", "docs", "docs-extra/guide.md", sha, hash, "known", false},
		{"parent traversal", "docs", "docs/../outside.md", sha, hash, "known", false},
		{"other commit", "docs", "docs/guide.md", strings.Repeat("d", 40), hash, "known", false},
		{"unknown ACL", "docs", "docs/guide.md", sha, hash, "unknown", false},
		{"missing content hash", "docs", "docs/guide.md", sha, "", "known", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := fmt.Sprintf(`{
				"format_version":"1","path_prefix":%q,"owner_front_matter_key":"owner",
				"documents":[{"document":{"container_native_id":"42","path":%q,"doc_format":"markdown","version_ref":%q,"acl_state":%q,"content_sha256":%q,"title":"Guide"},"owner":"Lead"}],
				"links":{"checked":1,"broken":0,"external":0}
			}`, tt.prefix, tt.p, tt.version, tt.acl, tt.hash)
			rep, err := parseDocsInventory(strings.NewReader(raw), parseInput{kind: entity.AnalysisDocs, sha: sha, container: "42"})
			if !tt.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, rep.docs.Documents, 1)
			assert.Equal(t, "Lead", rep.docs.Documents[0].Owner)
			assert.Equal(t, "Guide", rep.docs.Documents[0].Title)
			assert.Len(t, rep.docs.Documents[0].ContentHash, 32)
		})
	}
}

func TestNormalizeDocs_StableLinkFingerprint(t *testing.T) {
	t.Parallel()
	const old = `{
		"ruleId":"missing-anchor","message":{"text":"link target guide.md?ignored=1#go%2Dhere: no heading of docs/guide.md has this anchor"},
		"locations":[{"physicalLocation":{"artifactLocation":{"uri":"docs/README.md"},"region":{"startLine":3}}}]
	}`
	const current = `{
		"ruleId":"missing-anchor","properties":{"target_path":"docs/guide.md","anchor":"go-here"},
		"locations":[{"physicalLocation":{"artifactLocation":{"uri":"docs/README.md"},"region":{"startLine":19}}}]
	}`
	const other = `{
		"ruleId":"missing-anchor","properties":{"target_path":"docs/guide.md","anchor":"elsewhere"},
		"locations":[{"physicalLocation":{"artifactLocation":{"uri":"docs/README.md"},"region":{"startLine":19}}}]
	}`
	raw := fmt.Sprintf(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"craftpulse-docs"}},"results":[%s,%s,%s]}]}`, old, current, other)
	rep, err := parseSARIF(strings.NewReader(raw), parseInput{kind: entity.AnalysisDocs})
	require.NoError(t, err)
	require.Len(t, rep.findings, 3)
	assert.Equal(t, rep.findings[0].Fingerprint, rep.findings[1].Fingerprint)
	assert.NotEqual(t, rep.findings[1].Fingerprint, rep.findings[2].Fingerprint)
	assert.JSONEq(t, `{"tool":{"name":"craftpulse-docs"},"link":{"target_path":"docs/guide.md","anchor":"go-here"}}`, string(rep.findings[0].Evidence))
}
