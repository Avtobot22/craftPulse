package calc

import (
	"encoding/hex"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CalculateSummaryFindings invokes the finding definitions on the comparisons and decision versions
// selected for the summary. Results retain their required right alongside the metric values.
func (*UseCase) CalculateSummaryFindings(comparison entity.SummaryFindingComparison, decisions []entity.ExceptionVersion) (
	[]entity.SummaryFindingValues, []entity.RevisionInput,
) {
	kinds := summaryFindingKinds(comparison)
	values := make([]entity.SummaryFindingValues, 0, len(kinds))
	var definitions []entity.RevisionInput
	for _, kind := range kinds {
		in := FindingMetricsInput{
			AreaID: comparison.AreaID, ComponentID: comparison.ComponentID, Kind: kind,
			AsOf: comparison.AsOf, IncomparableReason: comparison.ReasonCode,
		}
		in.Head, in.HeadSHA = summaryFindingScan(&comparison, comparison.Head)
		in.Base, in.BaseSHA = summaryFindingScan(&comparison, comparison.Base)
		for i := range comparison.Findings {
			f := &comparison.Findings[i]
			if f.Kind != kind {
				continue
			}
			disposition := FindingDisposition(f.Lifecycle)
			if disposition == entity.FindingUnchanged {
				disposition = FindingPersisting
			}
			in.Findings = append(in.Findings, ClassifiedFinding{
				FindingCoordinates: f.FindingCoordinates, ReportID: f.ReportID, OccurrenceID: f.OccurrenceID,
				NativeSeverity: f.Severity, Disposition: disposition, LineStart: f.LineStart,
			})
		}
		result := findingMetrics(&in, decisions)
		right := "none"
		if entity.SensitiveFinding(kind) {
			right = "security"
		}
		for i := range result.Points {
			point := &result.Points[i]
			// The summary stores scalar values, with occurrence and decision evidence in its rows and inputs.
			point.End = time.Time{}
			point.Contributions = nil
			def := definition(point.MetricID)
			input := entity.RevisionInput{Kind: InputDefinition, ID: def.ID, Version: strconv.Itoa(def.Version)}
			if !slices.Contains(definitions, input) {
				definitions = append(definitions, input)
			}
		}
		values = append(values, entity.SummaryFindingValues{
			ComponentID: comparison.ComponentID, AnalysisKind: comparison.AnalysisKind, RequiredRight: right, Points: result.Points,
		})
	}
	return values, definitions
}

func summaryFindingKinds(c entity.SummaryFindingComparison) []string {
	var kinds []string
	switch c.AnalysisKind {
	case entity.AnalysisCode:
		kinds = append(kinds, entity.FindingCode)
	case entity.AnalysisSAST:
		kinds = append(kinds, entity.FindingSecurity)
	case entity.AnalysisSCA, entity.AnalysisImage:
		kinds = append(kinds, entity.FindingDependency)
	case entity.AnalysisSecrets:
		kinds = append(kinds, entity.FindingSecret)
	case entity.AnalysisIaC:
		kinds = append(kinds, entity.FindingIaC)
	}
	for i := range c.Findings {
		f := &c.Findings[i]
		if (f.Kind == entity.FindingCode || entity.SensitiveFinding(f.Kind)) && !slices.Contains(kinds, f.Kind) {
			kinds = append(kinds, f.Kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

func summaryFindingScan(c *entity.SummaryFindingComparison, r *entity.TechnicalReport) (scan *FindingScan, sha string) {
	if r == nil {
		return nil, ""
	}
	return &FindingScan{
		AreaID: c.AreaID, SHA: r.SHA,
		ReportEvidence: entity.ReportEvidence{
			ID: r.ID, ComponentID: &c.ComponentID, Kind: c.AnalysisKind, Language: r.Language, ScopeHash: hex.EncodeToString(r.ScopeHash),
			Format: r.Format, Mode: r.Mode, Producer: r.Producer, ProducerVersion: r.ProducerVersion, ProfileID: r.ProfileID,
			Completeness: r.Completeness, ExitClass: r.ExitClass,
		},
	}, r.SHA
}
