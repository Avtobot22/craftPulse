package ci

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitlabEnv(overrides map[string]string) func(string) string {
	env := map[string]string{
		"CRAFTPULSE_URL": "https://craftpulse.example", "CRAFTPULSE_UPLOAD_TOKEN": "cpt_secret",
		"CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
		"CI_COMMIT_SHA": "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac", "CI_SERVER_HOST": "GitLab.Example",
		"CI_SERVER_PORT": "443", "CI_SERVER_PROTOCOL": "https",
	}
	for k, v := range overrides {
		env[k] = v
	}
	return func(k string) string { return env[k] }
}

func TestJobFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		give            map[string]string
		wantErr         string
		wantInstanceKey string
	}{
		{name: "default port", wantInstanceKey: "gitlab.example"},
		{name: "own port", give: map[string]string{"CI_SERVER_PORT": "8443"}, wantInstanceKey: "gitlab.example:8443"},
		{name: "declared key", give: map[string]string{"CRAFTPULSE_INSTANCE_KEY": "gitlab.corp"}, wantInstanceKey: "gitlab.corp"},
		{name: "loopback http", give: map[string]string{"CRAFTPULSE_URL": "http://127.0.0.1:8080/"}, wantInstanceKey: "gitlab.example"},
		{name: "plain http", give: map[string]string{"CRAFTPULSE_URL": "http://craftpulse.example"}, wantErr: "CRAFTPULSE_URL must use https"},
		{name: "credentials in url", give: map[string]string{"CRAFTPULSE_URL": "https://cpt_secret@craftpulse.example"}, wantErr: "without credentials"},
		{name: "no token", give: map[string]string{"CRAFTPULSE_UPLOAD_TOKEN": ""}, wantErr: "CRAFTPULSE_UPLOAD_TOKEN is not set"},
		{name: "short sha", give: map[string]string{"CI_COMMIT_SHA": "d2126fc"}, wantErr: "CI_COMMIT_SHA must be a full lowercase commit SHA"},
		{name: "bad job", give: map[string]string{"CI_JOB_ID": "job-cpt_secret"}, wantErr: "CI_JOB_ID must be a GitLab ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, err := JobFromEnv(gitlabEnv(tt.give))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.NotContains(t, err.Error(), "cpt_secret")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantInstanceKey, j.InstanceKey)
			assert.Equal(t, filepath.Join(filepath.Dir(j.WorkDir), "craftpulse-ci-7002"), j.WorkDir)
		})
	}
}

func TestBackoff(t *testing.T) {
	tests := []struct {
		name       string
		giveJitter float64
		attempt    int
		want       time.Duration
	}{
		{name: "first, low spread", giveJitter: 0, attempt: 1, want: 500 * time.Millisecond},
		{name: "third, high spread", giveJitter: 1, attempt: 3, want: 6 * time.Second},
		{name: "capped", giveJitter: 0.5, attempt: 10, want: 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{jitter: func() float64 { return tt.giveJitter }}
			assert.Equal(t, tt.want, c.backoff(tt.attempt))
		})
	}
}

func TestSARIFFindings_PathsRelativeToSource(t *testing.T) {
	root := t.TempDir()
	report := `{"runs":[{"results":[
		{"ruleId":"G104","locations":[{"physicalLocation":{"artifactLocation":{"uri":"file://` + filepath.ToSlash(root) + `/cmd/main.go"},"region":{"startLine":3}}}]},
		{"rule":{"id":"unused"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"./pkg/../pkg/util.go"},"region":{"startLine":7}}}]}
	]}]}`

	got, err := SARIFFindings([]byte(report), root)

	require.NoError(t, err)
	assert.Equal(t, []Finding{{Rule: "G104", File: "cmd/main.go", Line: 3}, {Rule: "unused", File: "pkg/util.go", Line: 7}}, got)
}

func TestDiff_CountsRepeatedFindings(t *testing.T) {
	f := Finding{Rule: "G104", File: "main.go", Line: 3}
	g := Finding{Rule: "G101", File: "config.go", Line: 5}

	missing, extra := diff([]Finding{f, f, g}, []Finding{f, g, g}, compareFindings)

	assert.Equal(t, []Finding{f}, missing)
	assert.Equal(t, []Finding{g}, extra)
}
