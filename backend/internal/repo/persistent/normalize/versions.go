package normalize

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) CurrentTaskVersions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) ([]entity.TaskVersion, error) {
	rows, err := r.q(ctx).CurrentTaskVersions(ctx, CurrentTaskVersionsParams{OrganizationID: uuid.UUID(orgID), TaskIds: ids(taskIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: current task versions: %w", err)
	}
	return taskVersions(rows), nil
}

func (r *Repo) TaskRevisions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) (map[entity.ID][]entity.Revision, error) {
	rows, err := r.q(ctx).TaskRevisions(ctx, TaskRevisionsParams{OrganizationID: uuid.UUID(orgID), TaskIds: ids(taskIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: task revisions: %w", err)
	}
	out := make(map[entity.ID][]entity.Revision, len(rows))
	for _, row := range rows {
		id := entity.ID(row.TaskID)
		out[id] = append(out[id], entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash})
	}
	return out, nil
}

func (r *Repo) SupersedeTaskVersions(ctx context.Context, orgID entity.ID, versionIDs []entity.ID, seq int64, at time.Time) error {
	if len(versionIDs) == 0 {
		return nil
	}
	err := r.q(ctx).SupersedeTaskVersions(ctx, SupersedeTaskVersionsParams{KnownSeq: seq, At: at, OrganizationID: uuid.UUID(orgID), Ids: ids(versionIDs)})
	if err != nil {
		return fmt.Errorf("normalize: supersede task versions: %w", err)
	}
	return nil
}

func (r *Repo) InsertTaskVersions(ctx context.Context, orgID entity.ID, versions []entity.TaskVersion) error {
	params := make([]InsertTaskVersionParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertTaskVersionParams{
			OrganizationID: uuid.UUID(orgID), TaskID: uuid.UUID(v.TaskID), AreaID: uuid.UUID(v.AreaID), KnownSeq: v.KnownSeq,
			ReceiptID: uuid.UUID(v.Record.ReceiptID), RecordNo: no, SourceRevision: v.Revision.Source, ContentHash: v.Revision.Hash,
			ObservedAt: v.ObservedAt, SourceUpdatedAt: tstz(v.SourceUpdatedAt), TaskKey: text(v.TaskKey), Title: text(v.Title),
			TypeNativeID: v.TypeNativeID, StatusNativeID: v.StatusNativeID, ResolutionNativeID: text(v.ResolutionNative),
			CreatedSrcAt: v.CreatedSrcAt, SizeNative: text(v.SizeNative), TeamNative: text(v.TeamNative),
			AssigneeAccountID: nullID(v.AssigneeAccountID), ParentTaskID: nullID(v.ParentTaskID),
			RestrictionNative: text(v.RestrictionNative), RawFields: v.RawFields,
		}
	}
	var batchErr error
	r.q(ctx).InsertTaskVersion(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert task versions: %w", batchErr)
	}
	return nil
}

func (r *Repo) TaskVersionsAt(ctx context.Context, orgID, areaID entity.ID, seq int64) ([]entity.TaskVersion, error) {
	rows, err := r.q(ctx).TaskVersionsAt(ctx, TaskVersionsAtParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), KnownSeq: seq})
	if err != nil {
		return nil, fmt.Errorf("normalize: task versions at: %w", err)
	}
	return taskVersions(rows), nil
}

func (r *Repo) CurrentTasksByNative(ctx context.Context, orgID, areaID entity.ID, nativeIDs []string) (map[string]entity.TaskVersion, error) {
	rows, err := r.q(ctx).CurrentTasksByNative(ctx, CurrentTasksByNativeParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), NativeIds: nativeIDs})
	if err != nil {
		return nil, fmt.Errorf("normalize: current tasks: %w", err)
	}
	out := make(map[string]entity.TaskVersion, len(rows))
	for i := range rows {
		out[rows[i].NativeID] = taskVersion(&rows[i].NormalizeTaskVersion)
	}
	return out, nil
}

func taskVersions(rows []NormalizeTaskVersion) []entity.TaskVersion {
	out := make([]entity.TaskVersion, len(rows))
	for i := range rows {
		out[i] = taskVersion(&rows[i])
	}
	return out
}

func taskVersion(row *NormalizeTaskVersion) entity.TaskVersion {
	return entity.TaskVersion{
		ID: entity.ID(row.ID), TaskID: entity.ID(row.TaskID), AreaID: entity.ID(row.AreaID),
		Knowledge:  knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
		Record:     entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo)},
		Revision:   entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash},
		ObservedAt: row.ObservedAt, SourceUpdatedAt: timePtr(row.SourceUpdatedAt), TaskKey: row.TaskKey.String, Title: row.Title.String,
		TypeNativeID: row.TypeNativeID, StatusNativeID: row.StatusNativeID, ResolutionNative: row.ResolutionNativeID.String,
		CreatedSrcAt: row.CreatedSrcAt, SizeNative: row.SizeNative.String, TeamNative: row.TeamNative.String,
		AssigneeAccountID: idPtr(row.AssigneeAccountID), ParentTaskID: idPtr(row.ParentTaskID),
		RestrictionNative: row.RestrictionNative.String, RawFields: row.RawFields,
	}
}

func eventParams(orgID entity.ID, keys []entity.EventKey) (org uuid.UUID, tasks []uuid.UUID, eventKeys []string) {
	tasks, eventKeys = make([]uuid.UUID, len(keys)), make([]string, len(keys))
	for i, k := range keys {
		tasks[i], eventKeys[i] = uuid.UUID(k.TaskID), k.Key
	}
	return uuid.UUID(orgID), tasks, eventKeys
}

func (r *Repo) CurrentTaskEvents(ctx context.Context, orgID entity.ID, keys []entity.EventKey) ([]entity.TaskEvent, error) {
	org, tasks, eventKeys := eventParams(orgID, keys)
	rows, err := r.q(ctx).CurrentTaskEvents(ctx, CurrentTaskEventsParams{OrganizationID: org, TaskIds: tasks, EventKeys: eventKeys})
	if err != nil {
		return nil, fmt.Errorf("normalize: current task events: %w", err)
	}
	return taskEvents(rows), nil
}

func (r *Repo) TaskEventRevisions(ctx context.Context, orgID entity.ID, keys []entity.EventKey) (map[entity.EventKey][]entity.Revision, error) {
	org, tasks, eventKeys := eventParams(orgID, keys)
	rows, err := r.q(ctx).TaskEventRevisions(ctx, TaskEventRevisionsParams{OrganizationID: org, TaskIds: tasks, EventKeys: eventKeys})
	if err != nil {
		return nil, fmt.Errorf("normalize: task event revisions: %w", err)
	}
	out := make(map[entity.EventKey][]entity.Revision, len(rows))
	for _, row := range rows {
		k := entity.EventKey{TaskID: entity.ID(row.TaskID), Key: row.EventKey}
		out[k] = append(out[k], entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash})
	}
	return out, nil
}

func (r *Repo) SupersedeTaskEvents(ctx context.Context, orgID entity.ID, eventIDs []entity.ID, seq int64, at time.Time) error {
	if len(eventIDs) == 0 {
		return nil
	}
	err := r.q(ctx).SupersedeTaskEvents(ctx, SupersedeTaskEventsParams{KnownSeq: seq, At: at, OrganizationID: uuid.UUID(orgID), Ids: ids(eventIDs)})
	if err != nil {
		return fmt.Errorf("normalize: supersede task events: %w", err)
	}
	return nil
}

func (r *Repo) InsertTaskEvents(ctx context.Context, orgID entity.ID, events []entity.TaskEvent) error {
	params := make([]InsertTaskEventParams, len(events))
	for i := range events {
		e := &events[i]
		no, err := recordNo(e.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertTaskEventParams{
			OrganizationID: uuid.UUID(orgID), TaskID: uuid.UUID(e.TaskID), AreaID: uuid.UUID(e.AreaID), EventKey: e.EventKey,
			SourceRevision: e.Revision.Source, ContentHash: e.Revision.Hash, KnownSeq: e.KnownSeq, ReceiptID: uuid.UUID(e.Record.ReceiptID),
			RecordNo: no, EventKind: string(e.Kind), FieldNativeID: text(e.FieldNativeID), BeforeValue: textPtr(e.BeforeValue),
			AfterValue: textPtr(e.AfterValue), BeforeAccountID: nullID(e.BeforeAccountID), AfterAccountID: nullID(e.AfterAccountID),
			ActorAccountID: nullID(e.ActorAccountID), EventTime: e.EventTime, EventOffsetS: int32(e.EventOffsetS), //nolint:gosec // the offset is checked against ±18 h
			ObservedAt: e.ObservedAt, ObservedVia: string(e.ObservedVia),
		}
	}
	var batchErr error
	r.q(ctx).InsertTaskEvent(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert task events: %w", batchErr)
	}
	return nil
}

func (r *Repo) TaskEventsAt(ctx context.Context, orgID, areaID entity.ID, seq int64) ([]entity.TaskEvent, error) {
	rows, err := r.q(ctx).TaskEventsAt(ctx, TaskEventsAtParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), KnownSeq: seq})
	if err != nil {
		return nil, fmt.Errorf("normalize: task events at: %w", err)
	}
	return taskEvents(rows), nil
}

func taskEvents(rows []NormalizeTaskEvent) []entity.TaskEvent {
	out := make([]entity.TaskEvent, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.TaskEvent{
			ID: entity.ID(row.ID), TaskID: entity.ID(row.TaskID), AreaID: entity.ID(row.AreaID), EventKey: row.EventKey,
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record:    entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo)},
			Revision:  entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash},
			Kind:      entity.TaskEventKind(row.EventKind), FieldNativeID: row.FieldNativeID.String,
			BeforeValue: strPtr(row.BeforeValue), AfterValue: strPtr(row.AfterValue), BeforeAccountID: idPtr(row.BeforeAccountID),
			AfterAccountID: idPtr(row.AfterAccountID), ActorAccountID: idPtr(row.ActorAccountID), EventTime: row.EventTime,
			EventOffsetS: int(row.EventOffsetS), ObservedAt: row.ObservedAt, ObservedVia: entity.ObservedVia(row.ObservedVia),
		}
	}
	return out
}

func dictionaryParams(keys []entity.DictionaryKey) (areas []uuid.UUID, kinds, natives []string) {
	areas, kinds, natives = make([]uuid.UUID, len(keys)), make([]string, len(keys)), make([]string, len(keys))
	for i, k := range keys {
		areas[i], kinds[i], natives[i] = uuid.UUID(k.AreaID), string(k.Kind), k.NativeID
	}
	return areas, kinds, natives
}

func (r *Repo) CurrentDictionaryEntries(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) ([]entity.DictionaryEntry, error) {
	areas, kinds, natives := dictionaryParams(keys)
	rows, err := r.q(ctx).CurrentDictionaryEntries(ctx, CurrentDictionaryEntriesParams{
		OrganizationID: uuid.UUID(orgID), AreaIds: areas, Kinds: kinds, NativeIds: natives,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: current dictionary entries: %w", err)
	}
	return dictionaryEntries(rows), nil
}

func (r *Repo) DictionaryRevisions(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) (map[entity.DictionaryKey][]entity.Revision, error) {
	areas, kinds, natives := dictionaryParams(keys)
	rows, err := r.q(ctx).DictionaryRevisions(ctx, DictionaryRevisionsParams{OrganizationID: uuid.UUID(orgID), AreaIds: areas, Kinds: kinds, NativeIds: natives})
	if err != nil {
		return nil, fmt.Errorf("normalize: dictionary revisions: %w", err)
	}
	out := make(map[entity.DictionaryKey][]entity.Revision, len(rows))
	for _, row := range rows {
		k := entity.DictionaryKey{AreaID: entity.ID(row.AreaID), Kind: entity.DictionaryKind(row.EntryKind), NativeID: row.NativeID}
		out[k] = append(out[k], entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash})
	}
	return out, nil
}

func (r *Repo) SupersedeDictionaryEntries(ctx context.Context, orgID entity.ID, entryIDs []entity.ID, seq int64, at time.Time) error {
	if len(entryIDs) == 0 {
		return nil
	}
	err := r.q(ctx).SupersedeDictionaryEntries(ctx, SupersedeDictionaryEntriesParams{KnownSeq: seq, At: at, OrganizationID: uuid.UUID(orgID), Ids: ids(entryIDs)})
	if err != nil {
		return fmt.Errorf("normalize: supersede dictionary entries: %w", err)
	}
	return nil
}

func (r *Repo) InsertDictionaryEntries(ctx context.Context, orgID entity.ID, entries []entity.DictionaryEntry) error {
	params := make([]InsertDictionaryEntryParams, len(entries))
	for i := range entries {
		e := &entries[i]
		no, err := recordNo(e.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertDictionaryEntryParams{
			OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(e.AreaID), EntryKind: string(e.Kind), NativeID: e.NativeID,
			SourceRevision: e.Revision.Source, ContentHash: e.Revision.Hash, KnownSeq: e.KnownSeq, ReceiptID: uuid.UUID(e.Record.ReceiptID),
			RecordNo: no, Name: e.Name, Attributes: e.Attributes,
		}
	}
	var batchErr error
	r.q(ctx).InsertDictionaryEntry(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert dictionary entries: %w", batchErr)
	}
	return nil
}

func (r *Repo) Dictionary(ctx context.Context, orgID, areaID entity.ID, kind entity.DictionaryKind) ([]entity.DictionaryEntry, error) {
	rows, err := r.q(ctx).DictionaryOfArea(ctx, DictionaryOfAreaParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), EntryKind: string(kind)})
	if err != nil {
		return nil, fmt.Errorf("normalize: dictionary: %w", err)
	}
	return dictionaryEntries(rows), nil
}

func dictionaryEntries(rows []NormalizeDictionaryEntry) []entity.DictionaryEntry {
	out := make([]entity.DictionaryEntry, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.DictionaryEntry{
			ID: entity.ID(row.ID), AreaID: entity.ID(row.AreaID), Kind: entity.DictionaryKind(row.EntryKind), NativeID: row.NativeID,
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record:    entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo)},
			Revision:  entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash}, Name: row.Name, Attributes: row.Attributes,
		}
	}
	return out
}
