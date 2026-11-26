package calc

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _linkInputs are the inputs of the coverage of links: the mapping gives the types that need an MR.
var _linkInputs = []string{InputDefinition, InputMapping, InputStructure}

var _linkDefinitions = []Definition{
	{
		ID: "A18", Version: 1, Name: "Merged MRs linked to a task", Unit: UnitRatio, CatalogRef: _activityCatalog + "a18",
		Formula: "merged MRs of the repositories of the team with a confirmed link to a task / merged MRs of these repositories, by " +
			"the time of merge in [from,to); a candidate or a rejected link is not a link",
		Roles: []string{"numerator", "denominator"}, Inputs: _linkInputs,
	},
	{
		ID: "A20", Version: 1, Name: "Chain coverage", Unit: UnitRatio, CatalogRef: _activityCatalog + "a20",
		Formula: "tasks of the code-required types in work at the end of the period or completed in [from,to) with a confirmed " +
			"link to an MR, open or merged / such tasks; without code_required_type rules NOT_EVALUATED",
		Roles: []string{"numerator", "denominator", "excluded", "missing"}, Inputs: _linkInputs,
	},
	{
		ID: "T28", Version: 1, Name: "Task to MR traceability", Unit: UnitRatio, CatalogRef: _tasksCatalog + "t28",
		Formula: "tasks of the code-required types in work at the end of the period or completed in [from,to) with a confirmed " +
			"MR merged by the end of the period / such tasks; a task of another type is NOT_APPLICABLE, without " +
			"code_required_type rules NOT_EVALUATED",
		Roles: []string{"numerator", "denominator", "excluded", "missing"}, Inputs: _linkInputs,
	},
}

// Reasons of the contributions of the coverage of links: the status of the best link of an object.
const (
	_linkCandidate = "candidate"
	_linkRejected  = "rejected"
)

// linkInput is what a part of the coverage of links reads at the knowledge cutoff of the revision: the tasks of
// the team with their timelines, the MRs of its repositories and the links of both.
type linkInput struct {
	window   entity.Window
	asOf     time.Time
	location *time.Location
	// policy tells whether the mapping names the code-required types; reason is why not.
	policy bool
	reason string
	pr     *prepared
	tasks  []linkTask
	mrs    []entity.MRVersion
	known  map[entity.ID]*entity.MRVersion
	byTask map[entity.ID][]entity.LinkEdge
	byMR   map[entity.ID][]entity.LinkEdge
	edges  []entity.LinkEdge
}

type linkTask struct {
	tl       *timeline
	required bool
}

// linkInputOf joins the tasks of the team, the MRs of its areas and the links of their ends.
func linkInputOf(in input, facts []entity.ReviewFacts, edges []entity.LinkEdge) linkInput {
	pr := prepare(in)
	out := linkInput{
		window: in.window, asOf: in.asOf, location: in.location, pr: pr, known: map[entity.ID]*entity.MRVersion{},
		byTask: map[entity.ID][]entity.LinkEdge{}, byMR: map[entity.ID][]entity.LinkEdge{}, edges: edges,
	}
	required, policy, reason := codeRequiredTasks(&in)
	out.policy, out.reason = policy, reason
	for _, tl := range pr.tls {
		out.tasks = append(out.tasks, linkTask{tl: tl, required: required[tl.slice.TaskID]})
	}
	slices.SortFunc(out.tasks, func(a, b linkTask) int { return bytes.Compare(a.tl.slice.TaskID[:], b.tl.slice.TaskID[:]) })
	for i := range facts {
		out.mrs = append(out.mrs, facts[i].MRs...)
	}
	slices.SortFunc(out.mrs, func(a, b entity.MRVersion) int { return bytes.Compare(a.MRID[:], b.MRID[:]) })
	for i := range out.mrs {
		out.known[out.mrs[i].MRID] = &out.mrs[i]
	}
	slices.SortFunc(out.edges, func(a, b entity.LinkEdge) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	for i := range out.edges {
		e := &out.edges[i]
		out.byTask[e.From] = append(out.byTask[e.From], *e)
		out.byMR[e.To] = append(out.byMR[e.To], *e)
	}
	return out
}

// codeRequiredTasks applies the A20 policy to the report's task facts.
func codeRequiredTasks(in *input) (required map[entity.ID]bool, policy bool, reason string) {
	if in.mapping == nil {
		return nil, false, entity.ReasonMappingMissing
	}
	byInstance := rulesOf(in.mapping)
	for _, rs := range byInstance {
		for _, required := range rs.codeRequired {
			policy = policy || required
		}
	}
	if !policy {
		return nil, false, entity.ReasonCodePolicyMissing
	}
	required = make(map[entity.ID]bool, len(in.tasks))
	for i := range in.tasks {
		task := &in.tasks[i]
		if rs, ok := byInstance[task.instance]; ok && rs.codeRequired[task.version.TypeNativeID] {
			required[task.version.TaskID] = true
		}
	}
	return required, true, ""
}

// linkStatus is the reason of a contribution: the method of a confirmed link, else candidate or rejected, else none.
func linkStatus(edges []entity.LinkEdge) (reason string, confirmed bool) {
	for i := range edges {
		if edges[i].Status == entity.LinkConfirmed {
			return string(edges[i].Method), true
		}
	}
	for i := range edges {
		if edges[i].Status == entity.LinkCandidate {
			return _linkCandidate, false
		}
	}
	if len(edges) > 0 {
		return _linkRejected, false
	}
	return "", false
}

type linkCalc func(in *linkInput, p period) entity.Point

var _linkMetrics = map[string]linkCalc{"A18": linkedMergePoint, "A20": chainPoint, "T28": traceabilityPoint}

func (in *linkInput) series(periods []period) []entity.Series {
	out := make([]entity.Series, 0, len(_linkDefinitions))
	for i := range _linkDefinitions {
		def := &_linkDefinitions[i]
		s := entity.Series{MetricID: def.ID, DefinitionVersion: def.Version, Unit: def.Unit}
		for _, p := range periods {
			s.Points = append(s.Points, _linkMetrics[def.ID](in, p))
		}
		out = append(out, s)
	}
	return out
}

// linkedMergePoint is A18: the MRs of the repositories of the team merged in the period with a confirmed task.
func linkedMergePoint(in *linkInput, p period) entity.Point {
	t := mrTally(p)
	var numerator, denominator float64
	for i := range in.mrs {
		mr := &in.mrs[i]
		if mr.MergedAt == nil || !p.has(*mr.MergedAt) {
			continue
		}
		denominator++
		reason, confirmed := linkStatus(in.byMR[mr.MRID])
		t.add(mr.MRID, 1, entity.RoleDenominator, nil, reason)
		if confirmed {
			numerator++
			t.add(mr.MRID, 2, entity.RoleNumerator, nil, reason)
		}
	}
	return t.ratio(numerator, denominator, int(denominator), false)
}

// chainPoint is A20: the tasks of the code-required types of the period with a confirmed MR, open or merged.
func chainPoint(in *linkInput, p period) entity.Point {
	return in.taskPoint(p, func(mr *entity.MRVersion) bool { return !mr.OpenedAt.After(p.end) })
}

// traceabilityPoint is T28: the tasks of the code-required types of the period with a confirmed MR merged by its end.
func traceabilityPoint(in *linkInput, p period) entity.Point {
	return in.taskPoint(p, func(mr *entity.MRVersion) bool { return mr.MergedAt != nil && !mr.MergedAt.After(p.end) })
}

// taskPoint counts the tasks of the period whose confirmed MR satisfies counts; a confirmed MR outside the
// repositories of the team is unknown, so its task is missing from the numerator.
func (in *linkInput) taskPoint(p period, counts func(mr *entity.MRVersion) bool) entity.Point {
	if !in.policy {
		return entity.Point{Start: p.start, End: p.end, State: entity.StateNotEvaluated, ReasonCode: in.reason}
	}
	t := newTally(p)
	var numerator, denominator float64
	for i := range in.tasks {
		lt := &in.tasks[i]
		id := lt.tl.slice.TaskID
		switch taskInPeriod(lt.tl, p) {
		case memberOut:
			continue
		case memberUnknown:
			t.miss(lt.tl)
			continue
		case memberIn:
		}
		if !lt.required {
			t.add(id, 1, entity.RoleExcluded, nil, entity.ReasonNotCodeRequired)
			continue
		}
		denominator++
		edges := in.byTask[id]
		reason, _ := linkStatus(edges)
		t.add(id, 1, entity.RoleDenominator, nil, reason)
		linked, unknown := false, false
		for j := range edges {
			if edges[j].Status != entity.LinkConfirmed {
				continue
			}
			mr, ok := in.known[edges[j].To]
			switch {
			case !ok:
				unknown = true
			case counts(mr):
				linked = true
			}
		}
		switch {
		case linked:
			numerator++
			t.add(id, 2, entity.RoleNumerator, nil, reason)
		case unknown:
			t.add(id, 2, entity.RoleMissing, nil, entity.ReasonMRUnknown)
		}
	}
	return t.ratio(numerator, denominator, int(denominator), true)
}

// taskInPeriod tells whether the task is in work at the end of the period or first completed in it.
func taskInPeriod(tl *timeline, p period) membership {
	if !tl.knownAt(p.end) {
		return memberUnknown
	}
	if tl.episodeAt(p.end) != nil {
		return memberIn
	}
	return tl.firstCompletionIn(p)
}

type linkSource struct {
	in     linkInput
	series *entity.CalcSeries
}

func (l *linkSource) periods() []period { return periods(l.in.window, l.in.asOf, l.in.location) }

// hash is SHA-256 of the definitions, the scope, the step, the part, the mapping, the tasks it replays, the MRs and
// every version of a link it reads.
func (l *linkSource) hash(from, to time.Time) []byte {
	d := newDigest()
	for i := range _linkDefinitions {
		d.text(_linkDefinitions[i].ID)
		d.int(int64(_linkDefinitions[i].Version))
	}
	d.bytes(l.series.ScopeHash)
	d.text(entity.MetricSetLinks)
	d.text(string(l.series.Step))
	d.time(from)
	d.time(to)
	d.flag(l.in.policy)
	d.text(l.in.reason)
	if m := l.in.pr.in.mapping; m != nil {
		d.id(m.ID)
	}
	for i := range l.in.tasks {
		lt := &l.in.tasks[i]
		if lt.tl.slice.Created.After(to) {
			continue
		}
		lt.tl.digestPart(d, &l.in.pr.ctx, from, to)
		d.flag(lt.required)
	}
	for i := range l.in.mrs {
		d.id(l.in.mrs[i].ID)
	}
	for i := range l.in.edges {
		d.id(l.in.edges[i].ID)
	}
	return d.sum()
}

func (l *linkSource) stored(c []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetLinks, From: from, To: to}
	for _, s := range l.in.series(c) {
		for j := range s.Points {
			out.Points = append(out.Points, entity.SeriesPoint{MetricID: s.MetricID, Unit: s.Unit, Point: s.Points[j]})
		}
	}
	return out
}

// linkSourceOf reads the coverage of links of the team of a revision at its knowledge cutoff.
func (u *UseCase) linkSourceOf(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope seriesScope,
) (*linkSource, error) {
	w := entity.Window{From: rev.Window.From, To: rev.Window.To, Step: series.Step}
	in, err := u.input(ctx, entity.CalcRequest{
		OrganizationID: job.OrganizationID, TeamID: scope.TeamID, KnowledgeSeq: rev.KnowledgeSeq, Window: w, AsOf: rev.AsOf, Sources: job.Sources,
	})
	if err != nil {
		return nil, err
	}
	if in.location, err = time.LoadLocation(rev.Timezone); err != nil {
		return nil, fmt.Errorf("calc: time zone %q: %w", rev.Timezone, err)
	}
	areas, err := u.scopeAreas(ctx, job.OrganizationID, scope)
	if err != nil {
		return nil, err
	}
	facts := make([]entity.ReviewFacts, 0, len(areas))
	for _, area := range areas {
		f, err := u.d.Facts.ReviewFacts(ctx, job.OrganizationID, area, rev.KnowledgeSeq)
		if err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	var objects []entity.ID
	for i := range in.tasks {
		objects = append(objects, in.tasks[i].version.TaskID)
	}
	for i := range facts {
		for j := range facts[i].MRs {
			objects = append(objects, facts[i].MRs[j].MRID)
		}
	}
	var edges []entity.LinkEdge
	if u.d.Links != nil {
		if edges, err = u.d.Links.TaskMREdgesAt(ctx, job.OrganizationID, objects, rev.KnowledgeSeq); err != nil {
			return nil, err
		}
	}
	return &linkSource{in: linkInputOf(in, facts, edges), series: series}, nil
}
