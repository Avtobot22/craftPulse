// Package calc owns the formulas of the metrics of the catalog (TZ 10.3, rule 5): the registry of definitions
// and the calculation of workflow episodes, intervals and flow metrics from canonical facts and the confirmed
// mapping of a team. view, the API and saved views show its results and have no formulas of their own.
package calc

import (
	"context"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Structure is the org module: the source areas of the projects of a team.
type Structure interface {
	TeamProjectAreas(ctx context.Context, orgID, teamID entity.ID, at time.Time) ([]entity.Link, error)
}

// Facts is the normalize module: the tasks and events of an area at a knowledge number.
type Facts interface {
	AreaFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error)
}

// Settings is the process module: the mapping of a team in force at a knowledge number.
type Settings interface {
	MappingAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, bool, error)
}

type Deps struct {
	Structure Structure
	Facts     Facts
	Settings  Settings
	Clock     clock.Clock
}

type UseCase struct {
	d Deps
}

func New(d Deps) *UseCase {
	return &UseCase{d: d}
}

// Calculate computes the flow metrics of a team over the window at the knowledge number of the request: the
// facts and the mapping are read at that slice, the projects of the team by the current structure (M0,
// current_structure). Without a confirmed mapping every point is NOT_EVALUATED with mapping_missing.
func (u *UseCase) Calculate(ctx context.Context, req entity.CalcRequest) (entity.CalcResult, error) {
	org := req.OrganizationID
	in := input{team: req.TeamID, window: req.Window, asOf: req.AsOf, location: time.UTC, groupBy: req.GroupBy}
	mapping, ok, err := u.d.Settings.MappingAt(ctx, org, req.TeamID, req.KnowledgeSeq)
	if err != nil {
		return entity.CalcResult{}, err
	}
	if !ok {
		return compute(in), nil
	}
	in.mapping = &mapping
	if in.location, err = time.LoadLocation(mapping.Timezone); err != nil {
		return entity.CalcResult{}, fmt.Errorf("calc: time zone of mapping version %d: %w", mapping.VersionNo, err)
	}
	links, err := u.d.Structure.TeamProjectAreas(ctx, org, req.TeamID, u.d.Clock.Now())
	if err != nil {
		return entity.CalcResult{}, err
	}
	seen := map[entity.ID]bool{}
	for _, l := range links {
		if seen[l.To] {
			continue
		}
		seen[l.To] = true
		facts, err := u.d.Facts.AreaFacts(ctx, org, l.To, req.KnowledgeSeq)
		if err != nil {
			return entity.CalcResult{}, err
		}
		in.tasks = append(in.tasks, tasksOf(&facts, l.From)...)
	}
	return compute(in), nil
}

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
