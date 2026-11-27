package calc

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type technicalCohort struct {
	wip, chain, pipelines, fixes []crossTask
	chainPolicy                  bool
}

func (u *UseCase) loadTechnicalCross(ctx context.Context, org entity.ID, rev *entity.CalcRevision, flow *flowSource, periods []period) error {
	x := flow.signals
	if flow.in.mapping == nil {
		x.technical = map[int64]map[entity.ID]entity.TechnicalCrossEvidence{}
		return nil
	}
	snapshot, err := u.d.ReportEvidence(ctx, org, slices.Collect(maps.Keys(x.linked)), rev.KnowledgeSeq, rev.AsOf)
	if err != nil {
		return err
	}
	flow.prepare()
	x.technical = make(map[int64]map[entity.ID]entity.TechnicalCrossEvidence, len(periods))
	x.edges = map[entity.ID][]entity.LinkEdge{}
	taskIDs := make([]entity.ID, 0, len(flow.pr.tls))
	for _, tl := range flow.pr.tls {
		taskIDs = append(taskIDs, tl.slice.TaskID)
	}
	if u.d.Links != nil {
		edges, err := u.d.Links.TaskMREdgesAt(ctx, org, taskIDs, rev.KnowledgeSeq)
		if err != nil {
			return err
		}
		for i := range edges {
			edge := &edges[i]
			if edge.Status == entity.LinkConfirmed {
				x.edges[edge.From] = append(x.edges[edge.From], *edge)
			}
		}
	}
	for _, p := range periods {
		eligible := map[entity.ID]bool{}
		for _, tl := range flow.pr.tls {
			if taskInPeriod(tl, p) == memberOut {
				continue
			}
			for i := range x.edges[tl.slice.TaskID] {
				edge := &x.edges[tl.slice.TaskID][i]
				if crossLinkAt(edge, p) {
					eligible[edge.To] = true
				}
			}
		}
		_, heads, _ := x.crossObservations(p.end, true)
		heads = slices.DeleteFunc(heads, func(head entity.TechnicalHead) bool { return !eligible[head.MRID] })
		if len(heads) == 0 {
			continue
		}
		evidence, err := snapshot(ctx, p.end, heads)
		if err != nil {
			return err
		}
		x.technical[p.end.UnixNano()] = evidence
	}
	return nil
}

func (x *reportSignals) technicalCohort(pr *prepared, p period) technicalCohort {
	if pr.in.mapping == nil {
		return technicalCohort{}
	}
	required, policy, _ := codeRequiredTasks(&pr.in)
	out := technicalCohort{chainPolicy: policy}
	facts, _, pipelines := x.crossObservations(p.end, true)
	projections := x.technical[p.end.UnixNano()]
	mrs := map[entity.ID]crossMR{}
	runs := map[entity.ID]map[entity.ID]crossMR{}
	for _, tl := range pr.tls {
		id := tl.slice.TaskID
		member := taskInPeriod(tl, p)
		if member == memberOut {
			continue
		}
		b := crossEvidence{UnknownMembership: member == memberUnknown}
		ep := tl.episodeAt(p.end)
		if ep != nil && ep.startKnown {
			age := seconds(ep.start, p.end)
			b.Age = &age
			point := pointContext{targets: pr.in.targets}
			if target, reason := point.target(&tl.slice); reason == "" {
				b.OverTarget = new(age > target)
			}
		}
		for i := range x.edges[id] {
			edge := &x.edges[id][i]
			if !crossLinkAt(edge, p) {
				continue
			}
			m, ok := facts[edge.To]
			if !ok {
				b.UnknownLink = true
				continue
			}
			if projection, ok := projections[m.ID]; ok {
				m.Technical = &projection.Head
			}
			b.MRs = append(b.MRs, m)
			if member == memberIn {
				m.Tasks = mrs[m.ID].Tasks
				m.Tasks = append(m.Tasks, id)
				mrs[m.ID] = m
			}
		}
		slices.SortFunc(b.MRs, func(a, b crossMR) int { return strings.Compare(a.ID.String(), b.ID.String()) })
		if member == memberUnknown || ep != nil {
			out.wip = append(out.wip, technicalContribution(id, entity.ObjectTask, b))
		}
		if required[id] {
			out.chain = append(out.chain, technicalContribution(id, entity.ObjectTask, b))
		}
	}
	for _, id := range sortedIDs(slices.Collect(maps.Keys(mrs))) {
		m := mrs[id]
		m.Tasks = sortedIDs(m.Tasks)
		out.fixes = append(out.fixes, technicalContribution(id, entity.ObjectMergeRequest, crossEvidence{MRs: []crossMR{m}}))
		for i := range pipelines[m.ID] {
			run := &pipelines[m.ID][i]
			switch run.CIClass {
			case "success", "unknown", "running":
			default:
				continue
			}
			run.Tasks, run.Technical = m.Tasks, m.Technical
			if reports, ok := projections[id].PipelineReports[*run.PipelineID]; ok {
				run.PipelineReports = &reports
			}
			if runs[*run.PipelineID] == nil {
				runs[*run.PipelineID] = map[entity.ID]crossMR{}
			}
			runs[*run.PipelineID][id] = *run
		}
	}
	for _, id := range sortedIDs(slices.Collect(maps.Keys(runs))) {
		var b crossEvidence
		for _, mr := range sortedIDs(slices.Collect(maps.Keys(runs[id]))) {
			b.MRs = append(b.MRs, runs[id][mr])
		}
		out.pipelines = append(out.pipelines, technicalContribution(id, entity.ObjectCIRun, b))
	}
	return out
}

func crossLinkAt(edge *entity.LinkEdge, p period) bool {
	return edge.Validity.Contains(p.end) && (edge.DecidedAt == nil || !edge.DecidedAt.After(p.end))
}

func technicalContribution(id entity.ID, kind entity.ObjectKind, basis crossEvidence) crossTask {
	raw, _ := json.Marshal(basis)
	return crossTask{basis: basis, contribution: entity.Contribution{
		ObjectID: id, ObjectKind: kind, ItemNo: 1, Evidence: raw,
	}}
}

func (x *reportSignals) technicalPoints(pr *prepared, p period) []entity.SeriesPoint {
	if x.technical == nil {
		return nil
	}
	cohort := x.technicalCohort(pr, p)
	var points []entity.SeriesPoint
	add := func(metric, est, unit string, rows []crossTask) {
		pt := entity.Point{Start: p.start, End: p.end, State: entity.StateNotEvaluated}
		switch {
		case pr.in.mapping == nil:
			pt.ReasonCode = entity.ReasonMappingMissing
		case metric == "X05" && !cohort.chainPolicy:
			pt.ReasonCode = entity.ReasonCodePolicyMissing
		default:
			cs := make([]entity.Contribution, len(rows))
			for i := range rows {
				cs[i] = rows[i].contribution
				scoreTechnical(&cs[i], metric, est, &rows[i].basis)
			}
			pt = technicalPoint(metric, p, cs)
		}
		points = append(points, entity.SeriesPoint{MetricID: metric, Estimator: est, Unit: unit, Point: pt})
	}
	if pr.in.mapping != nil {
		add("X02", _ciMissingReport, UnitRatio, cohort.wip)
	}
	for i := range _technicalCrossDefinitions {
		def := &_technicalCrossDefinitions[i]
		rows := cohort.wip
		switch def.ID {
		case "X04":
			rows = cohort.pipelines
		case "X05":
			rows = cohort.chain
		case "X06":
			rows = cohort.fixes
		}
		for _, est := range def.Estimators {
			add(def.ID, est, def.Unit, rows)
		}
	}
	return points
}
