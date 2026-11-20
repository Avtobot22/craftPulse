package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _docsProfile = "../../internal/ci/profiles/git-markdown-openapi"

// _fakeSpectral reports a schema error for the legacy specification.
const _fakeSpectral = `echo "$PWD $*" >> "$FAKE_LOG/spectral.log"
while [ $# -gt 0 ]; do [ "$1" = --output ] && out=$2; shift; done
printf '[{"code":"oas3-schema","message":"description is required","severity":0,"source":"%s/docs/legacy.openapi.yaml","range":{"start":{"line":9,"character":8}}}]' "$PWD" > "$out"
exit 1`

func packageFile(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		require.NoError(t, err, "no %s in the package", name)
		if h.Name == name {
			content, err := io.ReadAll(tr)
			require.NoError(t, err)
			return content
		}
	}
}

func TestDocs_RecordsARunPerRootKindWithSARIFAndInventory(t *testing.T) {
	bin, logs := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "spectral"), []byte("#!/bin/sh\n"+_fakeSpectral+"\n"), 0o700)) //nolint:gosec // a test tool must be executable
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", logs)
	c := newCLI(t, newFakeAPI(t))
	repo := c.path("repo")
	for name, content := range map[string]string{
		"docs/README.md":           "---\nowner: lead\n---\n# Docs\n\n[api](api.md#errors) [legacy](legacy.openapi.yaml)\n",
		"docs/api.md":              "# API\n",
		"docs/legacy.openapi.yaml": "openapi: 3.1.0\ninfo:\n  title: Legacy\n  version: 1.0.0\n",
	} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	c.env["CI_PROJECT_DIR"] = repo
	digest := "sha256:" + strings.Repeat("d", 64)
	row := func(kind string) map[string]any {
		return map[string]any{
			"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "", "analysis_kind": kind,
			"mode": "builtin", "profile_id": "git-markdown-openapi", "image_digest": digest,
		}
	}
	saveProfile(t, c, row("docs"), row("openapi"))
	state := filepath.Join(c.env["CRAFTPULSE_WORK_DIR"], "profile.json")
	var saved map[string]any
	data, err := os.ReadFile(state)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &saved))
	saved["documentation_roots"] = []map[string]any{
		{"root_id": "0193a5b0-e4e4-7000-8000-000000000001", "kind": "docs", "path_prefix": "docs"},
		{"root_id": "0193a5b0-e4e4-7000-8000-000000000001", "kind": "openapi", "path_prefix": "docs"},
	}
	data, err = json.Marshal(saved)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(state, data, 0o600))

	out := c.mustRun("docs", "--profile-dir", _docsProfile)
	c.mustRun("upload", "--dry-run", "--save", c.path("package.tar.gz"))

	assert.Contains(t, out, "tool run docs-0193a5b0-e4e4-7000-8000-000000000001: docs builtin by craftpulse-docs 1, findings, complete, 2 report(s)")
	assert.Contains(t, out, "tool run openapi-0193a5b0-e4e4-7000-8000-000000000001: openapi builtin by spectral 6.17.1, findings, complete, 2 report(s)")
	pkg, err := os.ReadFile(c.path("package.tar.gz"))
	require.NoError(t, err)
	runs := map[envelope.ScanToolRunReportKind]envelope.ScanToolRun{}
	for _, run := range checkPackage(t, pkg).ScanManifest.Tools {
		runs[run.ReportKind] = run
	}
	docs, openapi := runs[envelope.ScanToolRunReportKindDocs], runs[envelope.ScanToolRunReportKindOpenapi]
	assert.Equal(t, "docs", docs.Component.PathPrefix)
	assert.Equal(t, "goldmark 1.8.7", *docs.ParserVersion)
	assert.Equal(t, 3, *docs.ScannedCount)
	require.Len(t, docs.Reports, 2)
	assert.Equal(t, envelope.ScanReportReportFormatDocsJson, docs.Reports[1].ReportFormat)
	var inventory ci.DocsInventory
	require.NoError(t, json.Unmarshal(packageFile(t, pkg, docs.Reports[1].File.Path), &inventory))
	assert.Len(t, inventory.Documents, 3)
	assert.Equal(t, &ci.DocsLinkStat{Checked: 2, Broken: 1}, inventory.Links)
	for _, e := range inventory.Documents {
		assert.Equal(t, c.env["CI_COMMIT_SHA"], e.Document.VersionRef)
	}
	assert.Contains(t, string(packageFile(t, pkg, docs.Reports[0].File.Path)), `"ruleId":"missing-anchor"`)
	var specs ci.DocsInventory
	require.NoError(t, json.Unmarshal(packageFile(t, pkg, openapi.Reports[1].File.Path), &specs))
	require.Len(t, specs.Documents, 1)
	assert.False(t, *specs.Documents[0].SpecValid)
	assert.Equal(t, "Legacy", *specs.Documents[0].Document.Title)
	assert.Contains(t, string(packageFile(t, pkg, openapi.Reports[0].File.Path)), `"startLine":10`)
	log, err := os.ReadFile(filepath.Join(logs, "spectral.log"))
	require.NoError(t, err)
	assert.Contains(t, string(log), "lint --ruleset "+filepath.Join(mustAbs(t, _docsProfile), "spectral.yaml"))
}
