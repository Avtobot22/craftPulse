package normalize

import (
	"bufio"
	"encoding/xml"
	"errors"
	"io"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	// _lcovVersion stands for the tracefile format of geninfo: LCOV declares no version.
	_lcovVersion      = "tracefile"
	_coberturaVersion = "04"
	_jacocoVersion    = "1.1"
	_maxLCOVLine      = 1 << 20
	_maxExcluded      = 10000
)

var (
	_coberturaDTD = regexp.MustCompile(`coverage-(\d+)\.dtd`)
	_jacocoDTD    = regexp.MustCompile(`-//JACOCO//DTD Report (\d+\.\d+)//EN`)
	_condition    = regexp.MustCompile(`\((\d+)/(\d+)\)`)
)

// parsedCoverage is the coverage of a file by one kind; points are its lines as [line, hits] for line coverage
// and [line, covered, total] for branch coverage.
type parsedCoverage struct {
	path                string
	kind                string
	covered, executable int
	points              [][]int64
}

type branchKey struct {
	line          int
	block, branch string
}

// fileCoverage gathers the records of one file: a file may come in several records of a report, so hits add up.
// counters replace the counts of lines and branches when the format gives them per file.
type fileCoverage struct {
	lines        map[int]int64
	lineBranches map[int][2]int64
	branches     map[branchKey]int64
	hasBranches  bool
	counters     map[string][2]int
}

// coverageSet is the coverage of a report by path of the repository with the files it left out. allBranches
// marks a report that measured branches of every file, so a file without branches has none to cover.
type coverageSet struct {
	in          parseInput
	files       map[string]*fileCoverage
	excluded    []entity.SkippedTarget
	seen        map[string]bool
	allBranches bool
}

func newCoverageSet(in parseInput) *coverageSet {
	return &coverageSet{in: in, files: map[string]*fileCoverage{}, seen: map[string]bool{}}
}

// file is the coverage of a file named by the report, nil when the file is outside the repository.
func (s *coverageSet) file(name string) *fileCoverage {
	p, ok := s.in.repoFile(name)
	if !ok {
		if !s.seen[name] && len(s.excluded) < _maxExcluded {
			s.seen[name] = true
			s.excluded = append(s.excluded, entity.SkippedTarget{Path: short(name), Reason: entity.SkippedOutsideRepository})
		}
		return nil
	}
	f := s.files[p]
	if f == nil {
		f = &fileCoverage{lines: map[int]int64{}, lineBranches: map[int][2]int64{}, branches: map[branchKey]int64{}}
		s.files[p] = f
	}
	return f
}

// repoFile turns a path of a coverage report into a path of the repository: an absolute path is taken under the
// checkout the tool ran in, a relative one from the directory of the repository its paths start from.
func (in parseInput) repoFile(name string) (string, bool) {
	p := strings.ReplaceAll(name, "\\", "/")
	if absolute(p) {
		root := strings.TrimSuffix(strings.ReplaceAll(in.sourceRoot, "\\", "/"), "/")
		if root == "" || !absolute(root+"/") || !strings.HasPrefix(p, root+"/") {
			return "", false
		}
		p = strings.TrimPrefix(p, root+"/")
	} else if in.sourceDir != "" {
		p = in.sourceDir + "/" + p
	}
	rel := repoPath(p)
	return rel, rel != ""
}

func absolute(p string) bool {
	return strings.HasPrefix(p, "/") || len(p) > 2 && p[1] == ':' && p[2] == '/' && ('a' <= p[0]|0x20 && p[0]|0x20 <= 'z')
}

// report gives a line row of every file and a branch row of a file whose records name branches.
func (s *coverageSet) report(out *parsedReport) error {
	for _, p := range slices.Sorted(maps.Keys(s.files)) {
		f := s.files[p]
		line := parsedCoverage{path: p, kind: entity.CoverageLine}
		for _, n := range slices.Sorted(maps.Keys(f.lines)) {
			hits := f.lines[n]
			line.points = append(line.points, []int64{int64(n), hits})
			line.executable++
			if hits > 0 {
				line.covered++
			}
		}
		if c, ok := f.counters["LINE"]; ok {
			line.covered, line.executable = c[0], c[0]+c[1]
		}
		out.coverage = append(out.coverage, line)
		branch, ok, err := f.branchRow(p, s.allBranches)
		if err != nil {
			return err
		}
		if ok {
			out.coverage = append(out.coverage, branch)
		}
	}
	out.excluded = s.excluded
	return nil
}

func (f *fileCoverage) branchRow(p string, measured bool) (parsedCoverage, bool, error) {
	perLine := maps.Clone(f.lineBranches)
	for k, taken := range f.branches {
		c := perLine[k.line]
		c[1]++
		if taken > 0 {
			c[0]++
		}
		perLine[k.line] = c
	}
	c, counted := f.counters["BRANCH"]
	if !measured && !f.hasBranches && len(perLine) == 0 && !counted {
		return parsedCoverage{}, false, nil
	}
	row := parsedCoverage{path: p, kind: entity.CoverageBranch}
	var covered, total int64
	for _, n := range slices.Sorted(maps.Keys(perLine)) {
		b := perLine[n]
		row.points = append(row.points, []int64{int64(n), b[0], b[1]})
		covered, total = addHits(covered, b[0]), addHits(total, b[1])
	}
	if total > math.MaxInt32 {
		return parsedCoverage{}, false, invalidReport("%.64q has more than %d branches", p, math.MaxInt32)
	}
	row.covered, row.executable = int(covered), int(total)
	if counted {
		row.covered, row.executable = c[0], c[0]+c[1]
	}
	return row, true, nil
}

// parseLCOV reads an LCOV tracefile: SF opens the record of a file, DA gives the hits of a line and BRDA the
// times a branch was taken, "-" when its block never ran; end_of_record closes the record.
func parseLCOV(r io.Reader, in parseInput) (*parsedReport, error) {
	s := newCoverageSet(in)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), _maxLCOVLine)
	var (
		f    *fileCoverage
		open bool
	)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		key, value, _ := strings.Cut(line, ":")
		switch {
		case line == "end_of_record":
			if !open {
				return nil, invalidReport("lcov: end_of_record without SF")
			}
			f, open = nil, false
		case key == "SF":
			if open {
				return nil, invalidReport("lcov: SF before end_of_record")
			}
			if value == "" {
				return nil, invalidReport("lcov: SF without a path")
			}
			f, open = s.file(value), true
		case key == "DA" || key == "BRDA" || key == "BRF":
			if !open {
				return nil, invalidReport("lcov: %s outside a record", key)
			}
			if f == nil {
				continue
			}
			if err := f.lcovRecord(key, value); err != nil {
				return nil, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if open {
		return nil, invalidReport("lcov: the last record has no end_of_record")
	}
	out := &parsedReport{formatVersion: _lcovVersion}
	return out, s.report(out)
}

func (f *fileCoverage) lcovRecord(key, value string) error {
	fields := strings.Split(value, ",")
	switch key {
	case "DA":
		if len(fields) < 2 {
			return invalidReport("lcov: DA %.32q", value)
		}
		n, err := sourceLine(fields[0])
		if err != nil {
			return err
		}
		hits, err := count(fields[1])
		if err != nil {
			return err
		}
		f.lines[n] = addHits(f.lines[n], hits)
	case "BRDA":
		if len(fields) != 4 {
			return invalidReport("lcov: BRDA %.32q", value)
		}
		n, err := sourceLine(fields[0])
		if err != nil {
			return err
		}
		var taken int64
		if fields[3] != "-" {
			if taken, err = count(fields[3]); err != nil {
				return err
			}
		}
		k := branchKey{line: n, block: fields[1], branch: fields[2]}
		f.branches[k] = addHits(f.branches[k], taken)
	case "BRF":
		// BRF:0 says the file has no branches; a count without BRDA records leaves them unknown.
		if strings.TrimSpace(value) == "0" {
			f.hasBranches = true
		}
	}
	return nil
}

// coberturaReader keeps where the decoder is: lines of a class count, the lines of its methods repeat them.
type coberturaReader struct {
	set       *coverageSet
	sources   []string
	stack     []string
	file      *fileCoverage
	inMethods bool
}

// parseCobertura reads a Cobertura XML report of the coverage-04 DTD: a class names its file relative to a
// source, and a line marked as a branch gives its conditions as "(covered/total)".
func parseCobertura(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := xml.NewDecoder(r)
	c := &coberturaReader{set: newCoverageSet(in)}
	out := &parsedReport{formatVersion: _coberturaVersion}
	var text strings.Builder
	root := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.Directive:
			if m := _coberturaDTD.FindSubmatch(t); m != nil && !root {
				out.formatVersion = string(m[1])
			}
		case xml.StartElement:
			if !root {
				if t.Name.Local != "coverage" {
					return nil, invalidReport("cobertura: root %.32q is not coverage", t.Name.Local)
				}
				if out.formatVersion != _coberturaVersion {
					return out, nil
				}
				root = true
				valid, err := strconv.Atoi(attr(t, "branches-valid"))
				c.set.allBranches = err == nil && valid > 0
			}
			text.Reset()
			if err := c.start(t); err != nil {
				return nil, err
			}
			c.stack = append(c.stack, t.Name.Local)
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			c.stack = c.stack[:len(c.stack)-1]
			switch t.Name.Local {
			case "source":
				if s := strings.TrimSpace(text.String()); s != "" {
					c.sources = append(c.sources, s)
				}
			case "class":
				c.file = nil
			case "methods":
				c.inMethods = false
			}
		}
	}
	if !root {
		return nil, invalidReport("cobertura: no root element")
	}
	return out, c.set.report(out)
}

func (c *coberturaReader) start(t xml.StartElement) error {
	switch t.Name.Local {
	case "class":
		name := attr(t, "filename")
		if name == "" {
			return invalidReport("cobertura: a class has no filename")
		}
		c.file = c.set.file(c.path(name))
	case "methods":
		c.inMethods = true
	case "line":
		if c.file == nil || c.inMethods || len(c.stack) == 0 || c.stack[len(c.stack)-1] != "lines" {
			return nil
		}
		n, err := sourceLine(attr(t, "number"))
		if err != nil {
			return err
		}
		hits, err := count(attr(t, "hits"))
		if err != nil {
			return err
		}
		c.file.lines[n] = addHits(c.file.lines[n], hits)
		if attr(t, "branch") != "true" {
			return nil
		}
		if m := _condition.FindStringSubmatch(attr(t, "condition-coverage")); m != nil {
			covered, err := count(m[1])
			if err != nil {
				return err
			}
			total, err := count(m[2])
			if err != nil {
				return err
			}
			if covered > total {
				return invalidReport("cobertura: line %d covers %d of %d conditions", n, covered, total)
			}
			b := c.file.lineBranches[n]
			c.file.lineBranches[n] = [2]int64{max(b[0], covered), max(b[1], total)}
		}
	}
	return nil
}

// path joins the filename of a class with the first source: Cobertura names files relative to one of its
// sources without saying which, and the first one is where the producers of the demo set count from.
func (c *coberturaReader) path(name string) string {
	if absolute(strings.ReplaceAll(name, "\\", "/")) || len(c.sources) == 0 {
		return name
	}
	return strings.TrimSuffix(c.sources[0], "/") + "/" + name
}

// parseJaCoCo reads a JaCoCo XML report: a source file of a package gives the counters LINE and BRANCH and its
// lines with covered and missed branches; JaCoCo measures branches always, so a file without them has none. The path is the package with the file name, relative to the source
// directory of the scan manifest.
func parseJaCoCo(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := xml.NewDecoder(r)
	set := newCoverageSet(in)
	set.allBranches = true
	out := &parsedReport{}
	var (
		stack []string
		pkg   string
		file  *fileCoverage
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.Directive:
			if m := _jacocoDTD.FindSubmatch(t); m != nil && len(stack) == 0 {
				out.formatVersion = string(m[1])
			}
		case xml.StartElement:
			if len(stack) == 0 && out.formatVersion == "" {
				return nil, invalidReport("jacoco: no DOCTYPE of the JaCoCo report")
			}
			if len(stack) == 0 && out.formatVersion != _jacocoVersion {
				return out, nil
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "package":
				pkg = attr(t, "name")
			case "sourcefile":
				if parent != "package" {
					continue
				}
				name := attr(t, "name")
				if name == "" {
					return nil, invalidReport("jacoco: a source file has no name")
				}
				if pkg != "" {
					name = pkg + "/" + name
				}
				if file = set.file(name); file != nil {
					file.counters = map[string][2]int{}
				}
			case "line":
				if file != nil && parent == "sourcefile" {
					if err := file.jacocoLine(t); err != nil {
						return nil, err
					}
				}
			case "counter":
				if file != nil && parent == "sourcefile" {
					if err := file.jacocoCounter(t); err != nil {
						return nil, err
					}
				}
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			switch t.Name.Local {
			case "sourcefile":
				file = nil
			case "package":
				pkg = ""
			}
		}
	}
	if out.formatVersion == "" {
		return nil, invalidReport("jacoco: no root element")
	}
	return out, set.report(out)
}

func (f *fileCoverage) jacocoLine(t xml.StartElement) error {
	n, err := sourceLine(attr(t, "nr"))
	if err != nil {
		return err
	}
	var v [4]int64
	for i, name := range []string{"mi", "ci", "mb", "cb"} {
		if v[i], err = count(attr(t, name)); err != nil {
			return err
		}
	}
	f.lines[n] = addHits(f.lines[n], v[1])
	if v[2]+v[3] > 0 {
		f.lineBranches[n] = [2]int64{v[3], v[2] + v[3]}
	}
	return nil
}

func (f *fileCoverage) jacocoCounter(t xml.StartElement) error {
	kind := attr(t, "type")
	if kind != "LINE" && kind != "BRANCH" {
		return nil
	}
	missed, err := count(attr(t, "missed"))
	if err != nil {
		return err
	}
	covered, err := count(attr(t, "covered"))
	if err != nil {
		return err
	}
	if missed+covered > math.MaxInt32 {
		return invalidReport("jacoco: counter %s is out of range", kind)
	}
	f.counters[kind] = [2]int{int(covered), int(missed)}
	return nil
}

func sourceLine(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 0, invalidReport("line %.16q is not a line number", s)
	}
	return n, nil
}

func count(s string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, invalidReport("count %.16q is not a count", s)
	}
	return n, nil
}

// addHits adds hits of the same line from several records without overflow.
func addHits(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
