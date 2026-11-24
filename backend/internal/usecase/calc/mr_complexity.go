package calc

import (
	"cmp"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _reasonAmbiguousFunction = "ambiguous_function_identity"

type MRFunctionContribution struct {
	Head       entity.CodeFunction
	Base       *entity.CodeFunction
	Delta      *int
	Role       entity.ContributionRole
	ReasonCode string
}

type MRComplexityMetric struct {
	State        entity.DataState
	ReasonCode   string
	BaseReportID entity.ID
	HeadReportID entity.ID
	Deltas       []MRFunctionContribution
	NewFunctions []MRFunctionContribution
	Excluded     []MRFunctionContribution
}

type functionKey struct{ path, name string }

type functionIdentity struct {
	base          map[functionKey]*entity.CodeFunction
	baseCount     map[functionKey]int
	headCount     map[functionKey]int
	unmatchedName map[string]bool
}

func mrComplexity(diff *entity.MRDiff, files map[string]*entity.MRDiffFile, pair *MRCodeComparison) MRComplexityMetric {
	state, reason := pair.state(diff, entity.CodeFormatLizard)
	out := MRComplexityMetric{State: state, ReasonCode: reason}
	if state == entity.StateNotEvaluated {
		return out
	}
	b, h := pair.Base.Report, pair.Head.Report
	out.BaseReportID, out.HeadReportID = b.ID, h.ID
	identity := indexFunctions(b.Facts.Functions, h.Facts.Functions, files)
	for i := range h.Facts.Functions {
		fn := &h.Facts.Functions[i]
		file := files[fn.Path]
		if file == nil || file.Binary {
			continue
		}
		out.add(fn, file, &identity)
	}
	return out
}

func indexFunctions(base, head []entity.CodeFunction, files map[string]*entity.MRDiffFile) functionIdentity {
	out := functionIdentity{
		base: make(map[functionKey]*entity.CodeFunction, len(base)), baseCount: make(map[functionKey]int, len(base)),
		headCount: make(map[functionKey]int, len(head)), unmatchedName: make(map[string]bool),
	}
	for i := range base {
		fn := &base[i]
		key := functionKey{path: fn.Path, name: fn.Name}
		out.base[key] = fn
		out.baseCount[key]++
	}
	for i := range head {
		fn := &head[i]
		key := previousFunction(fn, files[fn.Path])
		out.headCount[key]++
	}
	for key, count := range out.baseCount {
		if count != 1 || out.headCount[key] != 1 {
			out.unmatchedName[key.name] = true
		}
	}
	return out
}

func previousFunction(fn *entity.CodeFunction, file *entity.MRDiffFile) functionKey {
	key := functionKey{path: fn.Path, name: fn.Name}
	if file == nil {
		return key
	}
	if file.NewFile {
		key.path = ""
	} else if file.RenamedFile {
		key.path = file.OldPath
	}
	return key
}

func (m *MRComplexityMetric) add(fn *entity.CodeFunction, file *entity.MRDiffFile, identity *functionIdentity) {
	key := previousFunction(fn, file)
	base := identity.base[key]
	changed := overlapsChangedLines(file, fn.CodeRange)
	contribution := MRFunctionContribution{Head: *fn, Role: entity.RoleNumerator}
	switch {
	case base != nil && identity.baseCount[key] == 1 && identity.headCount[key] == 1:
		delta := fn.CCN - base.CCN
		if changed || delta != 0 {
			contribution.Base, contribution.Delta = new(*base), &delta
			m.Deltas = append(m.Deltas, contribution)
		}
	case !changed:
		return
	case base != nil || identity.unmatchedName[fn.Name]:
		contribution.Role, contribution.ReasonCode = entity.RoleExcluded, _reasonAmbiguousFunction
		m.Excluded = append(m.Excluded, contribution)
		m.State, m.ReasonCode = entity.StateProvisional, cmp.Or(m.ReasonCode, _reasonAmbiguousFunction)
	default:
		contribution.ReasonCode = "new_function"
		m.NewFunctions = append(m.NewFunctions, contribution)
	}
}
