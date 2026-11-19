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
)

var (
	_expectedLine = regexp.MustCompile(`^(\S+) (\S+):([1-9]\d*)$`)

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

// FixtureResult compares one case: findings it expects and does not get, and findings it gets unexpectedly.
type FixtureResult struct {
	Case    string
	Found   int
	Missing []Finding
	Extra   []Finding
}

// Fixtures runs a profile command on the cases of a profile fixture directory. Each case is a
// subdirectory with the checked code in src/ and expected.txt, one finding per line as
// "<rule> <path>:<line>", with the path relative to src/ and # starting a comment. The command runs in
// src/ and writes a SARIF report to the file named by CRAFTPULSE_REPORT.
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
	expected, err := readExpected(filepath.Join(dir, FixtureExpected))
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
	cmd.Env = toolEnv(FixtureReportEnv + "=" + report)
	cmd.WaitDelay = _killGrace
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		return FixtureResult{}, fmt.Errorf("run profile command: %w", err)
	}
	if runCtx.Err() != nil {
		return FixtureResult{}, errors.New("profile command ran out of time")
	}
	data, err := os.ReadFile(report)
	if err != nil {
		return FixtureResult{}, fmt.Errorf("profile command wrote no report: %w", err)
	}
	got, err := SARIFFindings(data, src)
	if err != nil {
		return FixtureResult{}, err
	}
	missing, extra := diff(expected, got)
	return FixtureResult{Case: name, Found: len(got), Missing: missing, Extra: extra}, nil
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
			fmt.Fprintln(w, "  missing "+f.String())
		}
		for _, f := range r.Extra {
			fmt.Fprintln(w, "  extra "+f.String())
		}
	}
	if failed {
		return ErrFixturesFailed
	}
	return nil
}

func readExpected(name string) ([]Finding, error) {
	f, err := os.Open(name) //nolint:gosec // the path is inside the fixture directory
	if err != nil {
		return nil, fmt.Errorf("read expected findings: %w", err)
	}
	defer f.Close()
	var out []Finding
	sc := bufio.NewScanner(f)
	for no := 1; sc.Scan(); no++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := _expectedLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("%s line %d: expected \"<rule> <path>:<line>\"", FixtureExpected, no)
		}
		n, _ := strconv.Atoi(m[3]) // the pattern allows only digits
		out = append(out, Finding{Rule: m[1], File: path.Clean(m[2]), Line: n})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read expected findings: %w", err)
	}
	return out, nil
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

// diff compares the findings as multisets: a finding expected twice must be found twice.
func diff(expected, got []Finding) (missing, extra []Finding) {
	count := map[Finding]int{}
	for _, f := range expected {
		count[f]++
	}
	for _, f := range got {
		count[f]--
	}
	for _, f := range slices.SortedFunc(maps.Keys(count), compareFindings) {
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
