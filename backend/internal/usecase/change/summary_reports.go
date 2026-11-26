package change

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) summarizeReports(ctx context.Context, org entity.ID, mr entity.SummaryMR, entries []entity.ProfileEntry,
	v *entity.SummaryRevision,
) (time.Time, error) {
	var latest time.Time
	versions, err := u.d.Decisions.ActiveAt(ctx, org, mr.AreaID, mr.KnowledgeSeq, u.d.Clock.Now())
	if err != nil {
		return latest, err
	}
	for i := range versions {
		v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "decision_version", ID: versions[i].ID})
	}
	for i := range entries {
		e := &entries[i]
		expected := entity.ExpectedReport{ComponentID: e.ComponentID, Kind: e.AnalysisKind, Language: e.Language}
		v.ExpectedReports = append(v.ExpectedReports, expected)
		v.Technical.Streams = append(v.Technical.Streams, entity.TechnicalStream{ExpectedReport: expected})
		at, err := u.summarizeRow(ctx, org, mr, e, versions, v)
		if err != nil {
			return latest, err
		}
		if at.After(latest) {
			latest = at
		}
	}
	return latest, nil
}

func (u *UseCase) summarizeRow(ctx context.Context, org entity.ID, mr entity.SummaryMR, e *entity.ProfileEntry, versions []entity.ExceptionVersion,
	v *entity.SummaryRevision,
) (time.Time, error) {
	rs, err := u.d.Summary.Facts.SummaryReports(ctx, org, mr, *e)
	if err != nil {
		return time.Time{}, err
	}
	rs = slices.DeleteFunc(rs, func(r entity.TechnicalReport) bool {
		pinned := e.ToolVersions[r.Producer]
		return !r.Rejected && pinned != "" && r.ProducerVersion != "" && r.ProducerVersion != pinned
	})
	head := findTechnicalReport(rs, func(r *entity.TechnicalReport) bool {
		return r.SHA == mr.HeadSHA && r.Complete() && !summaryCodeAuxiliary(r)
	})
	if e.AnalysisKind == entity.AnalysisCode {
		addSummaryCodePairs(mr, e, rs, v)
	}
	if head == nil {
		failed := findTechnicalReport(rs, func(r *entity.TechnicalReport) bool {
			return r.SHA == mr.HeadSHA && !summaryCodeAuxiliary(r)
		})
		if failed != nil {
			advanceTechnicalTime(v, failed.EventAt)
		}
		return time.Time{}, u.summaryMissing(ctx, org, mr, e, failed, v)
	}
	advanceTechnicalTime(v, head.EventAt)
	v.Technical.Streams[len(v.Technical.Streams)-1].Complete = true
	v.CompleteReports = append(v.CompleteReports, entity.SummaryReportEvidence{
		ExpectedReport: entity.ExpectedReport{ComponentID: e.ComponentID, Kind: e.AnalysisKind, Language: e.Language},
		ReportID:       head.ID,
	})
	v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: head.ID})
	base := summaryComparisonBase(mr.BaseSHA, e.AnalysisKind, rs, head)
	if e.AnalysisKind == entity.AnalysisJUnit || e.AnalysisKind == entity.AnalysisCoverage {
		return u.summarizeMeasurementReports(ctx, org, mr.HeadSHA, rs, v)
	}
	if base != nil {
		advanceTechnicalTime(v, base.EventAt)
		v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: base.ID})
		baselines, err := u.d.Repo.CurrentBaselines(ctx, org, []entity.ID{e.ComponentID})
		if err != nil {
			return time.Time{}, err
		}
		for i := range baselines {
			b := &baselines[i]
			if b.ReportID == base.ID && b.ProfileVersionID == v.ProfileVersionID {
				v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "baseline", ID: b.ID})
			}
		}
	}
	ids := []entity.ID{head.ID}
	if base != nil && base.ID != head.ID {
		ids = append(ids, base.ID)
	}
	reports, err := u.d.Reports.ReportsByID(ctx, org, ids)
	if err != nil {
		return time.Time{}, err
	}
	if err := u.summaryFindings(ctx, org, mr, e, base, head, reports, versions, v); err != nil {
		return time.Time{}, err
	}
	return u.d.Summary.Facts.SummaryReportReadyAt(ctx, org, head.ID)
}

func advanceTechnicalTime(v *entity.SummaryRevision, at time.Time) {
	if at.After(v.Technical.EffectiveAt) {
		v.Technical.EffectiveAt = at
	}
}

func (u *UseCase) summaryMissing(ctx context.Context, org entity.ID, mr entity.SummaryMR, e *entity.ProfileEntry,
	report *entity.TechnicalReport, v *entity.SummaryRevision,
) error {
	reason := "scan_missing"
	if report != nil {
		reason = summaryMissingReason(report.Rejected, report.Interrupted, report.ExitClass)
		v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: report.ID})
	} else {
		facts, err := u.d.Summary.Calculations.AnalysisFacts(ctx, org, entity.AnalysisScope{
			RepositoryID: mr.AreaID, ComponentID: e.ComponentID, SHA: mr.HeadSHA,
		}, mr.KnowledgeSeq, u.d.Clock.Now())
		if err != nil {
			return err
		}
		for i := range facts.Runs {
			r := &facts.Runs[i]
			if e.AnalysisKind == entity.AnalysisCode && (r.Producer == "lizard" || r.Producer == "jscpd") {
				continue
			}
			if r.Kind != e.AnalysisKind || (e.Language != "" && r.Language != e.Language) || r.Mode != e.Mode || r.Profile != e.ProfileID {
				continue
			}
			rejected := false
			for _, report := range r.Reports {
				v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: report.ID})
				rejected = rejected || report.Reason != ""
			}
			reason = summaryMissingReason(rejected, false, r.ExitClass)
			break
		}
	}
	v.PipelineState = "reports_missing"
	right := "none"
	if entity.SensitiveSummaryAnalysis(e.AnalysisKind) {
		right = "security"
	}
	v.Items = append(v.Items, entity.SummaryItem{
		Mode: e.Mode, Kind: "not_evaluated", ComponentID: &e.ComponentID, AnalysisKind: e.AnalysisKind, ReasonCode: reason, RequiredRight: right,
	})
	return nil
}

func summaryMissingReason(rejected, interrupted bool, exit string) string {
	switch {
	case rejected:
		return "import_failed"
	case interrupted || exit == "failed" || exit == "timeout":
		return "scan_failed"
	default:
		return "scan_incomplete"
	}
}

func (u *UseCase) summaryFindings(ctx context.Context, org entity.ID, mr entity.SummaryMR, e *entity.ProfileEntry,
	base, head *entity.TechnicalReport, reports []entity.ReportFindings, versions []entity.ExceptionVersion, v *entity.SummaryRevision,
) error {
	ids := make([]entity.ID, len(reports))
	for i := range reports {
		ids[i] = reports[i].ID
	}
	occurrences, err := u.d.Reports.TechnicalOccurrences(ctx, org, ids)
	if err != nil {
		return err
	}
	var keys []entity.ID
	// A report may become visible before its identify job. Apply the same identity rules under the
	// knowledge lock so the summary can include it immediately and the identify job remains idempotent.
	slices.SortFunc(reports, func(a, b entity.ReportFindings) int {
		return cmp.Or(cmp.Compare(a.KnownSeq, b.KnownSeq), compareIDs(a.ID, b.ID))
	})
	for i := range reports {
		r := &reports[i]
		r.Occurrences = occurrences[r.ID]
		if err := u.identify(ctx, org, r, mr.KnowledgeSeq); err != nil {
			return err
		}
		for j := range r.Occurrences {
			keys = append(keys, r.Occurrences[j].ID)
		}
	}
	head.Occurrences = occurrences[head.ID]
	if base != nil {
		base.Occurrences = occurrences[base.ID]
	}
	identities, err := u.d.Repo.OccurrenceIdentities(ctx, org, keys)
	if err != nil {
		return err
	}
	q := entity.TechnicalComparisonQuery{ComponentID: e.ComponentID, BaseSHA: mr.BaseSHA, HeadSHA: mr.HeadSHA, Kind: e.AnalysisKind}
	result := CompareReports(q, base, head, identities)
	v.ComparableBase = v.ComparableBase || result.Comparable
	headOccurrences := summaryOccurrences(head.Occurrences, identities)
	var baseOccurrences map[entity.ID][]entity.ReportOccurrence
	if base != nil {
		baseOccurrences = summaryOccurrences(base.Occurrences, identities)
	}
	calculation := entity.SummaryFindingComparison{
		Head: head, Base: base, AreaID: mr.AreaID, ComponentID: e.ComponentID, AnalysisKind: e.AnalysisKind,
		AsOf: u.d.Clock.Now(),
	}
	if len(result.IncomparableReasons) > 0 {
		calculation.ReasonCode = result.IncomparableReasons[0]
	}
	severities, err := u.d.Summary.Facts.SummarySeverities(ctx, org, keys)
	if err != nil {
		return err
	}
	start := len(v.Items)
	for _, f := range result.Findings {
		os, report := headOccurrences[f.ID], head.ID
		switch f.Lifecycle {
		case entity.FindingResolved:
			os, report = baseOccurrences[f.ID], base.ID
		case entity.FindingUncertain:
			if len(os) == 0 {
				continue
			}
			f.Lifecycle = entity.FindingNewlyObserved
		case entity.FindingOpen:
			if e.AnalysisKind != entity.AnalysisDocs && e.AnalysisKind != entity.AnalysisOpenAPI {
				continue
			}
		}
		summaryFinding(&calculation, report, f, os, versions, severities, v)
	}
	for i := start; i < len(v.Items); i++ {
		v.Items[i].Mode = e.Mode
	}
	stream := &v.Technical.Streams[len(v.Technical.Streams)-1]
	stream.Comparable = result.Comparable && !slices.ContainsFunc(result.Findings, func(f entity.TechnicalFinding) bool {
		return f.Lifecycle == entity.FindingUncertain
	})
	stream.Findings = calculation.Findings
	if e.AnalysisKind == entity.AnalysisDocs || e.AnalysisKind == entity.AnalysisOpenAPI {
		stream.DocumentationKey = documentationComparisonKey(head, identities)
	}
	values, definitions := u.d.Summary.Calculations.SummarizeFindings(calculation, versions)
	v.FindingValues = append(v.FindingValues, values...)
	for _, def := range definitions {
		if !slices.Contains(v.Definitions, def) {
			v.Definitions = append(v.Definitions, def)
		}
	}
	return nil
}

func summaryOccurrences(occurrences []entity.ReportOccurrence, ids []entity.OccurrenceIdentity) map[entity.ID][]entity.ReportOccurrence {
	byOccurrence := make(map[entity.ID]entity.ID, len(ids))
	for _, id := range ids {
		byOccurrence[id.OccurrenceID] = id.FindingIdentityID
	}
	out := make(map[entity.ID][]entity.ReportOccurrence)
	for i := range occurrences {
		o := &occurrences[i]
		id := byOccurrence[o.ID]
		out[id] = append(out[id], *o)
	}
	return out
}

func summaryFinding(calculation *entity.SummaryFindingComparison, report entity.ID, f entity.TechnicalFinding,
	occurrences []entity.ReportOccurrence, versions []entity.ExceptionVersion, severities map[entity.ID]string, v *entity.SummaryRevision,
) {
	right := "none"
	if entity.SensitiveFinding(f.Kind) {
		right = "security"
	}
	var accepted, applicable *entity.SummaryItem
	for i := range occurrences {
		o := &occurrences[i]
		item := entity.SummaryItem{
			Kind: f.Lifecycle, FindingID: &f.ID, OccurrenceID: &o.ID, RequiredRight: right,
			ComponentID: &calculation.ComponentID, AnalysisKind: calculation.AnalysisKind,
		}
		item.Severity = severities[o.ID]
		coords := entity.FindingCoordinates{
			FindingAccess: entity.FindingAccess{AreaID: calculation.AreaID, Kind: f.Kind}, ComponentID: calculation.ComponentID,
			FindingID: f.ID, RuleID: o.RuleID, Path: o.Path, PackagePURL: o.PackagePURL,
		}
		if right == "none" {
			item.Value, _ = json.Marshal(entity.SummarySignalLocation{Rule: o.RuleID, Path: o.Path, Package: o.PackagePURL, Line: o.LineStart})
		}
		calculation.Findings = append(calculation.Findings, entity.SummaryFinding{
			FindingCoordinates: coords, OccurrenceID: o.ID, ReportID: report, Lifecycle: f.Lifecycle, Severity: item.Severity, LineStart: o.LineStart,
		})
		for j := range versions {
			if versions[j].Matches(coords) {
				item.Kind = "accepted"
				break
			}
		}
		best := &applicable
		if item.Kind == "accepted" {
			best = &accepted
		}
		if *best == nil || entity.CompareSummarySignals(&item, *best) < 0 {
			*best = &item
		}
	}
	if f.Lifecycle == entity.FindingOpen {
		return
	}
	if accepted != nil {
		v.Items = append(v.Items, *accepted)
	}
	// An exception scoped to one path must not suppress other occurrences of the same finding.
	if applicable != nil {
		v.Items = append(v.Items, *applicable)
	}
}

func summaryCodeAuxiliary(r *entity.TechnicalReport) bool {
	return r.Format == entity.CodeFormatLizard || r.Format == entity.CodeFormatJscpd
}

func addSummaryCodePairs(mr entity.SummaryMR, e *entity.ProfileEntry, reports []entity.TechnicalReport, v *entity.SummaryRevision) {
	for _, format := range []string{entity.CodeFormatLizard, entity.CodeFormatJscpd} {
		head := findTechnicalReport(reports, func(r *entity.TechnicalReport) bool {
			return r.SHA == mr.HeadSHA && r.Format == format && r.Complete()
		})
		if head == nil {
			continue
		}
		base := summaryComparisonBase(mr.BaseSHA, e.AnalysisKind, reports, head)
		q := entity.TechnicalComparisonQuery{ComponentID: e.ComponentID, Kind: e.AnalysisKind, HeadSHA: mr.HeadSHA, BaseSHA: mr.BaseSHA}
		pair := entity.SummaryCodePair{ComponentID: e.ComponentID, Head: head, Base: base, Comparable: CompareReports(q, base, head, nil).Comparable}
		v.CodePairs = append(v.CodePairs, pair)
		v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: head.ID})
		if base != nil {
			v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: base.ID})
		}
	}
}

// Tests and coverage can be sharded across jobs of a pipeline. Their definitions select the applicable
// executions and coverage scope from all complete reports of this matrix row.
func (u *UseCase) summarizeMeasurementReports(ctx context.Context, org entity.ID, head string,
	reports []entity.TechnicalReport, v *entity.SummaryRevision,
) (time.Time, error) {
	var latest time.Time
	for i := range reports {
		r := &reports[i]
		if r.SHA != head || !r.Complete() {
			continue
		}
		v.Inputs = append(v.Inputs, entity.SummaryInput{Kind: "report", ID: r.ID})
		at, err := u.d.Summary.Facts.SummaryReportReadyAt(ctx, org, r.ID)
		if err != nil {
			return time.Time{}, err
		}
		if at.After(latest) {
			latest = at
		}
	}
	return latest, nil
}

func summaryComparisonBase(sha, kind string, reports []entity.TechnicalReport, head *entity.TechnicalReport) *entity.TechnicalReport {
	var fallback *entity.TechnicalReport
	for i := range reports {
		r := &reports[i]
		if r.SHA != sha || r.Format != head.Format || r.Language != head.Language || !r.Complete() {
			continue
		}
		if len(reportCauses(kind, r, head).reasons) == 0 {
			return r
		}
		if fallback == nil {
			fallback = r
		}
	}
	return fallback
}
