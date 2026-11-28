package dataset

import (
	"fmt"
	"strings"
)

func changeVariants(f *ChangeFile) []VariantObjects {
	out := []VariantObjects{}
	add := func(id string, objects ...string) {
		for i := range out {
			if out[i].VariantID == id {
				out[i].ObjectIDs = append(out[i].ObjectIDs, objects...)
				return
			}
		}
		out = append(out, VariantObjects{VariantID: id, ObjectIDs: objects})
	}
	for _, mr := range f.MergeRequests {
		if mr.OrganizationID != "org-1" {
			continue
		}
		for _, event := range mr.Events {
			if event.Kind == "comment" && strings.Contains(event.ActorID, "/bot-") {
				add("bot-response", mr.ID, event.ID)
			}
		}
		for _, link := range mr.TaskLinks {
			if link.Kind == "candidate" {
				add("ambiguous-link", mr.ID)
			}
			if len(link.TaskIDs) > 1 {
				add("same-key-instances", mr.ID)
			}
		}
		if strings.HasSuffix(mr.ID, "mr-04") {
			add("head-without-scan", mr.ID)
		}
	}
	for _, run := range f.Runs {
		if run.OrganizationID != "org-1" {
			continue
		}
		for _, job := range run.Jobs {
			if len(job.Attempts) > 1 {
				add("ci-retry", run.ID, job.ID)
			}
			if job.AllowFailure {
				add("ci-allow-failure", run.ID, job.ID)
			}
			if job.InitialStatus == "manual" {
				add("ci-manual", run.ID, job.ID)
			}
			if job.InitialStatus == "skipped" {
				add("ci-skipped", run.ID, job.ID)
			}
			if job.Axes["mode"] == "full" {
				add("ci-matrix", run.ID, job.ID)
				add("test-case-configurations", job.ID)
			}
			for _, a := range job.Attempts {
				if a.Outcome == "timeout" {
					add("ci-timeout", run.ID, a.ID)
				}
				if a.Outcome == "canceled" {
					add("ci-canceled", run.ID, a.ID)
				}
				hasJUnit, hasCoverage := false, false
				for _, r := range a.Reports {
					hasJUnit = hasJUnit || r.Format == "junit"
					hasCoverage = hasCoverage || r.Format == "cobertura"
					if r.SHA != run.SHA {
						add("report-other-sha", run.ID, r.ID)
					}
				}
				if !hasJUnit && a.Outcome == "success" && job.Name == "test" {
					add("run-without-junit", run.ID)
				}
				if !hasCoverage && a.Outcome == "success" && job.Name == "test" {
					add("run-without-coverage", run.ID)
				}
			}
		}
		if run.Provider == "jenkins" {
			if run.ExecutionManifest {
				add("jenkins-execution-manifest", run.ID)
			} else {
				add("jenkins-without-manifest", run.ID)
			}
		}
	}
	for i := range out {
		out[i].ObjectIDs = uniqueStrings(out[i].ObjectIDs)
	}
	return out
}

func uniqueStrings(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}

func (s *ChangeSet) buildPlan(repos *RepoFile) {
	last := map[string]string{}
	add := func(org, instance, kind, object, actor, expected string, params map[string]string, bindings []string, extra ...string) string {
		id := fmt.Sprintf("load-%04d", len(s.Plan.Actions)+1)
		deps := []string{}
		if prior := last[org+"/"+instance]; prior != "" {
			deps = append(deps, prior)
		}
		deps = append(deps, extra...)
		s.Plan.Actions = append(s.Plan.Actions, LoadAction{ID: id, OrganizationID: org, InstanceID: instance, Kind: kind, ObjectID: object, ActorID: actor, DependsOn: uniqueStrings(deps), Parameters: params, Expected: expected, Bindings: bindings})
		last[org+"/"+instance] = id
		return id
	}
	for _, repo := range repos.Repositories {
		actor := fixtureWriter(repo.OrganizationID, repo.InstanceID)
		add(repo.OrganizationID, repo.InstanceID, "push_repository", repo.ID, actor, "head_matches", map[string]string{"ref": repo.DefaultBranch, "sha": repo.Head}, []string{})
	}
	branches := map[string]string{}
	for _, b := range s.File.Branches {
		parts := strings.Split(b.RepositoryID, "/")
		branches[b.RepositoryID+":"+b.Name] = add(parts[0], parts[1], "push_branch", b.RepositoryID, b.Commit.AuthorID, "head_matches", map[string]string{"branch": b.Name, "sha": b.Commit.SHA}, []string{})
	}
	var merges []MergeRequest
	for _, mr := range s.File.MergeRequests {
		add(mr.OrganizationID, mr.InstanceID, "open_merge_request", mr.ID, mr.AuthorID, mr.InitialState, map[string]string{"repository_id": mr.RepositoryID, "branch": mr.Branch, "target": "main", "title": mr.Title, "head": mr.Head}, []string{mr.ID}, branches[mr.RepositoryID+":"+mr.Branch])
		for _, link := range mr.TaskLinks {
			if link.Kind == "explicit" {
				add(mr.OrganizationID, mr.InstanceID, "link_task", mr.ID, mr.AuthorID, "confirmed", map[string]string{"task_id": link.TaskIDs[0], "key": link.Key}, []string{})
			}
		}
		for _, event := range mr.Events {
			if event.Kind == "merge" {
				merges = append(merges, mr)
				continue
			}
			add(mr.OrganizationID, mr.InstanceID, event.Kind, event.ID, event.ActorID, event.Kind, map[string]string{"merge_request_id": mr.ID, "body": event.Body}, []string{event.ID})
		}
	}
	for _, run := range s.File.Runs {
		actor := fixtureWriter(run.OrganizationID, run.InstanceID)
		params := map[string]string{"repository_id": run.RepositoryID, "ref": run.Branch, "sha": run.SHA, "definition": ".gitlab-ci.yml"}
		if run.Provider == "github" {
			params["definition"] = ".github/workflows/demo.yml"
		}
		if run.Provider == "jenkins" {
			params["definition"] = "Jenkinsfile"
		}
		// The source first creates jobs/attempts; manual jobs and reruns are separate actions below.
		bindings := []string{run.ID}
		for _, job := range run.Jobs {
			bindings = append(bindings, job.ID)
			if job.InitialStatus != "manual" && len(job.Attempts) > 0 {
				bindings = append(bindings, job.Attempts[0].ID)
			}
		}
		add(run.OrganizationID, run.InstanceID, "start_ci", run.ID, actor, "created", params, bindings, branches[run.RepositoryID+":"+run.Branch])
		for _, job := range run.Jobs {
			if job.InitialStatus == "manual" {
				add(run.OrganizationID, run.InstanceID, "play_job", job.ID, actor, "success", map[string]string{"run_id": run.ID}, []string{job.Attempts[0].ID})
			}
			if len(job.Attempts) > 1 && run.Provider != "jenkins" {
				kind := "retry_job"
				if run.Provider == "github" {
					kind = "rerun_workflow"
				}
				add(run.OrganizationID, run.InstanceID, "wait_attempt", job.Attempts[0].ID, "", job.Attempts[0].Outcome, map[string]string{"run_id": run.ID}, []string{})
				add(run.OrganizationID, run.InstanceID, kind, run.ID, actor, "created", map[string]string{"job_id": job.ID, "attempt": "2"}, []string{job.Attempts[1].ID})
			}
			for _, a := range job.Attempts {
				if a.Outcome == "canceled" {
					add(run.OrganizationID, run.InstanceID, "wait_job_running", job.ID, "", "running", map[string]string{"run_id": run.ID}, []string{})
					add(run.OrganizationID, run.InstanceID, "cancel_ci", run.ID, actor, "canceled", map[string]string{}, []string{})
				}
			}
		}
		reportIDs := []string{}
		for _, j := range run.Jobs {
			for _, a := range j.Attempts {
				if run.Provider == "jenkins" && a.Number > 1 {
					reportIDs = append(reportIDs, a.ID)
				}
				for _, r := range a.Reports {
					reportIDs = append(reportIDs, r.ID)
				}
			}
		}
		add(run.OrganizationID, run.InstanceID, "collect_ci", run.ID, "", run.FinalStatus, map[string]string{"execution_manifest": fmt.Sprint(run.ExecutionManifest), "sha": run.SHA}, reportIDs)
	}
	for _, mr := range merges {
		for _, event := range mr.Events {
			if event.Kind == "merge" {
				add(mr.OrganizationID, mr.InstanceID, "merge", event.ID, mr.AuthorID, "merged", map[string]string{"merge_request_id": mr.ID, "head": mr.Head}, []string{event.ID})
			}
		}
	}
}
