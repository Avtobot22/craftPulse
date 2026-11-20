package ci

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestParseMarkdown_OwnerTitleAnchorsAndLinkLines(t *testing.T) {
	src := "---\ntitle: Ignored\nowner: \"team-lead\"\n---\n\n# Service `API` docs\n\nSee [guide](guide.md#set-up) and\n" +
		"![logo](img/logo.png).\n\n## Set up!\n\n## Set up\n\n[ref][r]\n\n[r]: other.md\n"

	doc := parseMarkdown([]byte(src), "owner")

	assert.Equal(t, "team-lead", doc.owner)
	assert.Equal(t, "Service API docs", doc.title)
	assert.Equal(t, []string{"service-api-docs", "set-up", "set-up-1"}, doc.anchors)
	assert.Equal(t, []markdownLink{{"guide.md#set-up", 8}, {"img/logo.png", 9}, {"other.md", 15}}, doc.links)

	plain := parseMarkdown([]byte("---\nno closing line\n# Title\n"), "owner")
	assert.Empty(t, plain.owner)
}

func TestCheckDocs_LinksInventoryAndCounts(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"docs/README.md": "---\nowner: lead\n---\n# Docs\n\n[ok](guide.md) [anchor](guide.md#setup) [self](#docs) [dir](sub/)\n" +
			"[root](/docs/guide.md#guide) [file](missing.md) [anchor](guide.md#absent) [case](Guide.md) [out](../../x.md)\n" +
			"[web](https://example.com) [mail](mailto:a@example.com) [yaml](api.openapi.yaml#L3)\n",
		"docs/guide.md":           "# Guide\n\n## Setup\n",
		"docs/sub/a.md":           "# A\n",
		"docs/api.openapi.yaml":   "openapi: 3.1.0\ninfo:\n  title: Orders API\n  version: 1.0.0\n",
		"docs/api.json":           `{"openapi":"3.0.3","info":{"title":"JSON API","version":"1"}}`,
		"docs/settings.yaml":      "replicas: 2\n",
		"docs/.hidden/secret.md":  "# Hidden\n",
		"outside/not-in-root.md":  "# Outside\n",
		"docs/big/ignored.txt":    "text\n",
		"docs/notes.markdown":     "# Notes\n",
		"docs/sub/broken-link.md": "[up](../README.md#nope)\n",
	})
	p, err := LoadDocsProfile("profiles/git-markdown-openapi")
	require.NoError(t, err)
	doc := func(rel, format string, data []byte) envelope.DocumentV1 {
		return docRecord(rel, format, data, "abc123", "42")
	}

	res, o, err := p.analyze(t.Context(), &Runner{Now: time.Now}, root, DocsRoot{ID: uuid.New(), Kind: "docs", PathPrefix: "docs"}, doc, time.Now().Add(time.Minute))

	require.NoError(t, err)
	assert.Equal(t, envelope.ScanToolRunExitClassFindings, o.exit)
	got := make([]string, 0, len(res.results))
	for _, r := range res.results {
		got = append(got, r.rule+" "+r.file+":"+strconv.Itoa(r.line))
	}
	assert.Equal(t, []string{
		"missing-file docs/README.md:7", "missing-anchor docs/README.md:7", "missing-file docs/README.md:7", "missing-file docs/README.md:7",
		"missing-anchor docs/sub/broken-link.md:1",
	}, got)
	assert.Equal(t, &DocsLinkStat{Checked: 11, Broken: 5, External: 2}, res.inventory.Links)
	paths := map[string]*string{}
	for _, e := range res.inventory.Documents {
		paths[*e.Document.Path] = e.Owner
		assert.Equal(t, "abc123", e.Document.VersionRef)
		assert.Equal(t, "42", e.Document.ContainerNativeId)
	}
	lead := "lead"
	assert.Equal(t, map[string]*string{
		"docs/README.md": &lead, "docs/guide.md": nil, "docs/sub/a.md": nil, "docs/notes.markdown": nil, "docs/sub/broken-link.md": nil,
		"docs/api.openapi.yaml": nil, "docs/api.json": nil,
	}, paths)
	assert.Equal(t, 7, res.scanned)
	for _, e := range res.inventory.Documents {
		if *e.Document.Path == "docs/api.openapi.yaml" {
			assert.Equal(t, "Orders API", *e.Document.Title)
			sum := sha256.Sum256([]byte("openapi: 3.1.0\ninfo:\n  title: Orders API\n  version: 1.0.0\n"))
			assert.Equal(t, hex.EncodeToString(sum[:]), *e.Document.ContentSha256)
		}
	}
}

func TestSpecTitle_YAMLAndJSON(t *testing.T) {
	assert.Equal(t, "Orders", specTitle([]byte("openapi: 3.1.0\ninfo:\n  version: 1\n  title: \"Orders\" # API\npaths: {}\n")))
	assert.Equal(t, "JSON", specTitle([]byte(`{"info":{"title":"JSON"}}`)))
	assert.Empty(t, specTitle([]byte("openapi: 3.1.0\npaths:\n  title: not info\n")))
}

func TestLoadDocsProfile_HashesProfileAndRuleset(t *testing.T) {
	p, err := LoadDocsProfile("profiles/git-markdown-openapi")

	require.NoError(t, err)
	assert.Equal(t, "git-markdown-openapi", p.ID)
	var content []byte
	for _, name := range []string{CodeProfileFile, DocsRulesetFile} {
		data, err := os.ReadFile(filepath.Join("profiles", "git-markdown-openapi", name))
		require.NoError(t, err)
		content = append(content, data...)
	}
	sum := sha256.Sum256(content)
	assert.Equal(t, hex.EncodeToString(sum[:]), p.RulesHash())

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		CodeProfileFile: `{"profile_id":"d","timeout":"1m","tools":{"spectral":"6"},"markdown_extensions":[],"anchor_rule":"gitea","owner_front_matter_key":"","spec_errors":[]}`,
		DocsRulesetFile: "extends: []\n",
	})
	_, err = LoadDocsProfile(dir)
	require.ErrorContains(t, err, "tools.goldmark: version is required")
	require.ErrorContains(t, err, `anchor_rule "gitea" is not supported`)
	require.ErrorContains(t, err, "owner_front_matter_key, markdown_extensions and spec_errors are required")
}
