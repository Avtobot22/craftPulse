package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_token     = "cpt_9f3c1a7be2d04c6e8a5b1f0d3c7e9a2b4d6f8e0c"
	_sha       = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"
	_sarif     = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"stub"}},"results":[]}]}`
	_receiptID = "0193a5b0-5858-7000-8000-000000000001"
)

var (
	_contractOnce sync.Once
	_contract     *openapi3.T
	_contractErr  error
)

func contract(t *testing.T) *openapi3.T {
	t.Helper()
	_contractOnce.Do(func() {
		_contract, _contractErr = openapi3.NewLoader().LoadFromFile("../../../docs/technical-specification/openapi.yaml")
	})
	require.NoError(t, _contractErr)
	return _contract
}

// reply is one scripted answer of the test server.
type reply struct {
	status  int
	header  map[string]string
	body    any
	problem string
}

// fakeAPI answers the three CI operations with the examples of openapi.yaml and checks every answer
// against the contract.
type fakeAPI struct {
	t        *testing.T
	doc      *openapi3.T
	srv      *httptest.Server
	mu       sync.Mutex
	uploads  []reply
	receipts []reply
	bodies   [][]byte
	polls    int
	auth     []string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, doc: contract(t)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) example(path, method, status string) map[string]any {
	op := f.doc.Paths.Value(path).GetOperation(method)
	content := op.Responses.Value(status).Value.Content.Get("application/json")
	data, err := json.Marshal(content.Example)
	require.NoError(f.t, err)
	var out map[string]any
	require.NoError(f.t, json.Unmarshal(data, &out))
	return out
}

func (f *fakeAPI) problem(name string) any {
	return f.doc.Components.Responses[name].Value.Content.Get("application/problem+json").Example
}

func (f *fakeAPI) accepted(status string, retryAfter any) reply {
	body := f.example("/ingest/packages", http.MethodPost, "202")
	body["status"], body["retry_after_seconds"] = status, retryAfter
	return reply{status: http.StatusAccepted, header: map[string]string{"Location": "/api/v1/receipts/" + _receiptID}, body: body}
}

func (f *fakeAPI) receipt(status string, mutate func(map[string]any)) reply {
	body := f.example("/receipts/{receipt_id}", http.MethodGet, "200")
	body["id"], body["channel"], body["status"], body["schema_id"] = _receiptID, "ci", status, "ci_runs"
	if status == "quarantined" {
		body["retry_after_seconds"] = 1
	}
	if mutate != nil {
		mutate(body)
	}
	return reply{status: http.StatusOK, body: body}
}

func (f *fakeAPI) failure(status int, name string, header map[string]string) reply {
	return reply{status: status, header: header, body: f.problem(name), problem: name}
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	require.NoError(f.t, err)
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	var answer reply
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ci/check-profile":
		answer = reply{status: http.StatusOK, body: f.example("/ci/check-profile", http.MethodGet, "200")}
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ingest/packages":
		f.bodies = append(f.bodies, body)
		answer = next(&f.uploads)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/receipts/"+_receiptID:
		f.polls++
		answer = next(&f.receipts)
	default:
		answer = reply{status: http.StatusNotFound, body: f.problem("NotFound"), problem: "NotFound"}
	}
	f.mu.Unlock()
	rec := httptest.NewRecorder()
	media := "application/json"
	if answer.problem != "" {
		media = "application/problem+json"
	}
	rec.Header().Set("Content-Type", media)
	for k, v := range answer.header {
		rec.Header().Set(k, v)
	}
	rec.WriteHeader(answer.status)
	require.NoError(f.t, json.NewEncoder(rec).Encode(answer.body))
	f.validate(r, rec)
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes()) // the client may hang up; the test checks what it printed
}

func (f *fakeAPI) validate(r *http.Request, rec *httptest.ResponseRecorder) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	if strings.HasPrefix(path, "/receipts/") {
		path = "/receipts/{receipt_id}"
	}
	item := f.doc.Paths.Value(path)
	require.NotNil(f.t, item, path)
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: r, Route: &routers.Route{
			Spec: f.doc, Path: path, PathItem: item, Method: r.Method, Operation: item.GetOperation(r.Method),
		}},
		Status: rec.Code, Header: rec.Header(), Body: io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	require.NoError(f.t, openapi3filter.ValidateResponse(r.Context(), input), rec.Body.String())
}

// next returns the first scripted answer; the last one repeats.
func next(script *[]reply) reply {
	r := (*script)[0]
	if len(*script) > 1 {
		*script = (*script)[1:]
	}
	return r
}

// cli runs the command against the test server with pauses shortened a thousand times.
type cli struct {
	t      *testing.T
	api    *fakeAPI
	env    map[string]string
	dir    string
	mu     sync.Mutex
	pauses []time.Duration
}

func newCLI(t *testing.T, api *fakeAPI) *cli {
	t.Helper()
	dir := t.TempDir()
	c := &cli{t: t, api: api, dir: dir, env: map[string]string{
		"CRAFTPULSE_URL": api.srv.URL, "CRAFTPULSE_UPLOAD_TOKEN": _token, "CRAFTPULSE_WORK_DIR": filepath.Join(dir, "state"),
		"CI_PROJECT_ID": "42", "CI_PROJECT_PATH": "demo/billing", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
		"CI_COMMIT_SHA": _sha, "CI_COMMIT_REF_NAME": "demo/PA-1-change", "CI_MERGE_REQUEST_ID": "9001",
		"CI_SERVER_HOST": "gitlab.example", "CI_SERVER_PORT": "443", "CI_SERVER_PROTOCOL": "https",
	}}
	require.NoError(t, os.WriteFile(c.path("fixture.sarif"), []byte(_sarif), 0o600))
	return c
}

func (c *cli) path(name string) string { return filepath.Join(c.dir, name) }

func (c *cli) run(args ...string) (code int, out string) {
	c.t.Helper()
	var buf bytes.Buffer
	cfg := ci.ClientConfig{
		HTTP:   c.api.srv.Client(),
		Jitter: func() float64 { return 0.5 },
		Sleep: func(ctx context.Context, d time.Duration) error {
			c.mu.Lock()
			c.pauses = append(c.pauses, d)
			c.mu.Unlock()
			t := time.NewTimer(d / 1000)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
	code = execute(c.t.Context(), args, &buf, &buf, func(k string) string { return c.env[k] }, cfg)
	return code, buf.String()
}

func (c *cli) mustRun(args ...string) string {
	c.t.Helper()
	code, out := c.run(args...)
	require.Equal(c.t, 0, code, out)
	return out
}

func (c *cli) goTool(args ...string) []string {
	return append([]string{
		"run", "--profile-id", "go-default", "--kind", "code", "--format", "sarif", "--format-version", "2.1.0",
		"--producer", "golangci-lint", "--producer-version", "2.6.0", "--report", c.path("report.sarif"),
	}, args...)
}

// copyReport is a tool that writes the fixture report and exits with code.
func (c *cli) copyReport(code string) []string {
	return []string{"--", "sh", "-c", `cp "$1" "$2"; exit "$3"`, "sh", c.path("fixture.sarif"), c.path("report.sarif"), code}
}

func checkPackage(t *testing.T, data []byte) *envelope.Result {
	t.Helper()
	result := envelope.Check(bytes.NewReader(data), envelope.DefaultLimits)
	require.Nil(t, result.Envelope)
	require.Zero(t, result.Rejected())
	require.NotNil(t, result.ScanManifest)
	return result
}

func TestUpload_PrintsReceiptOfAcceptedPackageWithFindings(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{api.accepted("quarantined", 3)}
	api.receipts = []reply{api.receipt("quarantined", nil), api.receipt("partial", func(b map[string]any) {
		b["record_count"], b["accepted_count"], b["rejected_count"], b["replay_count"], b["conflict_count"] = 3, 1, 1, 1, 0
		b["reason_code"] = "multiple_record_rejections"
	})}
	c := newCLI(t, api)

	out := c.mustRun("profile", "--kind", "code")
	assert.Contains(t, out, "profile row: code builtin go-default component 0193a5b0-7777-7000-8000-000000000001")
	assert.NotContains(t, out, "gitleaks-default")
	c.mustRun(c.goTool(c.copyReport("1")...)...)
	require.NoError(t, os.WriteFile(c.path("junit.xml"), []byte(`<testsuites/>`), 0o600))
	c.mustRun("import", "--report", "junit:junit_xml:"+c.path("junit.xml"), "--producer", "go test", "--producer-version", "go1.27.4", "--producer-job", "7001")
	out = c.mustRun("upload", "--save", c.path("package.tar.gz"))

	assert.Contains(t, out, "tool run go-default-code: code builtin by golangci-lint 2.6.0, findings, complete, 1 report(s)")
	assert.Contains(t, out, "expected reports not in this package: secrets, sca, coverage")
	assert.Contains(t, out, "receipt "+_receiptID+": partial")
	assert.Contains(t, out, "records: 3 total, 1 accepted, 1 rejected, 1 replays, 0 conflicts")
	assert.Contains(t, out, "reason: multiple_record_rejections")
	assert.Contains(t, out, "result:\n"+api.srv.URL+"/teams/0193a5b0-3333-7000-8000-000000000001/report")
	assert.Equal(t, 2, api.polls)

	saved, err := os.ReadFile(c.path("package.tar.gz"))
	require.NoError(t, err)
	require.Len(t, api.bodies, 1)
	assert.Equal(t, saved, api.bodies[0])
	result := checkPackage(t, api.bodies[0])
	assert.Equal(t, envelope.ManifestSchemaIdCiRuns, result.Manifest.SchemaId)
	assert.Equal(t, "gitlab.example", result.Manifest.Source.InstanceKey)
	assert.Equal(t, "42", result.Manifest.Area.NativeId)
	require.Len(t, result.ScanManifest.Tools, 2)
	builtin, imported := result.ScanManifest.Tools[0], result.ScanManifest.Tools[1]
	assert.Equal(t, _sha, *builtin.Sha)
	assert.Equal(t, envelope.ScanCiCoordinates{
		Provider: envelope.ScanCiCoordinatesProviderGitlab, ProjectNativeId: "42", RunNativeId: "5001", JobNativeId: ptr("7002"),
		AttemptNativeId: ptr("7002"), RefName: "demo/PA-1-change", MrNativeId: ptr("9001"),
	}, *builtin.Ci)
	assert.Equal(t, "sha256:6b2f0c1d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c", *builtin.ProfileImageDigest)
	assert.Equal(t, envelope.ScanToolRunModeImport, imported.Mode)
	assert.Equal(t, "7001", *imported.Ci.JobNativeId)
	assert.Equal(t, _sha, *imported.Sha)
	for _, auth := range api.auth {
		assert.Equal(t, "Bearer "+_token, auth)
	}
}

func TestRun_TimeoutAndCrashAreNeverComplete(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{api.accepted("quarantined", 1)}
	api.receipts = []reply{api.receipt("accepted", nil)}
	c := newCLI(t, api)

	out := c.mustRun(c.goTool("--id", "slow", "--timeout", "200ms", "--", "sleep", "5")...)
	assert.Contains(t, out, "tool run slow: code builtin by golangci-lint 2.6.0, timeout, partial, 0 report(s)")
	c.mustRun(c.goTool("--id", "crash", "--", "sh", "-c", "exit 3")...)
	c.mustRun(c.goTool("--id", "silent", "--", "true")...)
	c.mustRun(c.goTool(append([]string{"--id", "clean"}, c.copyReport("0")...)...)...)
	c.mustRun("upload")

	tools := map[string]envelope.ScanToolRun{}
	for _, run := range checkPackage(t, api.bodies[0]).ScanManifest.Tools {
		tools[run.Id] = run
	}
	assert.Equal(t, envelope.ScanToolRunExitClassTimeout, tools["slow"].ExitClass)
	assert.Equal(t, envelope.ScanToolRunCompletenessPartial, tools["slow"].Completeness)
	assert.Less(t, tools["slow"].FinishedAt.Sub(tools["slow"].StartedAt), 4*time.Second)
	assert.Equal(t, envelope.ScanToolRunExitClassFailed, tools["crash"].ExitClass)
	assert.Equal(t, envelope.ScanToolRunCompletenessUnknown, tools["crash"].Completeness)
	assert.Equal(t, envelope.ScanToolRunExitClassFailed, tools["silent"].ExitClass, "exit 0 without a report is not success")
	assert.Equal(t, envelope.ScanToolRunExitClassSuccess, tools["clean"].ExitClass)
	assert.Equal(t, envelope.ScanToolRunCompletenessComplete, tools["clean"].Completeness)
}

func TestUpload_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		uploads  func(*fakeAPI) []reply
		receipts func(*fakeAPI) []reply
		wait     string
		wantCode int
		wantOut  string
	}{
		{
			name: "validation failed",
			uploads: func(f *fakeAPI) []reply {
				return []reply{f.failure(http.StatusUnprocessableEntity, "ValidationFailed", nil)}
			},
			wantCode: _exitRefused, wantOut: "package refused: HTTP 422: validation_failed",
		},
		{
			name:    "rejected whole at parsing",
			uploads: func(f *fakeAPI) []reply { return []reply{f.accepted("quarantined", 1)} },
			receipts: func(f *fakeAPI) []reply {
				return []reply{f.receipt("rejected", func(b map[string]any) { b["reason_code"] = "envelope_invalid" })}
			},
			wantCode: _exitRefused, wantOut: "package refused: envelope_invalid",
		},
		{
			name:     "receipt not ready in time",
			uploads:  func(f *fakeAPI) []reply { return []reply{f.accepted("quarantined", 1)} },
			receipts: func(f *fakeAPI) []reply { return []reply{f.receipt("quarantined", nil)} },
			wait:     "50ms", wantCode: _exitUnknown, wantOut: "intake outcome is unknown",
		},
		{
			name:     "duplicate reads the first receipt",
			uploads:  func(f *fakeAPI) []reply { return []reply{f.accepted("duplicate", nil)} },
			receipts: func(f *fakeAPI) []reply { return []reply{f.receipt("accepted", nil)} },
			wantCode: 0, wantOut: "receipt " + _receiptID + ": accepted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t)
			api.uploads = tt.uploads(api)
			if tt.receipts != nil {
				api.receipts = tt.receipts(api)
			}
			if tt.name == "duplicate reads the first receipt" {
				body := api.uploads[0].body.(map[string]any)
				body["receipt_id"], body["duplicate_of"] = "0193a5b0-5858-7000-8000-0000000000ff", _receiptID
			}
			c := newCLI(t, api)
			c.mustRun(c.goTool(c.copyReport("0")...)...)
			args := []string{"upload"}
			if tt.wait != "" {
				args = append(args, "--wait", tt.wait)
			}
			code, out := c.run(args...)
			assert.Equal(t, tt.wantCode, code, out)
			assert.Contains(t, out, tt.wantOut)
		})
	}
}

func TestUpload_RetriesTheSamePackage(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{
		api.failure(http.StatusServiceUnavailable, "ServiceUnavailable", map[string]string{"Retry-After": "1"}),
		api.failure(http.StatusTooManyRequests, "TooManyRequests", map[string]string{"Retry-After": "7"}),
		api.accepted("quarantined", 1),
	}
	api.receipts = []reply{api.receipt("accepted", nil)}
	c := newCLI(t, api)
	c.mustRun(c.goTool(c.copyReport("0")...)...)

	out := c.mustRun("upload")

	assert.Contains(t, out, "upload: attempt 1 of 5 failed, HTTP 503; retry in 1s")
	assert.Contains(t, out, "upload: attempt 2 of 5 failed, HTTP 429; retry in 7s")
	require.Len(t, api.bodies, 3)
	assert.Equal(t, api.bodies[0], api.bodies[1])
	assert.Equal(t, api.bodies[0], api.bodies[2])
	assert.Equal(t, []time.Duration{time.Second, 7 * time.Second}, c.pauses[:2])
}

func TestUpload_RefusesAfterFiveFailedAttempts(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{api.failure(http.StatusServiceUnavailable, "ServiceUnavailable", map[string]string{"Retry-After": "1"})}
	c := newCLI(t, api)
	c.mustRun(c.goTool(c.copyReport("0")...)...)

	code, out := c.run("upload")

	assert.Equal(t, _exitRefused, code, out)
	assert.Contains(t, out, "upload: attempt 4 of 5 failed")
	assert.NotContains(t, out, "attempt 5 of 5 failed")
	assert.Contains(t, out, "package refused: HTTP 503: service_unavailable")
	assert.Len(t, api.bodies, 5)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, c.pauses)
}

func TestToken_NeverPrinted(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{api.failure(http.StatusServiceUnavailable, "ServiceUnavailable", map[string]string{"Retry-After": "1"}), api.accepted("quarantined", 1)}
	api.receipts = []reply{api.receipt("accepted", nil)}
	c := newCLI(t, api)
	leak := c.path("leak.txt")

	var out strings.Builder
	for _, args := range [][]string{
		{"--debug", "profile"},
		c.goTool(append([]string{"--debug"}, "--", "sh", "-c", `printf %s "${CRAFTPULSE_UPLOAD_TOKEN:-}" > "$1"; cp "$2" "$3"`, "sh", leak, c.path("fixture.sarif"), c.path("report.sarif"))...),
		{"--debug", "upload"},
	} {
		out.WriteString(c.mustRun(args...))
	}
	c.env["CRAFTPULSE_URL"] = "ftp://" + _token + "@example"
	_, failed := c.run("--debug", "profile")
	out.WriteString(failed)

	text := out.String()
	assert.Contains(t, text, "debug: > GET "+api.srv.URL+"/api/v1/ci/check-profile; Authorization: [REDACTED]")
	for i := 0; i+8 <= len(_token); i++ {
		require.NotContains(t, text, _token[i:i+8], "token part at %d", i)
	}
	leaked, err := os.ReadFile(leak)
	require.NoError(t, err)
	assert.Empty(t, leaked, "a tool must not receive the upload token")
}

func TestFixtures_MarksMissingAndExtraFindings(t *testing.T) {
	c := newCLI(t, newFakeAPI(t))
	command := []string{"--", "sh", "-c", `cp findings.sarif "$CRAFTPULSE_REPORT"`}

	out := c.mustRun(append([]string{"fixtures", "../../internal/ci/testdata/fixtures/stub-ok"}, command...)...)
	assert.Contains(t, out, "fixture unused-result: ok, 2 finding(s)")

	code, out := c.run(append([]string{"fixtures", "../../internal/ci/testdata/fixtures/stub-broken"}, command...)...)
	assert.Equal(t, _exitFailed, code, out)
	assert.Contains(t, out, "fixture sql-injection: differs\n  missing G201 db/query.go:12\n  extra G104 main.go:3\n")
}

func ptr[T any](v T) *T { return &v }

func TestSecretsProfile_RedactsLogsStateAndUpload(t *testing.T) {
	api := newFakeAPI(t)
	api.uploads = []reply{api.accepted("quarantined", 1)}
	api.receipts = []reply{api.receipt("accepted", nil)}
	c := newCLI(t, api)
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	marker := base64.RawURLEncoding.EncodeToString(sum[:])[:32]
	repo := filepath.Join(c.dir, "repo")
	require.NoError(t, os.Mkdir(repo, 0o700))
	git := func(args ...string) string {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
		data, err := cmd.CombinedOutput()
		require.NoError(t, err, "fixture Git failed")
		return strings.TrimSpace(string(data))
	}
	git("init", "--quiet")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "config.go"), []byte("package config\nconst defaultPartnerAPIKey = \""+marker+"\"\n"), 0o600))
	git("add", ".")
	git("commit", "--quiet", "-m", "fixture")
	c.env["CI_COMMIT_SHA"], c.env["CI_PROJECT_DIR"], c.env["CRAFTPULSE_SECRETS"] = git("rev-parse", "HEAD"), repo, "1"
	c.env["CI_COMMIT_REF_NAME"] = "test/" + marker
	logs := c.mustRun("secrets")
	// Raw tool stdout and stderr must never bypass the protected app writers.
	command := []string{"--", "sh", "-c", `printf '%s' "$1"; printf '%s' "$1" >&2; cp "$2" "$3"`, "sh", marker, c.path("fixture.sarif"), c.path("report.sarif")}
	logs += c.mustRun(c.goTool(command...)...)
	require.NoError(t, os.WriteFile(c.path("junit.xml"), []byte(`<testsuites><testsuite name="`+marker+`"/></testsuites>`), 0o600))
	logs += c.mustRun("import", "--report", "junit:junit_xml:"+c.path("junit.xml"))
	logs += c.mustRun("upload", "--save", c.path("secrets.tar.gz"))
	code, out := c.run("--unknown=" + marker)
	assert.Equal(t, _exitFailed, code)
	logs += out
	for i := 0; i+8 <= len(marker); i++ {
		assert.NotContains(t, logs, marker[i:i+8], "secret fragment in CLI output")
	}
	require.Len(t, api.bodies, 1)
	checkPackage(t, api.bodies[0])
	scan, err := ci.ScanSecrets(t.Context(), repo, "", c.env["CI_COMMIT_SHA"])
	require.NoError(t, err)
	require.NoError(t, scan.Scrubber.VerifyPackage(api.bodies[0]))
	state := filepath.Join(c.dir, "state")
	require.NoError(t, filepath.WalkDir(filepath.Join(state, "files"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		for i := 0; i+8 <= len(marker); i++ {
			assert.NotContains(t, string(data), marker[i:i+8], "secret fragment in stored report")
		}
		return nil
	}))
	c.env["CRAFTPULSE_SECRETS"] = ""
	code, _ = c.run("upload")
	assert.Equal(t, _exitFailed, code)
	require.Len(t, api.bodies, 1)
}

func TestSecretsProfile_ScanFailureReleasesNoRawOutput(t *testing.T) {
	var out bytes.Buffer
	env := func(key string) string {
		if key == "CRAFTPULSE_SECRETS" {
			return "1"
		}
		if key == "CI_COMMIT_SHA" {
			return _sha
		}
		if key == "CI_PROJECT_DIR" {
			return t.TempDir()
		}
		return ""
	}
	code := execute(t.Context(), []string{"secrets"}, &out, &out, env, ci.ClientConfig{})
	assert.Equal(t, _exitFailed, code)
	assert.Equal(t, "craftpulse-ci: secrets scan failed; no output or package released\n", out.String())
}
