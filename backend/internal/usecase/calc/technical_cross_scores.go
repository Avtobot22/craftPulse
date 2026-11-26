package calc

import (
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func technicalPoint(metric string, p period, cs []entity.Contribution) entity.Point {
	if metric != "X06" {
		pt := crossRatio(p, cs)
		if metric == "X05" && pt.MissingCount > 0 {
			// Unknown steps are part of the same eligible denominator, as in A20.
			n, d := 0.0, float64(len(cs))
			for i := range cs {
				if cs[i].ReasonCode == "task_state_unknown" {
					d--
				}
			}
			if pt.Numerator != nil {
				n = *pt.Numerator
			}
			pt.Denominator = num(d)
			if pt.MissingCount < len(cs) && d > 0 {
				pt.Numerator, pt.State, pt.Value = num(n), entity.StateProvisional, num(n/d)
			}
			pt.LowSample = len(cs) < _lowSample
		}
		return pt
	}
	t := newTally(p)
	var sum, assessed float64
	for _, c := range cs {
		t.add(c.ObjectID, c.ItemNo, c.Role, c.Value, c.ReasonCode)
		if c.Role == entity.RoleNumerator || c.Role == entity.RoleDenominator {
			sum += valueOf(c.Value)
			assessed++
		}
	}
	pt := t.ratio(sum, assessed, len(cs), true)
	if assessed > 0 {
		pt.Value = num(sum)
	}
	pt.Contributions = cs
	return pt
}

func scoreTechnical(c *entity.Contribution, metric, est string, b *crossEvidence) {
	c.Value = nil
	if b.UnknownMembership {
		c.Role, c.ReasonCode = entity.RoleMissing, "task_state_unknown"
		return
	}
	known, missing, count := false, b.UnknownLink, 0
	for i := range b.MRs {
		value, assessed, applicable := technicalSignal(&b.MRs[i], metric, est)
		count += value
		known = known || assessed
		missing = missing || (applicable && !assessed)
	}
	switch {
	case count > 0:
		c.Role, c.ReasonCode = entity.RoleNumerator, est
		if metric == "X06" {
			c.Value = num(float64(count))
		}
	case missing:
		c.Role, c.ReasonCode = entity.RoleMissing, "technical_evidence_unknown"
	case metric == "X05":
		c.Role, c.ReasonCode = entity.RoleDenominator, chainGap(b, est, known)
	case known:
		c.Role, c.ReasonCode = entity.RoleDenominator, ""
	case len(b.MRs) == 0:
		c.Role, c.ReasonCode = entity.RoleExcluded, _reasonNoTaskMR
	default:
		c.Role, c.ReasonCode = entity.RoleExcluded, "not_applicable"
	}
}

func chainGap(b *crossEvidence, est string, known bool) string {
	if len(b.MRs) == 0 {
		return _reasonNoTaskMR
	}
	if !slices.ContainsFunc(b.MRs, func(m crossMR) bool { return m.PipelineID != nil }) {
		return "current_head_ci_missing"
	}
	if !known && strings.HasPrefix(est, "checked_") {
		return "scope_not_expected"
	}
	return "scope_" + est + "_missing"
}

func technicalSignal(m *crossMR, metric, est string) (count int, known, applicable bool) {
	if metric == "X04" && m.CIClass != "success" {
		return 0, false, m.CIClass == "unknown" || m.CIClass == "running"
	}
	if metric == "X05" {
		switch est {
		case "confirmed_mr":
			return 1, true, true
		case "current_head_ci":
			return boolCount(m.PipelineID != nil), true, true
		}
		if m.PipelineID == nil {
			return 0, true, true
		}
	}
	switch est {
	case "fail_to_pass":
		return evidenceBool(m.FailToPass)
	case "allowed_failure":
		if !m.AllowFailureSupported {
			return 0, false, false
		}
		return evidenceBool(m.AllowedFailure)
	}
	if m.Technical == nil {
		return 0, false, true
	}
	switch est {
	case "caveat":
		return anyTechnicalSignal(m, metric, "new_code", "new_security", "docs_violation", "missing_report", "fail_to_pass", "allowed_failure")
	case "warning":
		return anyTechnicalSignal(m, metric, "new_code", "new_security", "docs_violation")
	case "missing_report", "complete_reports":
		return reportCompleteness(m, metric, est)
	}
	return scopedSignal(m.Technical.Scopes, metric, est)
}

func reportCompleteness(m *crossMR, metric, est string) (count int, known, applicable bool) {
	t := m.Technical
	if metric == "X02" && t.MissingChecks {
		return 1, true, true
	}
	if len(t.Scopes) == 0 {
		return 0, false, t.ProjectID.IsZero()
	}
	missing := slices.ContainsFunc(t.Scopes, func(s entity.TechnicalScope) bool { return s.Complete < s.Expected })
	if est == "complete_reports" {
		return boolCount(!missing), true, true
	}
	return boolCount(missing), true, true
}

func scopedSignal(scopes []entity.TechnicalScope, metric, est string) (matches int, known, applicable bool) {
	kind, disposition, _ := strings.Cut(est, ":")
	switch est {
	case "new_code", "code_exception", "checked_code":
		kind = entity.AnalysisCode
	case "new_security", "security_exception", "checked_security":
		kind = "security"
	case "docs_violation", "checked_docs":
		kind = entity.AnalysisDocs
	case "checked_tests":
		kind = entity.AnalysisJUnit
	case "checked_coverage":
		kind = entity.AnalysisCoverage
	}
	var expected, complete, compared, count int
	for _, s := range scopes {
		if s.Kind != kind && (kind != "security" || !entity.SensitiveSummaryAnalysis(s.Kind)) {
			continue
		}
		expected += s.Expected
		complete += s.Complete
		compared += s.Comparable
		switch {
		case est == "docs_violation":
			count += s.Current
		case disposition == "accepted" || est == "code_exception" || est == "security_exception":
			count += s.Accepted
		case disposition == "persisting":
			count += s.Persisting
		case disposition == "resolved":
			count += s.Resolved
		default:
			count += s.New
		}
	}
	if expected == 0 {
		return 0, false, len(scopes) == 0
	}
	switch {
	case metric == "X05":
		return boolCount(complete == expected), true, true
	case disposition == "unverified":
		return expected - compared, true, true
	case est == "docs_violation" || disposition == "accepted" || strings.HasSuffix(est, "_exception"):
		return count, complete == expected, true
	default:
		return count, compared == expected, true
	}
}

func anyTechnicalSignal(m *crossMR, metric string, estimators ...string) (matches int, known, applicable bool) {
	var missing bool
	for _, est := range estimators {
		count, assessed, applicable := technicalSignal(m, metric, est)
		if count > 0 {
			return 1, true, true
		}
		known = known || assessed
		missing = missing || applicable && !assessed
	}
	return 0, known && !missing, known || missing
}

func evidenceBool(value *bool) (count int, known, applicable bool) {
	if value == nil {
		return 0, false, true
	}
	return boolCount(*value), true, true
}

func boolCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
