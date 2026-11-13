package main

import (
	"os"
	"regexp"
	"testing"
)

// The release image builds only the binaries listed in its Dockerfile; the test adapter is not one.
func TestReleaseImageHasNoTestAdapter(t *testing.T) {
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	build := regexp.MustCompile(`(?s)go build .*?-o /out/ ([^\n]+)`).FindSubmatch(data)
	if build == nil {
		t.Fatal("the Dockerfile has no go build of the release binaries")
	}
	if got, want := string(build[1]), "./cmd/api ./cmd/worker"; got != want {
		t.Fatalf("release binaries %q, want %q", got, want)
	}
	if regexp.MustCompile(`adapter-test`).Match(data) {
		t.Fatal("the Dockerfile mentions the test adapter")
	}
}
