package ci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTrivyProfile_HashesProfileConfigurationAndIgnoredChecks(t *testing.T) {
	p, err := LoadTrivyProfile("profiles/trivy-default")

	require.NoError(t, err)
	assert.Equal(t, "trivy-default", p.ID)
	assert.Equal(t, map[string]string{ToolTrivy: "0.76.2"}, p.Tools)
	assert.Equal(t, []string{"dockerfile", "kubernetes"}, p.IaCTypes)
	var content []byte
	for _, name := range []string{CodeProfileFile, TrivyConfigFile, TrivyIgnoreFile} {
		data, err := os.ReadFile(filepath.Join("profiles", "trivy-default", name))
		require.NoError(t, err)
		content = append(content, data...)
	}
	sum := sha256.Sum256(content)
	assert.Equal(t, hex.EncodeToString(sum[:]), p.RulesHash())

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		CodeProfileFile: `{"profile_id":"trivy-default","timeout":"0s","tools":{},"iac_types":[],"excluded_dirs":{"a/b":"vendor"}}`,
		TrivyConfigFile: "{}\n", TrivyIgnoreFile: "",
	})
	_, err = LoadTrivyProfile(dir)
	require.ErrorContains(t, err, "tools.trivy: version is required")
	require.ErrorContains(t, err, "iac_types: at least one configuration kind is required")
	require.ErrorContains(t, err, "timeout must be a positive duration")
	require.ErrorContains(t, err, "excluded_dirs.a/b")
}

func TestCleanTrivy_PrefixesTargetsDropsConfigurationLinesAndNamesResources(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"svc/deploy/app.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n---\napiVersion: v1\nkind: Pod\n" +
			"metadata:\n  labels:\n    app: web\n  name: \"web\"\nspec:\n  containers:\n    - name: web\n",
		"svc/Dockerfile": "FROM golang:1.27 AS build\nRUN go build\nFROM --platform=$BUILDPLATFORM alpine:3.23\nUSER 0\n",
		"raw.json": `{"SchemaVersion":2,"Results":[
			{"Target":"go.mod","Class":"lang-pkgs","Type":"gomod","Vulnerabilities":[{"VulnerabilityID":"CVE-1","PkgName":"x","InstalledVersion":"v1"}]},
			{"Target":"deploy/app.yaml","Class":"config","Type":"kubernetes","Misconfigurations":[
				{"ID":"KSV-0001","CauseMetadata":{"StartLine":14,"Code":{"Lines":[{"Content":"FIXTURE_CONFIG_LINE"}]}}}]},
			{"Target":"Dockerfile","Class":"config","Type":"dockerfile","Misconfigurations":[
				{"ID":"DS-0002","CauseMetadata":{}},{"ID":"DS-0029","CauseMetadata":{"StartLine":2}},{"ID":"DS-0001","CauseMetadata":{"StartLine":4}}]},
			{"Target":"main.tf","Class":"config","Type":"terraform","Misconfigurations":[
				{"ID":"AWS-0086","CauseMetadata":{"Resource":"aws_s3_bucket.logs","StartLine":1}}]},
			{"Target":"clean.yaml","Class":"config","Type":"kubernetes","MisconfSummary":{"Successes":3,"Failures":0}}]}`,
	})

	report, targets, findings, err := cleanTrivy(filepath.Join(root, "raw.json"), root, "svc")

	require.NoError(t, err)
	assert.True(t, findings)
	assert.Equal(t, 5, targets)
	data, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "FIXTURE_CONFIG_LINE")
	assert.Equal(t, []string{
		"vulnerability CVE-1 x@v1 svc/go.mod",
		"misconfiguration KSV-0001 svc/deploy/app.yaml:14 Pod/web",
		"misconfiguration DS-0002 svc/Dockerfile:1 file",
		"misconfiguration DS-0029 svc/Dockerfile:2 stage build",
		"misconfiguration DS-0001 svc/Dockerfile:4 stage alpine:3.23",
		"misconfiguration AWS-0086 svc/main.tf:1 aws_s3_bucket.logs",
	}, trivyFacts(report))

	writeFiles(t, root, map[string]string{"raw.json": "not json"})
	_, _, _, err = cleanTrivy(filepath.Join(root, "raw.json"), root, "")
	require.ErrorContains(t, err, "not Trivy JSON")
}

func TestMarkDelta_AddedAndChangedPackagesOfHead(t *testing.T) {
	head := func() map[string]any {
		var report map[string]any
		require.NoError(t, json.Unmarshal([]byte(`{"Results":[{"Target":"go.mod","Class":"lang-pkgs","Packages":[
			{"Name":"example.com/app","Relationship":"root"},{"Name":"a","Version":"v1"},{"Name":"b","Version":"v2"},{"Name":"c","Version":"v1"}],
			"Vulnerabilities":[{"PkgName":"b","InstalledVersion":"v2"},{"PkgName":"c","InstalledVersion":"v1"}]},
			{"Target":"web/package-lock.json","Class":"lang-pkgs","Packages":[{"Name":"m","Version":"1.2.5"}]}]}`), &report))
		return report
	}
	base := map[string]map[string][]string{"go.mod": {"a": {"v1"}, "b": {"v1"}}}

	report := head()
	markDelta(report, "base-sha", base, true)

	assert.Equal(t, map[string]any{"BaseSHA": "base-sha", "Delta": "computed"}, report["CraftPulse"])
	marks := map[string]any{}
	for _, res := range trivyResults(report) {
		for _, item := range res["Packages"].([]any) {
			pkg := item.(map[string]any)
			marks[pkg["Name"].(string)] = pkg["CraftPulseDelta"]
		}
		vulns, _ := res["Vulnerabilities"].([]any)
		for _, item := range vulns {
			v := item.(map[string]any)
			marks["vuln "+v["PkgName"].(string)] = v["CraftPulseDelta"]
		}
	}
	assert.Equal(t, map[string]any{
		"example.com/app": nil, "a": nil, "b": DeltaChanged, "c": DeltaAdded, "m": DeltaAdded, "vuln b": DeltaChanged, "vuln c": DeltaAdded,
	}, marks)

	report = head()
	markDelta(report, "", nil, true)
	assert.Equal(t, map[string]any{"BaseSHA": nil, "Delta": "no_base"}, report["CraftPulse"])
	report = head()
	markDelta(report, "base-sha", nil, false)
	assert.Equal(t, map[string]any{"BaseSHA": "base-sha", "Delta": "unavailable"}, report["CraftPulse"])
	assert.NotContains(t, mustJSON(t, report), "CraftPulseDelta")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// writeTrivyDB writes a database snapshot as UpdateTrivyDB leaves it.
func writeTrivyDB(t *testing.T, cacheDir, content string, updated time.Time) {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	writeFiles(t, cacheDir, map[string]string{
		"db/trivy.db":      content,
		"db/metadata.json": `{"Version":2,"UpdatedAt":"` + updated.Format(time.RFC3339Nano) + `"}`,
		"db/craftpulse.json": `{"sha256":"` + hex.EncodeToString(sum[:]) + `","repository":"mirror.example/trivy-db:2","updated_at":"` +
			updated.Format(time.RFC3339Nano) + `"}`,
	})
}

func TestReadTrivyDB_VersionIsContentHashAndRecordMustMatch(t *testing.T) {
	cache := t.TempDir()
	published := time.Date(2026, 11, 20, 13, 11, 33, 75919767, time.UTC)
	writeTrivyDB(t, cache, "database", published)

	db, err := ReadTrivyDB(cache)

	require.NoError(t, err)
	sum := sha256.Sum256([]byte("database"))
	assert.Equal(t, "2@sha256:"+hex.EncodeToString(sum[:]), db.Version)
	assert.True(t, published.Equal(db.PublishedAt))
	assert.Equal(t, "mirror.example/trivy-db:2", db.Repository)

	writeFiles(t, cache, map[string]string{"db/metadata.json": `{"Version":2,"UpdatedAt":"2026-11-21T00:00:00Z"}`})
	_, err = ReadTrivyDB(cache)
	require.ErrorIs(t, err, ErrTrivyDBMissing)
	_, err = ReadTrivyDB(t.TempDir())
	require.ErrorIs(t, err, ErrTrivyDBMissing)
}

// _fakeTrivyDB downloads a database into --cache-dir, or fails when FAKE_FAIL is set.
const _fakeTrivyDB = `while [ $# -gt 0 ]; do [ "$1" = --cache-dir ] && dir=$2; shift; done
[ -n "$FAKE_FAIL" ] && exit 1
mkdir -p "$dir/db"
printf 'fresh database' > "$dir/db/trivy.db"
printf '{"Version":2,"UpdatedAt":"2026-11-21T07:00:00Z"}' > "$dir/db/metadata.json"`

func TestUpdateTrivyDB_ReplacesOnlyAfterACheckedDownload(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, ToolTrivy), []byte("#!/bin/sh\n"+_fakeTrivyDB+"\n"), 0o700)) //nolint:gosec // a test tool must be executable
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cache := t.TempDir()
	old := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	writeTrivyDB(t, cache, "old database", old)
	r := &Runner{Stdout: os.Stdout, Stderr: os.Stderr, Now: time.Now}

	t.Setenv("FAKE_FAIL", "1")
	_, err := r.UpdateTrivyDB(t.Context(), cache, "mirror.example/trivy-db:2")
	require.ErrorContains(t, err, "download of the vulnerability database from mirror.example/trivy-db:2 failed")
	kept, err := ReadTrivyDB(cache)
	require.NoError(t, err)
	assert.True(t, old.Equal(kept.PublishedAt))

	t.Setenv("FAKE_FAIL", "")
	db, err := r.UpdateTrivyDB(t.Context(), cache, "mirror.example/trivy-db:2")

	require.NoError(t, err)
	sum := sha256.Sum256([]byte("fresh database"))
	assert.Equal(t, "2@sha256:"+hex.EncodeToString(sum[:]), db.Version)
	assert.Equal(t, time.Date(2026, 11, 21, 7, 0, 0, 0, time.UTC), db.PublishedAt)
	entries, err := os.ReadDir(cache)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only db/ stays in the cache directory")
	_, err = r.UpdateTrivyDB(t.Context(), cache, "")
	require.ErrorContains(t, err, "repository")
}

func TestConfigResource_KubernetesObjectAndDockerfileStage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.yaml":     "# objects\nkind: Service\nmetadata:\n  name: api # public\n---\nkind: Deployment\nmetadata:\n  namespace: x\n  name: 'api'\nspec: {}\n",
		"Dockerfile": "# syntax=docker/dockerfile:1\nFROM node:26 AS deps\nRUN npm ci\nfrom alpine\nCOPY . .\n",
	})
	tests := []struct {
		name, kind string
		line       int
		want       string
	}{
		{"a.yaml", "kubernetes", 2, "Service/api"},
		{"a.yaml", "kubernetes", 10, "Deployment/api"},
		{"Dockerfile", "dockerfile", 3, "stage deps"},
		{"Dockerfile", "dockerfile", 5, "stage alpine"},
		{"Dockerfile", "dockerfile", 1, "file"},
		{"Dockerfile", "dockerfile", 0, "file"},
		{"missing", "kubernetes", 3, "file"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, configResource(filepath.Join(dir, tt.name), tt.kind, tt.line))
		})
	}
}
