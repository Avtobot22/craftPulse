package change

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// TechnicalSummaryStore supplies the history of summaries on a fixed report cut.
type TechnicalSummaryStore interface {
	TechnicalSummaries(context.Context, entity.ID, []entity.ID, int64, time.Time) ([]entity.TechnicalSummaryRevision, error)
}

// TechnicalPipelines reads the normalized graph of a pipeline at the report's knowledge cut.
type TechnicalPipelines interface {
	TechnicalPipeline(context.Context, entity.ID, entity.ID, entity.ID, int64) (entity.CIRun, error)
}

// TechnicalSnapshot binds report points to immutable summary inputs and evaluates exceptions at each point.
func (u *UseCase) TechnicalSnapshot(ctx context.Context, org entity.ID, mrs []entity.ID, seq int64, asOf time.Time) (entity.TechnicalCrossSnapshot, error) {
	byMR := make(map[entity.ID][]entity.TechnicalSummaryRevision, len(mrs))
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		rows, err := u.d.TechnicalSummaries.TechnicalSummaries(ctx, org, mrs, seq, asOf)
		if err != nil {
			return err
		}
		for i := range rows {
			row := &rows[i]
			byMR[row.MRID] = append(byMR[row.MRID], *row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, at time.Time, heads []entity.TechnicalHead) (map[entity.ID]entity.TechnicalCrossEvidence, error) {
		out := make(map[entity.ID]entity.TechnicalCrossEvidence, len(heads))
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			decisions := map[entity.ID][]entity.ExceptionVersion{}
			for i := range heads {
				head := &heads[i]
				history := byMR[head.MRID]
				j := slices.IndexFunc(history, func(row entity.TechnicalSummaryRevision) bool {
					return row.HeadSHA == head.HeadSHA && row.AppliesAt(at)
				})
				var row *entity.TechnicalSummaryRevision
				if j >= 0 && len(history[j].Streams) > 0 {
					if _, ok := decisions[head.AreaID]; !ok {
						var err error
						decisions[head.AreaID], err = u.d.Decisions.ActiveAt(ctx, org, head.AreaID, seq, at)
						if err != nil {
							return err
						}
					}
					selected := history[j]
					selected.Streams = recheckDocumentation(&selected, history[j+1:], at)
					row = &selected
				}
				evidence, err := u.technicalCrossAt(ctx, org, head, seq, at, row, decisions[head.AreaID])
				if err != nil {
					return err
				}
				out[head.MRID] = evidence
			}
			return nil
		})
		return out, err
	}, nil
}

func (u *UseCase) technicalCrossAt(ctx context.Context, org entity.ID, head *entity.TechnicalHead, seq int64, at time.Time,
	row *entity.TechnicalSummaryRevision, exceptions []entity.ExceptionVersion,
) (entity.TechnicalCrossEvidence, error) {
	mr := entity.SummaryMR{MRVersion: head.MRVersion, KnowledgeSeq: seq}
	var out entity.TechnicalCrossEvidence
	if row != nil {
		out.Head = technicalEvidence(row, exceptions)
	}
	var headRun entity.CIRun
	if len(head.SuccessfulPipelines) > 0 {
		out.PipelineReports = make(map[entity.ID]entity.TechnicalCross, len(head.SuccessfulPipelines))
	}
	for _, pipeline := range head.SuccessfulPipelines {
		project, entries, err := u.reportProfile(ctx, org, head.AreaID, seq, pipeline.ProfileAt)
		if err != nil {
			return out, err
		}
		reports := entity.TechnicalCross{ProjectID: project}
		if len(entries) > 0 {
			run, err := u.d.TechnicalPipelines.TechnicalPipeline(ctx, org, head.AreaID, pipeline.ID, seq)
			if err != nil {
				return out, err
			}
			if pipeline.ID == head.PipelineID {
				headRun = run
			}
			runIDs := make([]entity.ID, 1, len(run.Children)+1)
			runIDs[0] = pipeline.ID
			for i := range run.Children {
				runIDs = append(runIDs, run.Children[i].RunID)
			}
			reports, err = u.reportCompletenessAt(ctx, org, mr, project, entries, at, runIDs)
			if err != nil {
				return out, err
			}
		}
		out.PipelineReports[pipeline.ID] = reports
	}
	if row != nil {
		if row.PipelineID == head.PipelineID {
			out.Head.MissingChecks, out.Head.PublicMissingChecks = row.MissingChecks, row.PublicMissingChecks
		}
		return out, nil
	}
	profileAt := at
	if head.PipelineStarted != nil {
		profileAt = *head.PipelineStarted
	}
	project, entries, err := u.reportProfile(ctx, org, head.AreaID, seq, profileAt)
	if err != nil {
		return out, err
	}
	out.Head, err = u.reportCompletenessAt(ctx, org, mr, project, entries, at, nil)
	if err != nil {
		return out, err
	}
	jobs := u.summaryCheckJobs(entries)
	if len(jobs.All) == 0 {
		return out, nil
	}
	if headRun.Run.RunID.IsZero() && !head.PipelineID.IsZero() {
		headRun, err = u.d.TechnicalPipelines.TechnicalPipeline(ctx, org, head.AreaID, head.PipelineID, seq)
		if err != nil {
			return out, err
		}
	}
	out.Head.MissingChecks = u.d.MissingChecksAt(headRun, at, jobs.All)
	out.Head.PublicMissingChecks = u.d.MissingChecksAt(headRun, at, jobs.Public)
	return out, nil
}

func (u *UseCase) reportCompletenessAt(ctx context.Context, org entity.ID, mr entity.SummaryMR, project entity.ID,
	entries []entity.ProfileEntry, at time.Time, runIDs []entity.ID,
) (entity.TechnicalCross, error) {
	row := entity.TechnicalSummaryRevision{ProjectID: project}
	for _, entry := range entries {
		stream := entity.TechnicalStream{ExpectedReport: entity.ExpectedReport{ComponentID: entry.ComponentID, Kind: entry.AnalysisKind, Language: entry.Language}}
		reports, err := u.d.Summary.Facts.SummaryReports(ctx, org, mr, entry, runIDs)
		if err != nil {
			return entity.TechnicalCross{}, err
		}
		for i := range reports {
			r := &reports[i]
			pinned := entry.ToolVersions[r.Producer]
			if r.SHA != mr.HeadSHA || !r.Complete() || summaryCodeAuxiliary(r) || pinned != "" && r.ProducerVersion != "" && r.ProducerVersion != pinned {
				continue
			}
			if !r.EventAt.After(at) {
				stream.Complete = true
				break
			}
		}
		row.Streams = append(row.Streams, stream)
	}
	return technicalEvidence(&row, nil), nil
}

func technicalEvidence(rev *entity.TechnicalSummaryRevision, exceptions []entity.ExceptionVersion) entity.TechnicalCross {
	out := entity.TechnicalCross{ProjectID: rev.ProjectID}
	type findingKey struct {
		kind, lifecycle string
		id              entity.ID
	}
	indices := map[string]int{}
	counted := map[findingKey]bool{}
	for i := range rev.Streams {
		stream := &rev.Streams[i]
		kind := stream.Kind
		if kind == entity.AnalysisOpenAPI {
			kind = entity.AnalysisDocs
		}
		j, ok := indices[kind]
		if !ok {
			j = len(out.Scopes)
			indices[kind] = j
			out.Scopes = append(out.Scopes, entity.TechnicalScope{Kind: kind})
		}
		scope := &out.Scopes[j]
		scope.Expected++
		if stream.Complete {
			scope.Complete++
		}
		if stream.Comparable {
			scope.Comparable++
		}
		for k := range stream.Findings {
			f := &stream.Findings[k]
			if entity.SensitiveFinding(f.Kind) && !entity.SensitiveSummaryAnalysis(kind) {
				continue
			}
			// A resolved finding has no current occurrence to suppress.
			if f.Lifecycle != entity.FindingResolved && slices.ContainsFunc(exceptions, func(v entity.ExceptionVersion) bool {
				return v.Matches(f.FindingCoordinates)
			}) {
				key := findingKey{kind, "accepted", f.FindingID}
				if !counted[key] {
					counted[key] = true
					scope.Accepted++
				}
				continue
			}
			if kind == entity.AnalysisDocs && f.Lifecycle != entity.FindingResolved {
				key := findingKey{kind, "current", f.FindingID}
				if !counted[key] {
					counted[key] = true
					scope.Current++
				}
			}
			key := findingKey{kind, f.Lifecycle, f.FindingID}
			if counted[key] {
				continue
			}
			counted[key] = true
			switch f.Lifecycle {
			case entity.FindingNew:
				scope.New++
			case entity.FindingUnchanged:
				scope.Persisting++
			case entity.FindingResolved:
				scope.Resolved++
			}
		}
	}
	return out
}
