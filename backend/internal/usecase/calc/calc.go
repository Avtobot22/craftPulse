// Package calc owns the formulas of the metrics of the catalog (TZ 10.3, rule 5): the registry of definitions
// and the calculation of workflow episodes, intervals and flow metrics from canonical facts and the confirmed
// mapping of a team. view, the API and saved views show its results and have no formulas of their own.
package calc

import (
	"context"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Structure is the org module: the source areas of the projects of a team.
type Structure interface {
	TeamProjectAreas(ctx context.Context, orgID, teamID entity.ID, at time.Time) ([]entity.Link, error)
}

// Facts is the normalize module: the tasks and events of an area at a knowledge number, all of them for a
// series and the ones a scope allows for a reader.
type Facts interface {
	AreaFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error)
	VisibleAreaFacts(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error)
	RestrictedObjects(ctx context.Context, scope entity.Scope, orgID entity.ID, areaIDs []entity.ID) ([]entity.RestrictedObject, error)
}

// Settings is the process module: the mapping of a team in force at a knowledge number and the check of a draft
// of it; errors of the fields of a draft are *entity.ValidationError.
type Settings interface {
	MappingAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, bool, error)
	CheckDraft(ctx context.Context, orgID, teamID entity.ID, d entity.MappingDraft) error
}

// Rights is the access module: what the sources allow the subject to read now (TZ 13.4, 13.5).
type Rights interface {
	Decide(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.SourceQuery) (entity.Scope, error)
}

// Metrics count the revisions that failed and the parts a revision took from earlier ones.
type Metrics interface {
	RevisionFailed()
	PartReused()
}

type Deps struct {
	Structure Structure
	Facts     Facts
	Settings  Settings
	Rights    Rights
	Repo      repo.CalcRepo
	Tenants   repo.TenantRepo
	Queue     repo.JobQueue
	Tx        repo.Transactor
	Metrics   Metrics
	Clock     clock.Clock
	NewID     func() (entity.ID, error)
	// PartPoints is the points of one part of a window; _partPoints when zero.
	PartPoints int
	// GCBatch is the parts one portion of the collection deletes; _gcBatch when zero.
	GCBatch int
}

type UseCase struct {
	d Deps
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
	return &UseCase{d: d}
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
	in := input{team: req.TeamID, window: req.Window, asOf: req.AsOf, location: time.UTC, groupBy: req.GroupBy}
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
	for _, l := range links {
		if seen[l.To] {
			continue
		}
		seen[l.To] = true
		facts, err := u.d.Facts.AreaFacts(ctx, org, l.To, req.KnowledgeSeq)
		if err != nil {
			return in, err
		}
		in.tasks = append(in.tasks, tasksOf(&facts, l.From)...)
	}
	return in, nil
}

type noMetrics struct{}

func (noMetrics) RevisionFailed() {}

func (noMetrics) PartReused() {}

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
		out[i] = taskInput{version: v, instance: f.InstanceID, project: project, events: byTask[v.TaskID]}
	}
	return out
}
