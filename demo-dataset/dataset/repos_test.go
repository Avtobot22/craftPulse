package dataset_test

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func repos(t *testing.T, seed uint64) *dataset.RepoSet {
	t.Helper()
	set, err := dataset.GenerateRepos(model(t, seed))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func repoByID(t *testing.T, file *dataset.RepoFile, id string) dataset.Repository {
	t.Helper()
	for _, repo := range file.Repositories {
		if repo.ID == id {
			return repo
		}
	}
	t.Fatalf("repository %s is missing", id)
	return dataset.Repository{}
}

func TestGenerateReposDeterministic(t *testing.T) {
	t.Parallel()
	first, second, other := repos(t, 21), repos(t, 21), repos(t, 22)
	hash1, err := dataset.Hash(first.File)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := dataset.Hash(second.File)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Fatal("repository model is not deterministic")
	}
	if first.File.Repositories[0].Head == other.File.Repositories[0].Head {
		t.Fatal("author names come from the seed and must change the SHAs")
	}
	dirs := []string{t.TempDir(), t.TempDir()}
	for i, set := range []*dataset.RepoSet{first, second} {
		if err := set.Write(dirs[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, patch := range first.File.Patches {
		left, err := os.ReadFile(filepath.Join(dirs[0], patch.Path))
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(dirs[1], patch.Path))
		if err != nil {
			t.Fatal(err)
		}
		if string(left) != string(right) {
			t.Fatalf("patch %s is not deterministic", patch.ID)
		}
	}
	if err := first.Write(dirs[0]); err == nil {
		t.Fatal("Write must not overwrite existing repositories")
	}
}

func TestRepoDocumentsAndDefects(t *testing.T) {
	t.Parallel()
	file := repos(t, 21).File
	wantDocs := map[string]int{"org-1/gitlab-1/repo-1": 25, "org-1/github-1/repo-2": 25, "org-1/gitlab-1/restricted-repo": 0, "org-2/gitlab-1/private-repo": 1}
	total := 0
	for id, want := range wantDocs {
		repo := repoByID(t, file, id)
		if len(repo.Documents) != want {
			t.Fatalf("%s has %d documents, want %d", id, len(repo.Documents), want)
		}
		total += len(repo.Documents)
		if repo.Lines > 20000 || repo.Restricted == (id == "org-1/gitlab-1/repo-1") {
			t.Fatalf("%s: size %d or restriction is wrong", id, repo.Lines)
		}
	}
	if total != 51 {
		t.Fatalf("documents total %d, want 51", total)
	}
	orders := repoByID(t, file, "org-1/gitlab-1/repo-1")
	kinds := make(map[string]string)
	for _, link := range orders.BrokenLinks {
		kinds[link.ID] = link.Kind
	}
	if kinds["orders-doc-broken-file"] != dataset.BrokenFile || kinds["orders-doc-broken-anchor"] != dataset.BrokenAnchor || len(kinds) != 2 {
		t.Fatalf("broken links: %v", orders.BrokenLinks)
	}
	if !slices.Equal(orders.InvalidSpecs, []string{"docs/api/legacy-quotes.openapi.yaml"}) || len(orders.UnownedDocuments) != 5 {
		t.Fatalf("invalid specs %v, unowned %v", orders.InvalidSpecs, orders.UnownedDocuments)
	}
	if billing := repoByID(t, file, "org-1/github-1/repo-2"); len(billing.BrokenLinks) != 0 || len(billing.UnownedDocuments) != 3 {
		t.Fatal("billing documents must have valid links and three unowned documents")
	}
	byKind := make(map[string]int)
	for _, defect := range orders.Defects {
		byKind[defect.Kind]++
	}
	if byKind[dataset.DefectTest] != 1 || byKind[dataset.DefectLint] != 2 || byKind[dataset.DefectSecurity] != 1 {
		t.Fatalf("defects: %v", orders.Defects)
	}
	if len(file.Patches) != 2 || file.Patches[0].Step != 5 || file.Patches[1].Step != 7 {
		t.Fatalf("patches: %v", file.Patches)
	}
	if !slices.Equal(file.Patches[1].Fixes, []string{"orders-lint-errcheck", "orders-doc-broken-anchor"}) {
		t.Fatalf("step 7 fixes %v", file.Patches[1].Fixes)
	}
}

func TestRepoGitHistoryAndPatches(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	set := repos(t, 21)
	dir := t.TempDir()
	if err := set.Write(dir); err != nil {
		t.Fatal(err)
	}
	for _, repo := range set.File.Repositories {
		bare := filepath.Join(dir, repo.BarePath)
		gitRun(t, bare, "fsck", "--strict", "--no-dangling")
		var want []string
		for _, commit := range repo.Commits {
			want = append(want, commit.SHA)
		}
		if got := strings.Fields(gitRun(t, bare, "log", "--reverse", "--format=%H", repo.DefaultBranch)); !slices.Equal(got, want) {
			t.Fatalf("%s: git log %v, model %v", repo.ID, got, want)
		}
		if docs := strings.Fields(gitRun(t, bare, "ls-tree", "-r", "--name-only", repo.Head, "--", "docs")); len(docs) != len(repo.Documents) {
			t.Fatalf("%s: %d files under docs, model has %d", repo.ID, len(docs), len(repo.Documents))
		}
	}
	orders := repoByID(t, set.File, "org-1/gitlab-1/repo-1")
	head := cloneRepo(t, filepath.Join(dir, orders.BarePath))
	for _, defect := range orders.Defects {
		line := lineOf(t, filepath.Join(head, defect.File), defect.Line)
		if defect.Exception != "" && !strings.Contains(line, defect.Exception) {
			t.Fatalf("%s: line %q lacks the local exception", defect.ID, line)
		}
	}
	secretLine := lineOf(t, filepath.Join(head, "internal/config/config.go"), defectLineByID(t, orders, "orders-secret"))
	encoded, err := json.Marshal(set.File)
	if err != nil {
		t.Fatal(err)
	}
	if value := strings.Trim(strings.TrimSpace(strings.SplitN(secretLine, "=", 2)[1]), `"`); len(value) < 20 || strings.Contains(string(encoded), value) {
		t.Fatal("secret marker must be in the fixture and absent from the model")
	}
	for _, patch := range set.File.Patches {
		work := cloneRepo(t, filepath.Join(dir, orders.BarePath))
		gitRun(t, work, "apply", "--check", filepath.Join(dir, patch.Path))
		gitRun(t, work, "apply", filepath.Join(dir, patch.Path))
		gitRun(t, work, "add", "-A")
		if tree := strings.TrimSpace(gitRun(t, work, "write-tree")); tree != patch.ResultTree {
			t.Fatalf("%s: tree %s, model %s", patch.ID, tree, patch.ResultTree)
		}
		if patch.Step != 7 {
			continue
		}
		fixed := lineOf(t, filepath.Join(work, "internal/httpapi/handler.go"), defectLineByID(t, orders, "orders-lint-errcheck"))
		overview, err := os.ReadFile(filepath.Join(work, "docs/api/overview.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.TrimSpace(fixed), "json.NewEncoder") || strings.Contains(string(overview), "#rate-limits)") {
			t.Fatal("step 7 must remove the errcheck finding and the broken anchor")
		}
		if exception := lineOf(t, filepath.Join(work, "internal/report/report.go"), defectLineByID(t, orders, "orders-lint-exception")); !strings.Contains(exception, "//nolint:staticcheck") {
			t.Fatal("finding with the local exception must stay after step 7")
		}
	}
}

func TestFixtureGoTests(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the fixture repositories")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	set := repos(t, 21)
	dir := t.TempDir()
	if err := set.Write(dir); err != nil {
		t.Fatal(err)
	}
	for _, repo := range set.File.Repositories {
		var want []string
		for _, defect := range repo.Defects {
			if defect.Kind == dataset.DefectTest {
				want = append(want, defect.Rule)
			}
		}
		if got := failedTests(t, cloneRepo(t, filepath.Join(dir, repo.BarePath))); !slices.Equal(got, want) {
			t.Fatalf("%s: failed tests %v, want %v", repo.ID, got, want)
		}
	}
}

func failedTests(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	output, err := cmd.Output()
	if err != nil && len(output) == 0 {
		t.Fatalf("go test: %v", err)
	}
	var failed []string
	passed := 0
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		var event struct{ Action, Package, Test string }
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("go test output: %v", err)
		}
		switch {
		case event.Test == "" || strings.Contains(event.Test, "/"):
		case event.Action == "fail":
			failed = append(failed, event.Package+"."+event.Test)
		case event.Action == "pass":
			passed++
		}
	}
	if passed == 0 {
		t.Fatalf("no fixture test passed in %s", dir)
	}
	return failed
}

func defectLineByID(t *testing.T, repo dataset.Repository, id string) int {
	t.Helper()
	for _, defect := range repo.Defects {
		if defect.ID == id {
			return defect.Line
		}
	}
	t.Fatalf("defect %s is missing", id)
	return 0
}

func cloneRepo(t *testing.T, bare string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	gitRun(t, "", "clone", "--quiet", bare, work)
	return work
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func lineOf(t *testing.T, path string, number int) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Split(string(content), "\n")
	if number < 1 || number > len(all) {
		t.Fatalf("%s has no line %d", path, number)
	}
	return all[number-1]
}
