package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _trivyProfile = "../../internal/ci/profiles/trivy-default"

// _fakeTrivy logs its arguments and writes a vulnerability or a misconfiguration report by --scanners.
const _fakeTrivy = `echo "$PWD $*" >> "$FAKE_LOG/trivy.log"
while [ $# -gt 0 ]; do case "$1" in --output) out=$2;; --scanners) scanners=$2;; esac; shift; done
if [ "$scanners" = vuln ]; then
printf '%s' '{"SchemaVersion":2,"Results":[{"Target":"go.mod","Class":"lang-pkgs","Type":"gomod","Packages":[{"Name":"golang.org/x/text","Version":"v0.3.7"}],"Vulnerabilities":[{"VulnerabilityID":"CVE-2022-32149","PkgName":"golang.org/x/text","InstalledVersion":"v0.3.7","FixedVersion":"0.3.8","VendorIDs":["GO-2022-1059"]}]}]}' > "$out"
else
printf '%s' '{"SchemaVersion":2,"Results":[{"Target":"Dockerfile","Class":"config","Type":"dockerfile","Misconfigurations":[{"ID":"DS-0001","CauseMetadata":{"StartLine":1,"Code":{"Lines":[{"Content":"FIXTURE_CONFIG_LINE"}]}}}]}]}' > "$out"
fi`

func trivyJob(t *testing.T, c *cli) (cache, logs string) {
	t.Helper()
	bin, logs := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "trivy"), []byte("#!/bin/sh\n"+_fakeTrivy+"\n"), 0o700)) //nolint:gosec // a test tool must be executable
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", logs)
	repo := c.path("repo")
	for name, content := range map[string]string{
		"services/billing/go.mod": "module example.com/billing\n", "services/billing/Dockerfile": "FROM alpine:latest\n",
		"services/billing/vendor/lib/go.mod": "module lib\n", "services/billing/.cache/x": "x\n",
	} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	c.env["CI_PROJECT_DIR"] = repo
	row := func(kind string) map[string]any {
		return map[string]any{
			"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "services/billing", "language": "", "analysis_kind": kind,
			"mode": "builtin", "profile_id": "trivy-default", "image_digest": "sha256:" + strings.Repeat("e", 64),
		}
	}
	saveProfile(t, c, row("sca"), row("iac"))
	cache = c.path("trivy-cache")
	sum := sha256.Sum256([]byte("database"))
	for name, content := range map[string]string{
		"db/trivy.db": "database", "db/metadata.json": `{"Version":2,"UpdatedAt":"2026-11-20T13:11:33.075919767Z"}`,
		"db/craftpulse.json": `{"sha256":"` + hex.EncodeToString(sum[:]) + `","repository":"mirror.example/trivy-db:2","updated_at":"2026-11-20T13:11:33.075919767Z"}`,
	} {
		p := filepath.Join(cache, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return cache, logs
}

func TestTrivy_RecordsSCAWithDatabaseAndIaCWithoutConfigurationLines(t *testing.T) {
	c := newCLI(t, newFakeAPI(t))
	cache, logs := trivyJob(t, c)

	out := c.mustRun("trivy", "--profile-dir", _trivyProfile, "--cache-dir", cache)
	c.mustRun("upload", "--dry-run", "--save", c.path("package.tar.gz"))

	assert.Contains(t, out, "tool run trivy-default-sca: sca builtin by trivy 0.76.2, findings, complete, 1 report(s)")
	assert.Contains(t, out, "tool run trivy-default-iac: iac builtin by trivy 0.76.2, findings, complete, 1 report(s)")
	log, err := os.ReadFile(filepath.Join(logs, "trivy.log"))
	require.NoError(t, err)
	profile := mustAbs(t, _trivyProfile)
	assert.Contains(t, string(log), filepath.Join(c.env["CI_PROJECT_DIR"], "services", "billing")+" fs --config "+filepath.Join(profile, "trivy.yaml"))
	assert.Contains(t, string(log), "--ignorefile "+filepath.Join(profile, "trivyignore"))
	assert.Contains(t, string(log), "--skip-db-update --skip-check-update --skip-version-check --offline-scan")
	assert.Contains(t, string(log), "--misconfig-scanners dockerfile,kubernetes")
	assert.Contains(t, string(log), "--skip-dirs vendor --skip-dirs .cache .")
	data, err := os.ReadFile(c.path("package.tar.gz"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "FIXTURE_CONFIG_LINE")
	runs := map[envelope.ScanToolRunReportKind]envelope.ScanToolRun{}
	for _, run := range checkPackage(t, data).ScanManifest.Tools {
		runs[run.ReportKind] = run
	}
	sum := sha256.Sum256([]byte("database"))
	sca, iac := runs[envelope.ScanToolRunReportKindSca], runs[envelope.ScanToolRunReportKindIac]
	assert.Equal(t, "2@sha256:"+hex.EncodeToString(sum[:]), *sca.DbVersion)
	assert.Equal(t, "2026-11-20T13:11:33.075919767Z", sca.DbPublishedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))
	assert.Nil(t, iac.DbVersion)
	for _, run := range []envelope.ScanToolRun{sca, iac} {
		require.Len(t, run.Reports, 1)
		assert.Equal(t, envelope.ScanReportReportFormatTrivyJson, run.Reports[0].ReportFormat)
		assert.Equal(t, []envelope.ScanExcludedTarget{{Path: "services/billing/vendor", Reason: envelope.ScanExcludedTargetReasonVendor}}, *run.ExcludedTargets)
		assert.Equal(t, 1, *run.ScannedCount)
	}
}

func TestTrivy_MissingDatabaseFailsSCAWithoutReport(t *testing.T) {
	c := newCLI(t, newFakeAPI(t))
	_, _ = trivyJob(t, c)

	out := c.mustRun("trivy", "--profile-dir", _trivyProfile, "--cache-dir", c.path("empty-cache"))
	c.mustRun("upload", "--dry-run", "--save", c.path("package.tar.gz"))

	assert.Contains(t, out, "trivy vulnerability database is missing or incomplete")
	data, err := os.ReadFile(c.path("package.tar.gz"))
	require.NoError(t, err)
	for _, run := range checkPackage(t, data).ScanManifest.Tools {
		if run.ReportKind == envelope.ScanToolRunReportKindSca {
			assert.Equal(t, envelope.ScanToolRunExitClassFailed, run.ExitClass)
			assert.Empty(t, run.Reports)
			assert.Nil(t, run.DbVersion)
			continue
		}
		assert.Equal(t, envelope.ScanToolRunCompletenessComplete, run.Completeness)
	}
}
