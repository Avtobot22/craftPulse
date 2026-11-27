package calc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	// _partPoints keeps a part of a long window well within the 60 s of a job: ten months by weeks are five parts.
	_partPoints = 10
	// _staleAfter is the age of the progress of a building revision after which a new chain takes it over;
	// it bounds the delay only, the conditional updates keep the chain correct (TZ 12.7).
	_staleAfter = 30 * time.Minute
)

// errPublishOrder is a revision of the series published by someone else while this chain built: the job fails
// and does not overwrite it (TZ 12.7).
var errPublishOrder = errors.New("calc: the current revision of the series changed while the chain built")

// Complete ends the job in the transaction of its result.
type Complete = func(ctx context.Context) error

type teamScope struct {
	Kind   string    `json:"kind"`
	TeamID entity.ID `json:"team_id"`
}

// TeamSeries finds the series of the flow of a team by the step and creates it on the first request. The scope
// is canonical JSON with ordered keys, so one team always finds one series (TZ 12.7).
func (u *UseCase) TeamSeries(ctx context.Context, orgID, teamID entity.ID, step entity.Step) (entity.CalcSeries, error) {
	scope, sum, err := teamScopeOf(teamID)
	if err != nil {
		return entity.CalcSeries{}, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: new series id: %w", err)
	}
	var out entity.CalcSeries
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		out, err = u.d.Repo.EnsureSeries(ctx, orgID, entity.CalcSeries{
			ID: id, Scope: scope, ScopeHash: sum, MetricSet: entity.MetricSetFlow, Step: step,
		})
		return err
	})
	return out, err
}

// Refresh queues a new revision of the series over the window at as_of.
func (u *UseCase) Refresh(ctx context.Context, orgID, seriesID entity.ID, w entity.Window, asOf time.Time) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		return u.d.Queue.Enqueue(ctx, entity.CalcSliceJob{
			OrganizationID: orgID, SeriesID: seriesID, From: w.From, To: w.To, AsOf: asOf, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		}, entity.QueueBulk)
	})
}

// RunSlice is the job calc.slice: without a revision it starts a chain, otherwise it computes and saves the
// part of its number and queues the next one or publishes the revision (TZ 10.6).
func (u *UseCase) RunSlice(ctx context.Context, job entity.CalcSliceJob, complete Complete) error {
	if job.RevisionID.IsZero() {
		return u.start(ctx, job, complete)
	}
	return u.part(ctx, job, complete)
}

// FailSlice ends the building revision of a job that spent its attempts; the current revision stays.
func (u *UseCase) FailSlice(ctx context.Context, job entity.CalcSliceJob) error {
	if job.RevisionID.IsZero() {
		return nil
	}
	var failed bool
	err := u.d.Tx.InOrgTx(ctx, job.OrganizationID, func(ctx context.Context) error {
		var err error
		failed, err = u.d.Repo.FailRevision(ctx, job.OrganizationID, job.RevisionID)
		return err
	})
	if err == nil && failed {
		u.d.Metrics.RevisionFailed()
	}
	return err
}

// start fixes the knowledge cutoff of the organization and writes the building revision with its inputs; a
// series that builds another revision keeps it, and a revision whose progress stopped is taken over.
func (u *UseCase) start(ctx context.Context, job entity.CalcSliceJob, complete Complete) error {
	org := job.OrganizationID
	var (
		series entity.CalcSeries
		seq    int64
	)
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if series, err = u.d.Repo.Series(ctx, org, job.SeriesID); err != nil {
			return err
		}
		seq, _, err = u.d.Tenants.Clock(ctx, org)
		return err
	})
	if err != nil {
		return err
	}
	scope, err := parseScope(series.Scope)
	if err != nil {
		return err
	}
	team, flow := scope.TeamID, series.MetricSet == entity.MetricSetFlow
	var (
		mapping entity.MappingVersion
		mapped  bool
		targets []entity.TargetVersion
	)
	if !team.IsZero() && u.d.Settings != nil {
		if mapping, mapped, err = u.d.Settings.MappingAt(ctx, org, team, seq); err != nil {
			return err
		}
	}
	if flow {
		if targets, err = u.d.Settings.TargetVersionsAt(ctx, org, team, seq); err != nil {
			return err
		}
	}
	id, err := u.d.NewID()
	if err != nil {
		return fmt.Errorf("calc: new revision id: %w", err)
	}
	now := u.d.Clock.Now()
	rev := entity.CalcRevision{
		ID: id, SeriesID: series.ID, Window: entity.Window{From: job.From, To: job.To, Step: series.Step}, AsOf: job.AsOf,
		KnowledgeSeq: seq, Timezone: "UTC", StartedAt: now, PreviousRevisionID: series.CurrentRevisionID,
	}
	defs := definitionsOf(series.MetricSet)
	if scope.Kind == _scopeCode {
		defs = codeDefinitions(scope.Format)
	}
	if scope.Kind == _scopeDocumentation && scope.Documentation.SHA == "" {
		defs = []Definition{_wikiDefinition}
	}
	inputs := definitionInputs(defs)
	if scope.Kind == _scopeCode {
		report, err := u.d.Code.CodeReportAt(ctx, org, scope.CodeScope, seq)
		if err != nil {
			return err
		}
		inputs = append(inputs, codeInputs(report)...)
	}
	if scope.Kind == _scopeDocumentation {
		src, err := u.documentationSource(ctx, org, &rev, &series, scope.Documentation)
		if err != nil {
			return err
		}
		inputs = append(inputs, src.inputs()...)
	}
	if scope.Kind == _scopeSecurity {
		reports, err := u.d.Security.SecurityReports(ctx, org, scope.securityAt(seq))
		if err != nil {
			return err
		}
		inputs = append(inputs, securityInputs(reports)...)
	}
	if scope.Kind == _scopeAnalysis {
		src, err := u.analysisSource(ctx, org, &rev, &series, scope.analysisScope())
		if err != nil {
			return err
		}
		inputs = append(inputs, src.inputs()...)
	}
	if !team.IsZero() {
		inputs = append(inputs, entity.RevisionInput{Kind: entity.InputKindStructure, ID: team.String(), Version: entity.GroupingCurrentStructure})
	}
	if mapped {
		rev.Timezone = mapping.Timezone
	}
	if mapped && (flow || series.MetricSet == entity.MetricSetLinks) {
		inputs = append(inputs, entity.RevisionInput{Kind: entity.InputKindMapping, ID: mapping.ID.String(), Version: strconv.Itoa(mapping.VersionNo)})
	}
	for i := range targets {
		t := &targets[i]
		inputs = append(inputs, entity.RevisionInput{Kind: entity.InputKindTarget, ID: t.ID.String(), Version: strconv.Itoa(t.VersionNo)})
	}
	var structure int64
	if scope.Kind == _scopeTeam || scope.Kind == _scopeRepository {
		if structure, err = u.d.Structure.StructureVersion(ctx, org); err != nil {
			return err
		}
	}
	areas, err := u.scopeAreas(ctx, org, scope)
	if err != nil {
		return err
	}
	sources, err := u.seriesCollection(ctx, org, areas, series.MetricSet, scope.ConnectionID)
	if err != nil {
		return err
	}
	for _, c := range (&pointContext{collection: sources}).connections() {
		inputs = append(inputs, entity.RevisionInput{
			Kind: entity.InputKindWatermark, ID: c.ConnectionID.String(), Watermark: c.Watermark, Completeness: c.Completeness,
		})
	}
	rev.Loading = loading(sources)
	// Not nil, so the parts read this state even when no connection collects the areas.
	fixed := append([]entity.AreaCollection{}, sources...)
	rev.InputHash = revisionHash(&series, &rev, inputs)
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if err := u.d.Repo.FailStaleRevisions(ctx, org, series.ID, now.Add(-_staleAfter)); err != nil {
			return err
		}
		created, err := u.d.Repo.CreateRevision(ctx, org, rev)
		if err != nil || !created {
			return errors.Join(err, complete(ctx))
		}
		if err := u.followCurrent(ctx, org, &rev); err != nil {
			return err
		}
		if err := u.d.Repo.InsertRevisionInputs(ctx, org, rev.ID, inputs); err != nil {
			return err
		}
		if err := complete(ctx); err != nil {
			return err
		}
		next := job
		next.RevisionID, next.PartNo, next.Sources, next.Structure = rev.ID, 1, &fixed, structure
		if series.MetricSet == entity.MetricSetActivity {
			next.ActivityAreas = &areas
		}
		return u.d.Queue.Enqueue(ctx, next, entity.QueueBulk)
	})
}

// followCurrent makes the new revision replace the current one of the series as it is after the insert: another
// chain may have published since the start read the series. The pointer cannot move after that, as the series
// builds no other revision.
func (u *UseCase) followCurrent(ctx context.Context, org entity.ID, rev *entity.CalcRevision) error {
	s, err := u.d.Repo.Series(ctx, org, rev.SeriesID)
	if err != nil {
		return err
	}
	if sameID(s.CurrentRevisionID, rev.PreviousRevisionID) {
		return nil
	}
	rev.PreviousRevisionID = s.CurrentRevisionID
	return u.d.Repo.SetRevisionPrevious(ctx, org, rev.ID, rev.PreviousRevisionID)
}

func sameID(a, b *entity.ID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// part computes its part on the facts and the mapping of the knowledge cutoff of the revision outside a
// transaction, then saves it with the next job in one. A part whose input hash is stored is taken as it is and not
// computed. The last part publishes the revision and, when the facts or the settings of the team changed after
// the cutoff, queues the chain again after completing itself (TZ 10.6).
func (u *UseCase) part(ctx context.Context, job entity.CalcSliceJob, complete Complete) error {
	org := job.OrganizationID
	var (
		rev    entity.CalcRevision
		series entity.CalcSeries
	)
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if rev, err = u.d.Repo.Revision(ctx, org, job.RevisionID); err != nil {
			return err
		}
		series, err = u.d.Repo.Series(ctx, org, rev.SeriesID)
		return err
	})
	if err != nil {
		return err
	}
	if rev.Status != entity.RevisionBuilding {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	src, scope, err := u.source(ctx, job, &rev, &series)
	if err != nil {
		return err
	}
	chunks := chunk(src.periods(), u.d.PartPoints)
	var (
		stored *entity.StoredPart
		kept   entity.ID
		reused bool
	)
	if job.PartNo <= len(chunks) {
		c := chunks[job.PartNo-1]
		from, to := c[0].start, c[len(c)-1].end
		if flow, ok := src.(*flowSource); ok && u.d.ReportEvidence != nil {
			if err := u.loadTechnicalCross(ctx, org, &rev, flow, c); err != nil {
				return err
			}
		}
		hash := src.hash(from, to)
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			var err error
			kept, reused, err = u.d.Repo.PartByHash(ctx, org, hash)
			return err
		})
		if err != nil {
			return err
		}
		if !reused {
			stored = src.stored(c, hash, from, to)
		}
	}
	last := job.PartNo >= len(chunks)
	var (
		areas      []entity.ID
		collection []entity.AreaCollection
	)
	if last {
		if areas, err = u.scopeAreas(ctx, org, scope); err != nil {
			return err
		}
		if collection, err = u.seriesCollection(ctx, org, areas, series.MetricSet, scope.ConnectionID); err != nil {
			return err
		}
	}
	newID, err := u.d.NewID()
	if err != nil {
		return fmt.Errorf("calc: new part id: %w", err)
	}
	now := u.d.Clock.Now()
	var published bool
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		published = false
		alive, err := u.d.Repo.TouchRevision(ctx, org, rev.ID, now)
		if err != nil {
			return err
		}
		if !alive {
			return complete(ctx)
		}
		if stored != nil {
			var created bool
			if kept, created, err = u.d.Repo.SavePart(ctx, org, newID, stored, now); err != nil {
				return err
			}
			reused = !created
		}
		if job.PartNo <= len(chunks) {
			if err := u.d.Repo.LinkPart(ctx, org, rev.ID, job.PartNo, kept); err != nil {
				return err
			}
		}
		if !last {
			if err := complete(ctx); err != nil {
				return err
			}
			next := job
			next.PartNo++
			return u.d.Queue.Enqueue(ctx, next, entity.QueueBulk)
		}
		if err := u.publish(ctx, org, &rev, now); err != nil {
			return err
		}
		if err := complete(ctx); err != nil {
			return err
		}
		published = true
		if code, ok := src.(*codeSource); ok {
			return u.againCode(ctx, job, &rev, scope.CodeScope, code.report)
		}
		if docs, ok := src.(*documentationSource); ok {
			return u.againDocumentation(ctx, job, &rev, &series, scope.Documentation, docs)
		}
		if security, ok := src.(*securitySource); ok {
			return u.againSecurity(ctx, job, &rev, scope, security.revision.Reports)
		}
		if analysis, ok := src.(*analysisSource); ok {
			return u.againAnalysis(ctx, job, &rev, scope.analysisScope(), analysis)
		}
		return u.again(ctx, job, &rev, scope, areas, collection, now, series.MetricSet == entity.MetricSetActivity)
	})
	if err != nil {
		return err
	}
	switch {
	case job.PartNo > len(chunks):
	case reused:
		u.d.Metrics.PartReused()
	default:
		u.d.Metrics.PartComputed()
	}
	if published {
		u.d.Metrics.ChainPublished(now.Sub(rev.StartedAt))
	}
	return nil
}

// again queues the chain of the job once more when the facts of the areas or the settings of the team are newer
// than the cutoff of the revision, or the structure or the collection is not the one of the start: the recount a
// reconciliation asks for during the chain is dropped as its copy. It runs after the job completed itself, or the
// copy would match the job that is still active and be dropped (TZ 10.6).
func (u *UseCase) again(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, scope seriesScope, areas []entity.ID,
	collection []entity.AreaCollection, now time.Time, activity bool,
) error {
	facts, err := u.d.Facts.AreaChangeSeq(ctx, job.OrganizationID, areas)
	if err != nil {
		return err
	}
	if activity {
		seq, _, err := u.d.Tenants.Clock(ctx, job.OrganizationID)
		if err != nil {
			return err
		}
		facts = max(facts, seq)
	}
	var settings int64
	if !scope.TeamID.IsZero() && u.d.Settings != nil {
		if settings, err = u.d.Settings.LastSettingsSeq(ctx, job.OrganizationID, scope.TeamID); err != nil {
			return err
		}
	}
	var structure int64
	if scope.Kind != _scopeConnection && scope.Kind != _scopeCode && scope.Kind != _scopeDocumentation {
		if structure, err = u.d.Structure.StructureVersion(ctx, job.OrganizationID); err != nil {
			return err
		}
	}
	if max(facts, settings) <= rev.KnowledgeSeq && structure == job.Structure && (job.Sources == nil || sameCollection(*job.Sources, collection)) {
		return nil
	}
	at := now.Add(_recountDelay)
	if rev.Loading {
		at = now.Add(_loadingEvery)
	}
	next := job
	next.RevisionID, next.PartNo, next.AsOf, next.Sources, next.Structure, next.ActivityAreas = entity.ID{}, 0, at, nil, 0, nil
	return u.d.Queue.EnqueueAt(ctx, next, entity.QueueBulk, at)
}

// publish makes the revision current in one transaction: it becomes published, the one it replaces superseded,
// and the pointer of the series moves only from that one (TZ 12.7).
func (u *UseCase) publish(ctx context.Context, org entity.ID, rev *entity.CalcRevision, at time.Time) error {
	published, err := u.d.Repo.PublishRevision(ctx, org, rev.ID, at)
	if err != nil {
		return err
	}
	if !published {
		return fmt.Errorf("calc: revision %s is not building", rev.ID)
	}
	if prev := rev.PreviousRevisionID; prev != nil {
		if err := u.d.Repo.SupersedeRevision(ctx, org, *prev, at); err != nil {
			return err
		}
	}
	moved, err := u.d.Repo.MoveCurrentRevision(ctx, org, rev.SeriesID, rev.ID, rev.PreviousRevisionID, rev.KnowledgeSeq)
	if err != nil {
		return err
	}
	if !moved {
		return errPublishOrder
	}
	return nil
}

// chunk cuts the points of a window into parts of n points.
func chunk(ps []period, n int) [][]period {
	var out [][]period
	for start := 0; start < len(ps); start += n {
		out = append(out, ps[start:min(start+n, len(ps))])
	}
	return out
}

// storedPart is the result of a part as it is saved: intervals cut to the part and the attributes of the tasks
// its points count.
func storedPart(r entity.CalcResult, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetFlow, From: from, To: to}
	counted := map[entity.ID]bool{}
	for i := range r.Series {
		s := &r.Series[i]
		for j := range s.Points {
			p := &s.Points[j]
			out.Points = append(out.Points, entity.SeriesPoint{MetricID: s.MetricID, Estimator: s.Estimator, Unit: s.Unit, Point: *p})
			for _, c := range p.Contributions {
				counted[c.ObjectID] = true
			}
		}
	}
	for i := range r.Tasks {
		t := &r.Tasks[i]
		if counted[t.TaskID] {
			out.Tasks = append(out.Tasks, *t)
		}
		for _, iv := range t.Intervals {
			if !iv.From.Before(to) || !iv.To.After(from) {
				continue
			}
			if iv.From.Before(from) {
				iv.From = from
			}
			if iv.To.After(to) {
				iv.To = to
			}
			out.Intervals = append(out.Intervals, iv)
		}
	}
	return out
}

func definitionInputs(defs []Definition) []entity.RevisionInput {
	out := make([]entity.RevisionInput, len(defs))
	for i := range defs {
		out[i] = entity.RevisionInput{Kind: entity.InputKindDefinition, ID: defs[i].ID, Version: strconv.Itoa(defs[i].Version)}
	}
	return out
}

// revisionHash is SHA-256 of the scope, the window, as_of and the versions of the inputs of a revision.
func revisionHash(s *entity.CalcSeries, r *entity.CalcRevision, inputs []entity.RevisionInput) []byte {
	d := newDigest()
	d.bytes(s.ScopeHash)
	d.text(s.MetricSet)
	d.text(string(s.Step))
	d.time(r.Window.From)
	d.time(r.Window.To)
	d.time(r.AsOf)
	d.text(r.Timezone)
	for _, in := range inputs {
		d.text(in.Kind)
		d.text(in.ID)
		d.text(in.Version)
	}
	return d.sum()
}

// sameCollection tells whether two states of the collection give the parts the same input.
func sameCollection(a, b []entity.AreaCollection) bool {
	return slices.EqualFunc(a, b, func(x, y entity.AreaCollection) bool {
		return x.AreaID == y.AreaID && x.ConnectionID == y.ConnectionID && x.Completeness == y.Completeness && x.Phase == y.Phase &&
			x.DataKind == y.DataKind && x.LastErrorCode == y.LastErrorCode &&
			sameTime(x.Watermark, y.Watermark) && sameTime(x.LastSuccessAt, y.LastSuccessAt) && sameTime(x.HistoryFrom, y.HistoryFrom) &&
			sameRun(x.Reconcile, y.Reconcile) && sameRun(x.ConnectionReconcile, y.ConnectionReconcile)
	})
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sameRun(a, b *entity.ReconcileRun) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Outcome == b.Outcome && sameTime(a.FinishedAt, b.FinishedAt)
}

func (u *UseCase) collectionOf(ctx context.Context, org entity.ID, areas []entity.ID) ([]entity.AreaCollection, error) {
	if u.d.Collection == nil || len(areas) == 0 {
		return nil, nil
	}
	return u.d.Collection.AreaCollection(ctx, org, areas)
}

func (u *UseCase) seriesCollection(ctx context.Context, org entity.ID, areas []entity.ID, set string, connection entity.ID) ([]entity.AreaCollection, error) {
	if set == entity.MetricSetDocumentation {
		return nil, nil
	}
	if set == entity.MetricSetActivity {
		return u.d.Activity.ActivityCollection(ctx, org, areas, connection)
	}
	if set == entity.MetricSetCI && u.d.Collection != nil && len(areas) > 0 {
		return u.d.Collection.CICollection(ctx, org, areas)
	}
	return u.collectionOf(ctx, org, areas)
}

// teamAreas are the source areas of the projects of the team by the current structure.
func (u *UseCase) teamAreas(ctx context.Context, org, team entity.ID) ([]entity.ID, error) {
	links, err := u.d.Structure.TeamProjectAreas(ctx, org, team, u.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	areas := make([]entity.ID, 0, len(links))
	for _, l := range links {
		if !slices.Contains(areas, l.To) {
			areas = append(areas, l.To)
		}
	}
	return areas, nil
}

// loading tells a collection of the areas that still loads its history (FR-015).
func loading(sources []entity.AreaCollection) bool {
	return slices.ContainsFunc(sources, func(c entity.AreaCollection) bool { return c.Phase == entity.SyncBackfill })
}

// teamScopeOf is the canonical scope of the series of a team and its SHA-256.
func teamScopeOf(teamID entity.ID) (scope, hash []byte, err error) {
	scope, err = json.Marshal(teamScope{Kind: "team", TeamID: teamID})
	if err != nil {
		return nil, nil, fmt.Errorf("calc: encode scope: %w", err)
	}
	sum := sha256.Sum256(scope)
	return scope, sum[:], nil
}

func scopeTeam(raw []byte) (entity.ID, error) {
	var s teamScope
	if err := json.Unmarshal(raw, &s); err != nil || s.Kind != "team" {
		return entity.ID{}, fmt.Errorf("calc: scope of the series is not a team: %s", raw)
	}
	return s.TeamID, nil
}
