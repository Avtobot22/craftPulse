package dataset

import (
	"fmt"
	"slices"
	"strings"
)

func ValidateChanges(set *ChangeSet, model *Model, tasks *TaskFile, repos *RepoFile) error {
	f := set.File
	if f.SchemaVersion != "1" || f.Seed != model.Seed || f.Period != model.Period || f.AsOf != model.AsOf || set.Plan.SchemaVersion != "1" {
		return fmt.Errorf("change set does not match model")
	}
	hash, err := Hash(f)
	if err != nil {
		return err
	}
	if set.Plan.ModelHash != hash {
		return fmt.Errorf("load plan refers to another change model")
	}
	people := map[string]Person{}
	for _, p := range model.People {
		people[p.ID] = p
	}
	repository := map[string]Repository{}
	revisions := map[string]map[string]bool{}
	for _, r := range repos.Repositories {
		repository[r.ID] = r
		revisions[r.ID] = map[string]bool{}
		for _, c := range r.Commits {
			revisions[r.ID][c.SHA] = true
		}
	}
	taskByID := map[string]Task{}
	for _, t := range tasks.Tasks {
		taskByID[t.ID] = t
	}
	seen := map[string]string{}
	add := func(id, org string) error {
		if id == "" || seen[id] != "" {
			return fmt.Errorf("duplicate or empty object ID %s", id)
		}
		seen[id] = org
		return nil
	}
	branches := map[string]CIBranch{}
	for _, b := range f.Branches {
		repo, ok := repository[b.RepositoryID]
		at, err := parseSecond(b.Commit.CommitTime.UTC)
		asOf, _ := parseSecond(f.AsOf)
		if !ok || !validGitSHA(b.Commit.SHA) || len(b.Commit.Parents) != 1 || err != nil || !at.Before(asOf) {
			return fmt.Errorf("invalid branch %s", b.Name)
		}
		if !strings.HasPrefix(b.Name, "demo/") || strings.Contains(b.Name, "..") {
			return fmt.Errorf("invalid fixture branch")
		}
		if p := people[b.Commit.AuthorID]; p.OrganizationID != repo.OrganizationID || p.Bot {
			return fmt.Errorf("branch author is outside organization")
		}
		if b.DefinitionPath != "" && !validHash(b.DefinitionSHA256) {
			return fmt.Errorf("branch definition lacks hash")
		}
		branches[b.RepositoryID+":"+b.Name] = b
		revisions[b.RepositoryID][b.Commit.SHA] = true
	}
	mrCounts, runCounts := map[string]int{}, map[string]int{}
	for _, mr := range f.MergeRequests {
		if err := add(mr.ID, mr.OrganizationID); err != nil {
			return err
		}
		repo, ok := repository[mr.RepositoryID]
		branch, exists := branches[mr.RepositoryID+":"+mr.Branch]
		if !ok || !exists || repo.OrganizationID != mr.OrganizationID || repo.InstanceID != mr.InstanceID || branch.Commit.SHA != mr.Head || people[mr.AuthorID].OrganizationID != mr.OrganizationID {
			return fmt.Errorf("MR %s has invalid references", mr.ID)
		}
		mrCounts[mr.OrganizationID+"/"+mr.InstanceID]++
		for _, reviewer := range mr.Reviewers {
			if p := people[reviewer]; p.OrganizationID != mr.OrganizationID || p.Bot || reviewer == mr.AuthorID {
				return fmt.Errorf("invalid reviewer")
			}
		}
		for _, event := range mr.Events {
			if err := add(event.ID, mr.OrganizationID); err != nil {
				return err
			}
			if people[event.ActorID].OrganizationID != mr.OrganizationID {
				return fmt.Errorf("event actor is outside organization")
			}
		}
		for _, link := range mr.TaskLinks {
			if len(link.TaskIDs) == 0 || (link.Kind != "explicit" && link.Kind != "candidate") || (link.Kind == "explicit" && len(link.TaskIDs) != 1) {
				return fmt.Errorf("invalid task link")
			}
			for _, id := range link.TaskIDs {
				t, ok := taskByID[id]
				if !ok || t.OrganizationID != mr.OrganizationID || t.Key != link.Key {
					return fmt.Errorf("task link is outside organization or key")
				}
			}
		}
	}
	for _, run := range f.Runs {
		if err := add(run.ID, run.OrganizationID); err != nil {
			return err
		}
		branch, ok := branches[run.RepositoryID+":"+run.Branch]
		repo := repository[run.RepositoryID]
		if !ok || branch.Commit.SHA != run.SHA || branch.DefinitionPath == "" || repo.OrganizationID != run.OrganizationID {
			return fmt.Errorf("run %s has invalid branch/SHA", run.ID)
		}
		if run.MergeRequestID != "" && seen[run.MergeRequestID] != run.OrganizationID {
			return fmt.Errorf("run has an unknown MR")
		}
		runCounts[run.OrganizationID+"/"+run.InstanceID]++
		configurations := map[string]bool{}
		for _, job := range run.Jobs {
			if err := add(job.ID, run.OrganizationID); err != nil {
				return err
			}
			hash, err := Hash(job.Axes)
			if err != nil {
				return err
			}
			config := job.Name + "/" + job.ConfigurationHash
			if hash != job.ConfigurationHash || configurations[config] {
				return fmt.Errorf("job configuration is invalid or duplicated")
			}
			configurations[config] = true
			if len(job.Attempts) == 0 && job.InitialStatus != "skipped" {
				return fmt.Errorf("executing job has no attempts")
			}
			if job.InitialStatus == "skipped" && len(job.Attempts) != 0 {
				return fmt.Errorf("skipped job has attempts")
			}
			for i, a := range job.Attempts {
				if err := add(a.ID, run.OrganizationID); err != nil {
					return err
				}
				if a.Number != i+1 || !slices.Contains([]string{"success", "failure", "timeout", "canceled"}, a.Outcome) {
					return fmt.Errorf("invalid attempt")
				}
				paths := map[string]bool{}
				for _, r := range a.Reports {
					if err := add(r.ID, run.OrganizationID); err != nil {
						return err
					}
					if !validGitSHA(r.SHA) || !revisions[run.RepositoryID][r.SHA] || paths[r.Path] || !strings.HasPrefix(r.Path, "reports/") || strings.Contains(r.Path, "..") {
						return fmt.Errorf("invalid report reference")
					}
					paths[r.Path] = true
					if r.Format == "junit" {
						if r.TestCaseID == "" {
							return fmt.Errorf("missing stable test case")
						}
					} else if r.Format != "cobertura" || r.CoveredLines > r.TotalLines || r.CoveredBranches > r.TotalBranches {
						return fmt.Errorf("invalid coverage report")
					}
				}
			}
		}
	}
	for key, want := range map[string]int{"org-1/gitlab-1": 6, "org-1/github-1": 4, "org-2/gitlab-1": 1} {
		if mrCounts[key] != want {
			return fmt.Errorf("MR volume %s: %d != %d", key, mrCounts[key], want)
		}
	}
	for key, want := range map[string]int{"org-1/gitlab-1": 12, "org-1/github-1": 8, "org-1/jenkins-1": 6, "org-2/gitlab-1": 1} {
		if runCounts[key] != want {
			return fmt.Errorf("CI volume %s: %d != %d", key, runCounts[key], want)
		}
	}
	variants := map[string]bool{}
	for _, v := range f.Variants {
		if variants[v.VariantID] || len(v.ObjectIDs) == 0 {
			return fmt.Errorf("invalid variant mapping")
		}
		variants[v.VariantID] = true
		for _, id := range v.ObjectIDs {
			if seen[id] == "" {
				return fmt.Errorf("variant points to missing object")
			}
		}
	}
	for _, id := range []string{"bot-response", "ambiguous-link", "same-key-instances", "head-without-scan", "report-other-sha", "ci-retry", "ci-allow-failure", "ci-manual", "ci-skipped", "ci-timeout", "ci-canceled", "ci-matrix", "test-case-configurations", "run-without-junit", "run-without-coverage", "jenkins-execution-manifest", "jenkins-without-manifest"} {
		if !variants[id] {
			return fmt.Errorf("missing MR/CI variant %s", id)
		}
	}
	actions := map[string]bool{}
	bound := map[string]bool{}
	for _, a := range set.Plan.Actions {
		if a.ID == "" || actions[a.ID] || a.Expected == "" || a.Parameters == nil || a.Bindings == nil {
			return fmt.Errorf("invalid load action")
		}
		for _, dep := range a.DependsOn {
			if !actions[dep] {
				return fmt.Errorf("load dependency is missing or out of order")
			}
		}
		if a.ActorID != "" && people[a.ActorID].OrganizationID != a.OrganizationID {
			return fmt.Errorf("load actor is outside organization")
		}
		for _, id := range a.Bindings {
			if seen[id] != a.OrganizationID || bound[id] {
				return fmt.Errorf("invalid or duplicate binding %s", id)
			}
			bound[id] = true
		}
		actions[a.ID] = true
	}
	for id := range seen {
		if !bound[id] {
			return fmt.Errorf("object %s has no source binding action", id)
		}
	}
	return nil
}

func validGitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
