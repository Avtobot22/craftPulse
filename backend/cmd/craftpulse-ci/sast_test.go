package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_sastProfile = "../../internal/ci/profiles/opengrep-default"
	_sastDigest  = "sha256:" + "c0de" + "000000000000000000000000000000000000000000000000000000000000"
)

// _fakeOpengrep logs its arguments and writes a SARIF report with a prefixed rule and a snippet, and the JSON
// details with FAKE_ERRORS; FAKE_SLEEP delays it and FAKE_EXIT ends it with another code.
const _fakeOpengrep = `echo "$PWD $*" >> "$FAKE_LOG/opengrep.log"
for a in "$@"; do case "$a" in --sarif-output=*) sarif=${a#*=};; --json-output=*) details=${a#*=};; esac; done
[ -n "$FAKE_SLEEP" ] && sleep "$FAKE_SLEEP"
[ -n "$FAKE_EXIT" ] && exit "$FAKE_EXIT"
printf '%s' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Opengrep OSS","semanticVersion":"1.33.1","rules":[{"id":"opt.rules.go-shell-command","properties":{"security-severity":"8.8","tags":["CWE-78"]}}]}},"results":[{"ruleId":"opt.rules.go-shell-command","locations":[{"physicalLocation":{"artifactLocation":{"uri":"services/billing/run.go"},"region":{"startLine":6,"snippet":{"text":"FIXTURE_CODE_LINE"}}}}]}]}]}' > "$sarif"
printf '{"errors":[%s],"paths":{"scanned":["services/billing/run.go","services/billing/web/broken.js"]}}' "$FAKE_ERRORS" > "$details"
exit 1`

func fakeOpengrep(t *testing.T) string {
	t.Helper()
	bin, logs := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "opengrep"), []byte("#!/bin/sh\n"+_fakeOpengrep+"\n"), 0o700)) //nolint:gosec // a test tool must be executable
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", logs)
	return logs
}

// sastJob is a job whose saved profile has the sast row of opengrep-default on services/billing.
func sastJob(t *testing.T, c *cli) {
	t.Helper()
	repo := c.path("repo")
	for _, dir := range []string{"services/billing/vendor", "services/billing/node_modules/x", "services/billing/.cache", "services/billing/web"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, dir), 0o750))
	}
	c.env["CI_PROJECT_DIR"] = repo
	state := c.env["CRAFTPULSE_WORK_DIR"]
	require.NoError(t, os.MkdirAll(state, 0o700))
	profile := map[string]any{"entries": []map[string]any{{
		"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "services/billing", "language": "go", "analysis_kind": "sast",
		"mode": "builtin", "profile_id": "opengrep-default", "image_digest": _sastDigest,
	}}}
	data, err := json.Marshal(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(state, "profile.json"), data, 0o600))
}

func sastPackage(t *testing.T, c *cli) (run envelope.ScanToolRun, data []byte) {
	t.Helper()
	c.mustRun("upload", "--dry-run", "--save", c.path("package.tar.gz"))
	data, err := os.ReadFile(c.path("package.tar.gz"))
	require.NoError(t, err)
	tools := checkPackage(t, data).ScanManifest.Tools
	require.Len(t, tools, 1)
	return tools[0], data
}

func TestSAST_RecordsRunWithRulesHashSkippedFilesAndNoCode(t *testing.T) {
	logs := fakeOpengrep(t)
	t.Setenv("FAKE_ERRORS", `{"type":["PartialParsing",[]],"path":"services/billing/web/broken.js","message":"Syntax error"}`)
	c := newCLI(t, newFakeAPI(t))
	sastJob(t, c)

	out := c.mustRun("sast", "--profile-dir", _sastProfile)
	run, data := sastPackage(t, c)

	assert.Contains(t, out, "tool run opengrep-default-opengrep: sast builtin by opengrep 1.33.1, partial, partial, 1 report(s)")
	args, err := os.ReadFile(filepath.Join(logs, "opengrep.log"))
	require.NoError(t, err)
	rules, err := filepath.Abs(filepath.Join(_sastProfile, "rules"))
	require.NoError(t, err)
	assert.Contains(t, string(args), c.path("repo")+" scan --config "+rules+" ")
	assert.Contains(t, string(args), "--exclude=services/billing/node_modules --exclude=services/billing/vendor --exclude=services/billing/.cache services/billing")
	assert.Equal(t, envelope.ScanToolRunExitClassPartial, run.ExitClass)
	assert.Equal(t, envelope.ScanToolRunCompletenessPartial, run.Completeness, "an unparsed file is not complete")
	assert.Equal(t, []envelope.ScanSkippedTarget{{Path: "services/billing/web/broken.js", Reason: envelope.ScanSkippedTargetReasonUnsupported}}, run.SkippedTargets)
	assert.Equal(t, 2, *run.ScannedCount)
	assert.Equal(t, 1, *run.SkippedCount)
	assert.ElementsMatch(t, []envelope.ScanExcludedTarget{
		{Path: "services/billing/vendor", Reason: envelope.ScanExcludedTargetReasonVendor},
		{Path: "services/billing/node_modules", Reason: envelope.ScanExcludedTargetReasonVendor},
	}, *run.ExcludedTargets)
	assert.Equal(t, "1.33.1", run.ProducerVersion)
	assert.Equal(t, _sastDigest, *run.ProfileImageDigest)
	assert.Equal(t, fileHash(t, "../opengrep-default/profile.json"), *run.ConfigHash)
	require.Len(t, run.Reports, 1)
	assert.Equal(t, envelope.ScanReportReportKindSast, run.Reports[0].ReportKind)
	assert.Equal(t, "2.1.0", run.Reports[0].FormatVersion)
	assert.NotContains(t, string(data), "FIXTURE_CODE_LINE")
	assert.NotContains(t, string(data), "opt.rules.")

	t.Setenv("FAKE_ERRORS", "")
	clean := newCLI(t, newFakeAPI(t))
	sastJob(t, clean)
	assert.Contains(t, clean.mustRun("sast", "--profile-dir", _sastProfile), "findings, complete, 1 report(s)")
}

func TestSAST_TimeoutAndCrashAreNotComplete(t *testing.T) {
	fakeOpengrep(t)
	for _, tt := range []struct {
		name, env, value string
		args             []string
		exit             envelope.ScanToolRunExitClass
	}{
		{"timeout", "FAKE_SLEEP", "5", []string{"--timeout", "300ms"}, envelope.ScanToolRunExitClassTimeout},
		{"crash", "FAKE_EXIT", "2", nil, envelope.ScanToolRunExitClassFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)
			c := newCLI(t, newFakeAPI(t))
			sastJob(t, c)
			c.mustRun(append([]string{"sast", "--profile-dir", _sastProfile}, tt.args...)...)
			run, _ := sastPackage(t, c)
			assert.Equal(t, tt.exit, run.ExitClass)
			assert.NotEqual(t, envelope.ScanToolRunCompletenessComplete, run.Completeness)
			assert.Empty(t, run.Reports)
			assert.Less(t, run.FinishedAt.Sub(run.StartedAt).Seconds(), 4.0)
		})
	}
}
