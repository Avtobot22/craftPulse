package dataset

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Fixture sources carry a .txt suffix, so the Go tool, linters and govulncheck of this module skip the
// injected defects.
//
//go:embed fixtures
var _fixtures embed.FS

const (
	_fixtureSuffix = ".txt"
	_secretMarker  = "@@DEMO_SECRET_MARKER@@"
	_maxLines      = 20000
	_commitDelay   = 3 * time.Minute
)

// RepoSet is the generated repository model with the objects and patches that Write puts on disk.
type RepoSet struct {
	File    *RepoFile
	objects map[string]gitObjects
	patches map[string][]byte
}

type builtRepo struct {
	repo    Repository
	objects gitObjects
	head    map[string][]byte
}

func GenerateRepos(model *Model) (*RepoSet, error) {
	from, err := parseSecond(model.Period.From)
	if err != nil {
		return nil, fmt.Errorf("period.from: %w", err)
	}
	asOf, err := parseSecond(model.AsOf)
	if err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	people := make(map[string]Person, len(model.People))
	for _, person := range model.People {
		people[person.ID] = person
	}
	set := &RepoSet{
		File:    &RepoFile{SchemaVersion: "1", Seed: model.Seed, Period: model.Period, AsOf: model.AsOf, Repositories: []Repository{}, Patches: []Patch{}},
		objects: make(map[string]gitObjects),
		patches: make(map[string][]byte),
	}
	for _, spec := range repoSpecs() {
		built, err := buildRepo(spec, people, from, asOf)
		if err != nil {
			return nil, fmt.Errorf("repository %s: %w", spec.key, err)
		}
		for _, step := range spec.steps {
			patch, content, err := buildPatch(spec, built, step)
			if err != nil {
				return nil, fmt.Errorf("repository %s step %d: %w", spec.key, step.step, err)
			}
			set.File.Patches = append(set.File.Patches, patch)
			set.patches[patch.Path] = content
		}
		set.File.Repositories = append(set.File.Repositories, built.repo)
		set.objects[built.repo.BarePath] = built.objects
	}
	if err := ValidateRepos(set.File, model); err != nil {
		return nil, err
	}
	return set, nil
}

// Write creates the bare repositories and patch files under dir; existing repositories are not touched.
func (s *RepoSet) Write(dir string) error {
	for _, repo := range s.File.Repositories {
		if err := s.objects[repo.BarePath].writeBare(filepath.Join(dir, filepath.FromSlash(repo.BarePath)), repo.DefaultBranch, repo.Head); err != nil {
			return err
		}
	}
	for _, patch := range s.File.Patches {
		target := filepath.Join(dir, filepath.FromSlash(patch.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create patch directory: %w", err)
		}
		if err := os.WriteFile(target, s.patches[patch.Path], 0o600); err != nil {
			return fmt.Errorf("write patch: %w", err)
		}
	}
	return nil
}

func buildRepo(spec repoSpec, people map[string]Person, from, asOf time.Time) (builtRepo, error) {
	files, err := fixtureFiles(spec.key + "/base")
	if err != nil {
		return builtRepo{}, err
	}
	for p, content := range renderDocs(spec.docs, spec.specs) {
		if _, exists := files[p]; exists {
			return builtRepo{}, fmt.Errorf("document %s overrides a fixture file", p)
		}
		files[p] = content
	}
	repoID := spec.organizationID + "/" + spec.instanceID + "/" + spec.areaID
	repo := Repository{
		ID: repoID, OrganizationID: spec.organizationID, InstanceID: spec.instanceID, AreaID: spec.areaID,
		Path: spec.path, BarePath: "repos/" + repoID + ".git", DefaultBranch: "main", Restricted: spec.restricted,
		Language: "go", DocRoots: spec.docRoots, Lines: countLines(files),
	}
	if repo.Lines > _maxLines {
		return builtRepo{}, fmt.Errorf("fixture has %d lines, limit is %d", repo.Lines, _maxLines)
	}
	objects := gitObjects{}
	commits, err := buildHistory(spec, files, people, objects, from, asOf)
	if err != nil {
		return builtRepo{}, err
	}
	repo.Commits = commits
	repo.Head = commits[len(commits)-1].SHA
	repo.HeadTree = commits[len(commits)-1].Tree
	specValid := make(map[string]bool, len(spec.specs))
	for _, s := range spec.specs {
		specValid[s.path] = s.valid
	}
	repo.Documents = scanDocuments(repoID, spec.docRoots, files, specValid)
	repo.UnownedDocuments, repo.InvalidSpecs = []string{}, []string{}
	for _, doc := range repo.Documents {
		if doc.Format == FormatMarkdown && doc.Owner == "" {
			repo.UnownedDocuments = append(repo.UnownedDocuments, doc.Path)
		}
		if doc.SpecValid != nil && !*doc.SpecValid {
			repo.InvalidSpecs = append(repo.InvalidSpecs, doc.Path)
		}
	}
	if repo.BrokenLinks, err = matchBrokenLinks(spec.brokenLinks, repo.Documents); err != nil {
		return builtRepo{}, err
	}
	repo.Defects = []Defect{}
	for _, d := range spec.defects {
		line, err := findLine(files[d.file], d.linePrefix)
		if err != nil {
			return builtRepo{}, fmt.Errorf("defect %s: %w", d.id, err)
		}
		repo.Defects = append(repo.Defects, Defect{ID: d.id, Kind: d.kind, Tool: d.tool, Rule: d.rule, File: d.file, Line: line, Exception: d.exception, FixedByStep: d.fixedByStep})
	}
	return builtRepo{repo: repo, objects: objects, head: files}, nil
}

func buildHistory(spec repoSpec, files map[string][]byte, people map[string]Person, objects gitObjects, from, asOf time.Time) ([]Commit, error) {
	owner := make(map[string]int, len(files))
	for p := range files {
		best, length := -1, -1
		for i, plan := range spec.commits {
			for _, prefix := range plan.prefixes {
				if strings.HasPrefix(p, prefix) && len(prefix) > length {
					best, length = i, len(prefix)
				}
			}
		}
		if best < 0 {
			return nil, fmt.Errorf("file %s belongs to no commit", p)
		}
		owner[p] = best
	}
	tree := make(map[string][]byte, len(files))
	commits := make([]Commit, 0, len(spec.commits))
	var parents []string
	previous := from
	for i, plan := range spec.commits {
		at, err := parseSecond(plan.at)
		if err != nil {
			return nil, fmt.Errorf("commit %d: %w", i+1, err)
		}
		if at.Before(previous) || !at.Add(_commitDelay).Before(asOf) {
			return nil, fmt.Errorf("commit %d time is outside the period or out of order", i+1)
		}
		previous = at
		author, ok := people[plan.authorID]
		if !ok {
			return nil, fmt.Errorf("commit %d: unknown author", i+1)
		}
		signature, err := signatureOf(author, spec.instanceID, at)
		if err != nil {
			return nil, fmt.Errorf("commit %d: %w", i+1, err)
		}
		committed := signature
		committed.at = at.Add(_commitDelay)
		var changed []string
		for p, content := range files {
			if owner[p] == i {
				tree[p] = content
				changed = append(changed, p)
			}
		}
		if len(changed) == 0 {
			return nil, fmt.Errorf("commit %d changes no files", i+1)
		}
		slices.Sort(changed)
		treeID := objects.tree(tree)
		sha := objects.commit(treeID, parents, signature, committed, plan.message)
		commits = append(commits, Commit{
			SHA: sha, Tree: treeID, Parents: append([]string{}, parents...),
			AuthorID: author.ID, AuthorTime: sourceTime(at, signature.offset),
			CommitterID: author.ID, CommitTime: sourceTime(committed.at, signature.offset),
			Message: plan.message, Files: changed,
		})
		parents = []string{sha}
	}
	return commits, nil
}

func signatureOf(person Person, instanceID string, at time.Time) (gitSignature, error) {
	for _, account := range person.Accounts {
		if account.InstanceID == instanceID {
			return gitSignature{name: person.DisplayName, email: account.Login + "@users.demo.invalid", at: at, offset: offsetOf(person)}, nil
		}
	}
	return gitSignature{}, fmt.Errorf("person %s has no account in %s", person.ID, instanceID)
}

func sourceTime(at time.Time, offset string) SourceTime {
	return SourceTime{UTC: at.UTC().Format(time.RFC3339), OriginalOffset: offset}
}

func buildPatch(spec repoSpec, built builtRepo, step stepSpec) (Patch, []byte, error) {
	overlay, err := fixtureFiles(fmt.Sprintf("%s/step-%d", spec.key, step.step))
	if err != nil {
		return Patch{}, nil, err
	}
	after := make(map[string][]byte, len(built.head)+len(overlay))
	for p, content := range built.head {
		after[p] = content
	}
	for p, content := range overlay {
		after[p] = content
	}
	for _, edit := range step.edits {
		content := string(after[edit.file])
		if strings.Count(content, edit.old) != 1 {
			return Patch{}, nil, fmt.Errorf("edit of %s must match exactly once", edit.file)
		}
		after[edit.file] = []byte(strings.Replace(content, edit.old, edit.new, 1))
	}
	var changed []string
	for p, content := range after {
		if old, exists := built.head[p]; !exists || string(old) != string(content) {
			changed = append(changed, p)
		}
	}
	if len(changed) == 0 {
		return Patch{}, nil, fmt.Errorf("patch changes nothing")
	}
	slices.Sort(changed)
	if err := checkFixes(spec, built.repo, after, step); err != nil {
		return Patch{}, nil, err
	}
	content := []byte(unifiedDiff(built.head, after))
	sum := sha256.Sum256(content)
	patch := Patch{
		ID: fmt.Sprintf("%s-step-%d", spec.key, step.step), Step: step.step, RepositoryID: built.repo.ID,
		Path: fmt.Sprintf("patches/%s-step-%d.patch", spec.key, step.step), SHA256: hex.EncodeToString(sum[:]),
		Base: built.repo.Head, ResultTree: gitObjects{}.tree(after), Files: changed, Fixes: slices.Clone(step.fixes),
	}
	if patch.Fixes == nil {
		patch.Fixes = []string{}
	}
	return patch, content, nil
}

// checkFixes proves on the patched tree that exactly the listed defects and broken links are gone and that
// the remaining defects keep their line.
func checkFixes(spec repoSpec, repo Repository, after map[string][]byte, step stepSpec) error {
	for _, d := range spec.defects {
		fixed := slices.Contains(step.fixes, d.id)
		if fixed != (d.fixedByStep == step.step) {
			return fmt.Errorf("defect %s: fixed_by_step disagrees with the step", d.id)
		}
		line, err := findLine(after[d.file], d.linePrefix)
		switch {
		case fixed && err == nil:
			return fmt.Errorf("defect %s remains after the patch", d.id)
		case !fixed && err != nil:
			return fmt.Errorf("defect %s disappears after the patch", d.id)
		case !fixed && line != defectLine(repo, d.id):
			return fmt.Errorf("defect %s moves after the patch", d.id)
		}
	}
	specValid := make(map[string]bool, len(spec.specs))
	for _, s := range spec.specs {
		specValid[s.path] = s.valid
	}
	documents := scanDocuments(repo.ID, spec.docRoots, after, specValid)
	var remaining []BrokenLink
	for _, link := range spec.brokenLinks {
		if !slices.Contains(step.fixes, link.ID) {
			remaining = append(remaining, link)
		}
	}
	if _, err := matchBrokenLinks(remaining, documents); err != nil {
		return fmt.Errorf("after patch: %w", err)
	}
	return nil
}

func defectLine(repo Repository, id string) int {
	for _, d := range repo.Defects {
		if d.ID == id {
			return d.Line
		}
	}
	return 0
}

// matchBrokenLinks requires the scanned broken links to be exactly the expected ones and fills their lines.
func matchBrokenLinks(expected []BrokenLink, documents []GitDocument) ([]BrokenLink, error) {
	result := []BrokenLink{}
	for _, doc := range documents {
		for _, link := range doc.Links {
			if link.Broken == "" {
				continue
			}
			i := slices.IndexFunc(expected, func(e BrokenLink) bool {
				return e.Document == doc.Path && e.Target == link.Target && e.Kind == link.Broken
			})
			if i < 0 {
				return nil, fmt.Errorf("unexpected broken link %s in %s", link.Target, doc.Path)
			}
			found := expected[i]
			found.Line = link.Line
			result = append(result, found)
		}
	}
	if len(result) != len(expected) {
		return nil, fmt.Errorf("found %d broken links, expected %d", len(result), len(expected))
	}
	return result, nil
}

func findLine(content []byte, prefix string) (int, error) {
	found := 0
	for i, line := range lines(content) {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			if found != 0 {
				return 0, fmt.Errorf("line prefix matches more than once")
			}
			found = i + 1
		}
	}
	if found == 0 {
		return 0, fmt.Errorf("line prefix not found")
	}
	return found, nil
}

func fixtureFiles(dir string) (map[string][]byte, error) {
	root := path.Join("fixtures", dir)
	files := make(map[string][]byte)
	err := fs.WalkDir(_fixtures, root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name, ok := strings.CutSuffix(strings.TrimPrefix(p, root+"/"), _fixtureSuffix)
		if !ok {
			return fmt.Errorf("fixture %s lacks the %s suffix", p, _fixtureSuffix)
		}
		content, err := _fixtures.ReadFile(p)
		if err != nil {
			return err
		}
		if len(content) == 0 || content[len(content)-1] != '\n' {
			return fmt.Errorf("fixture %s must end with a newline", p)
		}
		files[name] = []byte(strings.ReplaceAll(string(content), _secretMarker, secretMarker()))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read fixtures: %w", err)
	}
	return files, nil
}

// secretMarker is the training value that secret scanners must flag. It is derived at run time so that no
// credential-like literal is stored in this repository; the value grants access to nothing.
func secretMarker() string {
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:32]
}

func countLines(files map[string][]byte) int {
	total := 0
	for _, content := range files {
		total += len(lines(content))
	}
	return total
}
