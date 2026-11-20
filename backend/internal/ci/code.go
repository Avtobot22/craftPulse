package ci

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Files of a code profile directory. RulesHash covers them in this order.
const (
	CodeProfileFile = "profile.json"
	LintConfigFile  = "golangci.yml"
	ClonesFile      = "jscpd.json"
)

// Tools of a code profile and the names of their versions in profile.json.
const (
	ToolLint      = "golangci-lint"
	ToolFunctions = "lizard"
	ToolClones    = "jscpd"
)

// CodeProfile is a code profile of an image: tool versions, the time limit and the thresholds of
// functions, with the tool configuration files next to profile.json.
type CodeProfile struct {
	ID        string            `json:"profile_id"`
	Language  string            `json:"language"`
	Timeout   string            `json:"timeout"`
	Tools     map[string]string `json:"tools"`
	Functions struct {
		CCN  int `json:"ccn_threshold"`
		NLOC int `json:"nloc_threshold"`
	} `json:"functions"`

	dir       string
	limit     time.Duration
	rulesHash string
}

// LoadCodeProfile reads and checks the profile in dir and hashes its rules.
func LoadCodeProfile(dir string) (*CodeProfile, error) {
	h := sha256.New()
	var raw []byte
	for _, name := range []string{CodeProfileFile, LintConfigFile, ClonesFile} {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // the image names its profile directory
		if err != nil {
			return nil, fmt.Errorf("read code profile: %w", err)
		}
		if name == CodeProfileFile {
			raw = data
		}
		h.Write(data)
	}
	p := &CodeProfile{dir: dir, rulesHash: hex.EncodeToString(h.Sum(nil))}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", CodeProfileFile, err)
	}
	var problems []error
	if p.ID == "" {
		problems = append(problems, errors.New("profile_id is required"))
	}
	if p.Language != "go" {
		problems = append(problems, fmt.Errorf("language %q is not supported", p.Language))
	}
	for _, tool := range []string{ToolLint, ToolFunctions, ToolClones} {
		if p.Tools[tool] == "" {
			problems = append(problems, fmt.Errorf("tools.%s: version is required", tool))
		}
	}
	timeout, err := time.ParseDuration(p.Timeout)
	if err != nil || timeout <= 0 {
		problems = append(problems, errors.New("timeout must be a positive duration"))
	}
	if p.Functions.CCN <= 0 || p.Functions.NLOC <= 0 {
		problems = append(problems, errors.New("functions thresholds must be positive"))
	}
	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("%s: %w", CodeProfileFile, err)
	}
	p.limit = timeout
	return p, nil
}

// RulesHash is the SHA-256 of profile.json, the linter configuration and the clone configuration.
func (p *CodeProfile) RulesHash() string { return p.rulesHash }

// CodeReports are the files a code profile writes for one component.
type CodeReports struct {
	Lint, Functions, Clones string
}

// codeStep is one tool on one component: its run record and the commands that produce the report.
type codeStep struct {
	tool     Tool
	commands []Tool
	finish   func() error
	// emptyOK accepts an empty report: a component without functions gives an empty Lizard CSV.
	emptyOK bool
}

func (p *CodeProfile) steps(root string, s *Scope, prefix, work string, out CodeReports) ([]codeStep, error) {
	lint, err := p.lintStep(root, s, work, out.Lint)
	if err != nil {
		return nil, err
	}
	functions, err := p.functionsStep(root, s, work, out.Functions)
	if err != nil {
		return nil, err
	}
	clones, err := p.clonesStep(root, s, prefix, work, out.Clones)
	if err != nil {
		return nil, err
	}
	return []codeStep{lint, functions, clones}, nil
}

func (p *CodeProfile) tool(name, format, formatVersion, report, config string) Tool {
	return Tool{
		ID: p.ID + "-" + name, ProfileID: p.ID, Kind: string(envelope.ScanReportReportKindCode), Format: format,
		FormatVersion: formatVersion, Producer: name, ProducerVersion: p.Tools[name], Report: report,
		ConfigFile: config, RulesHash: p.rulesHash, FindingsCodes: []int{1},
	}
}

// lintStep runs golangci-lint in every module of the component; their SARIF runs join in one report.
func (p *CodeProfile) lintStep(root string, s *Scope, work, report string) (codeStep, error) {
	config, err := filepath.Abs(filepath.Join(p.dir, LintConfigFile))
	if err != nil {
		return codeStep{}, fmt.Errorf("resolve linter configuration: %w", err)
	}
	step := codeStep{tool: p.tool(ToolLint, string(envelope.ScanReportReportFormatSarif), "2.1.0", report, config)}
	parts := make([]string, 0, len(s.Modules))
	for i, m := range s.Modules {
		part := filepath.Join(work, "lint-"+strconv.Itoa(i+1)+".sarif")
		parts = append(parts, part)
		cmd := []string{ToolLint, "run", "--config", config, "--output.text.path", "stdout", "--output.sarif.path", part}
		if m.Dir != "." {
			cmd = append(cmd, "--path-prefix", m.Dir)
		}
		step.commands = append(step.commands, Tool{Command: append(cmd, m.Pattern), Dir: filepath.Join(root, filepath.FromSlash(m.Dir)), FindingsCodes: []int{1}})
	}
	step.finish = func() error {
		if len(parts) == 0 {
			return errors.New("the component is in no Go module")
		}
		return mergeSARIF(parts, report)
	}
	return step, nil
}

// functionsStep runs Lizard on the files of the scope from the repository root.
func (p *CodeProfile) functionsStep(root string, s *Scope, work, report string) (codeStep, error) {
	list := filepath.Join(work, "functions.txt")
	if err := os.WriteFile(list, []byte(strings.Join(s.Files, "\n")+"\n"), 0o600); err != nil {
		return codeStep{}, fmt.Errorf("write file list: %w", err)
	}
	step := codeStep{tool: p.tool(ToolFunctions, string(envelope.ScanReportReportFormatLizardCsv), "1", report, ""), emptyOK: true}
	step.tool.FindingsCodes = nil
	step.commands = []Tool{{Command: []string{ToolFunctions, "--csv", "--no-gitignore", "-l", "go", "-f", list, "-o", report}, Dir: root}}
	return step, nil
}

// clonesStep runs jscpd from the repository root on the component; the clone report keeps the
// coordinates of copies, not their code.
func (p *CodeProfile) clonesStep(root string, s *Scope, prefix, work, report string) (codeStep, error) {
	base := filepath.Join(p.dir, ClonesFile)
	data, err := os.ReadFile(base) //nolint:gosec // the image names its profile directory
	if err != nil {
		return codeStep{}, fmt.Errorf("read clone configuration: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return codeStep{}, fmt.Errorf("decode %s: %w", ClonesFile, err)
	}
	pattern := "**/*.go"
	if prefix != "" {
		pattern = prefix + "/" + pattern
	}
	ignore := make([]string, 0, len(s.Hidden)+len(s.Excluded))
	for _, dir := range s.Hidden {
		ignore = append(ignore, dir+"/**")
	}
	for _, e := range s.Excluded {
		if e.Reason == envelope.ScanExcludedTargetReasonGenerated {
			ignore = append(ignore, e.Path)
			continue
		}
		ignore = append(ignore, e.Path+"/**")
	}
	config["pattern"], config["ignore"] = pattern, ignore
	generated := filepath.Join(work, "jscpd.json")
	if data, err = json.Marshal(config); err != nil {
		return codeStep{}, fmt.Errorf("encode clone configuration: %w", err)
	}
	if err := os.WriteFile(generated, data, 0o600); err != nil {
		return codeStep{}, fmt.Errorf("write clone configuration: %w", err)
	}
	output := filepath.Join(work, "clones")
	step := codeStep{tool: p.tool(ToolClones, string(envelope.ScanReportReportFormatJscpdJson), "1", report, base)}
	step.tool.FindingsCodes = nil
	step.commands = []Tool{{Command: []string{ToolClones, "--config", generated, "--no-gitignore", "--no-colors", "--reporters", "json", "--output", output, "."}, Dir: root}}
	step.finish = func() error { return stripClones(filepath.Join(output, "jscpd-report.json"), report) }
	return step, nil
}

// execute runs the commands of a step until the deadline and gives the outcome of the tool.
func (r *Runner) execute(ctx context.Context, step *codeStep, deadline time.Time) (toolOutcome, error) {
	findings := false
	for i := range step.commands {
		c := step.commands[i]
		if c.Timeout = deadline.Sub(r.Now()); c.Timeout <= 0 {
			return outcome(true, false, 0, nil), nil
		}
		code, timedOut, err := r.exec(ctx, &c)
		switch {
		case ctx.Err() != nil:
			return toolOutcome{}, ctx.Err()
		case timedOut:
			return outcome(true, false, 0, nil), nil
		case err != nil:
			return outcome(false, false, 0, nil), nil //nolint:nilerr // a tool that cannot start is a failed run, not a failed step
		case code != 0 && !slices.Contains(c.FindingsCodes, code):
			return outcome(false, false, code, nil), nil
		case code != 0:
			findings = true
		}
	}
	if step.finish != nil {
		if err := step.finish(); err != nil {
			fmt.Fprintf(r.Stderr, "craftpulse-ci: %s: %v\n", step.tool.Producer, err)
			return outcome(false, false, 0, nil), nil
		}
	}
	info, err := os.Stat(step.tool.Report)
	finished := err == nil && info.Mode().IsRegular() && (info.Size() > 0 || step.emptyOK)
	code := 0
	if findings {
		code = 1
	}
	return outcome(false, finished, code, step.tool.FindingsCodes), nil
}

// CodeRunner runs a code profile on the rows of the job: three tool runs per component.
type CodeRunner struct {
	*Runner
	Code *CodeProfile
	// Root is the repository checkout, CI_PROJECT_DIR in GitLab.
	Root string
	// Timeout overrides the time limit of the profile.
	Timeout time.Duration
}

// Run analyzes every component of the rows within one deadline and records the tool runs.
func (c *CodeRunner) Run(ctx context.Context, rows []Entry) ([]envelope.ScanToolRun, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("profile has no builtin code check of profile %s", c.Code.ID)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = c.Code.limit
	}
	deadline := c.Now().Add(timeout)
	var runs []envelope.ScanToolRun
	for _, row := range rows {
		if row.ImageDigest == nil {
			return nil, fmt.Errorf("profile row of %s has no image digest", c.Code.ID)
		}
		suffix := ""
		if len(rows) > 1 {
			suffix = "-" + row.ComponentID.String()
		}
		got, err := c.component(ctx, row, suffix, deadline)
		if err != nil {
			return runs, err
		}
		runs = append(runs, got...)
	}
	return runs, nil
}

func (c *CodeRunner) component(ctx context.Context, row Entry, suffix string, deadline time.Time) ([]envelope.ScanToolRun, error) {
	scope, err := GoScope(c.Root, row.PathPrefix)
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "craftpulse-code-")
	if err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	out := CodeReports{
		Lint: filepath.Join(work, "lint.sarif"), Functions: filepath.Join(work, "functions.csv"), Clones: filepath.Join(work, "clones.json"),
	}
	steps, err := c.Code.steps(c.Root, scope, row.PathPrefix, work, out)
	if err != nil {
		return nil, err
	}
	runs := make([]envelope.ScanToolRun, 0, len(steps))
	for i := range steps {
		step := &steps[i]
		step.tool.ID += suffix
		run, err := c.toolRun(&step.tool, row)
		if err != nil {
			return runs, err
		}
		setScope(run, scope)
		run.StartedAt = c.Now().UTC()
		o, err := c.execute(ctx, step, deadline)
		if err != nil {
			return runs, err
		}
		run.FinishedAt = c.Now().UTC()
		if err := c.record(run, &step.tool, o); err != nil {
			return runs, err
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

func setScope(run *envelope.ScanToolRun, s *Scope) {
	scanned, skipped := len(s.Files), 0
	excluded, tests := s.Excluded, s.Tests
	if excluded == nil {
		excluded = []envelope.ScanExcludedTarget{}
	}
	if tests == nil {
		tests = []string{}
	}
	run.ScannedCount, run.SkippedCount = &scanned, &skipped
	run.ExcludedTargets, run.TestTargets = &excluded, &tests
}

// FixtureScope is the scope a fixture run reports: what it marks as test code and what it excludes.
type FixtureScope struct {
	Tests    []string                      `json:"tests"`
	Excluded []envelope.ScanExcludedTarget `json:"excluded"`
}

// Fixture runs the profile on the current directory as one component and writes the reports where the
// fixtures command expects them. Any tool that does not finish fails the case.
func (p *CodeProfile) Fixture(ctx context.Context, stdout, stderr io.Writer, out CodeReports, scopeReport string) error {
	scope, err := GoScope(".", "")
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "craftpulse-code-")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	steps, err := p.steps(".", scope, "", work, out)
	if err != nil {
		return err
	}
	r := &Runner{Stdout: stdout, Stderr: stderr, Now: time.Now}
	deadline := time.Now().Add(p.limit)
	for i := range steps {
		o, err := r.execute(ctx, &steps[i], deadline)
		if err != nil {
			return err
		}
		if o.completeness != envelope.ScanToolRunCompletenessComplete {
			return fmt.Errorf("%s: %s", steps[i].tool.Producer, o.exit)
		}
	}
	data, err := json.Marshal(FixtureScope{Tests: scope.Tests, Excluded: scope.Excluded})
	if err != nil {
		return fmt.Errorf("encode fixture scope: %w", err)
	}
	return os.WriteFile(scopeReport, data, 0o600)
}

// CodeRows are the builtin code rows of the profile for this job.
func CodeRows(p *ciapi.CiCheckProfile, profileID, component string) []Entry {
	return Select(p, Filter{Kind: string(envelope.ScanReportReportKindCode), Mode: string(ciapi.CiCheckProfileEntriesModeBuiltin), ProfileID: profileID, ComponentID: component})
}
