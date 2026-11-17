package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

type ciScenario struct {
	name                                                 string
	outcomes                                             []string
	status                                               string
	matrix, allowed, junit, coverage, branches, manifest bool
}

func ciScenarios(provider string) []ciScenario {
	base := func(name, status string, outcomes ...string) ciScenario {
		return ciScenario{name: name, status: status, outcomes: outcomes, junit: true, coverage: true}
	}
	if provider == "jenkins" {
		out := []ciScenario{base("success", "success", "success"), base("failure", "failure", "failure"), base("retry", "success", "failure", "success"), base("without-manifest", "success", "success"), base("matrix", "success", "success"), base("retry-without-manifest", "success", "failure", "success")}
		for i := range out {
			out[i].manifest = i != 3 && i != 5
		}
		out[4].matrix = true
		return out
	}
	out := []ciScenario{base("success", "success", "success"), base("failure", "failure", "failure"), base("timeout", "timeout", "timeout"), base("canceled", "canceled", "canceled")}
	if provider == "gitlab" {
		out = append(out, base("skipped", "failure", "failure"), base("manual", "success", "success"), base("allow-failure", "success", "failure"))
	}
	out = append(out, base("retry", "success", "failure", "success"), base("matrix", "success", "success"), base("without-junit", "success", "success"), base("without-coverage", "success", "success"))
	if provider == "gitlab" {
		out = append(out, base("report-other-sha", "success", "success"))
	}
	for i := range out {
		out[i].matrix = out[i].name == "matrix"
		out[i].allowed = out[i].name == "allow-failure"
		out[i].junit = out[i].junit && out[i].name != "without-junit"
		out[i].coverage = out[i].coverage && out[i].name != "without-coverage"
		out[i].branches = out[i].matrix
		if out[i].name == "timeout" || out[i].name == "canceled" {
			out[i].junit = false
			out[i].coverage = false
		}
	}
	return out
}

func GenerateChanges(model *Model, tasks *TaskFile, repos *RepoSet) (*ChangeSet, error) {
	if err := ValidateTasks(tasks, model); err != nil {
		return nil, err
	}
	if err := ValidateRepos(repos.File, model); err != nil {
		return nil, err
	}
	f := &ChangeFile{SchemaVersion: "1", Seed: model.Seed, Period: model.Period, AsOf: model.AsOf, MergeRequests: []MergeRequest{}, Runs: []CIRun{}, Branches: []CIBranch{}, Variants: []VariantObjects{}}
	people := map[string]Person{}
	for _, p := range model.People {
		people[p.ID] = p
	}
	for i := range repos.File.Repositories {
		repo := &repos.File.Repositories[i]
		if repo.InstanceID != "github-1" {
			continue
		}
		run := makeRun(*repo, "github", 1, ciScenarios("github")[0])
		definition, content := pipelineDefinition(&run, ciScenarios("github")[0])
		branch, err := repos.addCIBranch(*repo, "main", repo.OrganizationID+"/person-03", people, map[string][]byte{definition: []byte(content)}, definition)
		if err != nil {
			return nil, err
		}
		repo.Commits = append(repo.Commits, branch.Commit)
		repo.Head = branch.Commit.SHA
		repo.HeadTree = branch.Commit.Tree
		repos.heads[repo.ID][definition] = []byte(content)
		repo.Lines = countLines(repos.heads[repo.ID])
	}
	for _, repo := range repos.File.Repositories {
		if strings.Contains(repo.ID, "restricted-repo") {
			continue
		}
		provider := "gitlab"
		if repo.InstanceID == "github-1" {
			provider = "github"
		}
		mrCount := 6
		if provider == "github" {
			mrCount = 4
		}
		if repo.OrganizationID == "org-2" {
			mrCount = 1
		}
		for i := 1; i <= mrCount; i++ {
			branch := fmt.Sprintf("demo/PA-%d-change", i)
			if repo.OrganizationID == "org-2" {
				branch = "demo/SEC-1-private"
			}
			mr := makeMR(repo, provider, i, branch, tasks)
			// A branch carries source changes but no CI definition: its head has no scan until an explicit load action runs one.
			files := map[string][]byte{"demo-change.txt": []byte(fmt.Sprintf("fixture change %s\n", mr.ID))}
			b, err := repos.addCIBranch(repo, branch, mr.AuthorID, people, files, "")
			if err != nil {
				return nil, err
			}
			mr.Head = b.Commit.SHA
			f.Branches = append(f.Branches, b)
			f.MergeRequests = append(f.MergeRequests, mr)
		}
		scenarios := ciScenarios(provider)
		if repo.OrganizationID == "org-2" {
			scenarios = scenarios[:1]
		}
		for i, scenario := range scenarios {
			run := makeRun(repo, provider, i+1, scenario)
			mrIndex := i
			if provider == "gitlab" && i == 11 {
				mrIndex = 5
			}
			if mrIndex < mrCount && (provider != "gitlab" || i < 5 || i == 11) {
				for _, mr := range f.MergeRequests {
					if mr.RepositoryID == repo.ID && mr.ID == fmt.Sprintf("%s/mr-%02d", repo.ID, mrIndex+1) {
						run.MergeRequestID = mr.ID
						run.Branch = mr.Branch
					}
				}
			}
			if err := finishRun(model, repos, people, repo, &run, scenario, f); err != nil {
				return nil, err
			}
		}
	}
	repo := repos.File.Repositories[0]
	for i, scenario := range ciScenarios("jenkins") {
		run := makeRun(repo, "jenkins", i+1, scenario)
		if err := finishRun(model, repos, people, repo, &run, scenario, f); err != nil {
			return nil, err
		}
	}
	f.Variants = changeVariants(f)
	hash, err := Hash(f)
	if err != nil {
		return nil, err
	}
	set := &ChangeSet{File: f, Plan: &LoadPlan{SchemaVersion: "1", ModelHash: hash, Actions: []LoadAction{}}}
	set.buildPlan(repos.File)
	if err := ValidateChanges(set, model, tasks, repos.File); err != nil {
		return nil, err
	}
	return set, nil
}

func makeRun(repo Repository, provider string, n int, s ciScenario) CIRun {
	instance := repo.InstanceID
	area := repo.AreaID
	if provider == "jenkins" {
		instance = "jenkins-1"
		area = "job-series"
	}
	id := fmt.Sprintf("%s/%s/%s/run-%02d", repo.OrganizationID, instance, area, n)
	run := CIRun{ID: id, OrganizationID: repo.OrganizationID, InstanceID: instance, Provider: provider, RepositoryID: repo.ID,
		Branch: fmt.Sprintf("demo/ci-%s-%02d", provider, n), InitialStatus: s.status, FinalStatus: s.status, ExecutionManifest: s.manifest, Jobs: []CIJob{}}
	if s.name == "retry" || s.name == "retry-without-manifest" {
		run.InitialStatus = "failure"
	}
	if s.name == "manual" {
		run.InitialStatus = "manual"
	}
	modes := []string{"fast"}
	if s.matrix {
		modes = append(modes, "full")
	}
	for _, mode := range modes {
		job := CIJob{ID: id + "/test-" + mode, Name: "test", Stage: "test", Axes: map[string]string{"mode": mode}, AllowFailure: s.allowed, InitialStatus: s.outcomes[0], Attempts: []CIAttempt{}}
		job.ConfigurationHash, _ = Hash(job.Axes)
		if s.name == "manual" {
			job.InitialStatus = "manual"
		}
		for i, outcome := range s.outcomes {
			a := CIAttempt{ID: fmt.Sprintf("%s/a%d", job.ID, i+1), Number: i + 1, Outcome: outcome, Reports: []CIReport{}}
			add := func(format, path string) {
				a.Reports = append(a.Reports, CIReport{ID: a.ID + "/" + format, Path: "reports/" + mode + fmt.Sprintf("-a%d/", i+1) + path, Format: format})
			}
			if s.junit {
				add("junit", "junit.xml")
				a.Reports[len(a.Reports)-1].TestCaseID = "demo/unit/TestRound"
			}
			if s.coverage {
				add("cobertura", "coverage.xml")
				r := &a.Reports[len(a.Reports)-1]
				r.CoveredLines = 3
				r.TotalLines = 4
				if s.branches {
					r.CoveredBranches = 1
					r.TotalBranches = 2
				}
			}
			job.Attempts = append(job.Attempts, a)
		}
		run.Jobs = append(run.Jobs, job)
	}
	if s.allowed {
		j := CIJob{ID: id + "/required", Name: "required", Stage: "verify", Axes: map[string]string{}, AllowFailure: false, InitialStatus: "success", Attempts: []CIAttempt{{ID: id + "/required/a1", Number: 1, Outcome: "success", Reports: []CIReport{}}}}
		j.ConfigurationHash, _ = Hash(j.Axes)
		run.Jobs = append(run.Jobs, j)
	}
	if s.name == "skipped" {
		j := CIJob{ID: id + "/skipped", Name: "after_failure", Stage: "verify", Axes: map[string]string{}, InitialStatus: "skipped", Attempts: []CIAttempt{}}
		j.ConfigurationHash, _ = Hash(j.Axes)
		run.Jobs = append(run.Jobs, j)
	}
	return run
}

func finishRun(model *Model, repos *RepoSet, people map[string]Person, repo Repository, run *CIRun, s ciScenario, file *ChangeFile) error {
	if s.name == "report-other-sha" {
		for i := range run.Jobs {
			for j := range run.Jobs[i].Attempts {
				for k := range run.Jobs[i].Attempts[j].Reports {
					run.Jobs[i].Attempts[j].Reports[k].SHA = repo.Head
				}
			}
		}
	}
	raw, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	definition, content := pipelineDefinition(run, s)
	runtime, err := _fixtures.ReadFile("fixtures/ci/main.go.txt")
	if err != nil {
		return fmt.Errorf("read CI runtime: %w", err)
	}
	files := map[string][]byte{definition: []byte(content), ".demo-ci/run.json": append(raw, '\n'), ".demo-ci/main.go": runtime, "demo-target.go": []byte("package demo\nfunc Fixture(a, b int) int {\n if a > 0 { return a + b }\n return b\n}\n")}
	author := repo.OrganizationID + "/person-03"
	if repo.OrganizationID == "org-2" {
		author = "org-2/person-01"
	}
	branch, err := repos.addCIBranch(repo, run.Branch, author, people, files, definition)
	if err != nil {
		return err
	}
	run.SHA = branch.Commit.SHA
	for i := range run.Jobs {
		for j := range run.Jobs[i].Attempts {
			for k := range run.Jobs[i].Attempts[j].Reports {
				r := &run.Jobs[i].Attempts[j].Reports[k]
				if r.SHA == "" {
					r.SHA = run.SHA
				}
			}
		}
	}
	for i := range file.MergeRequests {
		if file.MergeRequests[i].ID == run.MergeRequestID {
			file.MergeRequests[i].Head = run.SHA
		}
	}
	file.Branches = append(file.Branches, branch)
	file.Runs = append(file.Runs, *run)
	_ = model
	return nil
}

func (s *RepoSet) addCIBranch(repo Repository, name, authorID string, people map[string]Person, overlay map[string][]byte, definition string) (CIBranch, error) {
	if s.refs[repo.ID] == nil {
		s.refs[repo.ID] = map[string]string{}
	}
	parent := repo.Head
	headFiles := s.heads[repo.ID]
	if previous := s.refs[repo.ID][name]; previous != "" {
		parent = previous
		headFiles = s.heads[repo.ID+":"+name]
	}
	files := maps.Clone(headFiles)
	maps.Copy(files, overlay)
	s.heads[repo.ID+":"+name] = files
	at, _ := parseSecond(repo.Commits[len(repo.Commits)-1].CommitTime.UTC)
	at = at.Add(time.Hour)
	author, err := signatureOf(people[authorID], repo.InstanceID, at)
	if err != nil {
		return CIBranch{}, err
	}
	objects := s.objects[repo.BarePath]
	tree := objects.tree(files)
	message := "Add demo fixture " + name
	head := objects.commit(tree, []string{parent}, author, author, message)
	s.refs[repo.ID][name] = head
	changed := slices.Sorted(maps.Keys(overlay))
	sum := sha256.Sum256(overlay[definition])
	digest := ""
	if definition != "" {
		digest = hex.EncodeToString(sum[:])
	}
	return CIBranch{RepositoryID: repo.ID, Name: name, Commit: Commit{SHA: head, Tree: tree, Parents: []string{parent}, AuthorID: authorID, AuthorTime: sourceTime(at, author.offset), CommitterID: authorID, CommitTime: sourceTime(at, author.offset), Message: message, Files: changed}, DefinitionPath: definition, DefinitionSHA256: digest}, nil
}

func makeMR(repo Repository, provider string, n int, branch string, tasks *TaskFile) MergeRequest {
	id := fmt.Sprintf("%s/mr-%02d", repo.ID, n)
	author := repo.OrganizationID + "/person-03"
	reviewer := repo.OrganizationID + "/person-02"
	if repo.OrganizationID == "org-2" {
		author = "org-2/person-01"
		reviewer = author
	}
	mr := MergeRequest{ID: id, OrganizationID: repo.OrganizationID, InstanceID: repo.InstanceID, RepositoryID: repo.ID, Branch: branch,
		Title: fmt.Sprintf("Change %d", n), AuthorID: author, InitialState: "ready", FinalState: "ready", Reviewers: []string{}, Events: []MREvent{}, TaskLinks: []MRTaskLink{}}
	event := func(kind, actor, body string) {
		mr.Events = append(mr.Events, MREvent{ID: fmt.Sprintf("%s/e%02d", id, len(mr.Events)+1), Kind: kind, ActorID: actor, Body: body})
	}
	if n == 1 {
		mr.InitialState = "draft"
		event("ready", author, "")
		if reviewer != author {
			mr.Reviewers = []string{reviewer}
			event("reviewer_assigned", author, reviewer)
		}
		if repo.OrganizationID == "org-1" {
			event("comment", "org-1/bot-"+repo.InstanceID, "Automated check completed")
		}
		event("comment", author, "Author clarification")
		if reviewer != author {
			event("review", reviewer, "Reviewed")
		}
		event("merge", author, "")
		mr.FinalState = "merged"
	}
	if n == 3 {
		mr.InitialState = "draft"
		mr.FinalState = "draft"
	}
	if n >= 4 {
		mr.Reviewers = []string{reviewer}
		event("reviewer_assigned", author, reviewer)
	}
	key := "PA-1"
	kind := "explicit"
	if n == 2 {
		key = "PA-2"
		kind = "candidate"
		mr.Branch = "demo/PA-2-change"
	}
	if n == 5 {
		key = "PA-7"
		kind = "candidate"
		mr.Title = "PA-7: ambiguous source"
	}
	if repo.OrganizationID == "org-2" {
		key = "SEC-1"
		kind = "explicit"
	}
	candidates := []string{}
	for _, task := range tasks.Tasks {
		if task.OrganizationID == repo.OrganizationID && task.Key == key {
			candidates = append(candidates, task.ID)
			if kind == "explicit" {
				break
			}
		}
	}
	mr.TaskLinks = append(mr.TaskLinks, MRTaskLink{Kind: kind, Key: key, TaskIDs: candidates})
	_ = provider
	return mr
}
