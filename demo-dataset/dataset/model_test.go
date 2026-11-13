package dataset_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func TestGenerateDeterminismAndPeople(t *testing.T) {
	t.Parallel()
	first := model(t, 21)
	second := model(t, 21)
	other := model(t, 22)
	hash1, err := dataset.Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := dataset.Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	hash3, err := dataset.Hash(other)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 || hash1 == hash3 {
		t.Fatal("seed does not control deterministic model")
	}
	if first.People[0].DisplayName == other.People[0].DisplayName {
		t.Fatal("seed must affect generated data, not just metadata")
	}
	var team, bots, otherOrg int
	roles := make(map[string]bool)
	identities := make(map[string]string)
	for _, person := range first.People {
		if person.TeamID == "team-1" {
			team++
		}
		if person.Bot {
			bots++
		}
		if person.OrganizationID == "org-2" {
			otherOrg++
		}
		for _, role := range person.Roles {
			roles[role] = true
		}
		for _, account := range person.Accounts {
			key := person.OrganizationID + "/" + account.InstanceID + "/" + account.Login
			if _, exists := identities[key]; exists {
				t.Fatal("duplicate account identity")
			}
			identities[key] = person.ID
			if account.ExpectedPersonID != person.ID {
				t.Fatal("identity mapping mismatch")
			}
		}
	}
	if team != 20 || bots != 2 || otherOrg != 1 {
		t.Fatalf("people counts: %d/%d/%d", team, bots, otherOrg)
	}
	for _, role := range []string{"admin", "mapping_owner", "leader", "pm", "tech_lead", "developer", "restricted_user"} {
		if !roles[role] {
			t.Fatalf("missing role %s", role)
		}
	}
	if len(first.People[3].DeniedAreas) != 3 {
		t.Fatal("restricted actor needs repo and wiki denials")
	}
}

func TestVolumesAndCatalog(t *testing.T) {
	t.Parallel()
	generated := model(t, 21)
	totals := make(map[string]int)
	for _, volume := range generated.Volumes {
		totals[volume.OrganizationID+"/"+volume.Series+"/"+volume.Kind] += volume.Count
	}
	want := map[string]int{"org-1/response/task": 100, "org-1/response/task_event": 1000, "org-1/response/merge_request": 10, "org-1/response/ci_run": 20, "org-1/response/git_document": 50, "org-1/response/wiki_document": 10, "org-2/isolation/task": 1, "org-2/isolation/task_event": 10, "org-2/isolation/merge_request": 1, "org-2/isolation/ci_run": 1, "org-2/isolation/git_document": 1, "org-2/isolation/wiki_document": 1}
	for key, count := range want {
		if totals[key] != count {
			t.Fatalf("%s: %d != %d", key, totals[key], count)
		}
	}
	ids := make([]string, 0, len(generated.Variants))
	for _, variant := range generated.Variants {
		if slices.Contains(ids, variant.ID) || len(variant.Sources) == 0 || variant.Representation == "" || variant.ExpectedBehavior == "" {
			t.Fatal("invalid catalog entry")
		}
		ids = append(ids, variant.ID)
	}
	for _, id := range []string{"created-active", "intake-active-category", "blocked", "cancelled", "duplicate", "done-reopen-done", "size-change", "team-change", "old-open", "incomplete-history-page", "same-key-instances", "bot-response", "ambiguous-link", "head-without-scan", "report-other-sha", "multi-period-history", "types-sizes-targets", "complete-reopen-followup", "incomplete-reopen-followup", "late-event", "replay"} {
		if !slices.Contains(ids, id) {
			t.Fatalf("missing variant %s", id)
		}
	}
	if _, err := dataset.Generate(21, "2026-01-01T00:00:00Z"); err == nil {
		t.Fatal("accepted empty period")
	}
}

func model(t *testing.T, seed uint64) *dataset.Model {
	t.Helper()
	result, err := dataset.Generate(seed, "2026-12-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func clone[T any](t *testing.T, original T) T {
	t.Helper()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := dataset.Decode(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
