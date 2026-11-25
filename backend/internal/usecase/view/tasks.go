package view

import (
	"bytes"
	"cmp"
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_listTasks = "listTasks"
	_getTask   = "getTask"
)

type TaskFacts interface {
	VisibleAreaFacts(context.Context, entity.Scope, entity.ID, entity.ID, int64) (entity.AreaFacts, error)
	VisibleTaskFacts(context.Context, entity.Scope, entity.ID, entity.LinkEnd, int64) (entity.AreaFacts, error)
	Dictionaries(context.Context, entity.ID, []entity.ID) ([]entity.AreaDictionary, error)
	TaskMRFacts(context.Context, entity.Scope, entity.ID, entity.LinkEnd, int64) (entity.ReviewFacts, error)
}

type TaskStructure interface {
	TaskStructure(context.Context, entity.ID, time.Time) (projectAreas, teamProjects []entity.Link, err error)
}

type TaskSettings interface {
	MappingAt(context.Context, entity.ID, entity.ID, int64) (entity.MappingVersion, bool, error)
	TargetVersionsAt(context.Context, entity.ID, entity.ID, int64) ([]entity.TargetVersion, error)
}

type TaskCollection interface {
	AreaCollection(context.Context, entity.ID, []entity.ID) ([]entity.AreaCollection, error)
	MRCollection(context.Context, entity.ID, []entity.ID) ([]entity.AreaCollection, error)
}

type TaskCalculator interface {
	TaskBelongsToTeam(*entity.TaskObservationFacts, entity.ID) bool
	TaskWorkflow(*entity.TaskObservationFacts) entity.TaskWorkflow
	SortTaskObservations([]entity.TaskObservation)
}

type TaskObservationProvider func(*entity.TaskObservationFacts) []entity.TaskObservation

type TaskDeps struct {
	Facts        TaskFacts
	Structure    TaskStructure
	Settings     TaskSettings
	Collection   TaskCollection
	Calculator   TaskCalculator
	Observations []TaskObservationProvider
}

type taskCut struct {
	asOf time.Time
	seq  int64
}

func (u *UseCase) taskCut(ctx context.Context, org entity.ID, after *entity.TaskViewCursor) (taskCut, error) {
	now := u.d.Clock.Now().UTC()
	at := now
	var known *int64
	if after != nil {
		at, known = after.AsOf, &after.Knowledge
		if at.IsZero() || at.After(now) || now.Sub(at) > _keepFor {
			return taskCut{}, entity.ErrInvalidCursor
		}
	}
	seq, err := u.d.Code.ReportKnowledge(ctx, org, known)
	return taskCut{asOf: at, seq: seq}, err
}

type taskStructure struct {
	projects map[entity.ID][]entity.ID
	teams    map[entity.ID][]entity.ID
}

func (u *UseCase) taskStructure(ctx context.Context, org entity.ID, scope entity.Scope, at time.Time) (taskStructure, error) {
	areas, teams, err := u.d.Tasks.Structure.TaskStructure(ctx, org, at)
	if err != nil {
		return taskStructure{}, err
	}
	out := taskStructure{projects: map[entity.ID][]entity.ID{}, teams: map[entity.ID][]entity.ID{}}
	allowed := scope.Projects()
	for _, link := range areas {
		if scope.Whole() || slices.Contains(allowed, link.From) {
			out.projects[link.To] = append(out.projects[link.To], link.From)
		}
	}
	for _, link := range teams {
		out.teams[link.To] = append(out.teams[link.To], link.From)
	}
	return out, nil
}

type taskRead struct {
	uc         *UseCase
	org        entity.ID
	cut        taskCut
	structure  taskStructure
	history    map[entity.ID]*time.Time
	settings   map[entity.ID]taskSettings
	instances  map[entity.ID]entity.SourceInstance
	dictionary map[entity.ID]map[entity.DictionaryKind]map[string]string
}

type taskSettings struct {
	mapping *entity.MappingVersion
	targets []entity.TargetVersion
}

func (u *UseCase) taskRead(ctx context.Context, org entity.ID, cut taskCut, structure taskStructure, areas []entity.ID) (*taskRead, error) {
	collection, err := u.d.Tasks.Collection.AreaCollection(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	dict, err := u.d.Tasks.Facts.Dictionaries(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	r := &taskRead{
		uc: u, org: org, cut: cut, structure: structure, history: taskHistory(collection),
		settings: map[entity.ID]taskSettings{}, instances: map[entity.ID]entity.SourceInstance{},
		dictionary: make(map[entity.ID]map[entity.DictionaryKind]map[string]string, len(dict)),
	}
	for _, area := range dict {
		kinds := map[entity.DictionaryKind]map[string]string{}
		for _, item := range area.Items {
			if kinds[item.Kind] == nil {
				kinds[item.Kind] = map[string]string{}
			}
			kinds[item.Kind][item.NativeID] = item.Name
		}
		r.dictionary[area.AreaID] = kinds
	}
	return r, nil
}

func (r *taskRead) teamSettings(ctx context.Context, team entity.ID) (taskSettings, error) {
	if s, ok := r.settings[team]; ok {
		return s, nil
	}
	module := r.uc.d.Tasks.Settings
	mapping, found, err := module.MappingAt(ctx, r.org, team, r.cut.seq)
	if err != nil {
		return taskSettings{}, err
	}
	var s taskSettings
	if found {
		s.mapping = &mapping
		s.targets, err = module.TargetVersionsAt(ctx, r.org, team, r.cut.seq)
		if err != nil {
			return taskSettings{}, err
		}
	}
	r.settings[team] = s
	return s, nil
}

func (r *taskRead) taskTeam(ctx context.Context, area entity.ID, facts *entity.TaskObservationFacts) (*entity.ID, error) {
	var matched *entity.ID
	for _, project := range r.structure.projects[area] {
		for _, team := range r.structure.teams[project] {
			settings, err := r.teamSettings(ctx, team)
			if err != nil {
				return nil, err
			}
			facts.Mapping = settings.mapping
			if !r.uc.d.Tasks.Calculator.TaskBelongsToTeam(facts, team) {
				continue
			}
			if matched != nil && *matched != team {
				facts.Mapping, facts.Targets = nil, nil
				return nil, nil
			}
			matched = &team
		}
	}
	facts.Mapping, facts.Targets = nil, nil
	if matched != nil {
		s := r.settings[*matched]
		facts.Mapping, facts.Targets = s.mapping, s.targets
	}
	return matched, nil
}

func (r *taskRead) summary(ctx context.Context, task *entity.TaskVersion, instance entity.ID, events []entity.TaskEvent) (entity.TaskSummary, *entity.TaskObservationFacts, error) {
	f := &entity.TaskObservationFacts{
		Task: *task, InstanceID: instance, TaskEvents: events, HistoryFrom: r.history[task.AreaID], AsOf: r.cut.asOf, KnowledgeSeq: r.cut.seq,
	}
	team, err := r.taskTeam(ctx, task.AreaID, f)
	if err != nil {
		return entity.TaskSummary{}, nil, err
	}
	out := entity.TaskSummary{Version: *task, TeamID: team, TaskWorkflow: r.uc.d.Tasks.Calculator.TaskWorkflow(f)}
	if projects := r.structure.projects[task.AreaID]; len(projects) > 0 {
		out.ProjectID = &projects[0]
	}
	dict := r.dictionary[task.AreaID]
	out.StatusName = cmp.Or(dict[entity.DictionaryStatus][task.StatusNativeID], task.StatusNativeID)
	if out.Type == nil {
		if name := dict[entity.DictionaryType][task.TypeNativeID]; name != "" {
			out.Type = &name
		}
	}
	if task.TaskKey != "" {
		src, err := r.instance(ctx, instance)
		if err != nil {
			return entity.TaskSummary{}, nil, err
		}
		if src.Provider == entity.ProviderJiraCloud {
			out.SourceURL = sourceURL(src.BaseURL, "/browse/"+url.PathEscape(task.TaskKey))
		}
	}
	return out, f, nil
}

func (r *taskRead) instance(ctx context.Context, id entity.ID) (entity.SourceInstance, error) {
	if instance, ok := r.instances[id]; ok {
		return instance, nil
	}
	instance, err := r.uc.d.Instances.Instance(ctx, r.org, id)
	if err == nil {
		r.instances[id] = instance
	}
	return instance, err
}

func sourceURL(base, route string) *string {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil
	}
	u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
	value := strings.TrimRight(u.String(), "/") + route
	return &value
}

func taskPosition(row *entity.TaskSummary, cut taskCut) entity.TaskViewCursor {
	return entity.TaskViewCursor{
		AsOf: cut.asOf, Knowledge: cut.seq, ID: row.Version.TaskID,
		Age: row.AgeSeconds, UpdatedAt: row.Version.SourceUpdatedAt, Key: row.Version.TaskKey,
	}
}

func compareTaskPosition(a, b entity.TaskViewCursor, order string) int {
	key, desc := strings.CutPrefix(order, "-")
	var n int
	switch key {
	case "updated_at":
		n = compareTaskValue(a.UpdatedAt, b.UpdatedAt, desc, time.Time.Compare)
	case "key":
		n = compareTaskValue(taskKey(a.Key), taskKey(b.Key), desc, cmp.Compare[string])
	default:
		n = compareTaskValue(a.Age, b.Age, desc, cmp.Compare[int])
	}
	return cmp.Or(n, bytes.Compare(a.ID[:], b.ID[:]))
}

func taskKey(key string) *string {
	if key == "" {
		return nil
	}
	return &key
}

// Missing values sort after known values in either direction.
func compareTaskValue[T any](a, b *T, desc bool, compare func(T, T) int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case desc:
		return compare(*b, *a)
	default:
		return compare(*a, *b)
	}
}
