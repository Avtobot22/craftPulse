package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CurrentTasks reads the current versions of tasks of an area by their native IDs, with source_updated_at
// and source_revision for the comparison with the manifest of the source (TZ 12.6).
func (u *UseCase) CurrentTasks(ctx context.Context, orgID, areaID entity.ID, nativeIDs []string) (map[string]entity.TaskVersion, error) {
	var out map[string]entity.TaskVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CurrentTasksByNative(ctx, orgID, areaID, nativeIDs)
		return err
	})
	return out, err
}

// TasksAt reads the versions of tasks of an area that hold for the slice seq. An area outside the scope gives
// nothing, and a task with its own restriction comes only when the scope allows it (TZ 13.4, 13.5).
func (u *UseCase) TasksAt(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) ([]entity.TaskVersion, error) {
	if !scope.Area(areaID) {
		return nil, nil
	}
	var out []entity.TaskVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TaskVersionsAt(ctx, orgID, areaID, seq, scope.Objects())
		return err
	})
	return out, err
}

// TaskEventsAt reads the events of an area known at the slice seq, in the order of their event time, by the
// same rules as TasksAt: an event of a hidden task is neither shown nor counted.
func (u *UseCase) TaskEventsAt(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) ([]entity.TaskEvent, error) {
	if !scope.Area(areaID) {
		return nil, nil
	}
	var out []entity.TaskEvent
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TaskEventsAt(ctx, orgID, areaID, seq, scope.Objects())
		return err
	})
	return out, err
}

// RestrictedObjects are the objects with their own restriction in the areas of the scope; access checks them
// in the source one by one before they are shown or counted.
func (u *UseCase) RestrictedObjects(ctx context.Context, scope entity.Scope, orgID entity.ID, areaIDs []entity.ID) ([]entity.RestrictedObject, error) {
	areas := slices.DeleteFunc(slices.Clone(areaIDs), func(id entity.ID) bool { return !scope.Area(id) })
	if len(areas) == 0 {
		return nil, nil
	}
	var out []entity.RestrictedObject
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.RestrictedObjects(ctx, orgID, areas)
		return err
	})
	return out, err
}

// AreaFacts reads every task of an area known at the slice seq with its events, restricted ones included: calc
// computes the whole area once, and access narrows what a reader sees of it (TZ 12.7).
func (u *UseCase) AreaFacts(ctx context.Context, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error) {
	out := entity.AreaFacts{AreaID: areaID}
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		instances, err := u.d.Repo.AreaInstances(ctx, orgID, []entity.ID{areaID})
		if err != nil {
			return err
		}
		out.InstanceID = instances[areaID]
		restricted, err := u.d.Repo.RestrictedObjects(ctx, orgID, []entity.ID{areaID})
		if err != nil {
			return err
		}
		all := make([]entity.ID, len(restricted))
		for i := range restricted {
			all[i] = restricted[i].ID
		}
		if out.Tasks, err = u.d.Repo.TaskVersionsAt(ctx, orgID, areaID, seq, all); err != nil {
			return err
		}
		out.Events, err = u.d.Repo.TaskEventsAt(ctx, orgID, areaID, seq, all)
		return err
	})
	return out, err
}

// VisibleAreaFacts reads the tasks and events of an area known at the slice seq by the rules of TasksAt: an
// area outside the scope gives nothing, and a task with its own restriction comes only when the scope allows it.
func (u *UseCase) VisibleAreaFacts(ctx context.Context, scope entity.Scope, orgID, areaID entity.ID, seq int64) (entity.AreaFacts, error) {
	out := entity.AreaFacts{AreaID: areaID}
	if !scope.Area(areaID) {
		return out, nil
	}
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		instances, err := u.d.Repo.AreaInstances(ctx, orgID, []entity.ID{areaID})
		if err != nil {
			return err
		}
		out.InstanceID = instances[areaID]
		if out.Tasks, err = u.d.Repo.TaskVersionsAt(ctx, orgID, areaID, seq, scope.Objects()); err != nil {
			return err
		}
		out.Events, err = u.d.Repo.TaskEventsAt(ctx, orgID, areaID, seq, scope.Objects())
		return err
	})
	return out, err
}

// Dictionary reads the current statuses, resolutions, types or fields of an area.
func (u *UseCase) Dictionary(ctx context.Context, orgID, areaID entity.ID, kind entity.DictionaryKind) ([]entity.DictionaryEntry, error) {
	var out []entity.DictionaryEntry
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.Dictionary(ctx, orgID, areaID, kind)
		return err
	})
	return out, err
}

// Dictionaries reads the current dictionaries of the areas with the instance of each; an area normalize has not
// seen yet is left out. The status category and the field type come from the record as the source sent it.
func (u *UseCase) Dictionaries(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaDictionary, error) {
	if len(areaIDs) == 0 {
		return nil, nil
	}
	var (
		instances map[entity.ID]entity.ID
		entries   []entity.DictionaryEntry
	)
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if instances, err = u.d.Repo.AreaInstances(ctx, orgID, areaIDs); err != nil {
			return err
		}
		entries, err = u.d.Repo.Dictionaries(ctx, orgID, areaIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	byArea := make(map[entity.ID]int, len(instances))
	out := make([]entity.AreaDictionary, 0, len(instances))
	for _, area := range areaIDs {
		instance, ok := instances[area]
		if _, seen := byArea[area]; !ok || seen {
			continue
		}
		byArea[area] = len(out)
		out = append(out, entity.AreaDictionary{AreaID: area, InstanceID: instance})
	}
	for i := range entries {
		at, ok := byArea[entries[i].AreaID]
		if !ok {
			continue
		}
		item, err := dictionaryItem(&entries[i])
		if err != nil {
			return nil, err
		}
		out[at].Items = append(out[at].Items, item)
	}
	return out, nil
}

func dictionaryItem(e *entity.DictionaryEntry) (entity.DictionaryItem, error) {
	var attrs struct {
		Category  string `json:"category"`
		ValueType string `json:"value_type"`
	}
	if err := json.Unmarshal(e.Attributes, &attrs); err != nil {
		return entity.DictionaryItem{}, errorf("decode %s %s: %w", e.Kind, e.NativeID, err)
	}
	return entity.DictionaryItem{Kind: e.Kind, NativeID: e.NativeID, Name: e.Name, Category: attrs.Category, ValueType: attrs.ValueType}, nil
}

// AreaOf finds the area of a scope of a source instance; entity.ErrNotFound until it is first normalized.
func (u *UseCase) AreaOf(ctx context.Context, orgID, instanceID entity.ID, kind entity.ObjectKind, nativeID string) (entity.ID, error) {
	var out entity.ID
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.AreaOf(ctx, orgID, instanceID, kind, nativeID)
		return err
	})
	return out, err
}

// CurrentObjects reads where objects are now: the current area and restriction access decides them by, and
// what a reference shows (TZ 12.7). Deleted objects are absent; ids are read in one query.
func (u *UseCase) CurrentObjects(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.LinkEnd, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []entity.LinkEnd
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.LinkEnds(ctx, orgID, ids)
		return err
	})
	return out, err
}

// AreaChanges reads the changes of an area after a knowledge number, for the recount of calc.
func (u *UseCase) AreaChanges(ctx context.Context, orgID, areaID entity.ID, afterSeq int64) ([]entity.AreaChange, error) {
	var out []entity.AreaChange
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.AreaChanges(ctx, orgID, areaID, afterSeq)
		return err
	})
	return out, err
}

// AreaChangeSeq is the knowledge number of the last change of the facts of the areas, 0 without one; a job of
// calc compares it with the slice it read (TZ 10.6).
func (u *UseCase) AreaChangeSeq(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) (int64, error) {
	if len(areaIDs) == 0 {
		return 0, nil
	}
	var seq int64
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		seq, err = u.d.Repo.AreaChangeSeq(ctx, orgID, areaIDs)
		return err
	})
	return seq, err
}

func errorf(format string, args ...any) error { return fmt.Errorf("normalize: "+format, args...) }

func (u *UseCase) RepositoryAreas(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ID, error) {
	var out []entity.ID
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.RepositoryAreas(ctx, orgID, ids)
		return err
	})
	return out, err
}
