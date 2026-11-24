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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Files of a SAST profile directory.
const (
	SASTSourcesFile = "sources.json"
	SASTRulesDir    = "rules"
	ToolSAST        = "opengrep"
)

// _distributable are the licenses a rule may have to ship in the image; the rules of the project are its own.
var _distributable = []string{"LicenseRef-craftPulse", "MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause"}

// SASTProfile is a SAST profile of an image: the Opengrep version, the time limits, the excluded directories and
// the rule set with the source and the license of every rule.
type SASTProfile struct {
	ID           string            `json:"profile_id"`
	Timeout      string            `json:"timeout"`
	FileTimeout  int               `json:"file_timeout_seconds"`
	Tools        map[string]string `json:"tools"`
	ExcludedDirs map[string]string `json:"excluded_dirs"`

	dir       string
	limit     time.Duration
	rulesHash string
	sources   []RuleSource
}

// RuleSource is where a rule of the set comes from and under which license it ships.
type RuleSource struct {
	ID            string `json:"id"`
	File          string `json:"file"`
	Source        string `json:"source"`
	SourceVersion string `json:"source_version"`
	License       string `json:"license"`
}

// LoadSASTProfile reads and checks the profile in dir: every rule of the rule files has a source with a
// distributable license, and every source names a rule of its file. The rules hash covers profile.json,
// sources.json and the rule files in name order.
func LoadSASTProfile(dir string) (*SASTProfile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, CodeProfileFile)) //nolint:gosec // the image names its profile directory
	if err != nil {
		return nil, fmt.Errorf("read SAST profile: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve SAST profile directory: %w", err)
	}
	p := &SASTProfile{dir: abs}
	if err := decodeStrict(raw, p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", CodeProfileFile, err)
	}
	sourcesRaw, err := os.ReadFile(filepath.Join(dir, SASTSourcesFile)) //nolint:gosec // the image names its profile directory
	if err != nil {
		return nil, fmt.Errorf("read rule sources: %w", err)
	}
	var sources struct {
		Rules []RuleSource `json:"rules"`
	}
	if err := decodeStrict(sourcesRaw, &sources); err != nil {
		return nil, fmt.Errorf("decode %s: %w", SASTSourcesFile, err)
	}
	p.sources = sources.Rules
	files, err := filepath.Glob(filepath.Join(dir, SASTRulesDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no rule files in %s", SASTRulesDir)
	}
	sort.Strings(files)
	h := sha256.New()
	h.Write(raw)
	h.Write(sourcesRaw)
	rules := map[string]string{}
	var problems []error
	for _, name := range files {
		data, err := os.ReadFile(name) //nolint:gosec // the rule files of the profile directory
		if err != nil {
			return nil, fmt.Errorf("read rule file: %w", err)
		}
		h.Write(data)
		ids, err := ruleIDs(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(name), err)
		}
		rel := SASTRulesDir + "/" + filepath.Base(name)
		for _, id := range ids {
			if _, dup := rules[id]; dup {
				problems = append(problems, fmt.Errorf("rule %s is defined twice", id))
			}
			rules[id] = rel
		}
	}
	p.rulesHash = hex.EncodeToString(h.Sum(nil))
	problems = append(problems, p.check(rules)...)
	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("SAST profile %s: %w", p.ID, err)
	}
	return p, nil
}

func (p *SASTProfile) check(rules map[string]string) []error {
	var problems []error
	if p.ID == "" {
		problems = append(problems, errors.New("profile_id is required"))
	}
	if p.Tools[ToolSAST] == "" {
		problems = append(problems, errors.New("tools.opengrep: version is required"))
	}
	limit, err := time.ParseDuration(p.Timeout)
	if err != nil || limit <= 0 || p.FileTimeout <= 0 {
		problems = append(problems, errors.New("timeout and file_timeout_seconds must be positive"))
	}
	p.limit = limit
	for dir, reason := range p.ExcludedDirs {
		if !envelope.ScanExcludedTargetReason(reason).Valid() || strings.ContainsAny(dir, "/*") {
			problems = append(problems, fmt.Errorf("excluded_dirs.%s: a directory name with vendor, generated or build", dir))
		}
	}
	seen := map[string]bool{}
	for _, s := range p.sources {
		switch {
		case seen[s.ID]:
			problems = append(problems, fmt.Errorf("rule %s has two sources", s.ID))
		case rules[s.ID] != s.File:
			problems = append(problems, fmt.Errorf("rule %s is not in %s", s.ID, s.File))
		case s.Source == "" || s.SourceVersion == "":
			problems = append(problems, fmt.Errorf("rule %s has no source or source version", s.ID))
		case !slices.Contains(_distributable, s.License):
			problems = append(problems, fmt.Errorf("rule %s has license %q that does not allow distribution", s.ID, s.License))
		}
		seen[s.ID] = true
	}
	for id := range rules {
		if !seen[id] {
			problems = append(problems, fmt.Errorf("rule %s has no source", id))
		}
	}
	return problems
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// _ruleID is the line that opens a rule in a rule file of the profile.
var _ruleID = regexp.MustCompile(`(?m)^\s*- id:\s*(\S+)\s*$`)

// ruleIDs reads the IDs of the rules of a file; Opengrep prefixes an ID with the path of its file, so an ID with a
// dot could not be told from the prefix.
func ruleIDs(data []byte) ([]string, error) {
	matches := _ruleID.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return nil, errors.New("no rules")
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		id := string(m[1])
		if strings.Contains(id, ".") {
			return nil, fmt.Errorf("rule id %q must have no dots", id)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// RulesHash is the SHA-256 of profile.json, sources.json and the rule files.
func (p *SASTProfile) RulesHash() string { return p.rulesHash }

// Sources are the sources and licenses of the rules, in the order of sources.json.
func (p *SASTProfile) Sources() []RuleSource { return slices.Clone(p.sources) }

// sastScan is one Opengrep run on a target of the repository and what it found out of the analysis.
type sastScan struct {
	excluded []envelope.ScanExcludedTarget
	skipped  []envelope.ScanSkippedTarget
	scanned  int
}

// excludedDirs walks the target for the excluded directories of the profile and the directories starting with
// "."; Opengrep gets them as exclusions instead of its own default list.
func (p *SASTProfile) excludedDirs(root, target string) ([]envelope.ScanExcludedTarget, []string, error) {
	return excludedDirsOf(root, target, p.ExcludedDirs)
}

// excludedDirsOf walks the target for the directories with a name of dirs and the directories starting with ".";
// paths are relative to the root.
func excludedDirsOf(root, target string, dirs map[string]string) ([]envelope.ScanExcludedTarget, []string, error) {
	var excluded []envelope.ScanExcludedTarget
	var hidden []string
	base := filepath.Join(root, filepath.FromSlash(target))
	err := filepath.WalkDir(base, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || name == base {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") {
			hidden = append(hidden, rel)
			return filepath.SkipDir
		}
		if reason, ok := dirs[d.Name()]; ok {
			excluded = append(excluded, envelope.ScanExcludedTarget{Path: rel, Reason: envelope.ScanExcludedTargetReason(reason)})
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk %q: %w", target, err)
	}
	return excluded, hidden, nil
}

// command is the Opengrep run on the target from the repository root: the rule files of the profile, our
// exclusions instead of the default ignore list, no limit of file size so nothing is dropped silently.
func (p *SASTProfile) command(target, sarif, details string, excluded []envelope.ScanExcludedTarget, hidden []string) []string {
	cmd := []string{
		ToolSAST, "scan", "--config", filepath.Join(p.dir, SASTRulesDir), "--sarif-output=" + sarif, "--json-output=" + details,
		"--error", "--quiet", "--disable-version-check", "--no-git-ignore", "--x-ignore-semgrepignore-files",
		"--max-target-bytes=0", "--timeout=" + strconv.Itoa(p.FileTimeout),
	}
	for _, e := range excluded {
		cmd = append(cmd, "--exclude="+e.Path)
	}
	for _, h := range hidden {
		cmd = append(cmd, "--exclude="+h)
	}
	return append(cmd, target)
}

type opengrepDetails struct {
	Errors []struct {
		Type    any    `json:"type"`
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"errors"`
	Paths struct {
		Scanned []string `json:"scanned"`
	} `json:"paths"`
}

// readDetails lists the files Opengrep did not analyze in full: a file that ran out of time is a timeout,
// a file it could not parse is unsupported.
func readDetails(name string) (scanned int, skipped []envelope.ScanSkippedTarget, err error) {
	data, err := os.ReadFile(name) //nolint:gosec // the run names its own output
	if err != nil {
		return 0, nil, fmt.Errorf("read Opengrep output: %w", err)
	}
	var d opengrepDetails
	if err := json.Unmarshal(data, &d); err != nil {
		return 0, nil, fmt.Errorf("output of opengrep is not JSON: %w", err)
	}
	seen := map[string]bool{}
	for _, e := range d.Errors {
		target := path.Clean(e.Path)
		if e.Path == "" || seen[target] {
			continue
		}
		seen[target] = true
		reason := envelope.ScanSkippedTargetReasonUnsupported
		if kind, _ := e.Type.(string); strings.Contains(strings.ToLower(kind), "timeout") {
			reason = envelope.ScanSkippedTargetReasonTimeout
		}
		skipped = append(skipped, envelope.ScanSkippedTarget{Path: target, Reason: reason})
	}
	slices.SortFunc(skipped, func(a, b envelope.ScanSkippedTarget) int { return strings.Compare(a.Path, b.Path) })
	return len(d.Paths.Scanned), skipped, nil
}

// cleanSARIF writes the SARIF report with the bare IDs of the rules and without the code of the findings:
// Opengrep prefixes a rule with the path of its file and copies the matched lines into the report.
func cleanSARIF(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // the run names its own report
	if err != nil {
		return fmt.Errorf("read SARIF report: %w", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("report is not SARIF JSON: %w", err)
	}
	if report["version"] != _sarifVersion {
		return fmt.Errorf("report has SARIF version %v", report["version"])
	}
	runs, _ := report["runs"].([]any)
	for _, r := range runs {
		run, _ := r.(map[string]any)
		tool, _ := run["tool"].(map[string]any)
		driver, _ := tool["driver"].(map[string]any)
		rules, _ := driver["rules"].([]any)
		for _, rule := range rules {
			if m, ok := rule.(map[string]any); ok {
				m["id"], m["name"] = bareRule(m["id"]), bareRule(m["name"])
			}
		}
		results, _ := run["results"].([]any)
		for _, res := range results {
			m, ok := res.(map[string]any)
			if !ok {
				continue
			}
			m["ruleId"] = bareRule(m["ruleId"])
			locations, _ := m["locations"].([]any)
			for _, l := range locations {
				loc, _ := l.(map[string]any)
				physical, _ := loc["physicalLocation"].(map[string]any)
				if region, ok := physical["region"].(map[string]any); ok {
					delete(region, "snippet")
				}
			}
		}
	}
	if data, err = json.Marshal(report); err != nil {
		return fmt.Errorf("encode SARIF report: %w", err)
	}
	return os.WriteFile(dst, data, 0o600)
}

func bareRule(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	return s[strings.LastIndex(s, ".")+1:]
}

// scan runs Opengrep on the target until the deadline and writes the clean SARIF report.
func (p *SASTProfile) scan(ctx context.Context, r *Runner, root, target, report string, deadline time.Time) (sastScan, toolOutcome, error) {
	var s sastScan
	excluded, hidden, err := p.excludedDirs(root, target)
	if err != nil {
		return s, toolOutcome{}, err
	}
	s.excluded = excluded
	work, err := os.MkdirTemp("", "craftpulse-sast-")
	if err != nil {
		return s, toolOutcome{}, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	raw, details := filepath.Join(work, "raw.sarif"), filepath.Join(work, "details.json")
	t := Tool{Command: p.command(target, raw, details, excluded, hidden), Dir: root, Timeout: deadline.Sub(r.Now())}
	if t.Timeout <= 0 {
		return s, outcome(true, false, 0, nil), nil
	}
	code, timedOut, err := r.exec(ctx, &t)
	switch {
	case ctx.Err() != nil:
		return s, toolOutcome{}, ctx.Err()
	case timedOut:
		return s, outcome(true, false, 0, nil), nil
	case err != nil, code != 0 && code != 1:
		return s, outcome(false, false, code, nil), nil //nolint:nilerr // a tool that cannot start is a failed run, not a failed step
	}
	if s.scanned, s.skipped, err = readDetails(details); err == nil {
		err = cleanSARIF(raw, report)
	}
	if err != nil {
		fmt.Fprintf(r.Stderr, "craftpulse-ci: opengrep: %v\n", err)
		return s, outcome(false, false, 0, nil), nil
	}
	o := outcome(false, true, code, []int{1})
	if len(s.skipped) > 0 {
		o = toolOutcome{envelope.ScanToolRunExitClassPartial, envelope.ScanToolRunCompletenessPartial}
	}
	return s, o, nil
}

// SASTRunner runs a SAST profile on the rows of the job: one tool run per component.
type SASTRunner struct {
	*Runner
	SAST *SASTProfile
	// Root is the repository checkout, CI_PROJECT_DIR in GitLab.
	Root string
	// Timeout overrides the time limit of the profile.
	Timeout time.Duration
}

// Run analyzes every component of the rows within one deadline and records the tool runs.
func (s *SASTRunner) Run(ctx context.Context, rows []Entry) ([]envelope.ScanToolRun, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("profile has no builtin sast check of profile %s", s.SAST.ID)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = s.SAST.limit
	}
	deadline := s.Now().Add(timeout)
	runs := make([]envelope.ScanToolRun, 0, len(rows))
	for _, row := range rows {
		run, err := s.component(ctx, row, len(rows) > 1, deadline)
		if err != nil {
			return runs, err
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

func (s *SASTRunner) component(ctx context.Context, row Entry, several bool, deadline time.Time) (*envelope.ScanToolRun, error) {
	p := s.SAST
	work, err := os.MkdirTemp("", "craftpulse-sast-report-")
	if err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	t := Tool{
		ID: p.ID + "-" + ToolSAST, ProfileID: p.ID, Kind: string(envelope.ScanReportReportKindSast), Format: string(envelope.ScanReportReportFormatSarif),
		FormatVersion: _sarifVersion, Producer: ToolSAST, ProducerVersion: p.Tools[ToolSAST], Report: filepath.Join(work, "sast.sarif"),
		ConfigFile: filepath.Join(p.dir, CodeProfileFile), RulesHash: p.rulesHash,
	}
	if several {
		t.ID += "-" + row.ComponentID.String()
	}
	if row.ImageDigest == nil {
		return nil, fmt.Errorf("profile row of %s has no image digest", p.ID)
	}
	run, err := s.toolRun(&t, row)
	if err != nil {
		return nil, err
	}
	target := row.PathPrefix
	if target == "" {
		target = "."
	}
	run.StartedAt = s.Now().UTC()
	scan, o, err := p.scan(ctx, s.Runner, s.Root, target, t.Report, deadline)
	if err != nil {
		return nil, err
	}
	run.FinishedAt = s.Now().UTC()
	skipped := len(scan.skipped)
	if o.finished() {
		run.ScannedCount, run.SkippedCount, run.SkippedTargets = &scan.scanned, &skipped, scan.skipped
		run.EligibleFileCount = new(scan.scanned + skipped)
	}
	excluded := append([]envelope.ScanExcludedTarget{}, scan.excluded...)
	run.ExcludedTargets = &excluded
	if run.SkippedTargets == nil {
		run.SkippedTargets = []envelope.ScanSkippedTarget{}
	}
	return run, s.record(run, &t, o)
}

// Fixture runs the profile on the current directory and writes the SARIF report where the fixtures command
// expects it; a run that does not finish fails the case.
func (p *SASTProfile) Fixture(ctx context.Context, stdout, stderr io.Writer, report string) error {
	r := &Runner{Stdout: stdout, Stderr: stderr, Now: time.Now}
	_, o, err := p.scan(ctx, r, ".", ".", report, time.Now().Add(p.limit))
	if err != nil {
		return err
	}
	if !o.finished() {
		return fmt.Errorf("opengrep: %s", o.exit)
	}
	return nil
}

// SASTRows are the builtin sast rows of the profile for this job.
func SASTRows(p *ciapi.CiCheckProfile, profileID, component string) []Entry {
	return Select(p, Filter{Kind: string(envelope.ScanReportReportKindSast), Mode: string(ciapi.CiCheckProfileEntriesModeBuiltin), ProfileID: profileID, ComponentID: component})
}
