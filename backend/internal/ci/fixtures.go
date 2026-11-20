package ci

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	FixtureSourceDir = "src"
	FixtureExpected  = "expected.txt"
	// FixtureReportEnv names the file where the profile command writes its SARIF report.
	FixtureReportEnv = "CRAFTPULSE_REPORT"
	// FixtureFunctionsEnv, FixtureClonesEnv and FixtureScopeEnv name the files of a code profile: the
	// Lizard CSV of functions, the jscpd JSON of copies and the scope of the run.
	FixtureFunctionsEnv = "CRAFTPULSE_FUNCTIONS_REPORT"
	FixtureClonesEnv    = "CRAFTPULSE_CLONES_REPORT"
	FixtureScopeEnv     = "CRAFTPULSE_SCOPE_REPORT"
	// FixtureFactsEnv names the file where a profile without a SARIF report writes its facts, one per line.
	FixtureFactsEnv = "CRAFTPULSE_FACTS_REPORT"
)

var (
	_expectedLine     = regexp.MustCompile(`^(\S+) (\S+):([1-9]\d*)$`)
	_expectedFunction = regexp.MustCompile(`^function (\S+):([1-9]\d*) (\S+) ccn=(\d+) nloc=(\d+)$`)
	_expectedClone    = regexp.MustCompile(`^clone (\S+):([1-9]\d*)-([1-9]\d*) (\S+):([1-9]\d*)-([1-9]\d*)$`)
	_expectedTest     = regexp.MustCompile(`^test (\S+)$`)
	_expectedExcluded = regexp.MustCompile(`^excluded (\S+) (vendor|generated|build)$`)
	_expectedSkipped  = regexp.MustCompile(`^skipped (\S+) (unsupported|timeout)$`)
	_expectedVuln     = regexp.MustCompile(`^vulnerability (\S+) (\S+)@(\S+) (\S+)$`)
	_expectedMisconf  = regexp.MustCompile(`^misconfiguration (\S+) (\S+):([1-9]\d*) (.+)$`)

	// ErrFixturesFailed is returned when a fixture case gives other findings than it expects.
	ErrFixturesFailed = errors.New("rule fixtures do not match")
)

// Finding is the part of a finding that a rule fixture checks.
type Finding struct {
	Rule string
	File string
	Line int
}

func (f Finding) String() string { return f.Rule + " " + f.File + ":" + strconv.Itoa(f.Line) }

// FixtureResult compares one case: findings and facts it expects and does not get, and those it gets
// unexpectedly.
type FixtureResult struct {
	Case    string
	Found   int
	Missing []string
	Extra   []string
}

// fixtureReport is a report of a code profile that a case compares in full when the command writes it.
type fixtureReport struct {
	env   string
	file  string
	kinds []string
	read  func(io.Reader) ([]string, error)
}

var _fixtureReports = []fixtureReport{
	{env: FixtureFunctionsEnv, file: "functions.csv", kinds: []string{"function"}, read: functionFacts},
	{env: FixtureClonesEnv, file: "clones.json", kinds: []string{"clone"}, read: cloneFacts},
	{env: FixtureScopeEnv, file: "scope.json", kinds: []string{"test", "excluded", "skipped"}, read: scopeFacts},
	{env: FixtureFactsEnv, file: "facts.txt", kinds: []string{"vulnerability", "misconfiguration"}, read: lineFacts},
}

// Fixtures runs a profile command on the cases of a profile fixture directory. Each case is a
// subdirectory with the checked code in src/ and expected.txt, one finding per line as
// "<rule> <path>:<line>", with the path relative to src/ and # starting a comment. The command runs in
// src/ and writes a SARIF report to the file named by CRAFTPULSE_REPORT. A code profile also writes the
// functions, copies and scope reports; each one written is compared in full with the expected lines
// "function <path>:<line> <name> ccn=<n> nloc=<n>", "clone <path>:<start>-<end> <path>:<start>-<end>",
// "test <path>", "excluded <path> <reason>" and "skipped <path> <reason>" for a file the linter could not
// parse. A profile without a SARIF report writes its facts instead, such as "vulnerability <id>
// <package>@<version> <lockfile>" and "misconfiguration <id> <path>:<line> <resource>".
type Fixtures struct {
	Dir     string
	Command []string
	Timeout time.Duration
	Stdout  io.Writer
	Stderr  io.Writer
}

func (fx *Fixtures) Run(ctx context.Context) ([]FixtureResult, error) {
	if len(fx.Command) == 0 {
		return nil, errors.New("profile command is required")
	}
	entries, err := os.ReadDir(fx.Dir)
	if err != nil {
		return nil, fmt.Errorf("read fixture directory: %w", err)
	}
	var results []FixtureResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(fx.Dir, e.Name(), FixtureExpected)); err != nil {
			continue
		}
		res, err := fx.runCase(ctx, e.Name())
		if err != nil {
			return results, fmt.Errorf("fixture %s: %w", e.Name(), err)
		}
		results = append(results, res)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no fixture cases with %s in %s", FixtureExpected, fx.Dir)
	}
	return results, nil
}

func (fx *Fixtures) runCase(ctx context.Context, name string) (FixtureResult, error) {
	dir := filepath.Join(fx.Dir, name)
	expected, facts, err := readExpected(filepath.Join(dir, FixtureExpected))
	if err != nil {
		return FixtureResult{}, err
	}
	out, err := os.MkdirTemp("", "craftpulse-fixture-")
	if err != nil {
		return FixtureResult{}, fmt.Errorf("create report directory: %w", err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join(dir, FixtureSourceDir))
	if err != nil {
		return FixtureResult{}, fmt.Errorf("resolve source directory: %w", err)
	}
	report := filepath.Join(out, "report.sarif")
	runCtx, cancel := context.WithTimeout(ctx, fx.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, fx.Command[0], fx.Command[1:]...) //nolint:gosec // the profile names its command
	cmd.Dir, cmd.Stdout, cmd.Stderr = src, fx.Stdout, fx.Stderr
	env := []string{FixtureReportEnv + "=" + report}
	for _, r := range _fixtureReports {
		env = append(env, r.env+"="+filepath.Join(out, r.file))
	}
	cmd.Env = toolEnv(env...)
	cmd.WaitDelay = _killGrace
	ownGroup(cmd)
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		return FixtureResult{}, fmt.Errorf("run profile command: %w", err)
	}
	if runCtx.Err() != nil {
		return FixtureResult{}, errors.New("profile command ran out of time")
	}
	var got []Finding
	data, err := os.ReadFile(report)
	switch {
	case err == nil:
		if got, err = SARIFFindings(data, src); err != nil {
			return FixtureResult{}, err
		}
	case !fileExists(filepath.Join(out, "facts.txt")):
		return FixtureResult{}, fmt.Errorf("profile command wrote no report: %w", err)
	}
	missing, extra := diff(expected, got, compareFindings)
	res := FixtureResult{Case: name, Found: len(got), Missing: strs(missing), Extra: strs(extra)}
	for _, r := range _fixtureReports {
		want := factsOf(facts, r.kinds)
		got, written, err := readFacts(filepath.Join(out, r.file), r.read)
		if err != nil {
			return FixtureResult{}, err
		}
		if !written && len(want) == 0 {
			continue
		}
		missing, extra := diff(want, got, strings.Compare)
		res.Missing, res.Extra = append(res.Missing, missing...), append(res.Extra, extra...)
		if r.env == FixtureFactsEnv {
			res.Found += len(got)
		}
	}
	return res, nil
}

func readFacts(name string, read func(io.Reader) ([]string, error)) (facts []string, written bool, err error) {
	f, err := os.Open(name) //nolint:gosec // the path is inside the report directory of the case
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", filepath.Base(name), err)
	}
	defer f.Close()
	facts, err = read(f)
	return facts, true, err
}

func factsOf(facts map[string][]string, kinds []string) []string {
	var out []string
	for _, k := range kinds {
		out = append(out, facts[k]...)
	}
	return out
}

func strs[T fmt.Stringer](items []T) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.String())
	}
	return out
}

// PrintFixtures prints every case and returns ErrFixturesFailed when any case differs.
func PrintFixtures(w io.Writer, results []FixtureResult) error {
	failed := false
	for _, r := range results {
		if len(r.Missing) == 0 && len(r.Extra) == 0 {
			fmt.Fprintf(w, "fixture %s: ok, %d finding(s)\n", r.Case, r.Found)
			continue
		}
		failed = true
		fmt.Fprintf(w, "fixture %s: differs\n", r.Case)
		for _, f := range r.Missing {
			fmt.Fprintln(w, "  missing "+f)
		}
		for _, f := range r.Extra {
			fmt.Fprintln(w, "  extra "+f)
		}
	}
	if failed {
		return ErrFixturesFailed
	}
	return nil
}

// readExpected reads the findings of a case and its facts by kind, each fact in its canonical form.
func readExpected(name string) ([]Finding, map[string][]string, error) {
	f, err := os.Open(name) //nolint:gosec // the path is inside the fixture directory
	if err != nil {
		return nil, nil, fmt.Errorf("read expected findings: %w", err)
	}
	defer f.Close()
	var findings []Finding
	facts := map[string][]string{}
	sc := bufio.NewScanner(f)
	for no := 1; sc.Scan(); no++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, _, _ := strings.Cut(line, " ")
		if fact, ok := expectedFact(kind, line); ok {
			facts[kind] = append(facts[kind], fact)
			continue
		}
		m := _expectedLine.FindStringSubmatch(line)
		if m == nil || slices.ContainsFunc(_fixtureReports, func(r fixtureReport) bool { return slices.Contains(r.kinds, kind) }) {
			return nil, nil, fmt.Errorf("%s line %d: expected \"<rule> <path>:<line>\" or a fact of a code profile", FixtureExpected, no)
		}
		n, _ := strconv.Atoi(m[3]) // the pattern allows only digits
		findings = append(findings, Finding{Rule: m[1], File: path.Clean(m[2]), Line: n})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("read expected findings: %w", err)
	}
	return findings, facts, nil
}

func expectedFact(kind, line string) (string, bool) {
	switch kind {
	case "function":
		if m := _expectedFunction.FindStringSubmatch(line); m != nil {
			return functionFact(path.Clean(m[1]), m[2], m[3], m[4], m[5]), true
		}
	case "clone":
		if m := _expectedClone.FindStringSubmatch(line); m != nil {
			return cloneFact(path.Clean(m[1])+":"+m[2]+"-"+m[3], path.Clean(m[4])+":"+m[5]+"-"+m[6]), true
		}
	case "test":
		if m := _expectedTest.FindStringSubmatch(line); m != nil {
			return "test " + path.Clean(m[1]), true
		}
	case "excluded":
		if m := _expectedExcluded.FindStringSubmatch(line); m != nil {
			return "excluded " + path.Clean(m[1]) + " " + m[2], true
		}
	case "skipped":
		if m := _expectedSkipped.FindStringSubmatch(line); m != nil {
			return "skipped " + path.Clean(m[1]) + " " + m[2], true
		}
	case "vulnerability":
		if m := _expectedVuln.FindStringSubmatch(line); m != nil {
			return "vulnerability " + m[1] + " " + m[2] + "@" + m[3] + " " + path.Clean(m[4]), true
		}
	case "misconfiguration":
		if m := _expectedMisconf.FindStringSubmatch(line); m != nil {
			return "misconfiguration " + m[1] + " " + path.Clean(m[2]) + ":" + m[3] + " " + m[4], true
		}
	}
	return "", false
}

type sarifLog struct {
	Runs []struct {
		Results []struct {
			RuleID string `json:"ruleId"` //nolint:tagliatelle // SARIF wire format.
			Rule   *struct {
				ID string `json:"id"`
			} `json:"rule"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"` //nolint:tagliatelle // SARIF wire format.
					Region struct {
						StartLine int `json:"startLine"` //nolint:tagliatelle // SARIF wire format.
					} `json:"region"`
				} `json:"physicalLocation"` //nolint:tagliatelle // SARIF wire format.
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

// SARIFFindings reads rule, file and line of every result of a SARIF report; file paths become relative
// to root.
func SARIFFindings(data []byte, root string) ([]Finding, error) {
	var log sarifLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("report is not SARIF JSON: %w", err)
	}
	var out []Finding
	for _, run := range log.Runs {
		for _, r := range run.Results {
			f := Finding{Rule: r.RuleID}
			if f.Rule == "" && r.Rule != nil {
				f.Rule = r.Rule.ID
			}
			if len(r.Locations) > 0 {
				loc := r.Locations[0].PhysicalLocation
				f.File, f.Line = relative(loc.ArtifactLocation.URI, root), loc.Region.StartLine
			}
			out = append(out, f)
		}
	}
	return out, nil
}

func relative(uri, root string) string {
	p := uri
	if u, err := url.Parse(uri); err == nil && u.Scheme == "file" {
		p = filepath.FromSlash(u.Path)
	}
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(root, p); err == nil {
			p = rel
		}
	}
	return path.Clean(filepath.ToSlash(p))
}

// diff compares two lists as multisets: an item expected twice must be found twice.
func diff[T comparable](expected, got []T, compare func(a, b T) int) (missing, extra []T) {
	count := map[T]int{}
	for _, f := range expected {
		count[f]++
	}
	for _, f := range got {
		count[f]--
	}
	for _, f := range slices.SortedFunc(maps.Keys(count), compare) {
		for n := count[f]; n > 0; n-- {
			missing = append(missing, f)
		}
		for n := count[f]; n < 0; n++ {
			extra = append(extra, f)
		}
	}
	return missing, extra
}

func compareFindings(a, b Finding) int {
	return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), strings.Compare(a.Rule, b.Rule))
}

// lineFacts reads a facts file of a profile: one fact per non-empty line.
func lineFacts(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}
