package dataset_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func ciChanges(t *testing.T, seed uint64) (*dataset.Model, *dataset.RepoSet, *dataset.ChangeSet) {
	t.Helper()
	m := model(t, seed)
	tasks, err := dataset.GenerateTasks(m)
	if err != nil {
		t.Fatal(err)
	}
	r, err := dataset.GenerateRepos(m)
	if err != nil {
		t.Fatal(err)
	}
	set, err := dataset.GenerateChanges(m, tasks, r)
	if err != nil {
		t.Fatal(err)
	}
	return m, r, set
}

func TestChangesDeterministicVolumesAndBranches(t *testing.T) {
	t.Parallel()
	_, repos, first := ciChanges(t, 21)
	_, otherRepos, second := ciChanges(t, 21)
	_, _, other := ciChanges(t, 22)
	h1, err := dataset.Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := dataset.Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	h3, err := dataset.Hash(other)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 || h1 == h3 {
		t.Fatal("change model and load plan are not controlled by seed")
	}
	if len(first.File.MergeRequests) != 11 || len(first.File.Runs) != 27 {
		t.Fatal("wrong volumes")
	}
	dir, otherDir := t.TempDir(), t.TempDir()
	if err := repos.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := otherRepos.Write(otherDir); err != nil {
		t.Fatal(err)
	}
	for _, b := range first.File.Branches {
		repo := repoByID(t, repos.File, b.RepositoryID)
		bare := filepath.Join(dir, repo.BarePath)
		if out := gitRun(t, ".", "--git-dir", bare, "rev-parse", b.Commit.SHA+"^{commit}"); strings.TrimSpace(out) != b.Commit.SHA {
			t.Fatal("commit missing")
		}
		if b.DefinitionPath != "" {
			actual := gitRun(t, ".", "--git-dir", bare, "show", b.Commit.SHA+":"+b.DefinitionPath)
			if actual == "" {
				t.Fatal("pipeline definition missing")
			}
		}
	}
	for _, mr := range first.File.MergeRequests {
		repo := repoByID(t, repos.File, mr.RepositoryID)
		if got := gitRun(t, ".", "--git-dir", filepath.Join(dir, repo.BarePath), "rev-parse", mr.Branch); strings.TrimSpace(got) != mr.Head {
			t.Fatal("MR head not written")
		}
	}
	for _, repo := range repos.File.Repositories {
		if got := gitRun(t, ".", "--git-dir", filepath.Join(dir, repo.BarePath), "rev-parse", "main"); strings.TrimSpace(got) != repo.Head {
			t.Fatal("repository head disagrees with model")
		}
		if repo.InstanceID == "github-1" {
			if got := gitRun(t, ".", "--git-dir", filepath.Join(dir, repo.BarePath), "show", "main:.github/workflows/demo.yml"); !strings.Contains(got, "workflow_dispatch") {
				t.Fatal("dispatch workflow must exist on default branch")
			}
		}
	}
}

func TestChangesReviewAndReportSemantics(t *testing.T) {
	t.Parallel()
	_, _, set := ciChanges(t, 21)
	var sawBot, sawCandidate, sawSameKey, sawNoScan, sawOldSHA, sawMatrix, sawAllowed bool
	for _, mr := range set.File.MergeRequests {
		if mr.OrganizationID != "org-1" {
			continue
		}
		if strings.HasSuffix(mr.ID, "mr-01") {
			if mr.InitialState != "draft" || mr.FinalState != "merged" {
				t.Fatal("draft transition missing")
			}
			kinds := []string{}
			for _, e := range mr.Events {
				kinds = append(kinds, e.Kind)
			}
			if !slices.Equal(kinds, []string{"ready", "reviewer_assigned", "comment", "comment", "review", "merge"}) {
				t.Fatalf("human review chronology %v", kinds)
			}
			if !strings.Contains(mr.Events[2].ActorID, "/bot-") || mr.Events[3].ActorID != mr.AuthorID || mr.Events[4].ActorID == mr.AuthorID {
				t.Fatal("bot/author must precede external human review")
			}
			sawBot = true
		}
		if strings.HasSuffix(mr.ID, "mr-02") {
			if len(mr.Reviewers) != 0 || strings.Contains(mr.Title, mr.TaskLinks[0].Key) || !strings.Contains(mr.Branch, mr.TaskLinks[0].Key) {
				t.Fatal("branch-only candidate or no-reviewer variant missing")
			}
			sawCandidate = true
		}
		for _, l := range mr.TaskLinks {
			if len(l.TaskIDs) > 1 {
				if l.Kind != "candidate" {
					t.Fatal("same key confirmed without instance")
				}
				sawSameKey = true
			}
		}
		if strings.HasSuffix(mr.ID, "mr-04") {
			for _, run := range set.File.Runs {
				if run.MergeRequestID == mr.ID {
					for _, j := range run.Jobs {
						for _, a := range j.Attempts {
							if len(a.Reports) != 0 {
								t.Fatal("head without scan has reports")
							}
						}
					}
				}
			}
			sawNoScan = true
		}
	}
	for _, run := range set.File.Runs {
		names := map[string]int{}
		for _, j := range run.Jobs {
			names[j.Name]++
			if names[j.Name] > 1 {
				sawMatrix = true
			}
			if j.AllowFailure {
				if run.FinalStatus != "success" || j.Attempts[len(j.Attempts)-1].Outcome != "failure" {
					t.Fatal("allow_failure lost")
				}
				sawAllowed = true
			}
			for _, a := range j.Attempts {
				for _, r := range a.Reports {
					if r.SHA != run.SHA {
						sawOldSHA = true
					}
				}
			}
		}
	}
	if !sawBot || !sawCandidate || !sawSameKey || !sawNoScan || !sawOldSHA || !sawMatrix || !sawAllowed {
		t.Fatal("acceptance variants missing")
	}
}

func TestChangeValidationRejectsBrokenReferences(t *testing.T) {
	t.Parallel()
	m, r, s := ciChanges(t, 21)
	tasks, err := dataset.GenerateTasks(m)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*dataset.ChangeSet)
	}{
		{"outside organization actor", func(s *dataset.ChangeSet) { s.File.MergeRequests[0].AuthorID = "org-2/person-01" }},
		{"invented SHA", func(s *dataset.ChangeSet) { s.File.Runs[0].SHA = strings.Repeat("1", 40) }},
		{"duplicate configuration", func(s *dataset.ChangeSet) { run := &s.File.Runs[0]; run.Jobs = append(run.Jobs, run.Jobs[0]) }},
		{"attempt gap", func(s *dataset.ChangeSet) { s.File.Runs[0].Jobs[0].Attempts[0].Number = 2 }},
		{"missing source binding", func(s *dataset.ChangeSet) { s.Plan.Actions[len(s.Plan.Actions)-1].Bindings = []string{} }},
		{"future dependency", func(s *dataset.ChangeSet) { s.Plan.Actions[0].DependsOn = []string{"load-9999"} }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			copy := clone(t, *s)
			tt.change(&copy)
			copy.Plan.ModelHash, _ = dataset.Hash(copy.File)
			if err := dataset.ValidateChanges(&copy, m, tasks, r.File); err == nil {
				t.Fatal("accepted invalid change set")
			}
		})
	}
}

func TestCICommandsMatchEveryPlannedAttempt(t *testing.T) {
	_, repos, set := ciChanges(t, 21)
	dir := t.TempDir()
	if err := repos.Write(dir); err != nil {
		t.Fatal(err)
	}
	// Compile the exact fixture runtime stored in a generated commit once for all attempts.
	first := set.File.Runs[0]
	repo := repoByID(t, repos.File, first.RepositoryID)
	source := gitRun(t, ".", "--git-dir", filepath.Join(dir, repo.BarePath), "show", first.SHA+":.demo-ci/main.go")
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fixture")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile runtime: %v\n%s", err, out)
	}
	for _, run := range set.File.Runs {
		t.Run(run.ID, func(t *testing.T) {
			work := t.TempDir()
			if err := os.Mkdir(filepath.Join(work, ".demo-ci"), 0700); err != nil {
				t.Fatal(err)
			}
			repo := repoByID(t, repos.File, run.RepositoryID)
			raw := gitRun(t, ".", "--git-dir", filepath.Join(dir, repo.BarePath), "show", run.SHA+":.demo-ci/run.json")
			if err := os.WriteFile(filepath.Join(work, ".demo-ci/run.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			manifestSteps := 0
			for _, job := range run.Jobs {
				for _, attempt := range job.Attempts {
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, binary, "-job", job.Name, "-attempt", strconv.Itoa(attempt.Number))
					cmd.Dir = work
					cmd.Env = append(os.Environ(), "DEMO_MODE="+job.Axes["mode"], "CI_COMMIT_SHA="+run.SHA, "GIT_COMMIT="+run.SHA, "BUILD_NUMBER=11", "BUILD_URL=https://jenkins.demo.invalid/job/demo/11/", "JOB_NAME=demo", "NODE_NAME=demo-go")
					out, err := cmd.CombinedOutput()
					switch attempt.Outcome {
					case "timeout", "canceled":
						if ctx.Err() == nil {
							t.Fatalf("waiting attempt must be stopped by source: %v %s", err, out)
						}
					case "failure":
						exit, ok := err.(*exec.ExitError)
						if !ok || exit.ExitCode() != 1 {
							t.Fatalf("expected exit 1: %v %s", err, out)
						}
					default:
						if err != nil {
							t.Fatalf("expected success: %v %s", err, out)
						}
					}
					for _, r := range attempt.Reports {
						content, err := os.ReadFile(filepath.Join(work, r.Path))
						if err != nil {
							t.Fatal(err)
						}
						decoder := xml.NewDecoder(strings.NewReader(string(content)))
						for {
							_, err := decoder.Token()
							if err != nil {
								if err != io.EOF {
									t.Fatal(err)
								}
								break
							}
						}
						if r.Format == "junit" && strings.Contains(string(content), "<failure") != (attempt.Outcome == "failure") {
							t.Fatal("test result does not match attempt")
						}
						var metadata map[string]string
						data, err := os.ReadFile(filepath.Join(work, r.Path+".meta.json"))
						if err != nil {
							t.Fatal(err)
						}
						if err := json.Unmarshal(data, &metadata); err != nil {
							t.Fatal(err)
						}
						if metadata["source_sha"] != r.SHA || metadata["configuration_hash"] != job.ConfigurationHash {
							t.Fatal("report provenance disagrees with plan")
						}
					}
					if attempt.Outcome == "success" || attempt.Outcome == "failure" {
						manifestSteps++
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(work, "execution-manifest.json"))
			if run.ExecutionManifest {
				if err != nil {
					t.Fatal(err)
				}
				var manifest struct {
					SHA         string `json:"sha"`
					BuildNumber int    `json:"build_number"`
					Steps       []struct {
						Attempt           int    `json:"attempt"`
						ConfigurationHash string `json:"configuration_hash"`
						StartedAt         string `json:"started_at"`
						FinishedAt        string `json:"finished_at"`
					} `json:"steps"`
				}
				if err := json.Unmarshal(data, &manifest); err != nil {
					t.Fatal(err)
				}
				if len(manifest.Steps) != manifestSteps || manifest.SHA != run.SHA || manifest.BuildNumber != 11 {
					t.Fatal("execution manifest lost attempts or native build identity")
				}
				for _, step := range manifest.Steps {
					start, err := time.Parse(time.RFC3339Nano, step.StartedAt)
					if err != nil {
						t.Fatal(err)
					}
					end, err := time.Parse(time.RFC3339Nano, step.FinishedAt)
					if err != nil || end.Before(start) {
						t.Fatal("bad execution times")
					}
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("unexpected execution manifest")
			}
		})
	}
}
