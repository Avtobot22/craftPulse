package ci

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// Files of a docs profile directory and its tools.
const (
	DocsRulesetFile = "spectral.yaml"
	ToolDocs        = "craftpulse-docs"
	ToolGoldmark    = "goldmark"
	ToolSpectral    = "spectral"
)

// Rules of the docs findings.
const (
	RuleMissingFile   = "missing-file"
	RuleMissingAnchor = "missing-anchor"
	_unreadableLink   = "unreadable-link"
)

const _maxDocBytes = 5 << 20

var (
	_linkScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	_openAPIKey = regexp.MustCompile(`^(openapi|swagger)\s*:`)
	_jsonAPIKey = regexp.MustCompile(`"(openapi|swagger)"\s*:`)
)

// DocsProfile is the Git documentation profile of an image: tool versions, the time limit, the Markdown
// extensions, the anchor rule, the front matter key of the owner and the Spectral codes that make a
// specification invalid rather than only breaking a rule.
type DocsProfile struct {
	ID                 string            `json:"profile_id"`
	Timeout            string            `json:"timeout"`
	Tools              map[string]string `json:"tools"`
	MarkdownExtensions []string          `json:"markdown_extensions"`
	AnchorRule         string            `json:"anchor_rule"`
	OwnerKey           string            `json:"owner_front_matter_key"`
	SpecErrors         []string          `json:"spec_errors"`

	dir       string
	limit     time.Duration
	rulesHash string
}

// LoadDocsProfile reads and checks the profile in dir; the rules hash covers profile.json and spectral.yaml.
func LoadDocsProfile(dir string) (*DocsProfile, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve docs profile directory: %w", err)
	}
	h := sha256.New()
	var raw []byte
	for _, name := range []string{CodeProfileFile, DocsRulesetFile} {
		data, err := os.ReadFile(filepath.Join(abs, name)) //nolint:gosec // the image names its profile directory
		if err != nil {
			return nil, fmt.Errorf("read docs profile: %w", err)
		}
		if raw == nil {
			raw = data
		}
		h.Write(data)
	}
	p := &DocsProfile{dir: abs, rulesHash: hex.EncodeToString(h.Sum(nil))}
	if err := decodeStrict(raw, p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", CodeProfileFile, err)
	}
	var problems []error
	if p.ID == "" {
		problems = append(problems, errors.New("profile_id is required"))
	}
	for _, tool := range []string{ToolDocs, ToolGoldmark, ToolSpectral} {
		if p.Tools[tool] == "" {
			problems = append(problems, fmt.Errorf("tools.%s: version is required", tool))
		}
	}
	if p.AnchorRule != "github" {
		problems = append(problems, fmt.Errorf("anchor_rule %q is not supported", p.AnchorRule))
	}
	if p.OwnerKey == "" || len(p.MarkdownExtensions) == 0 || len(p.SpecErrors) == 0 {
		problems = append(problems, errors.New("owner_front_matter_key, markdown_extensions and spec_errors are required"))
	}
	limit, err := time.ParseDuration(p.Timeout)
	if err != nil || limit <= 0 {
		problems = append(problems, errors.New("timeout must be a positive duration"))
	}
	p.limit = limit
	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("docs profile %s: %w", p.ID, err)
	}
	return p, nil
}

// RulesHash is the SHA-256 of profile.json and spectral.yaml.
func (p *DocsProfile) RulesHash() string { return p.rulesHash }

type (
	DocsInventory = envelope.DocsInventory
	DocsEntry     = envelope.DocsEntry
	DocsLinkStat  = envelope.DocsLinkStat
)

// docsRoot is what a root holds: Markdown files and OpenAPI specifications, paths from the repository root.
type docsRoot struct {
	markdown, specs []string
	skipped         []envelope.ScanSkippedTarget
}

// walkRoot lists the documents under prefix; directories starting with "." are not documentation, a file
// above the size limit or unreadable is skipped.
func (p *DocsProfile) walkRoot(root, prefix string) (docsRoot, error) {
	var r docsRoot
	base := filepath.Join(root, filepath.FromSlash(prefix))
	err := filepath.WalkDir(base, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if name != base && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(path.Ext(rel))
		markdown := slices.Contains(p.MarkdownExtensions, ext)
		if !d.Type().IsRegular() || (!markdown && ext != ".yaml" && ext != ".yml" && ext != ".json") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > _maxDocBytes {
			r.skipped = append(r.skipped, envelope.ScanSkippedTarget{Path: rel, Reason: envelope.ScanSkippedTargetReasonLimit})
			return nil //nolint:nilerr // a file that cannot be read is skipped, not a failed walk
		}
		if markdown {
			r.markdown = append(r.markdown, rel)
			return nil
		}
		spec, err := isOpenAPI(name)
		if err != nil {
			r.skipped = append(r.skipped, envelope.ScanSkippedTarget{Path: rel, Reason: envelope.ScanSkippedTargetReasonUnavailable})
		} else if spec {
			r.specs = append(r.specs, rel)
		}
		return nil
	})
	if err != nil {
		return r, fmt.Errorf("walk documentation root %q: %w", prefix, err)
	}
	return r, nil
}

// isOpenAPI tells whether a YAML or JSON file is an OpenAPI or Swagger document by its top-level key.
func isOpenAPI(name string) (bool, error) {
	f, err := os.Open(name) //nolint:gosec // a file of the documentation root
	if err != nil {
		return false, err
	}
	defer f.Close()
	head, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return false, err
	}
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return _jsonAPIKey.Match(head), nil
	}
	sc := bufio.NewScanner(bytes.NewReader(head))
	for sc.Scan() {
		if _openAPIKey.MatchString(sc.Text()) {
			return true, nil
		}
	}
	return false, sc.Err()
}

// docsCheck checks the links of the Markdown files: a file that does not exist with exactly that name, or an
// anchor that no heading of the target makes, is a finding at the line of the link.
type docsCheck struct {
	root    string
	profile *DocsProfile
	parsed  map[string]markdownDoc
	entries map[string][]fs.DirEntry
}

func (c *docsCheck) markdown(rel string) (markdownDoc, []byte, error) {
	f, err := os.Open(filepath.Join(c.root, filepath.FromSlash(rel))) //nolint:gosec // a file of the repository
	if err != nil {
		return markdownDoc{}, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, _maxDocBytes+1))
	if err != nil {
		return markdownDoc{}, nil, err
	}
	if len(data) > _maxDocBytes {
		return markdownDoc{}, nil, fmt.Errorf("markdown file exceeds %d bytes", _maxDocBytes)
	}
	if doc, ok := c.parsed[rel]; ok {
		return doc, data, nil
	}
	doc := parseMarkdown(data, c.profile.OwnerKey)
	c.parsed[rel] = doc
	return doc, data, nil
}

// lookup resolves the exact case of a path. An unreadable directory or a symlink is an unknown target.
func (c *docsCheck) lookup(rel string) (fs.DirEntry, error) {
	dir, name := path.Dir(rel), path.Base(rel)
	if dir != "." {
		parent, err := c.lookup(dir)
		if err != nil || parent == nil || !parent.IsDir() {
			return nil, err
		}
	}
	entries, ok := c.entries[dir]
	if !ok {
		var err error
		entries, err = os.ReadDir(filepath.Join(c.root, filepath.FromSlash(dir)))
		if err != nil {
			return nil, err
		}
		c.entries[dir] = entries
	}
	at, found := slices.BinarySearchFunc(entries, name, func(e fs.DirEntry, name string) int {
		return strings.Compare(e.Name(), name)
	})
	if !found {
		return nil, nil
	}
	entry := entries[at]
	if entry.Type()&fs.ModeSymlink != 0 {
		return nil, fs.ErrPermission
	}
	return entry, nil
}

// link resolves a link of the document: external links are not checked; a path starting with "/" is from the
// repository root, others from the directory of the document.
func (c *docsCheck) link(from, target string) (rule, resolved string, external bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "//") || _linkScheme.MatchString(target) {
		return "", "", target != ""
	}
	file, anchor, _ := strings.Cut(target, "#")
	file, _, _ = strings.Cut(file, "?")
	if unescaped, err := url.PathUnescape(file); err == nil {
		file = unescaped
	}
	switch {
	case file == "":
		resolved = from
	case strings.HasPrefix(file, "/"):
		resolved = path.Clean(strings.TrimLeft(file, "/"))
	default:
		resolved = path.Join(path.Dir(from), file)
	}
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return RuleMissingFile, resolved, false
	}
	if resolved != "." {
		entry, err := c.lookup(resolved)
		if err != nil {
			return _unreadableLink, resolved, false
		}
		if entry == nil {
			return RuleMissingFile, resolved, false
		}
	}
	if anchor == "" || !slices.Contains(c.profile.MarkdownExtensions, strings.ToLower(path.Ext(resolved))) {
		return "", resolved, false
	}
	if unescaped, err := url.PathUnescape(anchor); err == nil {
		anchor = unescaped
	}
	doc, _, err := c.markdown(resolved)
	if err != nil {
		return _unreadableLink, resolved, false
	}
	if !slices.Contains(doc.anchors, anchor) {
		return RuleMissingAnchor, resolved, false
	}
	return "", resolved, false
}

// docsResult is the outcome of one kind on one root: the findings, the inventory and the counts.
type docsResult struct {
	results   []lintResult
	inventory DocsInventory
	scanned   int
	skipped   []envelope.ScanSkippedTarget
}

// checkDocs builds the inventory of the root and checks the local links of its Markdown documents.
func (p *DocsProfile) checkDocs(root string, r docsRoot, doc func(path, format string, data []byte) envelope.DocumentV1) docsResult {
	c := &docsCheck{root: root, profile: p, parsed: map[string]markdownDoc{}, entries: map[string][]fs.DirEntry{}}
	out := docsResult{skipped: r.skipped, inventory: DocsInventory{Links: &DocsLinkStat{}}}
	for _, rel := range r.markdown {
		md, data, err := c.markdown(rel)
		if err != nil {
			out.skipped = append(out.skipped, envelope.ScanSkippedTarget{Path: rel, Reason: envelope.ScanSkippedTargetReasonUnavailable})
			continue
		}
		out.scanned++
		entry := DocsEntry{Document: doc(rel, string(envelope.DocumentV1DocFormatMarkdown), data), Owner: optional(md.owner)}
		if md.title != "" {
			entry.Document.Title = &md.title
		}
		out.inventory.Documents = append(out.inventory.Documents, entry)
		for _, l := range md.links {
			rule, resolved, external := c.link(rel, l.target)
			switch {
			case external:
				out.inventory.Links.External++
				continue
			case resolved == "":
				continue
			case rule == _unreadableLink:
				out.skipped = append(out.skipped, envelope.ScanSkippedTarget{Path: resolved, Reason: envelope.ScanSkippedTargetReasonUnavailable})
				continue
			}
			out.inventory.Links.Checked++
			if rule == "" {
				continue
			}
			out.inventory.Links.Broken++
			message := "link target " + l.target + " does not exist"
			if rule == RuleMissingAnchor {
				message = "link target " + l.target + ": no heading of " + resolved + " has this anchor"
			}
			_, anchor, _ := strings.Cut(l.target, "#")
			if decoded, err := url.PathUnescape(anchor); err == nil {
				anchor = decoded
			}
			out.results = append(out.results, lintResult{
				rule: rule, level: "error", message: message, file: rel, line: l.line,
				linkTarget: &envelope.DocsLinkTarget{TargetPath: resolved, Anchor: anchor},
			})
		}
	}
	for _, rel := range r.specs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // a file of the root
		if err != nil {
			out.skipped = append(out.skipped, envelope.ScanSkippedTarget{Path: rel, Reason: envelope.ScanSkippedTargetReasonUnavailable})
			continue
		}
		out.scanned++
		out.inventory.Documents = append(out.inventory.Documents, DocsEntry{Document: doc(rel, string(envelope.DocumentV1DocFormatOpenapi), data)})
	}
	return out
}

type spectralResult struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Range    struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
	} `json:"range"`
}

// checkSpecs runs Spectral with the ruleset of the profile on the specifications of the root. A result with
// a code of spec_errors makes the specification invalid; other results break a rule of a valid one.
func (p *DocsProfile) checkSpecs(ctx context.Context, r *Runner, root string, specs []string, deadline time.Time) ([]lintResult, map[string]bool, toolOutcome, error) {
	valid := map[string]bool{}
	for _, s := range specs {
		valid[s] = true
	}
	if len(specs) == 0 {
		return nil, valid, outcome(false, true, 0, nil), nil
	}
	work, err := os.MkdirTemp("", "craftpulse-spectral-")
	if err != nil {
		return nil, nil, toolOutcome{}, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	output := filepath.Join(work, "spectral.json")
	cmd := []string{ToolSpectral, "lint", "--ruleset", filepath.Join(p.dir, DocsRulesetFile), "--format", "json", "--output", output, "--quiet"}
	for _, s := range specs {
		cmd = append(cmd, "./"+s)
	}
	t := Tool{Command: cmd, Dir: root, Timeout: deadline.Sub(r.Now())}
	if t.Timeout <= 0 {
		return nil, nil, outcome(true, false, 0, nil), nil
	}
	code, timedOut, err := r.exec(ctx, &t)
	switch {
	case ctx.Err() != nil:
		return nil, nil, toolOutcome{}, ctx.Err()
	case timedOut:
		return nil, nil, outcome(true, false, 0, nil), nil
	case err != nil, code != 0 && code != 1:
		return nil, nil, outcome(false, false, code, nil), nil //nolint:nilerr // a tool that cannot start is a failed run, not a failed step
	}
	var found []spectralResult
	if err := readJSON(output, &found); err != nil {
		fmt.Fprintf(r.Stderr, "craftpulse-ci: spectral: output is not JSON: %v\n", err)
		return nil, nil, outcome(false, false, 0, nil), nil
	}
	levels := []string{"error", "warning", "note", "note"}
	results := make([]lintResult, 0, len(found))
	for _, f := range found {
		file, err := repoPath(f.Source, root)
		if err != nil {
			return nil, nil, toolOutcome{}, err
		}
		if slices.Contains(p.SpecErrors, f.Code) {
			valid[file] = false
		}
		level := "note"
		if f.Severity >= 0 && f.Severity < len(levels) {
			level = levels[f.Severity]
		}
		results = append(results, lintResult{rule: f.Code, level: level, message: f.Message, file: file, line: f.Range.Start.Line + 1, column: f.Range.Start.Character + 1})
	}
	return results, valid, outcome(false, true, min(len(results), 1), []int{1}), nil
}

// DocsRoot is a documentation root of the project profile.
type DocsRoot struct {
	ID         uuid.UUID
	Kind       string
	PathPrefix string
}

// DocsRoots are the documentation roots of the profile.
func DocsRoots(p *ciapi.CiCheckProfile) []DocsRoot {
	out := make([]DocsRoot, 0, len(p.DocumentationRoots))
	for _, r := range p.DocumentationRoots {
		out = append(out, DocsRoot{ID: r.RootId, Kind: string(r.Kind), PathPrefix: r.PathPrefix})
	}
	return out
}

// DocsRunner runs the docs profile on the documentation roots of the job: one tool run per root and kind.
type DocsRunner struct {
	*Runner
	Docs *DocsProfile
	// Root is the repository checkout, CI_PROJECT_DIR in GitLab.
	Root string
	// Timeout overrides the time limit of the profile.
	Timeout time.Duration
}

// Run checks every root of a kind that the profile has a builtin row of: docs for links and the inventory,
// openapi for the specifications.
func (d *DocsRunner) Run(ctx context.Context, rows []Entry, roots []DocsRoot) ([]envelope.ScanToolRun, error) {
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = d.Docs.limit
	}
	deadline := d.Now().Add(timeout)
	var runs []envelope.ScanToolRun
	for _, root := range roots {
		i := slices.IndexFunc(rows, func(e Entry) bool { return e.Kind == root.Kind })
		if i < 0 {
			continue
		}
		var checks []string
		for _, other := range roots {
			if other.ID == root.ID && slices.ContainsFunc(rows, func(e Entry) bool { return e.Kind == other.Kind }) {
				checks = append(checks, other.Kind)
			}
		}
		slices.Sort(checks)
		run, err := d.root(ctx, rows[i], root, slices.Compact(checks), deadline)
		if err != nil {
			return runs, err
		}
		runs = append(runs, *run)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("profile has no documentation root with a builtin docs or openapi check of profile %s", d.Docs.ID)
	}
	return runs, nil
}

func (d *DocsRunner) root(ctx context.Context, row Entry, root DocsRoot, checks []string, deadline time.Time) (*envelope.ScanToolRun, error) {
	p := d.Docs
	if row.ImageDigest == nil {
		return nil, fmt.Errorf("profile row of %s has no image digest", p.ID)
	}
	work, err := os.MkdirTemp("", "craftpulse-docs-report-")
	if err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)
	openapi := root.Kind == string(envelope.ScanReportReportKindOpenapi)
	t := Tool{
		ID: root.Kind + "-" + root.ID.String(), ProfileID: p.ID, Kind: root.Kind, Format: string(envelope.ScanReportReportFormatSarif),
		FormatVersion: _sarifVersion, Producer: ToolDocs, ProducerVersion: p.Tools[ToolDocs], Report: filepath.Join(work, "docs.sarif"),
		ConfigFile: filepath.Join(p.dir, CodeProfileFile), RulesHash: p.rulesHash, ParserVersion: ToolGoldmark + " " + p.Tools[ToolGoldmark],
	}
	if openapi {
		t.Producer, t.ProducerVersion, t.ParserVersion = ToolSpectral, p.Tools[ToolSpectral], ""
		t.ConfigFile = filepath.Join(p.dir, DocsRulesetFile)
	}
	row.PathPrefix = root.PathPrefix
	run, err := d.toolRun(&t, row)
	if err != nil {
		return nil, err
	}
	run.StartedAt = d.Now().UTC()
	res, o, err := p.analyze(ctx, d.Runner, d.Root, root, d.document, deadline)
	if err != nil {
		return nil, err
	}
	run.FinishedAt = d.Now().UTC()
	run.ExitClass, run.Completeness = o.exit, o.completeness
	res.inventory.Checks = checks
	if o.finished() && len(res.skipped) != 0 {
		run.Completeness = envelope.ScanToolRunCompletenessPartial
	}
	if res.skipped == nil {
		res.skipped = []envelope.ScanSkippedTarget{}
	}
	skipped := len(res.skipped)
	run.ScannedCount, run.SkippedCount, run.SkippedTargets = &res.scanned, &skipped, res.skipped
	if !o.finished() {
		return run, d.Store.AddRun(run, nil)
	}
	inventory := filepath.Join(work, "docs.json")
	if err := writeLintSARIF(t.Report, t.Producer, t.ProducerVersion, "https://github.com/Avtobot22/craftPulse", res.results); err != nil {
		return nil, err
	}
	data, err := json.Marshal(res.inventory)
	if err != nil {
		return nil, fmt.Errorf("encode docs inventory: %w", err)
	}
	if err := os.WriteFile(inventory, data, 0o600); err != nil {
		return nil, fmt.Errorf("write docs inventory: %w", err)
	}
	run.Reports = []envelope.ScanReport{
		{
			ReportKind: envelope.ScanReportReportKind(root.Kind), ReportFormat: envelope.ScanReportReportFormatSarif, FormatVersion: _sarifVersion,
			File: envelope.FileRef{MediaType: mediaType(string(envelope.ScanReportReportFormatSarif))},
		},
		{
			ReportKind: envelope.ScanReportReportKind(root.Kind), ReportFormat: envelope.ScanReportReportFormatDocsJson, FormatVersion: "1",
			File: envelope.FileRef{MediaType: mediaType(string(envelope.ScanReportReportFormatDocsJson))},
		},
	}
	return run, d.Store.AddRun(run, []string{t.Report, inventory})
}

// document is the DocumentV1 record of a file at the SHA of the job.
func (d *DocsRunner) document(rel, format string, data []byte) envelope.DocumentV1 {
	return docRecord(rel, format, data, d.Job.SHA, d.Job.ProjectID)
}

func docRecord(rel, format string, data []byte, sha, container string) envelope.DocumentV1 {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	doc := envelope.DocumentV1{
		DocFormat: envelope.DocumentV1DocFormat(format), VersionRef: sha, ContainerNativeId: container, Path: &rel,
		AclState: envelope.DocumentV1AclStateKnown, ContentSha256: &hash,
	}
	if format == string(envelope.DocumentV1DocFormatOpenapi) {
		doc.Title = optional(specTitle(data))
	}
	return doc
}

// analyze runs one kind on the root: links and the inventory for docs, Spectral and the validity of the
// specifications for openapi.
func (p *DocsProfile) analyze(ctx context.Context, r *Runner, root string, dr DocsRoot, doc func(string, string, []byte) envelope.DocumentV1, deadline time.Time) (docsResult, toolOutcome, error) {
	walked, err := p.walkRoot(root, dr.PathPrefix)
	if err != nil {
		return docsResult{}, toolOutcome{}, err
	}
	if dr.Kind != string(envelope.ScanReportReportKindOpenapi) {
		res := p.checkDocs(root, walked, doc)
		res.inventory.FormatVersion, res.inventory.PathPrefix, res.inventory.AnchorRule = "1", dr.PathPrefix, p.AnchorRule
		res.inventory.OwnerRule = p.OwnerKey
		if dr.ID != uuid.Nil {
			res.inventory.RootID = &dr.ID
		}
		code := min(len(res.results), 1)
		return res, outcome(false, true, code, []int{1}), nil
	}
	results, valid, o, err := p.checkSpecs(ctx, r, root, walked.specs, deadline)
	if err != nil || !o.finished() {
		return docsResult{skipped: walked.skipped}, o, err
	}
	res := docsResult{results: results, skipped: walked.skipped, inventory: DocsInventory{FormatVersion: "1", PathPrefix: dr.PathPrefix, AnchorRule: p.AnchorRule}}
	if dr.ID != uuid.Nil {
		res.inventory.RootID = &dr.ID
	}
	for _, rel := range walked.specs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // a file of the root
		if err != nil {
			res.skipped = append(res.skipped, envelope.ScanSkippedTarget{Path: rel, Reason: envelope.ScanSkippedTargetReasonUnavailable})
			continue
		}
		res.scanned++
		ok := valid[rel]
		res.inventory.Documents = append(res.inventory.Documents, DocsEntry{Document: doc(rel, string(envelope.DocumentV1DocFormatOpenapi), data), SpecValid: &ok})
	}
	return res, o, nil
}

// specTitle reads info.title of a YAML or JSON specification without a YAML parser: the title line indented
// under the top-level info key.
func specTitle(data []byte) string {
	var doc struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	if json.Unmarshal(data, &doc) == nil {
		return doc.Info.Title
	}
	inInfo := false
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && line[0] != ' ' && line[0] != '#' {
			inInfo = strings.HasPrefix(line, "info:")
			continue
		}
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "title:"); ok && inInfo {
			return yamlScalar(value)
		}
	}
	return ""
}

// Fixture runs both kinds on the current directory as one root and writes the findings of both to the SARIF
// report and the inventory facts to the facts file of the fixtures command.
func (p *DocsProfile) Fixture(ctx context.Context, stdout, stderr io.Writer, report, facts string) error {
	r := &Runner{Stdout: stdout, Stderr: stderr, Now: time.Now}
	deadline := time.Now().Add(p.limit)
	var results []lintResult
	var lines []string
	doc := func(rel, format string, data []byte) envelope.DocumentV1 {
		return docRecord(rel, format, data, "fixture", "fixture")
	}
	for _, kind := range []envelope.ScanReportReportKind{envelope.ScanReportReportKindDocs, envelope.ScanReportReportKindOpenapi} {
		res, o, err := p.analyze(ctx, r, ".", DocsRoot{Kind: string(kind)}, doc, deadline)
		if err != nil {
			return err
		}
		if !o.finished() {
			return fmt.Errorf("%s: %s", kind, o.exit)
		}
		results = append(results, res.results...)
		for i := range res.inventory.Documents {
			e := &res.inventory.Documents[i]
			switch {
			case e.SpecValid != nil && *e.SpecValid:
				lines = append(lines, "spec "+*e.Document.Path+" valid")
			case e.SpecValid != nil:
				lines = append(lines, "spec "+*e.Document.Path+" invalid")
			case kind == envelope.ScanReportReportKindDocs:
				owner := "-"
				if e.Owner != nil {
					owner = *e.Owner
				}
				lines = append(lines, "document "+*e.Document.Path+" "+string(e.Document.DocFormat)+" owner="+owner)
			}
		}
	}
	if err := writeLintSARIF(report, ToolDocs, p.Tools[ToolDocs], "https://github.com/Avtobot22/craftPulse", results); err != nil {
		return err
	}
	return os.WriteFile(facts, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// DocsRows are the builtin docs and openapi rows of the profile for this job.
func DocsRows(p *ciapi.CiCheckProfile, profileID string) []Entry {
	mode := string(ciapi.CiCheckProfileEntriesModeBuiltin)
	rows := Select(p, Filter{Kind: string(envelope.ScanReportReportKindDocs), Mode: mode, ProfileID: profileID})
	return append(rows, Select(p, Filter{Kind: string(envelope.ScanReportReportKindOpenapi), Mode: mode, ProfileID: profileID})...)
}
