package calc

import (
	"bytes"
	"cmp"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonCoverageSHAMismatch = "coverage_sha_mismatch"
	_reasonCoverageMapMissing  = "coverage_line_map_missing"
)

type MRCoverageReport struct {
	SHA      string
	Evidence entity.ReportEvidence
	Files    []MRCoverageMap
}

type MRCoverageMap struct {
	Path       string
	Kind       string
	Executable int
	// Lines is nil if the mapping could not be read. An empty, loaded map describes a file with no executable lines.
	Lines []MRCoverageLine
}

type MRCoverageLine struct {
	Line int
	Hits int64
}

type MRPatchContribution struct {
	Path       string
	Range      entity.MRDiffRange
	ReportID   entity.ID
	Role       entity.ContributionRole
	ReasonCode string
}

type MRPatchCoverageMetric struct {
	entity.Point
	Contributions []MRPatchContribution
}

type mrCoverageFile struct {
	file     *MRCoverageMap
	evidence *entity.ReportEvidence
}

func mrPatchCoverage(diff *entity.MRDiff, reports []MRCoverageReport) MRPatchCoverageMetric {
	state, reason := mrDiffState(diff)
	out := MRPatchCoverageMetric{Point: entity.Point{State: state, ReasonCode: reason}}
	if state == entity.StateNotEvaluated {
		return out
	}
	files, evidence, otherSHA := mrCoverageFiles(diff.HeadSHA, reports)
	state, reason = reportState(evidence, entity.AnalysisCoverage)
	switch {
	case state == entity.StateNotEvaluated && otherSHA:
		out.State, out.ReasonCode = state, _reasonCoverageSHAMismatch
		return out
	case state == entity.StateNotEvaluated || state == entity.StateUnknown:
		out.State, out.ReasonCode = state, reason
		return out
	case len(files) == 0 && hasRejectedCoverage(evidence):
		out.State, out.ReasonCode = entity.StateUnknown, _reasonReportRejected
		return out
	case len(files) == 0:
		out.State, out.ReasonCode = entity.StateNotEvaluated, _reasonCoverageKindMissing
		return out
	case state == entity.StateProvisional:
		out.State, out.ReasonCode = state, cmp.Or(out.ReasonCode, reason)
	}
	var covered, executable int64
	for i := range diff.Files {
		f := &diff.Files[i]
		if f.Binary || f.DeletedFile || len(f.HeadRanges) == 0 {
			continue
		}
		mapping := files[f.NewPath]
		if !mapping.valid() {
			out.addUnmapped(f, mapping)
			continue
		}
		for _, r := range f.HeadRanges {
			yes, all := out.addMapped(mapping, r)
			covered += yes
			executable += all
		}
	}
	out.finish(covered, executable)
	return out
}

func mrCoverageFiles(head string, reports []MRCoverageReport) (map[string]mrCoverageFile, []entity.ReportEvidence, bool) {
	files := make(map[string]mrCoverageFile)
	var evidence []entity.ReportEvidence
	var otherSHA bool
	for i := range reports {
		r := &reports[i]
		if r.Evidence.Kind != entity.AnalysisCoverage {
			continue
		}
		if r.SHA != head {
			otherSHA = true
			continue
		}
		evidence = append(evidence, r.Evidence)
		if r.Evidence.RejectedReason != "" {
			continue
		}
		for j := range r.Files {
			f := &r.Files[j]
			if f.Kind == entity.CoverageLine && newerCoverage(&r.Evidence, files[f.Path].evidence) {
				files[f.Path] = mrCoverageFile{file: f, evidence: &r.Evidence}
			}
		}
	}
	return files, evidence, otherSHA
}

func newerCoverage(next, previous *entity.ReportEvidence) bool {
	return previous == nil || next.KnownSeq > previous.KnownSeq ||
		next.KnownSeq == previous.KnownSeq && bytes.Compare(next.ID[:], previous.ID[:]) > 0
}

func (f mrCoverageFile) valid() bool {
	if f.file == nil || f.file.Lines == nil || len(f.file.Lines) != f.file.Executable {
		return false
	}
	var previous int
	for _, line := range f.file.Lines {
		if line.Line <= previous || line.Hits < 0 {
			return false
		}
		previous = line.Line
	}
	return true
}

func (m *MRPatchCoverageMetric) addUnmapped(f *entity.MRDiffFile, mapping mrCoverageFile) {
	var report entity.ID
	if mapping.evidence != nil {
		report = mapping.evidence.ID
	}
	for _, r := range f.HeadRanges {
		m.add(MRPatchContribution{Path: f.NewPath, Range: r, ReportID: report, Role: entity.RoleExcluded, ReasonCode: _reasonCoverageMapMissing})
		m.MissingCount += r.To - r.From + 1
	}
}

func (m *MRPatchCoverageMetric) addMapped(f mrCoverageFile, changed entity.MRDiffRange) (covered, executable int64) {
	lines := f.file.Lines
	first, _ := slices.BinarySearchFunc(lines, changed.From, func(l MRCoverageLine, n int) int { return cmp.Compare(l.Line, n) })
	entry := MRPatchContribution{Path: f.file.Path, ReportID: f.evidence.ID, Role: entity.RoleExcluded, ReasonCode: _reasonNoExecutableUnits}
	next := changed.From
	for _, line := range lines[first:] {
		if line.Line > changed.To {
			break
		}
		if next < line.Line {
			entry.Range = entity.MRDiffRange{From: next, To: line.Line - 1}
			m.add(entry)
		}
		unit := MRPatchContribution{
			Path: f.file.Path, Range: entity.MRDiffRange{From: line.Line, To: line.Line}, ReportID: f.evidence.ID, Role: entity.RoleDenominator,
		}
		executable++
		if line.Hits > 0 {
			covered++
			unit.Role = entity.RoleNumerator
		}
		m.add(unit)
		next = line.Line + 1
	}
	if next <= changed.To {
		entry.Range = entity.MRDiffRange{From: next, To: changed.To}
		m.add(entry)
	}
	return covered, executable
}

func (m *MRPatchCoverageMetric) add(c MRPatchContribution) {
	if c.Role == entity.RoleExcluded {
		m.ExcludedCount += c.Range.To - c.Range.From + 1
	}
	if len(m.Contributions) > 0 {
		last := &m.Contributions[len(m.Contributions)-1]
		if last.Path == c.Path && last.ReportID == c.ReportID && last.Role == c.Role &&
			last.ReasonCode == c.ReasonCode && last.Range.To+1 == c.Range.From {
			last.Range.To = c.Range.To
			return
		}
	}
	m.Contributions = append(m.Contributions, c)
}

func (m *MRPatchCoverageMetric) finish(covered, executable int64) {
	switch {
	case executable > 0:
		m.Value = new(float64(covered) / float64(executable))
		m.Numerator, m.Denominator = new(float64(covered)), new(float64(executable))
		if m.MissingCount > 0 {
			m.State, m.ReasonCode = entity.StateProvisional, cmp.Or(m.ReasonCode, _reasonCoverageMapMissing)
		}
	case m.MissingCount > 0:
		m.State, m.ReasonCode = entity.StateUnknown, _reasonCoverageMapMissing
	case m.State == entity.StateAssessed:
		m.State, m.ReasonCode = entity.StateNotApplicable, _reasonNoExecutableUnits
		m.Numerator, m.Denominator = new(0.0), new(0.0)
	}
}
