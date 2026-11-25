package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
)

func newSummaryDeps(pool *pgxpool.Pool, source change.SummarySource, normalizer *normalize.UseCase, calculator *calc.UseCase,
	objects repo.ObjectStore, reg prometheus.Registerer,
) (change.SummaryDeps, error) {
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return change.SummaryDeps{}, err
	}
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "craftpulse_change_summary_report_to_ready_seconds",
		Help:    "Seconds from normalization of the last expected report to a ready merge request summary.",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
	})
	if err := reg.Register(latency); err != nil {
		return change.SummaryDeps{}, err
	}
	return change.SummaryDeps{
		Store: changerepo.NewRepo(pool), Publication: changerepo.NewRepo(pool), Facts: normalizerepo.NewRepo(pool), Source: source, Queue: queue.New(jobs),
		Calculations: summaryCalculations{UseCase: normalizer, calculator: calculator, objects: objects, facts: normalizerepo.NewRepo(pool)}, ObserveLatency: latency.Observe,
	}, nil
}

type summaryCalculations struct {
	*normalize.UseCase
	calculator *calc.UseCase
	objects    repo.ObjectStore
	facts      *normalizerepo.Repo
}

func addSummaryValue(v *entity.SummaryRevision, kind string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	v.Items = append(v.Items, entity.SummaryItem{Kind: kind, RequiredRight: "none", Value: data})
	return nil
}

func (s summaryCalculations) SummarizeMetrics(ctx context.Context, org entity.ID, mr entity.SummaryMR, run entity.CIRun,
	v *entity.SummaryRevision,
) error {
	points, definitions := s.calculator.CalculateSummaryCI(run)
	assessment, definition := s.calculator.CalculateSummaryCompleteness(mr, v.ExpectedReports, v.CompleteReports)
	points = append(points, assessment)
	definitions = append(definitions, definition)
	v.Definitions = append(v.Definitions, definitions...)
	var runs, attempts []entity.ID
	if !run.Run.RunID.IsZero() {
		runs = make([]entity.ID, 1, len(run.Children)+1)
		runs[0] = run.Run.ID
		for i := range run.Children {
			runs = append(runs, run.Children[i].ID)
		}
		attempts = make([]entity.ID, len(run.Attempts))
		for i := range run.Attempts {
			attempts[i] = run.Attempts[i].ID
		}
	}
	if err := addSummaryValue(v, "ci_attempts", struct {
		Pipeline        entity.ID            `json:"pipeline_id,omitzero"`
		RunVersions     []entity.ID          `json:"run_versions,omitempty"`
		AttemptVersions []entity.ID          `json:"attempt_versions,omitempty"`
		Metrics         []entity.SeriesPoint `json:"metrics"`
	}{run.Run.RunID, runs, attempts, points}); err != nil {
		return err
	}
	facts, err := s.TestCoverageFacts(ctx, org, entity.ReportScope{AreaID: mr.AreaID, SHA: mr.HeadSHA, KnownSeq: mr.KnowledgeSeq})
	if err != nil {
		return err
	}
	// Only reports accepted by the matrix supply metrics. Deduplicate IDs without conflating components.
	accepted := make(map[entity.ID]bool)
	for _, input := range v.Inputs {
		if input.Kind == "report" {
			accepted[input.ID] = true
		}
	}
	facts.Reports = slices.DeleteFunc(facts.Reports, func(r entity.ReportEvidence) bool {
		return !accepted[r.ID] || r.RejectedReason != "" || r.Completeness != "complete" ||
			r.ExitClass != "success" && r.ExitClass != "findings"
	})
	reportIDs := make(map[entity.ID]bool)
	for i := range facts.Reports {
		reportIDs[facts.Reports[i].ID] = true
	}
	facts.Executions = slices.DeleteFunc(facts.Executions, func(r entity.ReportExecution) bool { return !reportIDs[r.ReportID] })
	facts.Coverage = slices.DeleteFunc(facts.Coverage, func(r entity.ReportCoverage) bool { return !reportIDs[r.ReportID] })
	tc := s.calculator.CalculateTestCoverage(facts)
	v.Definitions = append(v.Definitions, tc.Definitions...)
	if err := addSummaryValue(v, "tests", tc.Tests); err != nil {
		return err
	}
	diff, err := s.SummaryDiff(ctx, org, mr)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return err
	}
	patch, err := s.summaryCoverage(ctx, mr.HeadSHA, facts)
	if err != nil {
		return err
	}
	if diff.BaseSHA != mr.BaseSHA {
		diff = entity.MRDiff{}
	}
	in := calc.MRStructureInput{Diff: diff, Coverage: patch}
	streams, err := s.summaryCode(ctx, org, mr, v.CodePairs, &in)
	if err != nil {
		return err
	}
	structure := s.calculator.CalculateMRStructure(&in)
	if len(streams) == 0 {
		streams = []summaryCodeMetrics{{Complexity: structure.Complexity, Clones: structure.Clones}}
	}
	v.Definitions = append(v.Definitions, structure.Definitions...)
	return addSummaryValue(v, "coverage", struct {
		Line, Branch entity.CoverageMetric
		Patch        calc.MRPatchCoverageMetric
		Size         calc.MRSizeMetric
		Code         []summaryCodeMetrics
	}{tc.LineCoverage, tc.BranchCoverage, structure.PatchCoverage, structure.Size, streams})
}

func (s summaryCalculations) summaryCoverage(ctx context.Context, head string, facts entity.ReportFacts) ([]calc.MRCoverageReport, error) {
	maps := make(map[string]map[string][]calc.MRCoverageLine)
	files := make(map[entity.ID][]calc.MRCoverageMap)
	for _, fact := range facts.Coverage {
		if fact.Kind != entity.CoverageLine {
			continue
		}
		file := calc.MRCoverageMap{Path: fact.Path, Kind: fact.Kind, Executable: fact.Executable}
		if key := fact.LineMapKey; key != "" {
			if _, ok := maps[key]; !ok {
				lines, err := readSummaryLineMap(ctx, s.objects, key)
				if err != nil {
					return nil, err
				}
				maps[key] = lines
			}
			file.Lines = maps[key][fact.Path]
		}
		files[fact.ReportID] = append(files[fact.ReportID], file)
	}
	var out []calc.MRCoverageReport
	for i := range facts.Reports {
		r := &facts.Reports[i]
		if r.Kind == entity.AnalysisCoverage {
			out = append(out, calc.MRCoverageReport{SHA: head, Evidence: *r, Files: files[r.ID]})
		}
	}
	return out, nil
}

func readSummaryLineMap(ctx context.Context, objects repo.ObjectStore, key string) (map[string][]calc.MRCoverageLine, error) {
	body, err := objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	lines := make(map[string][]calc.MRCoverageLine)
	scan := bufio.NewScanner(body)
	scan.Buffer(make([]byte, 64<<10), 100<<20)
	for scan.Scan() {
		var file struct {
			Path, Kind string
			Points     [][]int64
		}
		if err := json.Unmarshal(scan.Bytes(), &file); err != nil {
			return nil, fmt.Errorf("change summary: decode coverage line map: %w", err)
		}
		if file.Kind != entity.CoverageLine {
			continue
		}
		m := make([]calc.MRCoverageLine, 0, len(file.Points))
		for _, p := range file.Points {
			if len(p) != 2 {
				return nil, errors.New("change summary: invalid coverage line map point")
			}
			m = append(m, calc.MRCoverageLine{Line: int(p[0]), Hits: p[1]})
		}
		lines[file.Path] = m
	}
	return lines, scan.Err()
}

type summaryCodeMetrics struct {
	ComponentID entity.ID
	Language    string
	Complexity  calc.MRComplexityMetric
	Clones      calc.MRCloneMetric
}

func (s summaryCalculations) summaryCode(ctx context.Context, org entity.ID, mr entity.SummaryMR,
	pairs []entity.SummaryCodePair, in *calc.MRStructureInput,
) ([]summaryCodeMetrics, error) {
	type stream struct {
		component entity.ID
		language  string
	}
	streams := make(map[stream]*calc.MRStructureInput)
	var order []stream
	for _, pair := range pairs {
		k := stream{pair.ComponentID, pair.Head.Language}
		entry := streams[k]
		if entry == nil {
			entry = &calc.MRStructureInput{Diff: in.Diff}
			streams[k] = entry
			order = append(order, k)
		}
		c := calc.MRCodeComparison{Comparable: pair.Comparable}
		for _, side := range []struct {
			report   *entity.TechnicalReport
			snapshot *calc.MRCodeSnapshot
			excluded *[]entity.SkippedTarget
		}{{pair.Base, &c.Base, &in.BaseExcluded}, {pair.Head, &c.Head, &in.HeadExcluded}} {
			if side.report == nil {
				continue
			}
			r := side.report
			report, err := s.facts.SummaryCodeFacts(ctx, org, r.ID)
			if err != nil {
				return nil, err
			}
			side.snapshot.Report = report
			side.snapshot.Scope = entity.CodeScope{
				RepositoryID: mr.AreaID, ComponentID: pair.ComponentID, SHA: r.SHA,
				Format: r.Format, Producer: r.Producer, ProfileID: r.ProfileID,
			}
			if report != nil {
				*side.excluded = append(*side.excluded, report.Skipped...)
				*side.excluded = append(*side.excluded, report.Facts.ExcludedTargets...)
			}
		}
		switch pair.Head.Format {
		case entity.CodeFormatLizard:
			entry.Functions = c
		case entity.CodeFormatJscpd:
			entry.Clones = c
		}
	}
	out := make([]summaryCodeMetrics, 0, len(order))
	for _, k := range order {
		result := s.calculator.CalculateMRStructure(streams[k])
		out = append(out, summaryCodeMetrics{k.component, k.language, result.Complexity, result.Clones})
	}
	return out, nil
}

func (s summaryCalculations) SummarizeFindings(c entity.SummaryFindingComparison, decisions []entity.ExceptionVersion) (
	[]entity.SummaryFindingValues, []entity.RevisionInput,
) {
	return s.calculator.CalculateSummaryFindings(c, decisions)
}
