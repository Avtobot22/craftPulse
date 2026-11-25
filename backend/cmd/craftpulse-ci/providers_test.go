package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestUpload_AdditionalCIProviders(t *testing.T) {
	for _, system := range []string{"github", "jenkins"} {
		t.Run(system, func(t *testing.T) {
			api := newFakeAPI(t)
			api.uploads = []reply{api.accepted("quarantined", 1)}
			api.receipts = []reply{api.receipt("accepted", nil)}
			c := newCLI(t, api)
			for key := range c.env {
				if strings.HasPrefix(key, "CI_") {
					delete(c.env, key)
				}
			}
			c.env["CRAFTPULSE_DEBUG"] = "1"
			var (
				want       envelope.ScanCiCoordinates
				uploadArgs []string
			)
			switch system {
			case "github":
				c.env["GITHUB_ACTIONS"] = "true"
				c.env["GITHUB_REPOSITORY_ID"], c.env["GITHUB_REPOSITORY"] = "42", "demo/billing"
				c.env["GITHUB_RUN_ID"], c.env["GITHUB_RUN_ATTEMPT"], c.env["GITHUB_JOB"] = "5001", "2", "scan"
				c.env["GITHUB_SHA"], c.env["GITHUB_REF"] = _sha, "refs/pull/19/merge"
				c.env["GITHUB_EVENT_PATH"] = c.path("event.json")
				require.NoError(t, os.WriteFile(c.env["GITHUB_EVENT_PATH"], []byte(`{"pull_request":{"number":19}}`), 0o600))
				want = envelope.ScanCiCoordinates{
					Provider: envelope.ScanCiCoordinatesProviderGithub, ProjectNativeId: "42", RunNativeId: "5001",
					JobNativeId: ptr("scan"), AttemptNo: ptr(2), RefName: "refs/pull/19/merge", MrNativeId: ptr("19"),
				}
			case "jenkins":
				c.env["JENKINS_URL"], c.env["BUILD_URL"] = "https://jenkins.example/", "https://jenkins.example/job/folder/job/billing/7/"
				c.env["JOB_NAME"], c.env["BUILD_NUMBER"], c.env["NODE_NAME"] = "folder/billing", "7", "linux"
				c.env["GIT_COMMIT"], c.env["BRANCH_NAME"], c.env["CHANGE_ID"], c.env["CHANGE_BRANCH"] = _sha, "PR-19", "19", "feature"
				for _, attempt := range []struct{ number, outcome string }{{"1", "failure"}, {"2", "success"}} {
					args := []string{
						"jenkins", "record", "--attempts-dir", c.path("attempts"), "--stage", "checks", "--step", "scan",
						"--attempt", attempt.number, "--outcome", attempt.outcome, "--axes", "os=linux",
						"--started-at", "2026-11-17T10:00:00Z", "--finished-at", "2026-11-17T10:00:10Z",
					}
					out := runCIProvider(t, c, args...)
					if attempt.number == "2" {
						uploadArgs = []string{"--execution-manifest", c.path(envelope.ExecutionManifestName), "--jenkins-attempt-id", strings.TrimSpace(out)}
					}
				}
				runCIProvider(t, c, "jenkins", "finish", "--attempts-dir", c.path("attempts"), "--output", c.path(envelope.ExecutionManifestName))
				want = envelope.ScanCiCoordinates{
					Provider: envelope.ScanCiCoordinatesProviderJenkins, ProjectNativeId: "folder/billing", RunNativeId: "7",
					JobNativeId: ptr("folder/billing"), AttemptNativeId: &uploadArgs[3], AttemptNo: ptr(2), RefName: "feature", MrNativeId: ptr("19"),
				}
			}
			runCIProvider(t, c, c.goTool(c.copyReport("1")...)...)
			out := runCIProvider(t, c, append([]string{"upload"}, uploadArgs...)...)
			assert.Contains(t, out, "findings")
			assert.Contains(t, out, "debug:")
			require.Len(t, api.bodies, 1)
			result := checkPackage(t, api.bodies[0])
			assert.Equal(t, system, string(result.Manifest.Source.Provider))
			require.Len(t, result.ScanManifest.Tools, 1)
			assert.Equal(t, &want, result.ScanManifest.Tools[0].Ci)
			assert.Equal(t, ptr(_sha), result.ScanManifest.Tools[0].Sha)
			if system == "jenkins" {
				require.NotNil(t, result.ExecutionManifest)
				assert.Equal(t, c.env["BUILD_URL"], result.ExecutionManifest.BuildUrl)
				require.Len(t, result.ExecutionManifest.Steps, 2)
				for _, step := range result.ExecutionManifest.Steps {
					assert.Equal(t, "scan", step.Name)
					assert.Equal(t, map[string]string{"os": "linux"}, step.Axes)
					assert.Equal(t, "linux", step.AgentLabel)
					assert.Equal(t, map[int]string{1: "failure", 2: "success"}[step.Attempt], string(step.Outcome))
				}
			}
			for _, auth := range api.auth {
				assert.Equal(t, "Bearer "+_token, auth)
			}
			api.uploads = []reply{api.failure(422, "ValidationFailed", nil)}
			code, refused := runProviderCLI(t, c, append([]string{"upload"}, uploadArgs...)...)
			assert.Equal(t, _exitRefused, code, refused)
			api.uploads = []reply{api.accepted("quarantined", 1)}
			api.receipts = []reply{api.receipt("quarantined", nil)}
			code, unknown := runProviderCLI(t, c, append([]string{"upload", "--wait", "200ms"}, uploadArgs...)...)
			assert.Equal(t, _exitUnknown, code, unknown)
		})
	}
}

func runCIProvider(t *testing.T, c *cli, args ...string) string {
	t.Helper()
	code, out := runProviderCLI(t, c, args...)
	require.Zero(t, code, out)
	return out
}

// CRAFTPULSE_TEST_IMAGE runs the same scenarios with the Linux ci-runtime image
// and its installed CLI. Host networking reaches only the local fake API.
func runProviderCLI(t *testing.T, c *cli, args ...string) (code int, out string) {
	t.Helper()
	if image := os.Getenv("CRAFTPULSE_TEST_IMAGE"); image != "" {
		dockerArgs := []string{"run", "--rm", "--network", "host", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--volume", c.dir + ":" + c.dir}
		var env []string
		for key, value := range c.env {
			dockerArgs = append(dockerArgs, "--env", key)
			env = append(env, key+"="+value)
		}
		dockerArgs = append(dockerArgs, image, "craftpulse-ci")
		cmd := exec.CommandContext(t.Context(), "docker", append(dockerArgs, args...)...) //nolint:gosec // test selects its local image
		cmd.Env = append(os.Environ(), env...)
		output, err := cmd.CombinedOutput()
		assert.NotContains(t, string(output), _token)
		if err != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			return exit.ExitCode(), string(output)
		}
		return 0, string(output)
	}
	code, out = c.run(args...)
	assert.NotContains(t, out, _token)
	return code, out
}
