package calc

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_analysisMissing         = "analysis_run_missing"
	_analysisCompleteMissing = "complete_analysis_run_missing"
)

type analysisGroup struct {
	key  string
	runs []*entity.AnalysisRun
}

func analysisPoints(asOf time.Time, p period, facts *entity.AnalysisFacts) []entity.SeriesPoint {
	var runs []*entity.AnalysisRun
	for i := range facts.Runs {
		r := &facts.Runs[i]
		if r.FinishedAt == nil || !r.FinishedAt.After(asOf) {
			runs = append(runs, r)
		}
	}
	slices.SortFunc(runs, compareAnalysisRuns)
	security := selectAnalysisRuns(runs, securityAnalysis)
	docs := selectAnalysisRuns(runs, docsAnalysis)
	out := []entity.SeriesPoint{
		analysisCompletion(p, "C03", "", latestManifest(runs)),
		analysisCompletion(p, "S07", "", latestManifest(security)),
	}
	out = append(out, groupedAnalysis(p, "C02", selectAnalysisRuns(runs, codeAnalysis), fileScope, fileCoverage)...)
	out = append(out, groupedAnalysis(p, "S02", selectAnalysisRuns(security, func(r *entity.AnalysisRun) bool { return r.Kind == entity.AnalysisSAST }),
		fileScope, fileCoverage)...)
	out = append(out, analysisProfiles(p, asOf, runs)...)
	out = append(out, groupedAnalysis(p, "S01", security, securityProfile, func(pt *entity.Point, group []*entity.AnalysisRun) {
		lastCompleteAnalysis(pt, group, timestampSeconds)
	})...)
	out = append(out, groupedAnalysis(p, "S08", security, analysisScopeKey, func(pt *entity.Point, group []*entity.AnalysisRun) {
		lastCompleteAnalysis(pt, group, func(t time.Time) float64 { return asOf.Sub(t).Seconds() })
	})...)
	sca := selectAnalysisRuns(security, func(r *entity.AnalysisRun) bool { return r.Kind == entity.AnalysisSCA })
	out = append(out, groupedAnalysis(p, "S03", sca, analysisScopeKey, dependencyResolution)...)
	out = append(out, groupedAnalysis(p, "S04", selectAnalysisRuns(security, func(r *entity.AnalysisRun) bool {
		return r.Kind == entity.AnalysisSecrets
	}), analysisScopeKey, checkedHistory)...)
	out = append(out, groupedAnalysis(p, "S05", selectAnalysisRuns(security, usesVulnerabilityDB), analysisScopeKey,
		func(pt *entity.Point, group []*entity.AnalysisRun) { vulnerabilityDBAge(pt, group, asOf) })...)
	out = append(out, documentationCollection(p, docs, facts.Versions)...)
	return out
}

func compareAnalysisRuns(a, b *entity.AnalysisRun) int {
	return cmp.Or(cmp.Compare(b.KnownSeq, a.KnownSeq), slices.Compare(b.ReceiptID[:], a.ReceiptID[:]),
		cmp.Compare(a.ToolID, b.ToolID), slices.Compare(a.ID[:], b.ID[:]))
}

func analysisFinished(r *entity.AnalysisRun) time.Time {
	if r.FinishedAt == nil {
		return time.Time{}
	}
	return *r.FinishedAt
}

func selectAnalysisRuns(runs []*entity.AnalysisRun, keep func(*entity.AnalysisRun) bool) []*entity.AnalysisRun {
	return slices.DeleteFunc(slices.Clone(runs), func(r *entity.AnalysisRun) bool { return !keep(r) })
}

func securityAnalysis(r *entity.AnalysisRun) bool {
	switch r.Kind {
	case entity.AnalysisSAST, entity.AnalysisSCA, entity.AnalysisSecrets, entity.AnalysisIaC, entity.AnalysisImage:
		return true
	}
	return false
}

func docsAnalysis(r *entity.AnalysisRun) bool {
	return r.Kind == entity.AnalysisDocs || r.Kind == entity.AnalysisOpenAPI
}

func codeAnalysis(r *entity.AnalysisRun) bool {
	return r.Kind == entity.AnalysisCode || r.Kind == entity.AnalysisSonar
}

func usesVulnerabilityDB(r *entity.AnalysisRun) bool {
	return r.Kind == entity.AnalysisSCA || r.Kind == entity.AnalysisImage
}

func analysisProfile(r *entity.AnalysisRun) string {
	if r.Profile != "" {
		return r.Profile
	}
	return r.Producer
}

func securityProfile(r *entity.AnalysisRun) string {
	return analysisKey(r.Kind, analysisProfile(r))
}

func analysisScopeKey(r *entity.AnalysisRun) string {
	secretScope := ""
	if r.Kind == entity.AnalysisSecrets && r.History != nil {
		secretScope = entity.SecretScopeHead
		if r.History.FromSHA != r.History.ToSHA || r.History.Truncated || strings.HasSuffix(r.ToolID, "-diff") {
			secretScope = analysisKey(entity.SecretScopeDiff, r.History.FromSHA, r.History.ToSHA)
		}
	}
	return analysisKey(r.Kind, analysisProfile(r), r.Mode, r.Producer, r.Language, r.PathPrefix, r.ScopeHash, secretScope)
}

func fileScope(r *entity.AnalysisRun) string {
	return analysisKey(r.Language, r.Producer, analysisProfile(r), r.Mode, r.PathPrefix, r.ScopeHash)
}

func analysisKey(parts ...string) string {
	return strings.Join(parts, "\x00")
}

func analysisEstimator(key string) string {
	if len(key) <= 32 && !strings.ContainsRune(key, 0) && !strings.HasPrefix(key, "~") {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	return "~" + hex.EncodeToString(sum[:15])
}

func groupAnalysisRuns(runs []*entity.AnalysisRun, key func(*entity.AnalysisRun) string) []analysisGroup {
	groups := make(map[string][]*entity.AnalysisRun)
	for _, r := range runs {
		k := key(r)
		groups[k] = append(groups[k], r)
	}
	out := make([]analysisGroup, 0, len(groups))
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		out = append(out, analysisGroup{key: k, runs: groups[k]})
	}
	return out
}

func groupedAnalysis(p period, id string, runs []*entity.AnalysisRun, key func(*entity.AnalysisRun) string,
	measure func(*entity.Point, []*entity.AnalysisRun),
) []entity.SeriesPoint {
	if len(runs) == 0 {
		return []entity.SeriesPoint{analysisPoint(p, id, "", definition(id).Unit)}
	}
	groups := groupAnalysisRuns(runs, key)
	out := make([]entity.SeriesPoint, 0, len(groups))
	for _, g := range groups {
		pt := analysisPoint(p, id, analysisEstimator(g.key), definition(id).Unit)
		measure(&pt.Point, g.runs)
		out = append(out, pt)
	}
	return out
}

func analysisPoint(p period, id, estimator, unit string) entity.SeriesPoint {
	return entity.SeriesPoint{
		MetricID: id, Estimator: estimator, Unit: unit,
		Point: entity.Point{Start: p.start, End: p.end, State: entity.StateUnknown, ReasonCode: _analysisMissing},
	}
}

func latestManifest(runs []*entity.AnalysisRun) []*entity.AnalysisRun {
	if len(runs) == 0 {
		return nil
	}
	return selectAnalysisRuns(runs, func(r *entity.AnalysisRun) bool { return r.ReceiptID == runs[0].ReceiptID })
}

func analysisCompletion(p period, id, estimator string, runs []*entity.AnalysisRun) entity.SeriesPoint {
	pt := analysisPoint(p, id, estimator, UnitRatio)
	if len(runs) == 0 {
		return pt
	}
	var complete int
	for _, r := range runs {
		if reason := incompleteAnalysis(r); reason != "" {
			addAnalysisContribution(&pt.Point, r, entity.RoleMissing, nil, reason)
		} else {
			complete++
			addAnalysisContribution(&pt.Point, r, entity.RoleNumerator, num(1), "")
		}
		addAnalysisContribution(&pt.Point, r, entity.RoleDenominator, num(1), "")
	}
	analysisRatio(&pt.Point, complete, len(runs))
	return pt
}

func incompleteAnalysis(r *entity.AnalysisRun) string {
	switch r.ExitClass {
	case "failed", "timeout", "partial":
		return "analysis_" + r.ExitClass
	case "success", "findings":
	default:
		return "analysis_outcome_unknown"
	}
	if reason := invalidAnalysisReport(r); reason != "" {
		return reason
	}
	switch r.Completeness {
	case "complete":
		return ""
	case "partial":
		return "analysis_partial"
	default:
		return "analysis_completeness_unknown"
	}
}

func invalidAnalysisReport(r *entity.AnalysisRun) string {
	for i := range r.Reports {
		if r.Reports[i].Reason != "" {
			return r.Reports[i].Reason
		}
	}
	return ""
}

func usableAnalysis(r *entity.AnalysisRun) bool {
	return r.ExitClass != "failed" && r.ExitClass != "timeout" && invalidAnalysisReport(r) == ""
}

func analysisRatio(p *entity.Point, n, d int) {
	p.Numerator, p.Denominator = num(float64(n)), num(float64(d))
	p.N = new(d)
	if d == 0 {
		p.State, p.ReasonCode = entity.StateNotApplicable, "empty_eligible_scope"
		return
	}
	p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(float64(n)/float64(d))
}

func fileCoverage(p *entity.Point, runs []*entity.AnalysisRun) {
	r := latestAnalysisRun(runs)
	switch {
	case !usableAnalysis(r):
		unknownAnalysis(p, r, incompleteAnalysis(r))
	case r.EligibleFiles == nil:
		unknownAnalysis(p, r, "eligible_files_unknown")
	case r.ScannedFiles == nil || *r.ScannedFiles > *r.EligibleFiles:
		unknownAnalysis(p, r, "analyzed_files_unknown")
	default:
		analysisRatio(p, *r.ScannedFiles, *r.EligibleFiles)
		addAnalysisContribution(p, r, entity.RoleNumerator, p.Numerator, "")
		addAnalysisContribution(p, r, entity.RoleDenominator, p.Denominator, "")
		if reason := incompleteAnalysis(r); reason != "" && p.Value != nil {
			p.State, p.ReasonCode = entity.StateProvisional, reason
		}
	}
}

func latestAnalysisRun(runs []*entity.AnalysisRun) *entity.AnalysisRun {
	return slices.MaxFunc(runs, func(a, b *entity.AnalysisRun) int {
		return cmp.Or(analysisFinished(a).Compare(analysisFinished(b)), compareAnalysisRuns(b, a))
	})
}

func lastCompleteAnalysis(p *entity.Point, runs []*entity.AnalysisRun, measure func(time.Time) float64) {
	var last *entity.AnalysisRun
	for _, r := range runs {
		if incompleteAnalysis(r) == "" && !analysisFinished(r).IsZero() &&
			(last == nil || analysisFinished(r).After(analysisFinished(last))) {
			last = r
		}
	}
	if last == nil {
		p.ReasonCode = _analysisCompleteMissing
	} else {
		p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(measure(analysisFinished(last)))
		addAnalysisContribution(p, last, entity.RoleNumerator, p.Value, "")
	}
	for _, r := range runs {
		if last != nil && analysisFinished(r).Before(analysisFinished(last)) {
			continue
		}
		if reason := incompleteAnalysis(r); reason != "" {
			addAnalysisContribution(p, r, entity.RoleMissing, nil, reason)
		}
	}
}

func timestampSeconds(t time.Time) float64 {
	return float64(t.Unix()) + float64(t.Nanosecond())/float64(time.Second)
}

func unknownAnalysis(p *entity.Point, r *entity.AnalysisRun, reason string) {
	p.State, p.ReasonCode = entity.StateUnknown, reason
	addAnalysisContribution(p, r, entity.RoleMissing, nil, reason)
}

func addAnalysisContribution(p *entity.Point, r *entity.AnalysisRun, role entity.ContributionRole, value *float64, reason string) {
	raw, _ := json.Marshal(struct {
		entity.AnalysisRunMetadata
		ToolID    string                  `json:"tool_id"`
		ReceiptID entity.ID               `json:"receipt_id"`
		Reports   []entity.AnalysisReport `json:"reports,omitempty"`
	}{r.AnalysisRunMetadata, r.ToolID, r.ReceiptID, r.Reports})
	itemNo := 1
	if role == entity.RoleDenominator {
		itemNo = 2
	}
	if role == entity.RoleMissing {
		p.MissingCount++
	}
	p.Contributions = append(p.Contributions, entity.Contribution{
		ObjectID: r.ID, ObjectKind: entity.ObjectAnalysisRun, ItemNo: itemNo, Role: role, Value: value, ReasonCode: reason, Evidence: raw,
	})
}
