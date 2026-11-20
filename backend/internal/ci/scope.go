package ci

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _maxScopeTargets is the limit of excluded and test targets one tool run lists in the scan manifest.
const _maxScopeTargets = 10000

var (
	// _generated is the marker of generated Go code (https://go.dev/s/generatedcode).
	_generated = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

	_excludedDirs = map[string]envelope.ScanExcludedTargetReason{
		"vendor": envelope.ScanExcludedTargetReasonVendor,
		"build":  envelope.ScanExcludedTargetReasonBuild,
	}
)

// Scope is what a code profile analyzes in one component. Paths are relative to the repository root.
type Scope struct {
	// Files are the analyzed source files, test files included, sorted.
	Files []string
	// Tests are the analyzed files of test code.
	Tests []string
	// Excluded are the vendor and build directories and generated files left out of the analysis.
	Excluded []envelope.ScanExcludedTarget
	// Modules are the directories of the Go modules that cover the component.
	Modules []GoModule
	// Hidden are the directories starting with "." or "_": not source, and not reported.
	Hidden []string
}

// GoModule is a module the linter loads: its directory and the package pattern of the component in it.
type GoModule struct {
	Dir     string
	Pattern string
}

// GoScope walks the component at prefix of the repository at root. Vendor and build directories and
// generated files are excluded, _test.go files and testdata directories are test code; directories the
// go tool ignores, starting with "." or "_", are not source.
func GoScope(root, prefix string) (*Scope, error) {
	base := filepath.Join(root, filepath.FromSlash(prefix))
	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("component directory %q: %w", prefix, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("component path %q is not a directory", prefix)
	}
	s := &Scope{}
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return s.enterDir(rel, d.Name(), p != base)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if d.Name() == "go.mod" && !inTestdata(rel) {
			s.Modules = append(s.Modules, GoModule{Dir: path.Dir(rel), Pattern: "./..."})
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		return s.addFile(p, rel)
	})
	if err != nil {
		return nil, fmt.Errorf("walk component %q: %w", prefix, err)
	}
	if len(s.Excluded) > _maxScopeTargets || len(s.Tests) > _maxScopeTargets {
		return nil, fmt.Errorf("component %q has more than %d excluded or test targets", prefix, _maxScopeTargets)
	}
	if len(s.Modules) == 0 {
		if m, ok := enclosingModule(root, prefix); ok {
			s.Modules = []GoModule{m}
		}
	}
	slices.Sort(s.Files)
	slices.Sort(s.Tests)
	return s, nil
}

func (s *Scope) enterDir(rel, name string, nested bool) error {
	if !nested {
		return nil
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		s.Hidden = append(s.Hidden, rel)
		return filepath.SkipDir
	}
	if reason, ok := _excludedDirs[name]; ok {
		s.Excluded = append(s.Excluded, envelope.ScanExcludedTarget{Path: rel, Reason: reason})
		return filepath.SkipDir
	}
	return nil
}

func (s *Scope) addFile(p, rel string) error {
	generated, err := isGenerated(p)
	if err != nil {
		return err
	}
	if generated {
		s.Excluded = append(s.Excluded, envelope.ScanExcludedTarget{Path: rel, Reason: envelope.ScanExcludedTargetReasonGenerated})
		return nil
	}
	s.Files = append(s.Files, rel)
	if strings.HasSuffix(rel, "_test.go") || inTestdata(rel) {
		s.Tests = append(s.Tests, rel)
	}
	return nil
}

// isGenerated looks for the marker in the comments before the package clause.
func isGenerated(name string) (bool, error) {
	f, err := os.Open(name) //nolint:gosec // the path comes from the walk of the component
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if _generated.MatchString(line) {
			return true, nil
		}
		if strings.HasPrefix(line, "package ") {
			return false, nil
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return false, err
	}
	return false, nil
}

// enclosingModule finds the module of a component that has no go.mod of its own: the nearest directory
// above it inside the repository.
func enclosingModule(root, prefix string) (GoModule, bool) {
	component := path.Clean("./" + prefix)
	for dir := component; ; dir = path.Dir(dir) {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), "go.mod")); err == nil && info.Mode().IsRegular() {
			pattern := "./..."
			if dir != component {
				pattern = "./" + strings.TrimPrefix(strings.TrimPrefix(component, dir), "/") + "/..."
			}
			return GoModule{Dir: dir, Pattern: pattern}, true
		}
		if dir == "." {
			return GoModule{}, false
		}
	}
}

func inTestdata(rel string) bool {
	return slices.Contains(strings.Split(path.Dir(rel), "/"), "testdata")
}
