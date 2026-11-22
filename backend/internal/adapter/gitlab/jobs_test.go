package gitlab

import "testing"

func TestJobConfiguration(t *testing.T) {
	linux := jobConfig("verify: [linux, amd64]")
	if linux.name != "verify" || linux == jobConfig("verify: [windows, amd64]") {
		t.Fatalf("matrix combinations must share the job key and keep distinct fingerprints: %+v", linux)
	}
	first := jobConfig("verify 1/2")
	if first.name != "verify" || first != jobConfig("verify 1/4") || first == jobConfig("verify 2/2") {
		t.Fatalf("numeric parallel jobs must use the index independently of total size: %+v", first)
	}
	if plain := jobConfig("verify release"); plain.name != "verify release" || plain.fingerprint == first.fingerprint {
		t.Fatalf("plain job names must be preserved: %+v", plain)
	}
}
