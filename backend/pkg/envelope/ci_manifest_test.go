package envelope_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func ciData(t *testing.T, name, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ci", name, file))
	require.NoError(t, err)
	return data
}

// addFixtureScan upgrades legacy test packages; negative CI-manifest tests use archive directly.
func addFixtureScan(t *testing.T, data []byte) []byte {
	t.Helper()
	entries := unpackCI(t, data)
	var scan envelope.ScanManifestV1
	require.NoError(t, json.Unmarshal(ciData(t, "gitlab-builtin", envelope.ScanManifestName), &scan))
	scan.Tools = nil
	paths := map[string]bool{}
	files := map[string]bool{}
	for _, e := range entries {
		files[e.name] = true
	}
	for _, e := range entries {
		if e.name != envelope.RecordsName {
			continue
		}
		for _, line := range bytes.Split([]byte(e.content), []byte("\n")) {
			var rec envelope.Record
			if json.Unmarshal(line, &rec) != nil || rec.RecordKind != envelope.RecordRecordKindReportFile || rec.File == nil || rec.NativeId == "" || paths[rec.File.Path] || !files[rec.File.Path] {
				continue
			}
			var toolScan envelope.ScanManifestV1
			require.NoError(t, json.Unmarshal(ciData(t, "gitlab-builtin", envelope.ScanManifestName), &toolScan))
			var payload envelope.ReportFileV1
			require.NoError(t, json.Unmarshal(rec.Payload, &payload))
			tool := toolScan.Tools[0]
			tool.Id = rec.File.Path
			tool.Sha = payload.Sha
			tool.ArtifactDigest = payload.ArtifactDigest
			tool.Producer = payload.Producer
			tool.ProducerVersion = payload.ProducerVersion
			tool.ReportKind = envelope.ScanToolRunReportKind(payload.ReportKind)
			tool.Mode = envelope.ScanToolRunModeImport
			tool.Ci = nil
			if payload.RunNativeId != nil || payload.AttemptNativeId != nil {
				tool.Ci = toolScan.Tools[0].Ci
				if payload.RunNativeId != nil {
					tool.Ci.RunNativeId = *payload.RunNativeId
				}
				tool.Ci.AttemptNativeId = payload.AttemptNativeId
			}
			tool.ProfileId = payload.ProfileId
			tool.RulesHash = payload.RulesHash
			tool.ConfigHash = payload.ConfigHash
			tool.DbVersion = payload.DbVersion
			if payload.StartedAt != nil {
				tool.StartedAt = *payload.StartedAt
			}
			if payload.FinishedAt != nil {
				tool.FinishedAt = *payload.FinishedAt
			}
			if payload.ExitClass != nil {
				tool.ExitClass = envelope.ScanToolRunExitClass(*payload.ExitClass)
			}
			if payload.Completeness != nil {
				tool.Completeness = envelope.ScanToolRunCompleteness(*payload.Completeness)
			}
			tool.ProfileImageDigest = nil
			tool.Component = nil
			tool.Reports[0].File = *rec.File
			tool.Reports[0].ReportKind = envelope.ScanReportReportKind(payload.ReportKind)
			tool.Reports[0].ReportFormat = envelope.ScanReportReportFormat(payload.ReportFormat)
			scan.Tools = append(scan.Tools, tool)
			paths[rec.File.Path] = true
		}
	}
	if len(scan.Tools) == 0 {
		return data
	}
	entries = append(entries, file(envelope.ScanManifestName, mustJSON(t, scan)))
	return archive(t, entries...)
}

func unpackCI(t *testing.T, data []byte) []entry {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := []entry{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		raw, err := io.ReadAll(tr)
		require.NoError(t, err)
		out = append(out, file(h.Name, string(raw)))
	}
	return out
}

func ciArchive(t *testing.T, name string, mutate func(map[string][]byte)) []byte {
	t.Helper()
	dir := filepath.Join("testdata", "ci", name)
	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	}))
	if mutate != nil {
		mutate(files)
	}
	entries := []entry{file(envelope.ManifestName, string(files[envelope.ManifestName]))}
	delete(files, envelope.ManifestName)
	// Manifests deliberately follow records and report files, proving archive order is immaterial.
	for _, name := range []string{envelope.RecordsName} {
		if raw, ok := files[name]; ok {
			entries = append(entries, file(name, string(raw)))
			delete(files, name)
		}
	}
	for name, data := range files {
		if name != envelope.ScanManifestName && name != envelope.ExecutionManifestName {
			entries = append(entries, file(name, string(data)))
		}
	}
	for _, name := range []string{envelope.ExecutionManifestName, envelope.ScanManifestName} {
		if raw, ok := files[name]; ok {
			entries = append(entries, file(name, string(raw)))
		}
	}
	return archive(t, entries...)
}

func TestCIManifest_ExamplesAndCleanRoundTrip(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gitlab-builtin", "external-sarif", "timeout", "jenkins-retry"} {
		t.Run(name, func(t *testing.T) {
			data := ciArchive(t, name, nil)
			result := check(data)
			require.Nil(t, result.Envelope)
			require.Zero(t, result.Rejected())
			require.NotNil(t, result.ScanManifest)
			res, cleaned := clean(t, data)
			require.Nil(t, res.Envelope)
			require.NotEmpty(t, cleaned)
			again := check(cleaned)
			require.Nil(t, again.Envelope)
			require.Equal(t, result.ScanManifest, again.ScanManifest)
			p, err := envelope.ReadCleaned(bytes.NewReader(cleaned), envelope.DefaultLimits)
			require.NoError(t, err)
			require.Equal(t, result.ScanManifest, p.ScanManifest)
			twice, cleanedAgain := clean(t, cleaned)
			require.Nil(t, twice.Envelope)
			assert.Equal(t, cleaned, cleanedAgain)
			assert.Equal(t, res.ContentHash, twice.ContentHash)
			if name == "timeout" {
				assert.Equal(t, envelope.ScanToolRunExitClassTimeout, result.ScanManifest.Tools[0].ExitClass)
				assert.Equal(t, envelope.ScanToolRunCompletenessPartial, result.ScanManifest.Tools[0].Completeness)
			}
			if name == "jenkins-retry" {
				require.NotNil(t, result.ExecutionManifest)
				assert.Equal(t, result.ExecutionManifest, p.ExecutionManifest)
				steps := result.ExecutionManifest.Steps
				require.Len(t, steps, 3)
				assert.Equal(t, 1, steps[0].Attempt)
				assert.Equal(t, 2, steps[1].Attempt)
				assert.NotEqual(t, steps[0].ConfigurationHash, steps[2].ConfigurationHash)
			} else {
				assert.Nil(t, p.ExecutionManifest)
			}
		})
	}
}

func mutateJSON(t *testing.T, files map[string][]byte, name string, change func(map[string]any)) {
	t.Helper()
	var value map[string]any
	require.NoError(t, json.Unmarshal(files[name], &value))
	change(value)
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	files[name] = raw
}

func firstTool(value map[string]any) map[string]any {
	return value["tools"].([]any)[0].(map[string]any)
}

func TestCIManifest_RejectsWholePackageAndWritesNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, fixture, code string
		mutate              func(map[string][]byte)
	}{
		{"missing scan", "gitlab-builtin", envelope.ReasonMissingScanManifest, func(f map[string][]byte) { delete(f, envelope.ScanManifestName) }},
		{"unreferenced report without scan", "gitlab-builtin", envelope.ReasonMissingScanManifest, func(f map[string][]byte) {
			delete(f, envelope.ScanManifestName)
			f[envelope.RecordsName] = []byte("\n")
		}},
		{"unknown scan version", "gitlab-builtin", envelope.ReasonUnsupportedManifestVersion, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { v["schema_version"] = "99" })
		}},
		{"missing producer version", "gitlab-builtin", envelope.ReasonInvalidScanManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { delete(firstTool(v), "producer_version") })
		}},
		{"timeout complete", "timeout", envelope.ReasonInvalidScanManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { firstTool(v)["completeness"] = "complete" })
		}},
		{"secret value field", "gitlab-builtin", envelope.ReasonInvalidScanManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { firstTool(v)["secret_value"] = "SENSITIVE-TEST-VALUE" })
		}},
		{"wrong report hash", "gitlab-builtin", envelope.ReasonInvalidScanManifest, func(f map[string][]byte) { f["files/reports/junit.xml"] = []byte("changed") }},
		{"unknown execution version", "jenkins-retry", envelope.ReasonUnsupportedManifestVersion, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) { v["schema_version"] = "99" })
		}},
		{"missing internal attempt", "jenkins-retry", envelope.ReasonInvalidExecutionManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) { delete(v["steps"].([]any)[0].(map[string]any), "attempt") })
		}},
		{"wrong axes fingerprint", "jenkins-retry", envelope.ReasonInvalidExecutionManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) {
				v["steps"].([]any)[0].(map[string]any)["configuration_hash"] = strings.Repeat("a", 64)
			})
		}},
		{"duplicate logical attempt", "jenkins-retry", envelope.ReasonInvalidExecutionManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) { steps := v["steps"].([]any); steps[1].(map[string]any)["attempt"] = 1 })
		}},
		{"credentials in Jenkins URL", "jenkins-retry", envelope.ReasonInvalidExecutionManifest, func(f map[string][]byte) {
			mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) { v["build_url"] = "https://user:password@jenkins.demo.invalid/job/demo/7/" })
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := ciArchive(t, tt.fixture, tt.mutate)
			result := check(data)
			require.NotNil(t, result.Envelope)
			assert.Equal(t, tt.code, result.Envelope.Code, result.Envelope.Detail)
			assert.Empty(t, result.Records)
			assert.NotContains(t, result.Envelope.Error(), "SENSITIVE-TEST-VALUE")
			res, cleaned := clean(t, data)
			require.NotNil(t, res.Envelope)
			assert.Equal(t, tt.code, res.Envelope.Code)
			assert.Empty(t, cleaned)
		})
	}
}

func TestExecutionManifest_DemoFormatPassesUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("executes the independent demo generator")
	}
	root := filepath.Join(t.TempDir(), "repos")
	modelPath := filepath.Join(t.TempDir(), "changes.json")
	cmd := exec.CommandContext(t.Context(), "go", "run", "./cmd/dataset", "generate", "--out", filepath.Join(t.TempDir(), "model.json"), "--changes", modelPath, "--repo-dir", root)
	cmd.Dir = "../../../demo-dataset"
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	var model struct {
		Runs []struct {
			Provider          string `json:"provider"`
			SHA               string `json:"sha"`
			ExecutionManifest bool   `json:"execution_manifest"`
			Branch            string `json:"branch"`
		} `json:"runs"`
	}
	raw, err := os.ReadFile(modelPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &model))
	// Generate the actual manifest with the demo command from its generated Git commit.
	for _, run := range model.Runs {
		if run.Provider != "jenkins" || !run.ExecutionManifest {
			continue
		}
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".demo-ci"), 0o700))
		for _, name := range []string{"main.go", "run.json"} {
			cmd := exec.CommandContext(t.Context(), "git", "--git-dir", filepath.Join(root, "repos", "org-1", "gitlab-1", "repo-1.git"), "show", run.SHA+":.demo-ci/"+name)
			content, err := cmd.Output()
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".demo-ci", name), content, 0o600))
		}
		binary := filepath.Join(t.TempDir(), "fixture")
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, filepath.Join(dir, ".demo-ci", "main.go"))
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		cmd = exec.CommandContext(t.Context(), binary, "-job", "test", "-attempt", "1")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "BUILD_NUMBER=11", "BUILD_URL=https://jenkins.demo.invalid/job/demo/11/", "JOB_NAME=demo", "NODE_NAME=demo-go", "GIT_COMMIT="+run.SHA, "CI_COMMIT_SHA="+run.SHA)
		out, err = cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		data, err := os.ReadFile(filepath.Join(dir, envelope.ExecutionManifestName))
		require.NoError(t, err)
		_, rejection := envelope.ValidateExecutionManifest(data)
		require.Nil(t, rejection)
		return
	}
	t.Fatal("demo execution fixture missing")
}

func TestScanManifest_RequiredProvenanceAndExitClasses(t *testing.T) {
	t.Parallel()
	fields := []string{"id", "producer", "producer_version", "profile_image_digest", "profile_id", "mode", "report_kind", "component", "language", "rules_hash", "config_hash", "db_version", "db_published_at", "parser_version", "scope_hash", "sha", "artifact_digest", "ci", "started_at", "finished_at", "exit_class", "completeness", "scanned_count", "skipped_count", "skipped_targets", "history", "reports"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			data := ciArchive(t, "gitlab-builtin", func(f map[string][]byte) {
				mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { delete(firstTool(v), field) })
			})
			res := check(data)
			require.NotNil(t, res.Envelope)
			assert.Equal(t, envelope.ReasonInvalidScanManifest, res.Envelope.Code)
		})
	}
	for _, class := range []string{"failed", "timeout", "unknown"} {
		t.Run(class, func(t *testing.T) {
			data := ciArchive(t, "gitlab-builtin", func(f map[string][]byte) {
				mutateJSON(t, f, envelope.ScanManifestName, func(v map[string]any) { firstTool(v)["exit_class"] = class })
			})
			res := check(data)
			require.NotNil(t, res.Envelope)
			assert.Equal(t, envelope.ReasonInvalidScanManifest, res.Envelope.Code)
		})
	}
}

func TestCIManifest_IdentityAndProvenance(t *testing.T) {
	t.Parallel()
	config := strings.Repeat("a", 64)
	first := envelope.JenkinsAttemptKey("folder/demo", 7, "test", "unit", config, 1)
	assert.Len(t, first, 64)
	for _, other := range []string{
		envelope.JenkinsAttemptKey("other/demo", 7, "test", "unit", config, 1),
		envelope.JenkinsAttemptKey("folder/demo", 8, "test", "unit", config, 1),
		envelope.JenkinsAttemptKey("folder/demo", 7, "test", "unit", config, 2),
		envelope.JenkinsAttemptKey("folder/demo", 7, "test", "unit", strings.Repeat("b", 64), 1),
	} {
		assert.NotEqual(t, first, other)
	}
	data := ciArchive(t, "gitlab-builtin", func(files map[string][]byte) {
		var rec map[string]any
		require.NoError(t, json.Unmarshal(bytes.TrimSpace(files[envelope.RecordsName]), &rec))
		rec["payload"].(map[string]any)["producer_version"] = "other-version"
		files[envelope.RecordsName] = []byte(mustJSON(t, rec) + "\n")
	})
	res := check(data)
	require.NotNil(t, res.Envelope)
	assert.Equal(t, envelope.ReasonInvalidScanManifest, res.Envelope.Code)
}

// The same physical report must not carry contradictory provenance in two package entries.
func TestCIManifest_RejectsContradictoryReportProvenance(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"run_native_id", "attempt_native_id", "profile_id", "config_hash", "rules_hash", "db_version", "started_at", "finished_at", "exit_class", "completeness"} {
		t.Run(field, func(t *testing.T) {
			data := ciArchive(t, "timeout", func(f map[string][]byte) {
				mutateJSON(t, f, envelope.RecordsName, func(v map[string]any) {
					p := v["payload"].(map[string]any)
					switch field {
					case "config_hash", "rules_hash":
						p[field] = strings.Repeat("a", 64)
					case "started_at":
						p[field] = "2026-11-17T09:59:00Z"
					case "finished_at":
						p[field] = "2026-11-17T10:00:11Z"
					case "exit_class":
						p[field] = "success"
					case "completeness":
						p[field] = "complete"
					default:
						p[field] = "other"
					}
				})
			})
			res := check(data)
			require.NotNil(t, res.Envelope)
			assert.Equal(t, envelope.ReasonInvalidScanManifest, res.Envelope.Code)
			assert.Empty(t, res.Records)
			_, cleaned := clean(t, data)
			assert.Empty(t, cleaned)
		})
	}
}

func TestCIManifest_RejectsExecutionReportFromAnotherAttempt(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"job", "build_number", "attempt", "matrix", "format"} {
		t.Run(field, func(t *testing.T) {
			data := ciArchive(t, "jenkins-retry", func(f map[string][]byte) {
				mutateJSON(t, f, envelope.ExecutionManifestName, func(v map[string]any) {
					step := v["steps"].([]any)[0].(map[string]any)
					switch field {
					case "job":
						v["job"] = "other"
					case "build_number":
						v["build_number"] = 8
					case "attempt":
						step["attempt"] = 3
					case "matrix":
						step["axes"] = map[string]any{"mode": "other"}
						step["configuration_hash"] = "df1ef347d18e01e778a91e0a17c26a0c83402e95125fcece95c2b7a827d8b993"
					case "format":
						step["reports"].([]any)[0].(map[string]any)["format"] = "cobertura"
					}
				})
			})
			res := check(data)
			require.NotNil(t, res.Envelope)
			assert.Equal(t, envelope.ReasonInvalidExecutionManifest, res.Envelope.Code)
			assert.Empty(t, res.Records)
			_, cleaned := clean(t, data)
			assert.Empty(t, cleaned)
		})
	}
}

func TestCIManifest_OptionalReportProvenanceAndEquivalentTimes(t *testing.T) {
	t.Parallel()
	for _, omit := range []bool{false, true} {
		data := ciArchive(t, "gitlab-builtin", func(f map[string][]byte) {
			mutateJSON(t, f, envelope.RecordsName, func(v map[string]any) {
				p := v["payload"].(map[string]any)
				if omit {
					for _, field := range []string{"run_native_id", "attempt_native_id", "profile_id", "config_hash", "rules_hash", "db_version", "started_at", "finished_at", "exit_class", "completeness"} {
						delete(p, field)
					}
				} else {
					p["started_at"] = "2026-11-17T13:00:00+03:00"
					p["finished_at"] = "2026-11-17T13:00:10+03:00"
				}
			})
		})
		res := check(data)
		require.Nil(t, res.Envelope)
		require.Zero(t, res.Rejected())
	}
}
