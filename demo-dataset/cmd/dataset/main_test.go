package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func TestGenerateAndManifestCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	var output bytes.Buffer
	if err := run([]string{"generate", "--seed", "21", "--out", first}, &output); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"generate", "--seed", "21", "--out", second}, &output); err != nil {
		t.Fatal(err)
	}
	left, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("CLI generation is not byte deterministic")
	}
	parts := filepath.Join(dir, "parts")
	if err := os.Mkdir(parts, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := run([]string{"manifest", "--model", first, "--sources", parts, "--objects", parts, "--out", path}, &output); err != nil {
		t.Fatal(err)
	}
	var manifest dataset.Manifest
	if err := dataset.ReadJSON(path, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Variants) == 0 || len(manifest.Sources) != 0 {
		t.Fatal("scaffold manifest output")
	}
}

func TestCompareCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	left := filepath.Join(dir, "left.json")
	right := filepath.Join(dir, "right.json")
	record := dataset.Expected{ID: "fixture", MetricID: "fixture-count", DefinitionVersion: "1", Scope: map[string]string{"organization_id": "org-1"}, Filters: map[string][]string{}, Period: dataset.Window{From: "2026-01-01T00:00:00Z", To: "2026-02-01T00:00:00Z"}, Step: "month", AsOf: "2026-12-01T00:00:00Z", Unit: "count", N: 1, State: "ASSESSED", Cohort: []string{"task-a"}, Intervals: []dataset.Interval{{ObjectID: "task-a", Kind: "cycle", From: "2026-01-01T00:00:00Z", To: "2026-01-02T00:00:00Z", OriginalOffset: "Z"}}}
	file := dataset.ExpectedFile{SchemaVersion: "1", Records: []dataset.Expected{record}}
	if err := dataset.WriteJSON(left, file); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"compare", left, left}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "differences: 0") {
		t.Fatal(output.String())
	}
	value := json.Number("2")
	file.Records[0].Value = &value
	file.Records[0].Intervals[0].To = "2026-01-03T00:00:00Z"
	file.Records[0].Cohort = append(file.Records[0].Cohort, "task-b")
	if err := dataset.WriteJSON(right, file); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"compare", left, right}, &output); err == nil {
		t.Fatal("comparison did not fail")
	}
	if !strings.Contains(output.String(), "differences: 3") {
		t.Fatal(output.String())
	}
}

func TestGenerateTasksCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	objects := filepath.Join(dir, "objects")
	sources := filepath.Join(dir, "sources")
	for _, d := range []string{objects, sources} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	outputs := func(run string) []string {
		return []string{filepath.Join(dir, run+"-model.json"), filepath.Join(dir, run+"-tasks.json"),
			filepath.Join(objects, run+"-tasks.json"), filepath.Join(dir, run+"-settings.json")}
	}
	var output bytes.Buffer
	for _, name := range []string{"first", "second"} {
		paths := outputs(name)
		args := []string{"generate", "--seed", "21", "--out", paths[0], "--tasks", paths[1], "--task-objects", paths[2], "--settings", paths[3]}
		if err := run(args, &output); err != nil {
			t.Fatal(err)
		}
	}
	for i, first := range outputs("first") {
		left, err := os.ReadFile(first)
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(outputs("second")[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("%s is not byte deterministic", filepath.Base(first))
		}
	}
	var tasks dataset.TaskFile
	if err := dataset.ReadJSON(outputs("first")[1], &tasks); err != nil {
		t.Fatal(err)
	}
	var settings dataset.Settings
	if err := dataset.ReadJSON(outputs("first")[3], &settings); err != nil {
		t.Fatal(err)
	}
	if len(tasks.Tasks) != 101 || len(settings.PreviewTaskIDs) != 10 {
		t.Fatal("generated files do not decode back")
	}
	if err := os.Remove(outputs("second")[2]); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := run([]string{"manifest", "--model", outputs("first")[0], "--sources", sources, "--objects", objects, "--out", path}, &output); err != nil {
		t.Fatal(err)
	}
	var manifest dataset.Manifest
	if err := dataset.ReadJSON(path, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, v := range manifest.Variants {
		if v.VariantID == "blocked" && !slices.Contains(v.ObjectIDs, "org-1/jira-dc-1/PA-3") {
			t.Fatal("the manifest does not take the task variants")
		}
	}
	if !strings.Contains(output.String(), "tasks SHA-256:") {
		t.Fatal(output.String())
	}
}

func TestGenerateReposCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	objects := filepath.Join(dir, "objects")
	sources := filepath.Join(dir, "sources")
	for _, d := range []string{objects, sources} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	modelPath := filepath.Join(dir, "model.json")
	reposPath := filepath.Join(dir, "repos.json")
	var output bytes.Buffer
	args := []string{"generate", "--out", modelPath, "--repos", reposPath, "--repo-objects", filepath.Join(objects, "repos.json"), "--repo-dir", filepath.Join(dir, "out")}
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	var repos dataset.RepoFile
	if err := dataset.ReadJSON(reposPath, &repos); err != nil {
		t.Fatal(err)
	}
	for _, repo := range repos.Repositories {
		if !strings.Contains(output.String(), repo.ID+" head: "+repo.Head) {
			t.Fatalf("output lacks the head of %s", repo.ID)
		}
		if _, err := os.Stat(filepath.Join(dir, "out", repo.BarePath, "HEAD")); err != nil {
			t.Fatal(err)
		}
	}
	for _, patch := range repos.Patches {
		if _, err := os.Stat(filepath.Join(dir, "out", patch.Path)); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"manifest", "--model", modelPath, "--sources", sources, "--objects", objects, "--out", filepath.Join(dir, "manifest.json")}, &output); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &output); err == nil {
		t.Fatal("a second run must not overwrite the repositories")
	}
}
