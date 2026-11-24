package calc

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonMRDiffMissing     = "mr_diff_missing"
	_reasonMRDiffPartial     = "mr_diff_partial"
	_reasonIncomparableBase  = "incomparable_base"
	_reasonCodeScopeMismatch = "code_report_scope_mismatch"
	_reasonLineCountsMissing = "line_counts_missing"
)

type MRStructureInput struct {
	Diff         entity.MRDiff
	BaseExcluded []entity.SkippedTarget
	HeadExcluded []entity.SkippedTarget
	Functions    MRCodeComparison
	Clones       MRCodeComparison
	Coverage     []MRCoverageReport
}

// MRCodeComparison is one report stream at base and head; change supplies the comparability decision.
type MRCodeComparison struct {
	Base       MRCodeSnapshot
	Head       MRCodeSnapshot
	Comparable bool
	ReasonCode string
}

type MRCodeSnapshot struct {
	Scope  entity.CodeScope
	Report *entity.CodeReport
}

type MRStructureMetrics struct {
	Definitions   []entity.RevisionInput
	Size          MRSizeMetric
	Complexity    MRComplexityMetric
	Clones        MRCloneMetric
	PatchCoverage MRPatchCoverageMetric
}

// CalculateMRStructure computes the four metrics from the immutable facts of one diff revision.
func (*UseCase) CalculateMRStructure(in *MRStructureInput) MRStructureMetrics {
	defs := make([]entity.RevisionInput, 0, len(_mrStructureDefinitions))
	for i := range _mrStructureDefinitions {
		d := &_mrStructureDefinitions[i]
		defs = append(defs, entity.RevisionInput{Kind: InputDefinition, ID: d.ID, Version: strconv.Itoa(d.Version)})
	}
	files := make(map[string]*entity.MRDiffFile, len(in.Diff.Files))
	for i := range in.Diff.Files {
		f := &in.Diff.Files[i]
		if !f.DeletedFile {
			files[f.NewPath] = f
		}
	}
	return MRStructureMetrics{
		Definitions:   defs,
		Size:          mrSize(in),
		Complexity:    mrComplexity(&in.Diff, files, &in.Functions),
		Clones:        mrClones(&in.Diff, files, &in.Clones),
		PatchCoverage: mrPatchCoverage(&in.Diff, in.Coverage),
	}
}

func mrDiffState(diff *entity.MRDiff) (state entity.DataState, reason string) {
	switch diff.Completeness {
	case entity.MRDiffComplete:
		return entity.StateAssessed, ""
	case entity.MRDiffPartial:
		return entity.StateProvisional, _reasonMRDiffPartial
	default:
		return entity.StateNotEvaluated, _reasonMRDiffMissing
	}
}

func (c *MRCodeComparison) state(diff *entity.MRDiff, format string) (state entity.DataState, reason string) {
	if !c.Comparable {
		return entity.StateNotEvaluated, cmp.Or(c.ReasonCode, _reasonIncomparableBase)
	}
	state, reason = mrDiffState(diff)
	if state == entity.StateNotEvaluated {
		return state, reason
	}
	if c.Base.Report == nil || c.Head.Report == nil {
		return entity.StateNotEvaluated, _reasonReportMissing
	}
	b, h := &c.Base.Scope, &c.Head.Scope
	if b.SHA != diff.BaseSHA || h.SHA != diff.HeadSHA || b.Format != format || h.Format != format ||
		b.Producer == "" || b.Producer != h.Producer || b.RepositoryID != h.RepositoryID ||
		b.ComponentID != h.ComponentID || b.ProfileID != h.ProfileID {
		return entity.StateNotEvaluated, _reasonCodeScopeMismatch
	}
	if c.Base.Report.Completeness != "complete" || c.Head.Report.Completeness != "complete" {
		return entity.StateNotEvaluated, _reasonReportPartial
	}
	return state, reason
}

func overlapsChangedLines(f *entity.MRDiffFile, r entity.CodeRange) bool {
	if f == nil || f.Binary {
		return false
	}
	i, _ := slices.BinarySearchFunc(f.HeadRanges, r.Start, func(h entity.MRDiffRange, start int) int {
		return cmp.Compare(h.To, start)
	})
	return i < len(f.HeadRanges) && f.HeadRanges[i].From <= r.End
}

type MRFileKind string

const (
	MRFileRegular   MRFileKind = "regular"
	MRFileGenerated MRFileKind = "generated"
	MRFileVendor    MRFileKind = "vendor"
)

type MRSizeCounts struct {
	Files   int
	Added   int64
	Deleted int64
}

func (c *MRSizeCounts) add(f *entity.MRDiffFile) {
	c.Files++
	if f.AddedLines != nil {
		c.Added += int64(*f.AddedLines)
	}
	if f.DeletedLines != nil {
		c.Deleted += int64(*f.DeletedLines)
	}
}

type MRSizeContribution struct {
	File       entity.MRDiffFile
	Kind       MRFileKind
	Role       entity.ContributionRole
	ReasonCode string
}

type MRSizeMetric struct {
	State         entity.DataState
	ReasonCode    string
	Total         MRSizeCounts
	Regular       MRSizeCounts
	Generated     MRSizeCounts
	Vendor        MRSizeCounts
	Contributions []MRSizeContribution
}

func mrSize(in *MRStructureInput) MRSizeMetric {
	state, reason := mrDiffState(&in.Diff)
	out := MRSizeMetric{State: state, ReasonCode: reason}
	if state == entity.StateNotEvaluated {
		return out
	}
	baseExcluded := mrFileKinds(in.BaseExcluded, exclusionsAt(&in.Functions.Base, in.Diff.BaseSHA), exclusionsAt(&in.Clones.Base, in.Diff.BaseSHA))
	headExcluded := mrFileKinds(in.HeadExcluded, exclusionsAt(&in.Functions.Head, in.Diff.HeadSHA), exclusionsAt(&in.Clones.Head, in.Diff.HeadSHA))
	for i := range in.Diff.Files {
		f := &in.Diff.Files[i]
		kind := mrFileKind(f.NewPath, headExcluded)
		if f.DeletedFile {
			kind = mrFileKind(f.OldPath, baseExcluded)
		}
		out.Total.add(f)
		switch kind {
		case MRFileGenerated:
			out.Generated.add(f)
		case MRFileVendor:
			out.Vendor.add(f)
		default:
			out.Regular.add(f)
		}
		c := MRSizeContribution{File: *f, Kind: kind, Role: entity.RoleNumerator}
		if f.AddedLines == nil || f.DeletedLines == nil {
			c.Role, c.ReasonCode = entity.RoleMissing, _reasonLineCountsMissing
			out.State = entity.StateProvisional
			out.ReasonCode = cmp.Or(out.ReasonCode, _reasonLineCountsMissing)
		}
		out.Contributions = append(out.Contributions, c)
	}
	return out
}

func exclusionsAt(s *MRCodeSnapshot, sha string) []entity.SkippedTarget {
	if s.Report == nil || sha == "" || s.Scope.SHA != sha {
		return nil
	}
	return s.Report.Facts.ExcludedTargets
}

func mrFileKinds(targets ...[]entity.SkippedTarget) map[string]MRFileKind {
	out := make(map[string]MRFileKind)
	for _, list := range targets {
		for _, item := range list {
			kind := MRFileKind(item.Reason)
			if kind == MRFileVendor || kind == MRFileGenerated && out[item.Path] != MRFileVendor {
				out[item.Path] = kind
			}
		}
	}
	return out
}

func mrFileKind(path string, excluded map[string]MRFileKind) MRFileKind {
	kind := MRFileRegular
	for {
		flag := excluded[path]
		if flag == MRFileVendor {
			return MRFileVendor
		}
		if flag == MRFileGenerated {
			kind = MRFileGenerated
		}
		slash := strings.LastIndexByte(path, '/')
		if slash < 0 {
			return kind
		}
		path = path[:slash]
	}
}
