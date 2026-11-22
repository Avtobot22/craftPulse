// Package calc owns the formulas of the metrics of the catalog (TZ 10.3, rule 5): the registry of definitions
// and the calculation of workflow episodes, intervals and flow metrics from canonical facts and the confirmed
// mapping of a team. view, the API and saved views show its results and have no formulas of their own.
package calc

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Structure is the org module: the source areas of the projects of a team and the version of the structure.
type Structure interface {
	TeamProjectAreas(ctx context.Context, orgID, teamID entity.ID, at time.Time) ([]entity.Link, error)
	StructureVersion(ctx context.Context, orgID entity.ID) (int64, error)
}

// Facts is the normalize module: the tasks and events of an area at a knowledge number, all of them for a
// series and the ones a scope allows for a reader, the knowledge number of the last change of areas, where
// counted objects are now, and the MRs and events of an area with the kinds of their participants.
type Facts interface {
	AreaFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error)
	AreaChangeSeq(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) (int64, error)
	VisibleAreaFacts(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error)
	RestrictedObjects(ctx context.Context, scope entity.Scope, orgID entity.ID, areaIDs []entity.ID) ([]entity.RestrictedObject, error)
	CurrentObjects(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.LinkEnd, error)
	ReviewFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.ReviewFacts, error)
	CIFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.CIFacts, error)
}

// Settings is the process module: the mapping and the targets of a team in force at a knowledge number, the
// knowledge number of its last version of settings, the checks of their drafts and the confirmation of a target
// in the transaction of the caller. Errors of the fields of a draft are *entity.ValidationError.
type Settings interface {
	MappingAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, bool, error)
	LastSettingsSeq(ctx context.Context, orgID, teamID entity.ID) (int64, error)
	CheckDraft(ctx context.Context, orgID, teamID entity.ID, d entity.MappingDraft) error
	TargetVersionsAt(ctx context.Context, orgID, teamID entity.ID, seq int64) ([]entity.TargetVersion, error)
	CheckTargetDraft(d entity.TargetDraft) error
	ConfirmTargetVersion(ctx context.Context, s entity.Subject, teamID entity.ID, baseVersion int, key entity.ID, d entity.TargetDraft,
		basis *entity.TargetBasis) (entity.TargetVersion, entity.SaveResult, error)
}

// Collection is the sync module: the state and the phase of the collection of the source areas, for the
// completeness of a calculation (T39, T40, T42) and the initial load of history.
type Collection interface {
	AreaCollection(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaCollection, error)
	CICollection(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaCollection, error)
}

// ActivityCollection is the sync evidence used by the activity series.
type ActivityCollection interface {
	ActivityConnectionAreas(ctx context.Context, org, connection entity.ID) ([]entity.ID, error)
	ActivityCollection(ctx context.Context, org entity.ID, areas []entity.ID, connection entity.ID) ([]entity.AreaCollection, error)
	ActivityHistory(ctx context.Context, org entity.ID, areas []entity.ID, connection entity.ID, at time.Time) ([]entity.ActivityHistoryEvidence, error)
	ActivityReconciliations(ctx context.Context, org entity.ID, connections []entity.ID, from, at time.Time) ([]entity.ReconcileRun, error)
	ActivityReceiptConnections(ctx context.Context, org entity.ID, receipts []entity.ID) (map[entity.ID]entity.ID, error)
}

// Links is the link module: the links of tasks to MRs with an end among the objects that hold at a knowledge number.
type Links interface {
	TaskMREdgesAt(ctx context.Context, orgID entity.ID, objectIDs []entity.ID, seq int64) ([]entity.LinkEdge, error)
	MREdgesAt(ctx context.Context, orgID entity.ID, mrIDs []entity.ID, seq int64) ([]entity.LinkEdge, error)
}

// Rights is the access module: what the sources allow the subject to read now (TZ 13.4, 13.5).
type Rights interface {
	Decide(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.SourceQuery) (entity.Scope, error)
}

// Metrics count the revisions that failed, the parts a revision took from earlier ones and the parts it computed,
// and time the chains of published revisions.
type Metrics interface {
	RevisionFailed()
	PartReused()
	PartComputed()
	ChainPublished(d time.Duration)
}

type Deps struct {
	Structure    Structure
	Facts        Facts
	TestCoverage TestCoverageFacts
	Settings     Settings
	// Collection is nil where no source is collected; then only the history of the facts tells completeness.
	Collection Collection
	Activity   ActivityCollection
	Rights     Rights
	// Links is nil where the coverage of links is not computed; its series then get no links.
	Links   Links
	Repo    repo.CalcRepo
	Tenants repo.TenantRepo
	Queue   repo.JobQueue
	Tx      repo.Transactor
	Metrics Metrics
	Clock   clock.Clock
	NewID   func() (entity.ID, error)
	// PartPoints is the points of one part of a window; _partPoints when zero.
	PartPoints int
	// GCBatch is the parts one portion of the collection deletes; _gcBatch when zero.
	GCBatch int
}

type UseCase struct {
	d       Deps
	cohorts *expirable.LRU[entity.ID, *cohort]
	reports *expirable.LRU[string, *report]
	flight  singleflight.Group
}

func New(d Deps) *UseCase {
	if d.PartPoints == 0 {
		d.PartPoints = _partPoints
	}
	if d.GCBatch == 0 {
		d.GCBatch = _gcBatch
	}
	if d.Metrics == nil {
		d.Metrics = noMetrics{}
	}
	return &UseCase{
		d: d, cohorts: expirable.NewLRU[entity.ID, *cohort](_cohortEntries, nil, _readAge),
		reports: expirable.NewLRU[string, *report](_reportEntries, nil, _readAge),
	}
}

// Calculate computes the flow metrics of a team over the window at the knowledge number of the request: the
// facts and the mapping are read at that slice, the projects of the team by the current structure (M0,
// current_structure). Without a confirmed mapping every point is NOT_EVALUATED with mapping_missing.
func (u *UseCase) Calculate(ctx context.Context, req entity.CalcRequest) (entity.CalcResult, error) {
	in, err := u.input(ctx, req)
	if err != nil {
		return entity.CalcResult{}, err
	}
	return compute(in), nil
}

// input reads what a calculation of the request needs; it holds no transaction across modules.
func (u *UseCase) input(ctx context.Context, req entity.CalcRequest) (input, error) {
	org := req.OrganizationID
	in := input{team: req.TeamID, window: req.Window, asOf: req.AsOf, location: time.UTC, groupBy: req.GroupBy, followUp: _followUp}
	mapping, ok, err := u.d.Settings.MappingAt(ctx, org, req.TeamID, req.KnowledgeSeq)
	if err != nil || !ok {
		return in, err
	}
	in.mapping = &mapping
	if in.location, err = time.LoadLocation(mapping.Timezone); err != nil {
		return in, fmt.Errorf("calc: time zone of mapping version %d: %w", mapping.VersionNo, err)
	}
	links, err := u.d.Structure.TeamProjectAreas(ctx, org, req.TeamID, u.d.Clock.Now())
	if err != nil {
		return in, err
	}
	seen := map[entity.ID]bool{}
	var areas []entity.ID
	for _, l := range links {
		if seen[l.To] {
			continue
		}
		seen[l.To] = true
		areas = append(areas, l.To)
		facts, err := u.d.Facts.AreaFacts(ctx, org, l.To, req.KnowledgeSeq)
		if err != nil {
			return in, err
		}
		in.tasks = append(in.tasks, tasksOf(&facts, l.From)...)
	}
	targets, err := u.d.Settings.TargetVersionsAt(ctx, org, req.TeamID, req.KnowledgeSeq)
	if err != nil {
		return in, err
	}
	in.targets = targetsOf(targets)
	if req.Sources != nil {
		in.collection = *req.Sources
		return in, nil
	}
	if u.d.Collection != nil && len(areas) > 0 {
		if in.collection, err = u.d.Collection.AreaCollection(ctx, org, areas); err != nil {
			return in, err
		}
	}
	return in, nil
}

// targetsOf are the durations of the versions in force by group.
func targetsOf(versions []entity.TargetVersion) map[entity.TargetGroup]int64 {
	out := make(map[entity.TargetGroup]int64, len(versions))
	for i := range versions {
		out[versions[i].TargetGroup] = versions[i].DurationSeconds
	}
	return out
}

type noMetrics struct{}

func (noMetrics) RevisionFailed() {}

func (noMetrics) PartReused() {}

func (noMetrics) PartComputed() {}

func (noMetrics) ChainPublished(time.Duration) {}

// tasksOf joins the events of an area to their tasks; the history of an area is complete from the creation of
// its tasks until the collection reports a boundary.
func tasksOf(f *entity.AreaFacts, project entity.ID) []taskInput {
	byTask := make(map[entity.ID][]entity.TaskEvent, len(f.Tasks))
	for i := range f.Events {
		byTask[f.Events[i].TaskID] = append(byTask[f.Events[i].TaskID], f.Events[i])
	}
	out := make([]taskInput, len(f.Tasks))
	for i := range f.Tasks {
		v := f.Tasks[i]
		out[i] = taskInput{version: v, instance: f.InstanceID, area: f.AreaID, project: project, events: byTask[v.TaskID]}
	}
	return out
}
