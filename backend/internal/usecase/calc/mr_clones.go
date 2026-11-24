package calc

import (
	"cmp"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonKnownClone     = "known_base_clone"
	_reasonAmbiguousClone = "ambiguous_clone_identity"
)

type MRCloneContribution struct {
	Group      entity.CloneGroup
	Role       entity.ContributionRole
	ReasonCode string
}

type MRCloneMetric struct {
	entity.Point
	BaseReportID  entity.ID
	HeadReportID  entity.ID
	Contributions []MRCloneContribution
}

type clonePath struct{ language, path string }

type cloneRange struct {
	language string
	entity.CodeRange
}

type cloneIndex struct {
	groups  []entity.CloneGroup
	byRange map[cloneRange][]int
	byPath  map[clonePath][]int
}

type previousCloneMember struct {
	location  entity.CodeRange
	uncertain bool
}

func mrClones(diff *entity.MRDiff, files map[string]*entity.MRDiffFile, pair *MRCodeComparison) MRCloneMetric {
	state, reason := pair.state(diff, entity.CodeFormatJscpd)
	out := MRCloneMetric{Point: entity.Point{State: state, ReasonCode: reason}}
	if state == entity.StateNotEvaluated {
		return out
	}
	b, h := pair.Base.Report, pair.Head.Report
	out.BaseReportID, out.HeadReportID = b.ID, h.ID
	index := indexClones(b.Facts.Groups)
	var count float64
	for i := range h.Facts.Groups {
		g := &h.Facts.Groups[i]
		if !slices.ContainsFunc(g.Members, func(m entity.CodeRange) bool { return overlapsChangedLines(files[m.Path], m) }) {
			continue
		}
		c := MRCloneContribution{Group: *g, Role: entity.RoleNumerator}
		switch index.priorGroup(g, files) {
		case _reasonKnownClone:
			c.Role, c.ReasonCode = entity.RoleExcluded, _reasonKnownClone
			out.ExcludedCount++
		case _reasonAmbiguousClone:
			c.Role, c.ReasonCode = entity.RoleExcluded, _reasonAmbiguousClone
			out.ExcludedCount++
			out.State, out.ReasonCode = entity.StateProvisional, cmp.Or(out.ReasonCode, _reasonAmbiguousClone)
		default:
			count++
		}
		out.Contributions = append(out.Contributions, c)
	}
	out.Value, out.Numerator = &count, &count
	return out
}

func indexClones(groups []entity.CloneGroup) cloneIndex {
	out := cloneIndex{groups: groups, byRange: make(map[cloneRange][]int), byPath: make(map[clonePath][]int)}
	for i := range groups {
		g := &groups[i]
		for _, member := range g.Members {
			key := cloneRange{language: g.Language, CodeRange: member}
			out.byRange[key] = append(out.byRange[key], i)
			path := clonePath{language: g.Language, path: member.Path}
			ids := out.byPath[path]
			if len(ids) == 0 || ids[len(ids)-1] != i {
				out.byPath[path] = append(ids, i)
			}
		}
	}
	return out
}

func (idx *cloneIndex) priorGroup(g *entity.CloneGroup, files map[string]*entity.MRDiffFile) string {
	members := make([]previousCloneMember, len(g.Members))
	var candidates []int
	for i, member := range g.Members {
		m := previousClone(member, files[member.Path])
		if m.location.Path == "" {
			return ""
		}
		members[i] = m
		matches := idx.byRange[cloneRange{language: g.Language, CodeRange: m.location}]
		if m.uncertain {
			matches = idx.byPath[clonePath{language: g.Language, path: m.location.Path}]
		}
		if i == 0 || len(matches) < len(candidates) {
			candidates = matches
		}
		if len(candidates) == 0 {
			return ""
		}
	}
	var prior string
	for _, id := range candidates {
		switch containsCloneMembers(idx.groups[id].Members, members) {
		case _reasonKnownClone:
			return _reasonKnownClone
		case _reasonAmbiguousClone:
			prior = _reasonAmbiguousClone
		}
	}
	return prior
}

func previousClone(member entity.CodeRange, file *entity.MRDiffFile) previousCloneMember {
	if file == nil {
		return previousCloneMember{location: member}
	}
	if file.NewFile {
		return previousCloneMember{}
	}
	if file.RenamedFile {
		member.Path = file.OldPath
	}
	if file.DeletedLines == nil || *file.DeletedLines > 0 {
		return previousCloneMember{location: member, uncertain: true}
	}
	// With insertions only, the unchanged part of the range has an exact base coordinate.
	before, through := addedThrough(file.HeadRanges, member.Start-1), addedThrough(file.HeadRanges, member.End)
	if through-before == member.End-member.Start+1 {
		return previousCloneMember{}
	}
	member.Start -= before
	member.End -= through
	return previousCloneMember{location: member}
}

func addedThrough(ranges []entity.MRDiffRange, line int) int {
	var count int
	for _, r := range ranges {
		if r.From > line {
			break
		}
		count += min(r.To, line) - r.From + 1
	}
	return count
}

func containsCloneMembers(base []entity.CodeRange, head []previousCloneMember) string {
	if len(base) < len(head) {
		return ""
	}
	remaining := make(map[entity.CodeRange]bool, len(base))
	available := make(map[string]int)
	for _, member := range base {
		remaining[member] = true
		available[member.Path]++
	}
	for _, m := range head {
		if !m.uncertain {
			if !remaining[m.location] {
				return ""
			}
			delete(remaining, m.location)
			available[m.location.Path]--
		}
	}
	prior := _reasonKnownClone
	for _, m := range head {
		if !m.uncertain {
			continue
		}
		p := m.location.Path
		if available[p] == 0 {
			return ""
		}
		available[p]--
		if !remaining[m.location] {
			prior = _reasonAmbiguousClone
		}
		delete(remaining, m.location)
	}
	return prior
}
