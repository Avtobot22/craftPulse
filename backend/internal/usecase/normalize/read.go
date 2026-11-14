package normalize

import (
	"context"
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

func errorf(format string, args ...any) error { return fmt.Errorf("normalize: "+format, args...) }
