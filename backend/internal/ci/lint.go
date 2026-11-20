package ci

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _ruffSyntaxError is the code Ruff gives a file it cannot parse.
const _ruffSyntaxError = "invalid-syntax"

// lintResult is one finding of a linter in the SARIF report: rule, level, message and region.
type lintResult struct {
	rule, level, message, file string
	line, column, endLine      int
	endColumn                  int
}

// lintChunks runs the linter command on the files of the scope, at most _lintChunk files a command; each
// command writes its own output file. Paths start with "./" so a file name is never taken for a flag.
func lintChunks(root string, s *Scope, work string, command func(output string) []string) (commands []Tool, outputs []string) {
	for i := 0; i < len(s.Files); i += _lintChunk {
		output := filepath.Join(work, "lint-"+strconv.Itoa(i/_lintChunk+1)+".json")
		outputs = append(outputs, output)
		cmd := command(output)
		for _, f := range s.Files[i:min(i+_lintChunk, len(s.Files))] {
			cmd = append(cmd, "./"+f)
		}
		commands = append(commands, Tool{Command: cmd, Dir: root, FindingsCodes: []int{1}})
	}
	return commands, outputs
}

// ruffStep runs Ruff with the configuration of the profile only: a configuration of the repository is not
// read, Ruff writes no cache into the checkout and fixes nothing.
func (p *CodeProfile) ruffStep(root string, s *Scope, work, report string) (codeStep, error) {
	config, err := p.lintConfig()
	if err != nil {
		return codeStep{}, err
	}
	step := codeStep{tool: p.tool(ToolRuff, string(envelope.ScanReportReportFormatSarif), _sarifVersion, report, config)}
	commands, outputs := lintChunks(root, s, work, func(output string) []string {
		return []string{ToolRuff, "check", "--config", config, "--no-cache", "--no-fix", "--output-format", "json", "--output-file", output}
	})
	step.commands = commands
	step.finish = func() ([]envelope.ScanSkippedTarget, error) {
		results, skipped, err := readRuff(outputs, root)
		if err != nil {
			return nil, err
		}
		return skipped, writeLintSARIF(report, ToolRuff, p.Tools[ToolRuff], "https://docs.astral.sh/ruff/", results)
	}
	return step, nil
}

// eslintStep runs ESLint with the configuration and plugins of the profile: a configuration of the
// repository is not looked up, so no code of the repository runs.
func (p *CodeProfile) eslintStep(root string, s *Scope, work, report string) (codeStep, error) {
	config, err := p.lintConfig()
	if err != nil {
		return codeStep{}, err
	}
	step := codeStep{tool: p.tool(ToolESLint, string(envelope.ScanReportReportFormatSarif), _sarifVersion, report, config)}
	step.tool.ParserVersion = ToolTypeScriptESLint + " " + p.Tools[ToolTypeScriptESLint] + ", " + ToolTypeScript + " " + p.Tools[ToolTypeScript]
	commands, outputs := lintChunks(root, s, work, func(output string) []string {
		return []string{ToolESLint, "--config", config, "--format", "json", "--output-file", output, "--no-warn-ignored"}
	})
	step.commands = commands
	step.finish = func() ([]envelope.ScanSkippedTarget, error) {
		results, skipped, err := readESLint(outputs, root)
		if err != nil {
			return nil, err
		}
		return skipped, writeLintSARIF(report, ToolESLint, p.Tools[ToolESLint], "https://eslint.org/", results)
	}
	return step, nil
}

type ruffDiagnostic struct {
	Code     *string `json:"code"`
	Message  string  `json:"message"`
	Filename string  `json:"filename"`
	Location struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
	EndLocation struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"end_location"` //nolint:tagliatelle // Ruff wire format.
}

// readRuff reads the JSON outputs of Ruff: a diagnostic with a rule is a finding, a file Ruff cannot parse is
// skipped as unsupported and its other diagnostics are dropped.
func readRuff(outputs []string, root string) ([]lintResult, []envelope.ScanSkippedTarget, error) {
	var results []lintResult
	broken := map[string]bool{}
	for _, name := range outputs {
		var diagnostics []ruffDiagnostic
		if err := readJSON(name, &diagnostics); err != nil {
			return nil, nil, fmt.Errorf("output of ruff: %w", err)
		}
		for _, d := range diagnostics {
			file, err := repoPath(d.Filename, root)
			if err != nil {
				return nil, nil, err
			}
			if d.Code == nil || *d.Code == _ruffSyntaxError {
				broken[file] = true
				continue
			}
			results = append(results, lintResult{
				rule: *d.Code, level: "error", message: d.Message, file: file,
				line: d.Location.Row, column: d.Location.Column, endLine: d.EndLocation.Row, endColumn: d.EndLocation.Column,
			})
		}
	}
	results, skipped := dropBroken(results, broken)
	return results, skipped, nil
}

type eslintFile struct {
	FilePath string `json:"filePath"` //nolint:tagliatelle // ESLint wire format.
	Messages []struct {
		RuleID    *string `json:"ruleId"` //nolint:tagliatelle // ESLint wire format.
		Fatal     bool    `json:"fatal"`
		Severity  int     `json:"severity"`
		Message   string  `json:"message"`
		Line      int     `json:"line"`
		Column    int     `json:"column"`
		EndLine   int     `json:"endLine"`   //nolint:tagliatelle // ESLint wire format.
		EndColumn int     `json:"endColumn"` //nolint:tagliatelle // ESLint wire format.
	} `json:"messages"`
}

// readESLint reads the JSON outputs of ESLint: a message of a rule is a finding, a file with a fatal parsing
// error is skipped as unsupported. The source of the files in the output is not read.
func readESLint(outputs []string, root string) ([]lintResult, []envelope.ScanSkippedTarget, error) {
	var results []lintResult
	broken := map[string]bool{}
	for _, name := range outputs {
		var files []eslintFile
		if err := readJSON(name, &files); err != nil {
			return nil, nil, fmt.Errorf("output of eslint: %w", err)
		}
		for _, f := range files {
			file, err := repoPath(f.FilePath, root)
			if err != nil {
				return nil, nil, err
			}
			for _, m := range f.Messages {
				if m.Fatal || m.RuleID == nil {
					broken[file] = true
					continue
				}
				level := "warning"
				if m.Severity == 2 {
					level = "error"
				}
				results = append(results, lintResult{
					rule: *m.RuleID, level: level, message: m.Message, file: file,
					line: m.Line, column: m.Column, endLine: m.EndLine, endColumn: m.EndColumn,
				})
			}
		}
	}
	results, skipped := dropBroken(results, broken)
	return results, skipped, nil
}

// dropBroken leaves out the findings of the files the linter could not parse and lists those files.
func dropBroken(results []lintResult, broken map[string]bool) ([]lintResult, []envelope.ScanSkippedTarget) {
	results = slices.DeleteFunc(results, func(r lintResult) bool { return broken[r.file] })
	skipped := make([]envelope.ScanSkippedTarget, 0, len(broken))
	for file := range broken {
		skipped = append(skipped, envelope.ScanSkippedTarget{Path: file, Reason: envelope.ScanSkippedTargetReasonUnsupported})
	}
	slices.SortFunc(skipped, func(a, b envelope.ScanSkippedTarget) int { return strings.Compare(a.Path, b.Path) })
	return results, skipped
}

func readJSON(name string, v any) error {
	data, err := os.ReadFile(name) //nolint:gosec // the step names its own output
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// repoPath makes a path of a linter output relative to the repository root.
func repoPath(name, root string) (string, error) {
	if !filepath.IsAbs(name) {
		return path.Clean(filepath.ToSlash(name)), nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	rel, err := filepath.Rel(abs, name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("linter reported %s outside the repository", name)
	}
	return filepath.ToSlash(rel), nil
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`             //nolint:tagliatelle // SARIF wire format.
	StartColumn int `json:"startColumn,omitempty"` //nolint:tagliatelle // SARIF wire format.
	EndLine     int `json:"endLine,omitempty"`     //nolint:tagliatelle // SARIF wire format.
	EndColumn   int `json:"endColumn,omitempty"`   //nolint:tagliatelle // SARIF wire format.
}

type sarifResult struct {
	RuleID  string `json:"ruleId"` //nolint:tagliatelle // SARIF wire format.
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"` //nolint:tagliatelle // SARIF wire format.
		Region sarifRegion `json:"region"`
	} `json:"physicalLocation"` //nolint:tagliatelle // SARIF wire format.
}

type sarifRule struct {
	ID string `json:"id"`
}

// writeLintSARIF writes the findings as one SARIF 2.1.0 run of the linter, results in file and line order.
func writeLintSARIF(dst, tool, version, uri string, results []lintResult) error {
	slices.SortFunc(results, func(a, b lintResult) int {
		return cmp.Or(strings.Compare(a.file, b.file), cmp.Compare(a.line, b.line), cmp.Compare(a.column, b.column), strings.Compare(a.rule, b.rule))
	})
	out := make([]sarifResult, 0, len(results))
	var ruleIDs []string
	for _, r := range results {
		res := sarifResult{RuleID: r.rule, Level: r.level}
		res.Message.Text = r.message
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI = r.file
		loc.PhysicalLocation.Region = sarifRegion{StartLine: max(r.line, 1), StartColumn: r.column, EndLine: r.endLine, EndColumn: r.endColumn}
		res.Locations = []sarifLocation{loc}
		out = append(out, res)
		ruleIDs = append(ruleIDs, r.rule)
	}
	slices.Sort(ruleIDs)
	ruleIDs = slices.Compact(ruleIDs)
	rules := make([]sarifRule, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		rules = append(rules, sarifRule{ID: id})
	}
	type driver struct {
		Name           string      `json:"name"`
		Version        string      `json:"version"`
		InformationURI string      `json:"informationUri"` //nolint:tagliatelle // SARIF wire format.
		Rules          []sarifRule `json:"rules"`
	}
	type run struct {
		Tool struct {
			Driver driver `json:"driver"`
		} `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	r := run{Results: out}
	r.Tool.Driver = driver{Name: tool, Version: version, InformationURI: uri, Rules: rules}
	data, err := json.Marshal(struct {
		Version string `json:"version"`
		Schema  string `json:"$schema"` //nolint:tagliatelle // SARIF wire format.
		Runs    []run  `json:"runs"`
	}{Version: _sarifVersion, Schema: "https://json.schemastore.org/sarif-2.1.0.json", Runs: []run{r}})
	if err != nil {
		return fmt.Errorf("encode linter report: %w", err)
	}
	return os.WriteFile(dst, data, 0o600)
}
