package normalize

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_maxRuleID      = 255
	_maxNativeID    = 255
	_maxPath        = 4096
	_maxSeverity    = 64
	_maxDescription = 1024
)

// _findingKinds is the kind of a finding by the kind of its report.
var _findingKinds = map[string]string{
	entity.AnalysisCode: entity.FindingCode, entity.AnalysisSAST: entity.FindingSecurity, entity.AnalysisSecrets: entity.FindingSecret,
	entity.AnalysisSCA: entity.FindingDependency, entity.AnalysisImage: entity.FindingDependency, entity.AnalysisIaC: entity.FindingIaC,
	entity.AnalysisDocs: entity.FindingDocs, entity.AnalysisOpenAPI: entity.FindingOpenAPI,
}

// reportParser reads one format of report. A parser reads its file as a stream, since a format may put its version
// after the findings; the version it read then has to be among those it accepts. Its version changes whenever the
// same file would give other rows, so a report is read again the same way by the same parser.
type reportParser struct {
	format  string
	version string
	kinds   []string
	accepts func(formatVersion string) bool
	parse   func(r io.Reader, in parseInput) (*parsedReport, error)
}

// parseInput is what a parser reads a file with: the kind of the report and, for the paths of coverage, the checkout
// the tool ran in and the directory of the repository its relative paths start from.
type parseInput struct {
	kind       string
	sourceRoot string
	sourceDir  string
}

type parsedReport struct {
	formatVersion string
	findings      []entity.FindingOccurrence
	execution     *envelope.JenkinsExecutionManifestV1
	tests         []parsedTest
	coverage      []parsedCoverage
	// excluded are the files of a coverage report outside the repository.
	excluded []entity.SkippedTarget
}

// _parsers is the registry of report parsers; a format without a parser here is not read yet, and its record
// stays accepted without a report.
var _parsers = []reportParser{
	{
		format: "sarif", version: "sarif-2.1.0/1",
		kinds: []string{
			entity.AnalysisCode, entity.AnalysisSAST, entity.AnalysisSecrets, entity.AnalysisSCA, entity.AnalysisImage, entity.AnalysisIaC,
			entity.AnalysisDocs, entity.AnalysisOpenAPI,
		},
		accepts: func(v string) bool { return v == "2.1.0" },
		parse:   parseSARIF,
	},
	{
		format: "gitlab_security", version: "gitlab-security-15/1",
		kinds:   []string{entity.AnalysisSAST, entity.AnalysisSecrets, entity.AnalysisSCA},
		accepts: regexp.MustCompile(`^15\.[012]\.\d+$`).MatchString,
		parse:   parseGitLabSecurity,
	},
	{
		format: "junit_xml", version: "junit-xml/1",
		kinds:   []string{entity.AnalysisJUnit},
		accepts: func(v string) bool { return v == _junitSuites || v == _junitSuite },
		parse:   parseJUnit,
	},
	{
		format: "lcov", version: "lcov/1",
		kinds:   []string{entity.AnalysisCoverage},
		accepts: func(v string) bool { return v == _lcovVersion },
		parse:   parseLCOV,
	},
	{
		format: "cobertura", version: "cobertura-04/1",
		kinds:   []string{entity.AnalysisCoverage},
		accepts: func(v string) bool { return v == _coberturaVersion },
		parse:   parseCobertura,
	},
	{
		format: "jacoco", version: "jacoco-1.1/1",
		kinds:   []string{entity.AnalysisCoverage},
		accepts: func(v string) bool { return v == _jacocoVersion },
		parse:   parseJaCoCo,
	},
	{
		format: _formatJenkinsExecution, version: "jenkins-execution-1/1",
		kinds:   []string{_recordExecutionManifest},
		accepts: func(v string) bool { return v == "1" },
		parse:   parseJenkinsExecution,
	},
}

// errReport is a file a parser cannot read; reason is the code its record is rejected with.
type errReport struct {
	reason string
	detail string
}

func (e *errReport) Error() string { return e.reason + ": " + e.detail }

func invalidReport(format string, args ...any) error {
	return &errReport{reason: entity.RecordReportInvalid, detail: fmt.Sprintf(format, args...)}
}

func parserOf(format string) (*reportParser, bool) {
	i := slices.IndexFunc(_parsers, func(p reportParser) bool { return p.format == format })
	if i < 0 {
		return nil, false
	}
	return &_parsers[i], true
}

// read parses a file and checks the kind and the version of its format.
func (p *reportParser) read(r io.Reader, in parseInput) (*parsedReport, error) {
	if !slices.Contains(p.kinds, in.kind) {
		return nil, &errReport{reason: entity.RecordReportFormatUnsupported, detail: p.format + " does not carry " + in.kind}
	}
	out, err := p.parse(r, in)
	if err != nil {
		if e := (*errReport)(nil); errors.As(err, &e) {
			return nil, err
		}
		return nil, invalidReport("%s: %v", p.format, err)
	}
	if !p.accepts(out.formatVersion) {
		return nil, &errReport{reason: entity.RecordReportVersionUnsupported, detail: p.format + " version " + out.formatVersion}
	}
	return out, nil
}

// object reads the members of a JSON object one by one: member gets each key with the decoder before its value.
func object(dec *json.Decoder, member func(key string) error) error {
	if err := delim(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := t.(string)
		if !ok {
			return errors.New("object key is not a string")
		}
		if err := member(key); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

// array reads the elements of a JSON array one by one; null is an empty array.
func array(dec *json.Decoder, element func() error) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return nil
	}
	if d, ok := t.(json.Delim); !ok || d != '[' {
		return errors.New("expected an array")
	}
	for dec.More() {
		if err := element(); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func delim(dec *json.Decoder, want json.Delim) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != want {
		return fmt.Errorf("expected %q", want)
	}
	return nil
}

// skip passes a value of any size without keeping it.
func skip(dec *json.Decoder) error {
	depth := 0
	for {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := t.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
		if depth == 0 {
			return nil
		}
	}
}

// end checks that nothing follows the document.
func end(dec *json.Decoder) error {
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("data after the document")
	}
	return nil
}

// repoPath is a path relative to the root of the repository; an absolute path or one that leaves the root is
// unknown.
func repoPath(p string) string {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) || len(p) > _maxPath || !utf8.ValidString(p) {
		return ""
	}
	return p
}

// uriPath turns an artifact URI of SARIF into a path of the repository: a relative URI is relative to the root of
// the scan, an absolute one only when it is under a root the run declares.
func uriPath(uri string, roots []string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	if u.Scheme == "" && u.Host == "" {
		if p, err := url.PathUnescape(u.Path); err == nil && !path.IsAbs(p) {
			return repoPath(p)
		}
		return ""
	}
	if u.Scheme != "file" {
		return ""
	}
	for _, root := range roots {
		if r, err := url.Parse(root); err == nil && r.Scheme == "file" && strings.HasPrefix(u.Path, strings.TrimSuffix(r.Path, "/")+"/") {
			return repoPath(strings.TrimPrefix(u.Path, strings.TrimSuffix(r.Path, "/")+"/"))
		}
	}
	return ""
}

// lines keeps a start line of 1 or more and an end line not before it.
func lines(start, finish int) (first, last *int) {
	if start < 1 {
		return nil, nil
	}
	s := start
	if finish < start {
		return &s, nil
	}
	f := finish
	return &s, &f
}

// fingerprint hashes the identity of a finding by a method named in its first part; the content of the line is
// never part of it.
func fingerprint(parts ...string) []byte {
	h := sha256.New()
	var size [binary.MaxVarintLen64]byte
	for _, p := range parts {
		n := binary.PutUvarint(size[:], uint64(len(p)))
		h.Write(size[:n])
		h.Write([]byte(p))
	}
	return h.Sum(nil)
}

func lineText(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

var _cwe = regexp.MustCompile(`(?i)cwe[-_/: ]?0*([1-9]\d{0,5})\b`)

// cwes finds the weaknesses named in texts as CWE-n, each once and in numeric order.
func cwes(texts ...string) []string {
	var nums []int
	for _, t := range texts {
		for _, m := range _cwe.FindAllStringSubmatch(t, -1) {
			n, err := strconv.Atoi(m[1])
			if err == nil && !slices.Contains(nums, n) {
				nums = append(nums, n)
			}
		}
	}
	slices.Sort(nums)
	out := make([]string, len(nums))
	for i, n := range nums {
		out[i] = "CWE-" + strconv.Itoa(n)
	}
	return out
}

// short cuts a text of a rule to the length evidence keeps.
func short(s string) string {
	if utf8.RuneCountInString(s) <= _maxDescription {
		return s
	}
	return string([]rune(s)[:_maxDescription])
}

// bounded is a native value of the source or empty when it does not fit its column.
func bounded(s string, limit int) string {
	if len(s) > limit {
		return ""
	}
	return s
}

// evidence is what a finding keeps besides its coordinates: names and descriptions of the rule and the tool,
// weaknesses and packages, never a fragment of code or the value of a secret (BR-017).
type evidence struct {
	Tool             *toolRef     `json:"tool,omitempty"`
	RuleName         string       `json:"rule_name,omitempty"`
	RuleDescription  string       `json:"rule_description,omitempty"`
	CWE              []string     `json:"cwe,omitempty"`
	SecuritySeverity string       `json:"security_severity,omitempty"`
	Identifiers      []identifier `json:"identifiers,omitempty"`
	Package          *packageRef  `json:"package,omitempty"`
	Suppressed       bool         `json:"suppressed,omitempty"`
}

type toolRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type identifier struct {
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value"`
}

type packageRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

func (e *evidence) encode() (json.RawMessage, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if string(raw) == "{}" {
		return nil, nil
	}
	return raw, nil
}

// newFinding checks the fields a finding needs before it becomes a row.
func newFinding(kind, rule, native string) (entity.FindingOccurrence, error) {
	switch {
	case rule == "":
		return entity.FindingOccurrence{}, invalidReport("a finding has no rule")
	case len(rule) > _maxRuleID:
		return entity.FindingOccurrence{}, invalidReport("rule %.32q is longer than %d bytes", rule, _maxRuleID)
	}
	return entity.FindingOccurrence{RuleID: rule, Kind: _findingKinds[kind], NativeID: bounded(native, _maxNativeID)}, nil
}
