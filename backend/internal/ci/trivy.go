package ci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Files of a Trivy profile directory and the tool.
const (
	TrivyConfigFile = "trivy.yaml"
	TrivyIgnoreFile = "trivyignore"
	ToolTrivy       = "trivy"
)

// Marks of a package instance of the head scan relative to the lockfiles of the base commit (S16).
const (
	DeltaAdded   = "added"
	DeltaChanged = "changed"
)

// TrivyProfile is the Trivy profile of an image: the Trivy version, the time limit, the configuration kinds
// of the IaC checks and the excluded directories.
type TrivyProfile struct {
	ID           string            `json:"profile_id"`
	Timeout      string            `json:"timeout"`
	Tools        map[string]string `json:"tools"`
	IaCTypes     []string          `json:"iac_types"`
	ExcludedDirs map[string]string `json:"excluded_dirs"`

	dir       string
	limit     time.Duration
	rulesHash string
}

// LoadTrivyProfile reads and checks the profile in dir. The rules hash covers profile.json, trivy.yaml and
// trivyignore with the checks the profile leaves out; the checks themselves are embedded in the Trivy binary of
// the version the profile names.
func LoadTrivyProfile(dir string) (*TrivyProfile, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve Trivy profile directory: %w", err)
	}
	h := sha256.New()
	var raw []byte
	for _, name := range []string{CodeProfileFile, TrivyConfigFile, TrivyIgnoreFile} {
		data, err := os.ReadFile(filepath.Join(abs, name)) //nolint:gosec // the image names its profile directory
		if err != nil {
			return nil, fmt.Errorf("read Trivy profile: %w", err)
		}
		if name == CodeProfileFile {
			raw = data
		}
		h.Write(data)
	}
	p := &TrivyProfile{dir: abs, rulesHash: hex.EncodeToString(h.Sum(nil))}
	if err := decodeStrict(raw, p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", CodeProfileFile, err)
	}
	var problems []error
	if p.ID == "" {
		problems = append(problems, errors.New("profile_id is required"))
	}
	if p.Tools[ToolTrivy] == "" {
		problems = append(problems, errors.New("tools.trivy: version is required"))
	}
	if len(p.IaCTypes) == 0 {
		problems = append(problems, errors.New("iac_types: at least one configuration kind is required"))
	}
	limit, err := time.ParseDuration(p.Timeout)
	if err != nil || limit <= 0 {
		problems = append(problems, errors.New("timeout must be a positive duration"))
	}
	p.limit = limit
	for name, reason := range p.ExcludedDirs {
		if !envelope.ScanExcludedTargetReason(reason).Valid() || strings.ContainsAny(name, "/*") {
			problems = append(problems, fmt.Errorf("excluded_dirs.%s: a directory name with vendor, generated or build", name))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("trivy profile %s: %w", p.ID, err)
	}
	return p, nil
}

// RulesHash is the SHA-256 of profile.json, trivy.yaml and trivyignore.
func (p *TrivyProfile) RulesHash() string { return p.rulesHash }

// command is the Trivy run of one kind on the current directory: the configuration and ignore file of the
// profile instead of those of the repository, the database and checks of the image without updates.
func (p *TrivyProfile) command(kind, cacheDir, output string, skip []string) []string {
	cmd := []string{
		ToolTrivy, "fs", "--config", filepath.Join(p.dir, TrivyConfigFile), "--ignorefile", filepath.Join(p.dir, TrivyIgnoreFile),
		"--cache-dir", cacheDir, "--cache-backend", "memory", "--skip-db-update", "--skip-check-update", "--skip-version-check",
		"--offline-scan", "--no-progress", "--quiet", "--format", "json", "--output", output, "--exit-code", "0",
	}
	if kind == string(envelope.ScanReportReportKindSca) {
		cmd = append(cmd, "--scanners", "vuln", "--pkg-types", "library", "--list-all-pkgs")
	} else {
		cmd = append(cmd, "--scanners", "misconfig", "--misconfig-scanners", strings.Join(p.IaCTypes, ","))
	}
	for _, dir := range skip {
		cmd = append(cmd, "--skip-dirs", dir)
	}
	return append(cmd, ".")
}

// trivyScan is one Trivy run on a component: its outcome, the excluded paths and the cleaned report.
type trivyScan struct {
	excluded []envelope.ScanExcludedTarget
	report   map[string]any
	targets  int
}

// scan runs Trivy of the kind on the component at prefix until the deadline and returns the report with paths
// relative to the repository root.
func (p *TrivyProfile) scan(ctx context.Context, r *Runner, root, prefix, kind, cacheDir string, deadline time.Time) (trivyScan, toolOutcome, error) {
	var s trivyScan
	target := prefix
	if target == "" {
		target = "."
	}
	excluded, hidden, err := excludedDirsOf(root, target, p.ExcludedDirs)
	if err != nil {
		return s, toolOutcome{}, err
	}
	s.excluded = excluded
	skip := make([]string, 0, len(excluded)+len(hidden))
	for _, e := range excluded {
		skip = append(skip, relTo(prefix, e.Path))
	}
	for _, h := range hidden {
		skip = append(skip, relTo(prefix, h))
	}
	work, err := os.MkdirTemp("", "craftpulse-trivy-")
	if err != nil {
		return s, toolOutcome{}, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	output := filepath.Join(work, "trivy.json")
	t := Tool{Command: p.command(kind, cacheDir, output, skip), Dir: filepath.Join(root, filepath.FromSlash(prefix)), Timeout: deadline.Sub(r.Now())}
	if t.Timeout <= 0 {
		return s, outcome(true, false, 0, nil), nil
	}
	code, timedOut, err := r.exec(ctx, &t)
	switch {
	case ctx.Err() != nil:
		return s, toolOutcome{}, ctx.Err()
	case timedOut:
		return s, outcome(true, false, 0, nil), nil
	case err != nil, code != 0:
		return s, outcome(false, false, code, nil), nil //nolint:nilerr // a tool that cannot start is a failed run, not a failed step
	}
	var findings bool
	if s.report, s.targets, findings, err = cleanTrivy(output, root, prefix); err != nil {
		fmt.Fprintf(r.Stderr, "craftpulse-ci: trivy: %v\n", err)
		return s, outcome(false, false, 0, nil), nil
	}
	code = 0
	if findings {
		code = 1
	}
	return s, outcome(false, true, code, []int{1}), nil
}

// cleanTrivy reads the JSON report of Trivy, makes the targets relative to the repository root, drops the
// lines of configuration that Trivy copies into a misconfiguration and names the resource of each one.
func cleanTrivy(name, root, prefix string) (report map[string]any, targets int, findings bool, err error) {
	if err := readJSON(name, &report); err != nil {
		return nil, 0, false, fmt.Errorf("report is not Trivy JSON: %w", err)
	}
	results, _ := report["Results"].([]any)
	for _, item := range results {
		res, ok := item.(map[string]any)
		if !ok {
			continue
		}
		file, _ := res["Target"].(string)
		file = path.Join(prefix, file)
		res["Target"] = file
		vulns, _ := res["Vulnerabilities"].([]any)
		misconfigs, _ := res["Misconfigurations"].([]any)
		if res["Class"] == "lang-pkgs" || len(misconfigs) > 0 || res["MisconfSummary"] != nil {
			targets++
		}
		findings = findings || len(vulns) > 0 || len(misconfigs) > 0
		kind, _ := res["Type"].(string)
		for _, m := range misconfigs {
			cause, _ := m.(map[string]any)["CauseMetadata"].(map[string]any)
			if cause == nil {
				continue
			}
			delete(cause, "Code")
			if resource, _ := cause["Resource"].(string); resource == "" {
				line, _ := cause["StartLine"].(float64)
				cause["Resource"] = configResource(filepath.Join(root, filepath.FromSlash(file)), kind, int(line))
			}
		}
	}
	return report, targets, findings, nil
}

// TrivyRunner runs the Trivy profile on the sca and iac rows of the job: one tool run per row.
type TrivyRunner struct {
	*Runner
	Trivy *TrivyProfile
	// Root is the repository checkout, CI_PROJECT_DIR in GitLab; CacheDir holds the vulnerability database.
	Root, CacheDir string
	// BaseSHA is the commit the lockfiles of head are compared with; empty when the job has none.
	BaseSHA string
	// Timeout overrides the time limit of the profile.
	Timeout time.Duration
}

// Run scans every row within one deadline and records the tool runs.
func (t *TrivyRunner) Run(ctx context.Context, rows []Entry) ([]envelope.ScanToolRun, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("profile has no builtin sca or iac check of profile %s", t.Trivy.ID)
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = t.Trivy.limit
	}
	deadline := t.Now().Add(timeout)
	perKind := map[string]int{}
	for _, row := range rows {
		perKind[row.Kind]++
	}
	runs := make([]envelope.ScanToolRun, 0, len(rows))
	for _, row := range rows {
		run, err := t.row(ctx, row, perKind[row.Kind] > 1, deadline)
		if err != nil {
			return runs, err
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

func (t *TrivyRunner) row(ctx context.Context, row Entry, several bool, deadline time.Time) (*envelope.ScanToolRun, error) {
	p := t.Trivy
	if row.ImageDigest == nil {
		return nil, fmt.Errorf("profile row of %s has no image digest", p.ID)
	}
	work, err := os.MkdirTemp("", "craftpulse-trivy-report-")
	if err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	tool := Tool{
		ID: p.ID + "-" + row.Kind, ProfileID: p.ID, Kind: row.Kind, Format: string(envelope.ScanReportReportFormatTrivyJson),
		FormatVersion: "2", Producer: ToolTrivy, ProducerVersion: p.Tools[ToolTrivy], Report: filepath.Join(work, "trivy.json"),
		ConfigFile: filepath.Join(p.dir, CodeProfileFile), RulesHash: p.rulesHash,
	}
	if several {
		tool.ID += "-" + row.ComponentID.String()
	}
	run, err := t.toolRun(&tool, row)
	if err != nil {
		return nil, err
	}
	run.StartedAt = t.Now().UTC()
	s, o, err := t.scanRow(ctx, run, row, deadline)
	if err != nil {
		return nil, err
	}
	run.FinishedAt = t.Now().UTC()
	excluded := append([]envelope.ScanExcludedTarget{}, s.excluded...)
	run.ExcludedTargets = &excluded
	if o.finished() {
		run.ScannedCount = &s.targets
		data, err := json.Marshal(s.report)
		if err != nil {
			return nil, fmt.Errorf("encode Trivy report: %w", err)
		}
		if err := os.WriteFile(tool.Report, data, 0o600); err != nil {
			return nil, fmt.Errorf("write Trivy report: %w", err)
		}
	}
	return run, t.record(run, &tool, o)
}

// scanRow checks the database for an sca row, runs Trivy and marks the new dependencies of an sca report.
// A missing database fails the run without a report.
func (t *TrivyRunner) scanRow(ctx context.Context, run *envelope.ScanToolRun, row Entry, deadline time.Time) (trivyScan, toolOutcome, error) {
	sca := row.Kind == string(envelope.ScanReportReportKindSca)
	if sca {
		db, err := ReadTrivyDB(t.CacheDir)
		if err != nil {
			fmt.Fprintf(t.Stderr, "craftpulse-ci: trivy: %v\n", err)
			return trivyScan{}, outcome(false, false, 0, nil), nil
		}
		run.DbVersion, run.DbPublishedAt = &db.Version, &db.PublishedAt
	}
	s, o, err := t.Trivy.scan(ctx, t.Runner, t.Root, row.PathPrefix, row.Kind, t.CacheDir, deadline)
	if err != nil || !o.finished() || !sca {
		return s, o, err
	}
	base, err := t.basePackages(ctx, s.report, row.PathPrefix, deadline)
	if err != nil {
		fmt.Fprintf(t.Stderr, "craftpulse-ci: trivy: lockfiles of the base commit: %v\n", err)
	}
	markDelta(s.report, t.BaseSHA, base, err == nil)
	return s, o, nil
}

// basePackages scans the lockfiles of the head report as they are in the base commit and gives the versions of
// every package by lockfile. Nil without a base commit.
func (t *TrivyRunner) basePackages(ctx context.Context, head map[string]any, prefix string, deadline time.Time) (map[string]map[string][]string, error) {
	if t.BaseSHA == "" {
		return nil, nil
	}
	work, err := os.MkdirTemp("", "craftpulse-trivy-base-")
	if err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	tree := filepath.Join(work, "tree")
	for _, lockfile := range lockfiles(head) {
		names := []string{lockfile}
		if path.Base(lockfile) == "go.mod" {
			names = append(names, path.Join(path.Dir(lockfile), "go.sum"))
		}
		for _, name := range names {
			if err := t.baseFile(ctx, name, filepath.Join(tree, filepath.FromSlash(name))); err != nil {
				return nil, err
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(tree, filepath.FromSlash(prefix)), 0o750); err != nil {
		return nil, fmt.Errorf("create base tree: %w", err)
	}
	s, o, err := t.Trivy.scan(ctx, t.Runner, tree, prefix, string(envelope.ScanReportReportKindSca), t.CacheDir, deadline)
	if err != nil {
		return nil, err
	}
	if !o.finished() {
		return nil, fmt.Errorf("trivy on the base lockfiles: %s", o.exit)
	}
	return packageVersions(s.report), nil
}

// baseFile writes the file of the base commit; a file the base commit does not have is left out.
func (t *TrivyRunner) baseFile(ctx context.Context, name, dst string) error {
	cmd := exec.CommandContext(ctx, "git", "-c", "safe.directory=*", "-C", t.Root, "show", t.BaseSHA+":"+name) //nolint:gosec // the SHA and path come from the job and the report
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	cmd.Env = toolEnv()
	if err := cmd.Run(); err != nil {
		check := exec.CommandContext(ctx, "git", "-c", "safe.directory=*", "-C", t.Root, "cat-file", "-e", t.BaseSHA+"^{commit}") //nolint:gosec // see above
		check.Env = toolEnv()
		if check.Run() != nil {
			return fmt.Errorf("base commit %s is not in the checkout", t.BaseSHA)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("create base tree: %w", err)
	}
	return os.WriteFile(dst, []byte(out.String()), 0o600)
}

// lockfiles are the targets of the dependency results of a report.
func lockfiles(report map[string]any) []string {
	var out []string
	for _, res := range trivyResults(report) {
		if res["Class"] == "lang-pkgs" {
			if target, ok := res["Target"].(string); ok && !slices.Contains(out, target) {
				out = append(out, target)
			}
		}
	}
	return out
}

func packageVersions(report map[string]any) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, res := range trivyResults(report) {
		target, _ := res["Target"].(string)
		packages, _ := res["Packages"].([]any)
		for _, item := range packages {
			pkg, _ := item.(map[string]any)
			name, _ := pkg["Name"].(string)
			version, _ := pkg["Version"].(string)
			if out[target] == nil {
				out[target] = map[string][]string{}
			}
			out[target][name] = append(out[target][name], version)
		}
	}
	return out
}

// markDelta marks the package instances and vulnerabilities of head that the base lockfiles do not have:
// added when the lockfile had no such package, changed when it had another version. CraftPulse.Delta tells
// whether the comparison was made: computed, no_base without a base commit, unavailable when it failed.
func markDelta(report map[string]any, baseSHA string, base map[string]map[string][]string, ok bool) {
	state := map[string]any{"BaseSHA": nil, "Delta": "no_base"}
	switch {
	case baseSHA != "" && !ok:
		state["BaseSHA"], state["Delta"] = baseSHA, "unavailable"
	case baseSHA != "":
		state["BaseSHA"], state["Delta"] = baseSHA, "computed"
	}
	report["CraftPulse"] = state
	if state["Delta"] != "computed" {
		return
	}
	mark := func(entry map[string]any, target, name, version string) {
		versions, known := base[target][name]
		switch {
		case !known:
			entry["CraftPulseDelta"] = DeltaAdded
		case !slices.Contains(versions, version):
			entry["CraftPulseDelta"] = DeltaChanged
		}
	}
	for _, res := range trivyResults(report) {
		target, _ := res["Target"].(string)
		packages, _ := res["Packages"].([]any)
		for _, item := range packages {
			pkg, _ := item.(map[string]any)
			if pkg["Relationship"] == "root" {
				continue
			}
			name, _ := pkg["Name"].(string)
			version, _ := pkg["Version"].(string)
			mark(pkg, target, name, version)
		}
		vulns, _ := res["Vulnerabilities"].([]any)
		for _, item := range vulns {
			v, _ := item.(map[string]any)
			name, _ := v["PkgName"].(string)
			version, _ := v["InstalledVersion"].(string)
			mark(v, target, name, version)
		}
	}
}

func trivyResults(report map[string]any) []map[string]any {
	results, _ := report["Results"].([]any)
	out := make([]map[string]any, 0, len(results))
	for _, item := range results {
		if res, ok := item.(map[string]any); ok {
			out = append(out, res)
		}
	}
	return out
}

// trivyFacts are the fixture facts of a cleaned report: "vulnerability <id> <package>@<version> <lockfile>"
// and "misconfiguration <id> <file>:<line> <resource>".
func trivyFacts(report map[string]any) []string {
	var out []string
	for _, res := range trivyResults(report) {
		target, _ := res["Target"].(string)
		vulns, _ := res["Vulnerabilities"].([]any)
		for _, item := range vulns {
			v, _ := item.(map[string]any)
			out = append(out, fmt.Sprintf("vulnerability %v %v@%v %s", v["VulnerabilityID"], v["PkgName"], v["InstalledVersion"], target))
		}
		misconfigs, _ := res["Misconfigurations"].([]any)
		for _, item := range misconfigs {
			m, _ := item.(map[string]any)
			cause, _ := m["CauseMetadata"].(map[string]any)
			line, _ := cause["StartLine"].(float64)
			out = append(out, fmt.Sprintf("misconfiguration %v %s:%d %v", m["ID"], target, max(int(line), 1), cause["Resource"]))
		}
	}
	return out
}

// Fixture runs both kinds of the profile on the current directory and writes their facts, one per line, to the
// file the fixtures command names. A run that does not finish fails the case.
func (p *TrivyProfile) Fixture(ctx context.Context, stdout, stderr io.Writer, cacheDir, factsReport string) error {
	if _, err := ReadTrivyDB(cacheDir); err != nil {
		return err
	}
	r := &Runner{Stdout: stdout, Stderr: stderr, Now: time.Now}
	deadline := time.Now().Add(p.limit)
	var facts []string
	for _, kind := range []envelope.ScanReportReportKind{envelope.ScanReportReportKindSca, envelope.ScanReportReportKindIac} {
		s, o, err := p.scan(ctx, r, ".", "", string(kind), cacheDir, deadline)
		if err != nil {
			return err
		}
		if !o.finished() {
			return fmt.Errorf("trivy %s: %s", kind, o.exit)
		}
		facts = append(facts, trivyFacts(s.report)...)
	}
	return os.WriteFile(factsReport, []byte(strings.Join(facts, "\n")+"\n"), 0o600)
}

// TrivyRows are the builtin sca and iac rows of the profile for this job.
func TrivyRows(p *ciapi.CiCheckProfile, profileID, component string) []Entry {
	mode := string(ciapi.CiCheckProfileEntriesModeBuiltin)
	rows := Select(p, Filter{Kind: string(envelope.ScanReportReportKindSca), Mode: mode, ProfileID: profileID, ComponentID: component})
	return append(rows, Select(p, Filter{Kind: string(envelope.ScanReportReportKindIac), Mode: mode, ProfileID: profileID, ComponentID: component})...)
}

// relTo makes a path relative to the repository root relative to the component at prefix.
func relTo(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return strings.TrimPrefix(strings.TrimPrefix(name, prefix), "/")
}
