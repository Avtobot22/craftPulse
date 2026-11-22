package normalize

import (
	"cmp"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func hasCodeReports(reports []*reportCandidate) bool {
	return slices.ContainsFunc(reports, func(c *reportCandidate) bool { return c.code != nil })
}

func parseLizard(r io.Reader, in parseInput) (*parsedReport, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	out := &parsedReport{formatVersion: "1", code: &entity.CodeFacts{}}
	files := map[string]bool{}
	for row := 0; ; row++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch {
		case row == 0 && slices.Equal(record, []string{"craftpulse-lizard", "2"}):
			out.formatVersion = "2"
			out.code.Files = new([]entity.CodeFile{})
		case row == 0 && len(record) == 11 && record[0] == "NLOC" && record[1] == "CCN":
			// Lizard --csv --verbose starts with a header.
		case out.code.Files != nil && len(record) == 3 && record[0] == "file":
			p, n := codePath(record[1], in), codeInt(record[2])
			if p == "" || n < 0 || files[p] {
				return nil, fmt.Errorf("invalid file inventory at row %d", row+1)
			}
			files[p] = true
			*out.code.Files = append(*out.code.Files, entity.CodeFile{Path: p, NLOC: n})
		case len(record) == 11:
			fn, err := lizardFunction(record, in)
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", row+1, err)
			}
			out.code.Functions = append(out.code.Functions, fn)
		default:
			return nil, fmt.Errorf("invalid CSV layout at row %d", row+1)
		}
	}
	for _, fn := range out.code.Functions {
		if out.code.Files != nil && !files[fn.Path] {
			return nil, errors.New("function is outside the file inventory")
		}
	}
	return out, nil
}

func lizardFunction(row []string, in parseInput) (entity.CodeFunction, error) {
	fn := entity.CodeFunction{
		CodeRange: entity.CodeRange{Path: codePath(row[6], in), Start: codeInt(row[9]), End: codeInt(row[10])},
		Name:      row[7], NLOC: codeInt(row[0]), CCN: codeInt(row[1]),
	}
	if !validCodeRange(fn.CodeRange) || fn.Name == "" || len(fn.Name) > _maxPath || strings.ContainsRune(fn.Name, 0) ||
		fn.NLOC < 0 || fn.CCN < 0 || fn.NLOC > fn.End-fn.Start+1 {
		return entity.CodeFunction{}, errors.New("invalid function coordinates or metrics")
	}
	return fn, nil
}

func codeInt(s string) int {
	n, err := strconv.ParseUint(s, 10, 31)
	if err != nil {
		return -1
	}
	return int(n)
}

func codePath(s string, in parseInput) string {
	p, _ := in.repoFile(s)
	if strings.ContainsRune(p, 0) {
		return ""
	}
	return p
}

func validCodeRange(r entity.CodeRange) bool {
	return r.Path != "" && r.Start > 0 && r.End >= r.Start && r.End < 1<<31
}

type jscpdMember struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type jscpdPair struct {
	Language string      `json:"format"`
	First    jscpdMember `json:"firstFile"`  //nolint:tagliatelle // jscpd wire format.
	Second   jscpdMember `json:"secondFile"` //nolint:tagliatelle // jscpd wire format.
}

type cloneMember struct {
	entity.CodeRange
	language string
}

func parseJscpd(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	grouping := cloneGrouping{indices: map[cloneMember]int{}}
	facts := &entity.CodeFacts{}
	out := &parsedReport{formatVersion: "1", code: facts}
	var pairs bool
	err := object(dec, func(key string) error {
		switch key {
		case "duplicates":
			pairs = true
			return array(dec, func() error { return grouping.readPair(dec, in) })
		case "scope_excluded_paths":
			out.formatVersion = "2"
			excluded, err := readCloneScope(dec)
			out.cloneExcluded = excluded
			return err
		case "statistics":
			var stats struct {
				Total struct {
					Lines *int64 `json:"lines"`
				} `json:"total"`
			}
			if err := dec.Decode(&stats); err != nil {
				return err
			}
			facts.PhysicalLines = stats.Total.Lines
		default:
			return skip(dec)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := end(dec); err != nil {
		return nil, err
	}
	if !pairs || facts.PhysicalLines == nil || *facts.PhysicalLines < 0 {
		return nil, errors.New("missing duplicates or invalid statistics.total.lines")
	}
	facts.Groups = grouping.groups()
	return out, nil
}

func readCloneScope(dec *json.Decoder) (map[string]bool, error) {
	var paths []string
	if err := dec.Decode(&paths); err != nil {
		return nil, err
	}
	if paths == nil {
		return nil, errors.New("scope_excluded_paths must be an array")
	}
	out := make(map[string]bool, len(paths))
	for _, raw := range paths {
		p := codePath(raw, parseInput{})
		if p == "" {
			return nil, errors.New("invalid excluded path in clone report scope")
		}
		out[p] = true
	}
	return out, nil
}

// cloneGrouping joins pairs that share the same occurrence. Overlapping but different ranges are
// separate occurrences, and are unioned by the line-coverage calculation rather than by group.
type cloneGrouping struct {
	indices map[cloneMember]int
	members []cloneMember
	parent  []int
}

func (g *cloneGrouping) readPair(dec *json.Decoder, in parseInput) error {
	var p jscpdPair
	if err := dec.Decode(&p); err != nil {
		return err
	}
	a := entity.CodeRange{Path: codePath(p.First.Name, in), Start: p.First.Start, End: p.First.End}
	b := entity.CodeRange{Path: codePath(p.Second.Name, in), Start: p.Second.Start, End: p.Second.End}
	if !validCodeRange(a) || !validCodeRange(b) || a == b || len(p.Language) > 128 || strings.ContainsRune(p.Language, 0) {
		return errors.New("invalid clone pair")
	}
	i, j := g.index(cloneMember{a, p.Language}), g.index(cloneMember{b, p.Language})
	x, y := g.root(i), g.root(j)
	g.parent[max(x, y)] = min(x, y)
	return nil
}

func (g *cloneGrouping) index(m cloneMember) int {
	i, ok := g.indices[m]
	if !ok {
		i = len(g.parent)
		g.indices[m] = i
		g.parent = append(g.parent, i)
		g.members = append(g.members, m)
	}
	return i
}

func (g *cloneGrouping) root(i int) int {
	for i != g.parent[i] {
		g.parent[i] = g.parent[g.parent[i]]
		i = g.parent[i]
	}
	return i
}

func (g *cloneGrouping) groups() []entity.CloneGroup {
	var out []entity.CloneGroup
	indices := map[int]int{}
	for i, m := range g.members {
		root := g.root(i)
		at, ok := indices[root]
		if !ok {
			at = len(out)
			indices[root] = at
			out = append(out, entity.CloneGroup{Language: m.language})
		}
		out[at].Members = append(out[at].Members, m.CodeRange)
	}
	for i := range out {
		slices.SortFunc(out[i].Members, compareCodeRanges)
	}
	slices.SortFunc(out, func(a, b entity.CloneGroup) int {
		if c := cmp.Compare(a.Language, b.Language); c != 0 {
			return c
		}
		return slices.CompareFunc(a.Members, b.Members, compareCodeRanges)
	})
	return out
}

// scopedCodeFacts applies the manifest of the report candidate to parsed code facts.
func (p *parsedReport) scopedCodeFacts(tool *envelope.ScanToolRun) *entity.CodeFacts {
	code := p.code
	if code == nil || (tool == nil && len(p.cloneExcluded) == 0) {
		return code
	}
	out := *code
	excluded := map[string]bool{}
	unlistedSkips := false
	if tool != nil {
		if tool.ExcludedTargets != nil {
			for _, target := range *tool.ExcludedTargets {
				excluded[target.Path] = true
			}
		}
		for _, target := range tool.SkippedTargets {
			excluded[target.Path] = true
		}
		unlistedSkips = tool.SkippedCount != nil && *tool.SkippedCount > len(tool.SkippedTargets)
	}
	if out.PhysicalLines != nil {
		p.scopeCloneFacts(&out, excluded, unlistedSkips)
		return &out
	}
	if t := tool.FunctionThresholds; t != nil {
		out.Thresholds = &entity.CodeThresholds{CCN: t.CcnThreshold, NLOC: t.NlocThreshold}
	}
	if len(excluded) > 0 {
		out.Functions = slices.DeleteFunc(slices.Clone(code.Functions), func(fn entity.CodeFunction) bool {
			return excludedCodePath(fn.Path, excluded)
		})
	}
	if out.Files != nil {
		tests := map[string]bool{}
		if tool.TestTargets != nil {
			for _, p := range *tool.TestTargets {
				tests[p] = true
			}
		}
		files := make([]entity.CodeFile, 0, len(*code.Files))
		for _, file := range *code.Files {
			if !excludedCodePath(file.Path, excluded) {
				file.Test = tests[file.Path]
				files = append(files, file)
			}
		}
		out.Files = &files
	}
	return &out
}

func (p *parsedReport) scopeCloneFacts(out *entity.CodeFacts, excluded map[string]bool, unlistedSkips bool) {
	unverified := unlistedSkips
	for name := range excluded {
		if !excludedCodePath(name, p.cloneExcluded) {
			unverified = true
		}
	}
	for name := range p.cloneExcluded {
		excluded[name] = true
	}
	if len(excluded) > 0 {
		groups := make([]entity.CloneGroup, 0, len(out.Groups))
		for _, group := range out.Groups {
			members := slices.DeleteFunc(slices.Clone(group.Members), func(m entity.CodeRange) bool {
				return excludedCodePath(m.Path, excluded)
			})
			unverified = unverified || len(members) != len(group.Members)
			if len(members) >= 2 {
				groups = append(groups, entity.CloneGroup{Language: group.Language, Members: members})
			}
		}
		out.Groups = groups
	}
	if unverified {
		// A native jscpd total has no per-file line counts to subtract from it.
		out.PhysicalLines = nil
	}
}

func excludedCodePath(p string, excluded map[string]bool) bool {
	for {
		if excluded[p] {
			return true
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			return false
		}
		p = p[:i]
	}
}

func compareCodeRanges(a, b entity.CodeRange) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End))
}
