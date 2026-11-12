package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
