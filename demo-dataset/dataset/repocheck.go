package dataset

import (
	"fmt"
	"slices"
	"time"
)

// ValidateRepos checks the repository file against the model: period, authors, history order, document
// volumes and references of defects, broken links and patches.
func ValidateRepos(file *RepoFile, model *Model) error {
	if file.SchemaVersion != "1" || file.Seed != model.Seed || file.Period != model.Period || file.AsOf != model.AsOf {
		return fmt.Errorf("repository file does not match the model")
	}
	from, err := parseSecond(model.Period.From)
	if err != nil {
		return err
	}
	asOf, err := parseSecond(model.AsOf)
	if err != nil {
		return err
	}
	people := make(map[string]Person, len(model.People))
	for _, person := range model.People {
		people[person.ID] = person
	}
	documents := make(map[string]int)
	for _, volume := range model.Volumes {
		if volume.Kind == "git_document" {
			documents[volume.OrganizationID+"/"+volume.InstanceID+"/"+volume.AreaID] = volume.Count
		}
	}
	repos := make(map[string]Repository, len(file.Repositories))
	for _, repo := range file.Repositories {
		if _, exists := repos[repo.ID]; exists {
			return fmt.Errorf("duplicate repository %s", repo.ID)
		}
		repos[repo.ID] = repo
		if err := validateRepo(repo, people, from, asOf); err != nil {
			return fmt.Errorf("repository %s: %w", repo.ID, err)
		}
		if len(repo.Documents) != documents[repo.ID] {
			return fmt.Errorf("repository %s has %d documents, volume is %d", repo.ID, len(repo.Documents), documents[repo.ID])
		}
	}
	for repoID, count := range documents {
		if _, exists := repos[repoID]; !exists && count > 0 {
			return fmt.Errorf("volume of %s has no repository", repoID)
		}
	}
	paths := make(map[string]bool, len(file.Patches))
	for _, patch := range file.Patches {
		repo, exists := repos[patch.RepositoryID]
		if !exists || patch.Base != repo.Head || paths[patch.Path] || !validHash(patch.SHA256) || len(patch.Files) == 0 {
			return fmt.Errorf("patch %s has invalid references", patch.ID)
		}
		paths[patch.Path] = true
		for _, fix := range patch.Fixes {
			knownDefect := slices.ContainsFunc(repo.Defects, func(d Defect) bool { return d.ID == fix && d.FixedByStep == patch.Step })
			knownLink := slices.ContainsFunc(repo.BrokenLinks, func(l BrokenLink) bool { return l.ID == fix })
			if !knownDefect && !knownLink {
				return fmt.Errorf("patch %s fixes unknown %s", patch.ID, fix)
			}
		}
	}
	return nil
}

func validateRepo(repo Repository, people map[string]Person, from, asOf time.Time) error {
	if len(repo.Commits) == 0 || repo.Head != repo.Commits[len(repo.Commits)-1].SHA || repo.Lines > _maxLines {
		return fmt.Errorf("invalid head or size")
	}
	var parent string
	for i, commit := range repo.Commits {
		if (i == 0) != (len(commit.Parents) == 0) || (i > 0 && commit.Parents[0] != parent) {
			return fmt.Errorf("commit %s breaks the linear history", commit.SHA)
		}
		parent = commit.SHA
		for _, id := range []string{commit.AuthorID, commit.CommitterID} {
			if person, ok := people[id]; !ok || person.OrganizationID != repo.OrganizationID {
				return fmt.Errorf("commit %s has an author outside the organization", commit.SHA)
			}
		}
		for _, t := range []SourceTime{commit.AuthorTime, commit.CommitTime} {
			at, err := parseSecond(t.UTC)
			if err != nil || at.Before(from) || !at.Before(asOf) || !validOffset(t.OriginalOffset) {
				return fmt.Errorf("commit %s time is outside the period", commit.SHA)
			}
		}
	}
	docs := make(map[string]GitDocument, len(repo.Documents))
	for _, doc := range repo.Documents {
		docs[doc.Path] = doc
		if (doc.Format == FormatOpenAPI) != (doc.SpecValid != nil) {
			return fmt.Errorf("document %s: spec_valid is set only for OpenAPI", doc.Path)
		}
	}
	for _, p := range repo.UnownedDocuments {
		if doc, ok := docs[p]; !ok || doc.Owner != "" {
			return fmt.Errorf("unowned document %s is not listed or has an owner", p)
		}
	}
	for _, p := range repo.InvalidSpecs {
		if doc, ok := docs[p]; !ok || doc.SpecValid == nil || *doc.SpecValid {
			return fmt.Errorf("invalid spec %s is not an invalid OpenAPI document", p)
		}
	}
	for _, link := range repo.BrokenLinks {
		doc, ok := docs[link.Document]
		if !ok || !slices.ContainsFunc(doc.Links, func(l DocLink) bool {
			return l.Line == link.Line && l.Target == link.Target && l.Broken == link.Kind
		}) {
			return fmt.Errorf("broken link %s does not match its document", link.ID)
		}
	}
	for _, defect := range repo.Defects {
		if defect.Line <= 0 || defect.File == "" || defect.Rule == "" {
			return fmt.Errorf("defect %s lacks a location or rule", defect.ID)
		}
	}
	return nil
}
