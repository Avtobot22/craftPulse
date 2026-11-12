package dataset_test

import (
	"strings"
	"testing"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func source() dataset.SourceManifest {
	return dataset.SourceManifest{SchemaVersion: "1", OrganizationID: "org-1", InstanceID: "jira-dc-1", SourceURL: "https://jira.example.test", Edition: "Data Center", Version: "fixture-1", Queries: []string{"project = DEMO"}, Scopes: []string{"read:DEMO"}, Backfill: dataset.Window{From: "2026-01-01T00:00:00Z", To: "2026-12-01T00:00:00Z"}, AsOf: "2026-12-01T00:00:00Z", Completeness: "complete", Pages: []dataset.Page{{Query: "project = DEMO", Count: 1, Complete: true}}, Objects: []dataset.Binding{{DatasetID: "org-1/task-a", Identity: dataset.Identity{OrganizationID: "org-1", InstanceID: "jira-dc-1", ObjectKind: "task", NativeNS: "", NativeID: "42"}, SHA: "", EventTime: dataset.SourceTime{UTC: "2026-01-01T00:00:00Z", OriginalOffset: "+07:00"}, ObservedTime: dataset.SourceTime{UTC: "2026-11-12T00:00:00Z", OriginalOffset: "Z"}}}, Versions: map[string]string{"mapping": "1", "calculation": "1", "parser": "1", "tool": "fixture-1"}, Hashes: map[string]string{"rules": strings.Repeat("a", 64), "config": strings.Repeat("b", 64), "db": strings.Repeat("c", 64)}, CleanedFiles: map[string]string{}, ExpectedFiles: []string{"expected.json"}}
}

func objects() dataset.ObjectFile {
	return dataset.ObjectFile{SchemaVersion: "1", Objects: []dataset.ModelObject{{ID: "org-1/task-a", OrganizationID: "org-1", Kind: "task"}}, Variants: []dataset.VariantObjects{{VariantID: "created-active", ObjectIDs: []string{"org-1/task-a"}}, {VariantID: "multi-period-history", ObjectIDs: []string{"org-1/task-a"}}}}
}

func TestManifestJoinsModelFilesAndSourceIdentity(t *testing.T) {
	t.Parallel()
	generated := model(t, 21)
	first := source()
	second := clone(t, first)
	second.InstanceID = "jira-dc-2"
	second.Objects[0].Identity.InstanceID = "jira-dc-2"
	manifest, err := dataset.Assemble(generated, []dataset.SourceManifest{second, first}, []dataset.ObjectFile{objects()})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Sources) != 2 || manifest.Sources[0].InstanceID != "jira-dc-1" {
		t.Fatal("unstable source ordering")
	}
	for _, binding := range manifest.Variants {
		if binding.VariantID == "created-active" && (len(binding.ObjectIDs) != 1 || binding.ObjectIDs[0] != "org-1/task-a") {
			t.Fatal("missing variant object join")
		}
	}
	reordered, err := dataset.Assemble(generated, []dataset.SourceManifest{first, second}, []dataset.ObjectFile{objects()})
	if err != nil {
		t.Fatal(err)
	}
	hash1, err := dataset.Hash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := dataset.Hash(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Fatal("source file order affects manifest")
	}
}

func TestManifestRejectsInconsistentInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*dataset.SourceManifest)
	}{
		{name: "future observation", mutate: func(s *dataset.SourceManifest) { s.Objects[0].ObservedTime.UTC = s.AsOf }},
		{name: "wrong organization", mutate: func(s *dataset.SourceManifest) { s.Objects[0].Identity.OrganizationID = "org-2" }},
		{name: "duplicate identity", mutate: func(s *dataset.SourceManifest) { s.Objects = append(s.Objects, s.Objects[0]) }},
		{name: "false completeness", mutate: func(s *dataset.SourceManifest) { s.Pages[0].Complete = false }},
		{name: "page size", mutate: func(s *dataset.SourceManifest) { s.Pages[0].Count = 11 }},
		{name: "missing hash", mutate: func(s *dataset.SourceManifest) { delete(s.Hashes, "db") }},
		{name: "credentials URL", mutate: func(s *dataset.SourceManifest) { s.SourceURL = "https://user:fixture@jira.example.test" }},
		{name: "unknown object", mutate: func(s *dataset.SourceManifest) { s.Objects[0].DatasetID = "unknown" }},
		{name: "changed period", mutate: func(s *dataset.SourceManifest) { s.Backfill.From = "2026-02-01T00:00:00Z" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			part := source()
			test.mutate(&part)
			if _, err := dataset.Assemble(model(t, 21), []dataset.SourceManifest{part}, []dataset.ObjectFile{objects()}); err == nil {
				t.Fatal("accepted inconsistent manifest")
			}
		})
	}
	badObjects := objects()
	badObjects.Variants[0].ObjectIDs = []string{"missing"}
	if _, err := dataset.Assemble(model(t, 21), []dataset.SourceManifest{source()}, []dataset.ObjectFile{badObjects}); err == nil {
		t.Fatal("accepted dangling variant")
	}
}
