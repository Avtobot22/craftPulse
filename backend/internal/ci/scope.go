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

// Languages of the code profiles.
const (
	LanguageGo         = "go"
	LanguagePython     = "python"
	LanguageJavaScript = "javascript"
)

var (
	// _generated is the marker of generated Go code (https://go.dev/s/generatedcode).
	_generated = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
	// _generatedHeader marks generated Python and JS/TS code in the comments the file starts with.
	_generatedHeader = regexp.MustCompile(`(?i)(\bdo not edit\b|@generated\b)`)

	_sourceRules = map[string]*sourceRules{
		LanguageGo: {
			extensions: []string{".go"},
			excludedDirs: map[string]envelope.ScanExcludedTargetReason{
				"vendor": envelope.ScanExcludedTargetReasonVendor,
				"build":  envelope.ScanExcludedTargetReasonBuild,
			},
			hidden:    func(name string) bool { return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") },
			test:      func(rel string) bool { return strings.HasSuffix(rel, "_test.go") || inDir(rel, "testdata") },
			generated: isGeneratedGo,
			modules:   true,
		},
		LanguagePython: {
			extensions: []string{".py"},
			excludedDirs: map[string]envelope.ScanExcludedTargetReason{
				"site-packages":  envelope.ScanExcludedTargetReasonVendor,
				"__pypackages__": envelope.ScanExcludedTargetReasonVendor,
				"node_modules":   envelope.ScanExcludedTargetReasonVendor,
				"build":          envelope.ScanExcludedTargetReasonBuild,
				"dist":           envelope.ScanExcludedTargetReasonBuild,
			},
			hidden: func(name string) bool { return strings.HasPrefix(name, ".") || name == "__pycache__" },
			test: func(rel string) bool {
				name := path.Base(rel)
				return strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py") || name == "conftest.py" ||
					inDir(rel, "tests", "test")
			},
			generated:  func(name string) (bool, error) { return hasGeneratedHeader(name, "#") },
			venvMarker: "pyvenv.cfg",
		},
		LanguageJavaScript: {
			extensions: []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"},
			excludedDirs: map[string]envelope.ScanExcludedTargetReason{
				"node_modules":     envelope.ScanExcludedTargetReasonVendor,
				"bower_components": envelope.ScanExcludedTargetReasonVendor,
				"build":            envelope.ScanExcludedTargetReasonBuild,
				"dist":             envelope.ScanExcludedTargetReasonBuild,
				"coverage":         envelope.ScanExcludedTargetReasonBuild,
			},
			hidden: func(name string) bool { return strings.HasPrefix(name, ".") },
			test: func(rel string) bool {
				name := path.Base(rel)
				return strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") ||
					inDir(rel, "__tests__", "__mocks__", "tests", "test")
			},
			generated: func(name string) (bool, error) {
				base := path.Base(filepath.ToSlash(name))
				for _, ext := range []string{".min.js", ".min.mjs", ".min.cjs"} {
					if strings.HasSuffix(base, ext) {
						return true, nil
					}
				}
				return hasGeneratedHeader(name, "//", "/*", "*", "#!")
			},
		},
	}
)

// sourceRules tell which files of a language a code profile analyzes and which it leaves out.
type sourceRules struct {
	extensions   []string
	excludedDirs map[string]envelope.ScanExcludedTargetReason
	// hidden directories are not source and are not reported.
	hidden    func(name string) bool
	test      func(rel string) bool
	generated func(name string) (bool, error)
	// venvMarker is the file that makes a directory a virtual environment of dependencies.
	venvMarker string
	// modules finds the Go modules of the component.
	modules bool
}

// Scope is what a code profile analyzes in one component. Paths are relative to the repository root.
type Scope struct {
	// Files are the analyzed source files, test files included, sorted.
	Files []string
	// Tests are the analyzed files of test code.
	Tests []string
	// Excluded are the dependency and build directories and generated files left out of the analysis.
	Excluded []envelope.ScanExcludedTarget
	// Modules are the directories of the Go modules that cover the component.
	Modules []GoModule
	// Hidden are the directories that are not source and are not reported.
	Hidden []string
}

// GoModule is a module the linter loads: its directory and the package pattern of the component in it.
type GoModule struct {
	Dir     string
	Pattern string
}

// GoScope walks a Go component. Vendor and build directories and generated files are excluded,
// _test.go files and testdata directories are test code; directories the go tool ignores, starting with
// "." or "_", are not source.
func GoScope(root, prefix string) (*Scope, error) {
	return LanguageScope(root, prefix, LanguageGo)
}

// LanguageScope walks the component at prefix of the repository at root for the source files of the
// language: Go, Python or JavaScript, which covers TypeScript.
//
// Python leaves out virtual environments (directories with pyvenv.cfg), site-packages, __pypackages__,
// node_modules, build and dist; test code is test_*.py, *_test.py, conftest.py and the tests and test
// directories. JS/TS leaves out node_modules, bower_components, build, dist, coverage and minified files;
// test code is *.test.* and *.spec.* files and the __tests__, __mocks__, tests and test directories. A file
// whose leading comments say "DO NOT EDIT" or "@generated" is generated. Directories starting with "." are
// not source.
func LanguageScope(root, prefix, language string) (*Scope, error) {
	rules, ok := _sourceRules[language]
	if !ok {
		return nil, fmt.Errorf("language %q has no code profile", language)
	}
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
			if p == base {
				return nil
			}
			return s.enterDir(rules, p, rel, d.Name())
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if rules.modules && d.Name() == "go.mod" && !inDir(rel, "testdata") {
			s.Modules = append(s.Modules, GoModule{Dir: path.Dir(rel), Pattern: "./..."})
		}
		if !slices.Contains(rules.extensions, path.Ext(rel)) {
			return nil
		}
		return s.addFile(rules, p, rel)
	})
	if err != nil {
		return nil, fmt.Errorf("walk component %q: %w", prefix, err)
	}
	if len(s.Excluded) > _maxScopeTargets || len(s.Tests) > _maxScopeTargets {
		return nil, fmt.Errorf("component %q has more than %d excluded or test targets", prefix, _maxScopeTargets)
	}
	if rules.modules && len(s.Modules) == 0 {
		if m, ok := enclosingModule(root, prefix); ok {
			s.Modules = []GoModule{m}
		}
	}
	slices.Sort(s.Files)
	slices.Sort(s.Tests)
	return s, nil
}

func (s *Scope) enterDir(rules *sourceRules, p, rel, name string) error {
	if rules.hidden(name) {
		s.Hidden = append(s.Hidden, rel)
		return filepath.SkipDir
	}
	if reason, ok := rules.excludedDirs[name]; ok {
		s.Excluded = append(s.Excluded, envelope.ScanExcludedTarget{Path: rel, Reason: reason})
		return filepath.SkipDir
	}
	if rules.venvMarker != "" {
		if info, err := os.Stat(filepath.Join(p, rules.venvMarker)); err == nil && info.Mode().IsRegular() {
			s.Excluded = append(s.Excluded, envelope.ScanExcludedTarget{Path: rel, Reason: envelope.ScanExcludedTargetReasonVendor})
			return filepath.SkipDir
		}
	}
	return nil
}

func (s *Scope) addFile(rules *sourceRules, p, rel string) error {
	generated, err := rules.generated(p)
	if err != nil {
		return err
	}
	if generated {
		s.Excluded = append(s.Excluded, envelope.ScanExcludedTarget{Path: rel, Reason: envelope.ScanExcludedTargetReasonGenerated})
		return nil
	}
	s.Files = append(s.Files, rel)
	if rules.test(rel) {
		s.Tests = append(s.Tests, rel)
	}
	return nil
}

// isGeneratedGo looks for the marker in the comments before the package clause.
func isGeneratedGo(name string) (bool, error) {
	return scanHeader(name, func(line string) (generated, done bool) {
		if _generated.MatchString(line) {
			return true, true
		}
		return false, strings.HasPrefix(line, "package ")
	})
}

// hasGeneratedHeader looks for the marker in the comment lines the file starts with.
func hasGeneratedHeader(name string, comments ...string) (bool, error) {
	return scanHeader(name, func(line string) (generated, done bool) {
		if line == "" {
			return false, false
		}
		if !slices.ContainsFunc(comments, func(c string) bool { return strings.HasPrefix(line, c) }) {
			return false, true
		}
		marked := _generatedHeader.MatchString(line)
		return marked, marked
	})
}

// scanHeader reads the trimmed lines of the file until check is done with them.
func scanHeader(name string, check func(line string) (generated, done bool)) (bool, error) {
	f, err := os.Open(name) //nolint:gosec // the path comes from the walk of the component
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if generated, done := check(strings.TrimSpace(sc.Text())); done {
			return generated, nil
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

// inDir tells whether a directory of the path has one of the names.
func inDir(rel string, names ...string) bool {
	return slices.ContainsFunc(strings.Split(path.Dir(rel), "/"), func(dir string) bool { return slices.Contains(names, dir) })
}
