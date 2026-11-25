package calc

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func analysisProfiles(p period, asOf time.Time, runs []*entity.AnalysisRun) []entity.SeriesPoint {
	if len(runs) == 0 {
		return []entity.SeriesPoint{
			analysisPoint(p, "C35", "", UnitSeconds),
			analysisPoint(p, "C36", "p95", UnitSeconds),
		}
	}
	groups := groupAnalysisRuns(runs, analysisProfile)
	out := make([]entity.SeriesPoint, 0, len(groups)*3)
	for _, g := range groups {
		age := analysisPoint(p, "C35", analysisEstimator(g.key), UnitSeconds)
		snapshotAge(&age.Point, asOf, g.runs)
		est := "p95"
		if g.key != "" {
			est += "/" + g.key
		}
		duration := analysisPoint(p, "C36", analysisEstimator(est), UnitSeconds)
		analysisP95(&duration.Point, g.runs, analysisDuration)
		out = append(out, age, duration)
		if slices.ContainsFunc(g.runs, func(r *entity.AnalysisRun) bool { return r.QueueSeconds != nil }) {
			queue := analysisPoint(p, "C36", analysisEstimator("queue_"+est), UnitSeconds)
			analysisP95(&queue.Point, g.runs, func(r *entity.AnalysisRun) *float64 { return r.QueueSeconds })
			out = append(out, queue)
		}
	}
	return out
}

func snapshotAge(p *entity.Point, asOf time.Time, runs []*entity.AnalysisRun) {
	var latest []*entity.AnalysisRun
	var finished time.Time
	for _, snapshot := range groupAnalysisRuns(runs, func(r *entity.AnalysisRun) string { return r.ReceiptID.String() }) {
		if slices.ContainsFunc(snapshot.runs, func(r *entity.AnalysisRun) bool {
			return incompleteAnalysis(r) != "" || analysisFinished(r).IsZero()
		}) {
			continue
		}
		at := analysisFinished(latestAnalysisRun(snapshot.runs))
		if at.After(finished) {
			latest, finished = snapshot.runs, at
		}
	}
	if finished.IsZero() {
		p.ReasonCode = _analysisCompleteMissing
	} else {
		p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(asOf.Sub(finished).Seconds())
		for _, r := range latest {
			addAnalysisContribution(p, r, entity.RoleNumerator, num(asOf.Sub(analysisFinished(r)).Seconds()), "")
		}
	}
	for _, r := range runs {
		if reason := incompleteAnalysis(r); reason != "" && !analysisFinished(r).Before(finished) {
			addAnalysisContribution(p, r, entity.RoleMissing, nil, reason)
		}
	}
}

func analysisDuration(r *entity.AnalysisRun) *float64 {
	if r.StartedAt == nil || r.FinishedAt == nil {
		return nil
	}
	return num(r.FinishedAt.Sub(*r.StartedAt).Seconds())
}

func analysisP95(p *entity.Point, runs []*entity.AnalysisRun, measure func(*entity.AnalysisRun) *float64) {
	samples := make([]float64, 0, len(runs))
	for _, r := range runs {
		value := measure(r)
		if value == nil {
			addAnalysisContribution(p, r, entity.RoleMissing, nil, "analysis_timing_unknown")
			continue
		}
		samples = append(samples, *value)
		addAnalysisContribution(p, r, entity.RoleNumerator, value, incompleteAnalysis(r))
	}
	p.N = new(len(samples))
	if len(samples) == 0 {
		p.ReasonCode = "analysis_timing_unknown"
		return
	}
	slices.Sort(samples)
	p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(percentile(samples, 95))
}

func dependencyResolution(p *entity.Point, runs []*entity.AnalysisRun) {
	r := latestAnalysisRun(runs)
	if !usableAnalysis(r) {
		unknownAnalysis(p, r, incompleteAnalysis(r))
		return
	}
	var totals entity.DependencyInventory
	var known bool
	for _, report := range r.Reports {
		if inventory := report.Dependencies; inventory != nil {
			known = true
			totals.Entries += inventory.Entries
			totals.Resolved += inventory.Resolved
			totals.Partial = totals.Partial || inventory.Partial
		} else {
			totals.Partial = true
		}
	}
	if !known {
		unknownAnalysis(p, r, "dependency_inventory_unknown")
		return
	}
	if totals.Entries == 0 && totals.Partial {
		unknownAnalysis(p, r, "dependency_inventory_partial")
		return
	}
	analysisRatio(p, totals.Resolved, totals.Entries)
	addAnalysisContribution(p, r, entity.RoleNumerator, p.Numerator, "")
	addAnalysisContribution(p, r, entity.RoleDenominator, p.Denominator, "")
	if totals.Partial {
		p.State, p.ReasonCode = entity.StateProvisional, "dependency_inventory_partial"
	} else if reason := incompleteAnalysis(r); reason != "" && p.Value != nil {
		p.State, p.ReasonCode = entity.StateProvisional, reason
	}
}

func checkedHistory(p *entity.Point, runs []*entity.AnalysisRun) {
	r := latestAnalysisRun(runs)
	switch {
	case !usableAnalysis(r):
		unknownAnalysis(p, r, incompleteAnalysis(r))
	case r.History == nil:
		unknownAnalysis(p, r, "history_range_unknown")
	default:
		p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(0)
		if r.History.Truncated {
			p.State, p.ReasonCode, p.Value = entity.StateProvisional, "history_truncated", num(1)
		} else if reason := incompleteAnalysis(r); reason != "" {
			p.State, p.ReasonCode = entity.StateProvisional, reason
		}
		addAnalysisContribution(p, r, entity.RoleNumerator, p.Value, p.ReasonCode)
	}
}

func vulnerabilityDBAge(p *entity.Point, runs []*entity.AnalysisRun, asOf time.Time) {
	r := latestAnalysisRun(runs)
	switch {
	case !usableAnalysis(r):
		unknownAnalysis(p, r, incompleteAnalysis(r))
	case r.DBPublished == nil || r.DBPublished.After(asOf):
		unknownAnalysis(p, r, "db_publication_unknown")
	default:
		p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(asOf.Sub(*r.DBPublished).Seconds())
		addAnalysisContribution(p, r, entity.RoleNumerator, p.Value, "")
	}
}

func documentationCollection(p period, runs []*entity.AnalysisRun, versions []entity.AnalysisDocumentVersion) []entity.SeriesPoint {
	completeness := analysisCompletion(p, "D27", "", latestManifest(runs))
	last := analysisPoint(p, "D27", "last_success", UnitSeconds)
	lastCompleteAnalysis(&last.Point, runs, timestampSeconds)
	first := analysisPoint(p, "D27", "earliest_version", UnitSeconds)
	first.ReasonCode = "document_version_unknown"
	for _, v := range versions {
		at := num(timestampSeconds(v.ObservedAt))
		if first.Value == nil || *at < *first.Value {
			first.State, first.ReasonCode, first.Value = entity.StateAssessed, "", at
		}
		raw, _ := json.Marshal(v)
		first.Contributions = append(first.Contributions, entity.Contribution{
			ObjectID: v.ID, ObjectKind: entity.ObjectDocument, ItemNo: 1, Role: entity.RoleNumerator, Value: at, Evidence: raw,
		})
	}
	return []entity.SeriesPoint{completeness, last, first}
}
