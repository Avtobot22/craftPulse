package normalize

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_classSnapshot     = "snapshot"
	_providerUniversal = "universal"
)

var _dictionaries = map[envelope.RecordRecordKind]entity.DictionaryKind{
	"task_status":     entity.DictionaryStatus,
	"task_resolution": entity.DictionaryResolution,
	"task_type":       entity.DictionaryType,
	"task_field":      entity.DictionaryField,
}

// input is a part of a receipt parsed and canonized before the transaction (TZ 12.6): only the decisions
// against the current versions are left for the time the knowledge counter is locked.
type input struct {
	instance entity.ID
	area     entity.SourceKey
	accounts map[string]entity.SourceAccount
	tasks    []*taskCandidate
	events   []*eventCandidate
	entries  []*entryCandidate
}

type taskCandidate struct {
	key        entity.SourceKey
	area       entity.SourceKey
	parent     *entity.SourceKey
	assignee   string
	snapshot   bool
	restricted bool
	version    entity.TaskVersion
}

type eventCandidate struct {
	task                 entity.SourceKey
	before, after, actor string
	event                entity.TaskEvent
}

type entryCandidate struct {
	entry entity.DictionaryEntry
}

// prepare reads the records of a part from the package. A later record of the same object in the part
// wins by the order of the source, so a transaction writes one new version of an object (TZ 12.6).
func prepare(rc entity.Receipt, pkg *envelope.Package, part []entity.IngestRecord, via entity.ObservedVia) (*input, error) {
	m := pkg.Manifest
	in := &input{
		instance: *rc.SourceInstanceID,
		area:     entity.SourceKey{InstanceID: *rc.SourceInstanceID, Kind: entity.ObjectKind(m.Area.Kind), NativeID: m.Area.NativeId},
		accounts: map[string]entity.SourceAccount{},
	}
	taskNS := ""
	if m.Source.Provider == _providerUniversal {
		taskNS = m.Area.NativeId
	}
	tasks := map[entity.SourceKey]int{}
	events := map[[2]string]int{}
	entries := map[entity.DictionaryKey]int{}
	for i := range part {
		r := &part[i]
		rec, ok, err := pkg.Record(r.No)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("normalize: record %d of receipt %s is not in its package", r.No, rc.ID)
		}
		base := baseFact{
			ref: entity.RecordRef{ReceiptID: rc.ID, RecordNo: r.No}, revision: entity.Revision{Source: r.SourceRevision, Hash: r.ContentHash},
			observed: rc.ReceivedAt,
		}
		switch kind := rec.RecordKind; {
		case kind == "task":
			c, err := in.task(rec, base, taskNS)
			if err != nil {
				return nil, err
			}
			keep(&in.tasks, tasks, c.key, c, func(old *taskCandidate) bool { return !older(c, old) })
		case kind == "task_event":
			c, err := in.event(rec, base, taskNS, via)
			if err != nil {
				return nil, err
			}
			keep(&in.events, events, [2]string{c.task.NativeID, c.event.EventKey}, c, func(*eventCandidate) bool { return true })
		case kind == "account":
			if err := in.account(rec); err != nil {
				return nil, err
			}
		case _dictionaries[kind] != "":
			c, err := entry(rec, base, _dictionaries[kind])
			if err != nil {
				return nil, err
			}
			keep(&in.entries, entries, entity.DictionaryKey{Kind: c.entry.Kind, NativeID: c.entry.NativeID}, c, func(*entryCandidate) bool { return true })
		}
	}
	slices.SortFunc(in.tasks, func(a, b *taskCandidate) int { return compareKeys(a.key, b.key) })
	return in, nil
}

type baseFact struct {
	ref      entity.RecordRef
	revision entity.Revision
	observed time.Time
}

// keep puts a candidate in the list, or in the place of an earlier one of the same key when wins says so.
func keep[K comparable, C any](list *[]C, index map[K]int, key K, c C, wins func(old C) bool) {
	if i, ok := index[key]; ok {
		if wins((*list)[i]) {
			(*list)[i] = c
		}
		return
	}
	index[key] = len(*list)
	*list = append(*list, c)
}

func (in *input) task(rec *envelope.Record, base baseFact, ns string) (*taskCandidate, error) {
	var p envelope.TaskV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, fmt.Errorf("normalize: decode task %s: %w", rec.NativeId, err)
	}
	c := &taskCandidate{
		key:      entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectTask, NativeNS: ns, NativeID: rec.NativeId},
		area:     entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectTrackerProject, NativeID: p.ProjectNativeId},
		snapshot: string(rec.RecordClass) == _classSnapshot,
		version: entity.TaskVersion{
			Record: base.ref, Revision: base.revision, ObservedAt: base.observed, SourceUpdatedAt: p.UpdatedAt,
			TaskKey: deref(p.TaskKey), Title: deref(p.Title), TypeNativeID: p.TypeNativeId, StatusNativeID: p.StatusNativeId,
			ResolutionNative: deref(p.ResolutionNativeId), CreatedSrcAt: p.CreatedAt, SizeNative: deref(p.Size), TeamNative: deref(p.Team),
			RestrictionNative: deref(p.Restriction),
		},
	}
	c.restricted = c.version.RestrictionNative != ""
	if p.ParentNativeId != nil && *p.ParentNativeId != "" {
		c.parent = &entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectTask, NativeNS: ns, NativeID: *p.ParentNativeId}
	}
	if p.RawFields != nil {
		raw, err := json.Marshal(*p.RawFields)
		if err != nil {
			return nil, fmt.Errorf("normalize: encode raw fields of %s: %w", rec.NativeId, err)
		}
		c.version.RawFields = raw
	}
	c.assignee = in.refer(p.AssigneeAccountNativeId)
	return c, nil
}

func (in *input) event(rec *envelope.Record, base baseFact, ns string, via entity.ObservedVia) (*eventCandidate, error) {
	var p envelope.TaskEventV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, fmt.Errorf("normalize: decode task event %s: %w", rec.NativeId, err)
	}
	if rec.EventTime == nil {
		return nil, fmt.Errorf("normalize: task event %s has no event time", rec.NativeId)
	}
	_, offset := rec.EventTime.Zone()
	return &eventCandidate{
		task:   entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectTask, NativeNS: ns, NativeID: p.TaskNativeId},
		before: in.refer(p.BeforeAccountNativeId), after: in.refer(p.AfterAccountNativeId), actor: in.refer(p.ActorAccountNativeId),
		event: entity.TaskEvent{
			EventKey: rec.NativeId, Record: base.ref, Revision: base.revision, Kind: entity.TaskEventKind(p.EventKind),
			FieldNativeID: deref(p.FieldNativeId), BeforeValue: p.BeforeValue, AfterValue: p.AfterValue,
			EventTime: rec.EventTime.UTC(), EventOffsetS: offset, ObservedAt: base.observed, ObservedVia: via,
		},
	}, nil
}

// account describes an account; a later record of the same account in the part replaces the earlier one.
func (in *input) account(rec *envelope.Record) error {
	var p envelope.AccountV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return fmt.Errorf("normalize: decode account %s: %w", rec.NativeId, err)
	}
	in.accounts[rec.NativeId] = entity.SourceAccount{
		NativeID: rec.NativeId, Login: deref(p.Login), DisplayName: deref(p.DisplayName), Email: deref(p.Email),
		Kind: entity.PersonKind(p.AccountKind), Described: true,
	}
	return nil
}

// refer notes an account a fact names; it does not change a described one.
func (in *input) refer(native *string) string {
	if native == nil || *native == "" {
		return ""
	}
	if _, ok := in.accounts[*native]; !ok {
		in.accounts[*native] = entity.SourceAccount{NativeID: *native, Kind: entity.PersonUnknown}
	}
	return *native
}

func entry(rec *envelope.Record, base baseFact, kind entity.DictionaryKind) (*entryCandidate, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, fmt.Errorf("normalize: decode %s %s: %w", kind, rec.NativeId, err)
	}
	return &entryCandidate{entry: entity.DictionaryEntry{
		Kind: kind, NativeID: rec.NativeId, Record: base.ref, Revision: base.revision, Name: p.Name, Attributes: rec.Payload,
	}}, nil
}

// older tells whether a state of a task is older than another by the pair (source_updated_at, time of
// receipt); a snapshot and a state without the time of change are ordered by the time of receipt only.
func older(c, than *taskCandidate) bool {
	return olderState(c.version, than.version, c.snapshot)
}

func olderState(v, than entity.TaskVersion, snapshot bool) bool {
	if !snapshot && v.SourceUpdatedAt != nil && than.SourceUpdatedAt != nil && !v.SourceUpdatedAt.Equal(*than.SourceUpdatedAt) {
		return v.SourceUpdatedAt.Before(*than.SourceUpdatedAt)
	}
	return v.ObservedAt.Before(than.ObservedAt)
}

func compareKeys(a, b entity.SourceKey) int {
	return cmp.Or(
		slices.Compare(a.InstanceID[:], b.InstanceID[:]), cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.NativeNS, b.NativeNS), cmp.Compare(a.NativeID, b.NativeID),
	)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
