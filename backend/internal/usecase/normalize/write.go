package normalize

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_factTask       = "task"
	_factTaskEvent  = "task_event"
	_factDictionary = "dictionary"
)

// write applies a part under the lock of the knowledge counter: it reads the current versions of the
// objects the part touches, decides which versions become current and writes them with seq. resolved is
// the record a conflict resolution accepted, the only one that may replace a version of the same revision.
func (u *UseCase) write(ctx context.Context, org entity.ID, seq int64, in *input, resolved int) error {
	now := u.d.Clock.Now()
	accounts, err := u.d.Accounts.EnsureSourceAccounts(ctx, org, in.instance, sortedAccounts(in.accounts))
	if err != nil {
		return err
	}
	objects, err := u.d.Repo.EnsureObjects(ctx, org, in.keys(), entity.IdentityStable, seq)
	if err != nil {
		return err
	}
	if err := u.placeAreas(ctx, org, in, objects); err != nil {
		return err
	}
	w := &writer{u: u, org: org, seq: seq, now: now, resolved: resolved, objects: objects, accounts: accounts, changes: changes{}}
	if err := w.tasks(ctx, in.tasks); err != nil {
		return err
	}
	if err := w.events(ctx, in.events, objects[in.area].ID); err != nil {
		return err
	}
	if err := w.entries(ctx, in.entries, objects[in.area].ID); err != nil {
		return err
	}
	return u.d.Repo.RecordAreaChanges(ctx, org, seq, w.changes.list(), now)
}

// keys are the identities a part names: areas, tasks, their parents and the tasks of events.
func (in *input) keys() []entity.SourceKey {
	set := map[entity.SourceKey]bool{in.area: true}
	for _, t := range in.tasks {
		set[t.key], set[t.area] = true, true
		if t.parent != nil {
			set[*t.parent] = true
		}
	}
	for _, e := range in.events {
		set[e.task] = true
	}
	return slices.SortedFunc(maps.Keys(set), compareKeys)
}

// placeAreas makes an area of every area object of the part; an area belongs to itself.
func (u *UseCase) placeAreas(ctx context.Context, org entity.ID, in *input, objects map[entity.SourceKey]entity.SourceObject) error {
	areas := map[entity.ID]entity.ObjectKind{}
	var placements []entity.ObjectPlacement
	for _, k := range in.keys() {
		o := objects[k]
		if k.Kind == entity.ObjectTask {
			continue
		}
		areas[o.ID] = k.Kind
		if o.CurrentAreaID == nil {
			placements = append(placements, entity.ObjectPlacement{ObjectID: o.ID, AreaID: o.ID})
		}
	}
	if err := u.d.Repo.EnsureAreas(ctx, org, areas); err != nil {
		return err
	}
	return u.d.Repo.PlaceObjects(ctx, org, placements)
}

type writer struct {
	u        *UseCase
	org      entity.ID
	seq      int64
	now      time.Time
	resolved int
	objects  map[entity.SourceKey]entity.SourceObject
	accounts map[string]entity.ID
	changes  changes
}

func (w *writer) tasks(ctx context.Context, candidates []*taskCandidate) error {
	ids := make([]entity.ID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, w.objects[c.key].ID)
	}
	if len(ids) == 0 {
		return nil
	}
	current, err := w.u.d.Repo.CurrentTaskVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	byTask := make(map[entity.ID]entity.TaskVersion, len(current))
	for i := range current {
		byTask[current[i].TaskID] = current[i]
	}
	revisions, err := w.u.d.Repo.TaskRevisions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	var (
		supersede  []entity.ID
		insert     []entity.TaskVersion
		placements []entity.ObjectPlacement
	)
	for _, c := range candidates {
		obj := w.objects[c.key]
		if obj.DeletedAt != nil || !w.admits(revisions[obj.ID], c.version.Revision, c.version.Record.RecordNo) {
			continue
		}
		cur, has := byTask[obj.ID]
		if has && (olderState(c.version, cur, c.snapshot) || bytes.Equal(c.version.Revision.Hash, cur.Revision.Hash)) {
			continue
		}
		v := c.version
		v.TaskID, v.AreaID, v.KnownSeq = obj.ID, w.objects[c.area].ID, w.seq
		v.AssigneeAccountID = w.account(c.assignee)
		if c.parent != nil {
			parent := w.objects[*c.parent].ID
			v.ParentTaskID = &parent
		}
		changed := v.SourceUpdatedAt
		if changed == nil {
			changed = &v.CreatedSrcAt
		}
		if has {
			supersede = append(supersede, cur.ID)
			if cur.AreaID != v.AreaID {
				w.changes.add(cur.AreaID, _factTask, changed)
			}
		}
		insert = append(insert, v)
		if obj.CurrentAreaID == nil || *obj.CurrentAreaID != v.AreaID || obj.Restricted != c.restricted {
			placements = append(placements, entity.ObjectPlacement{ObjectID: obj.ID, AreaID: v.AreaID, Restricted: c.restricted})
		}
		w.changes.add(v.AreaID, _factTask, changed)
	}
	if err := w.u.d.Repo.SupersedeTaskVersions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	if err := w.u.d.Repo.InsertTaskVersions(ctx, w.org, insert); err != nil {
		return err
	}
	return w.u.d.Repo.PlaceObjects(ctx, w.org, placements)
}

// events writes new events and corrections: a new revision of an event replaces its current version, the
// former one stays for slices taken before (TZ 12.6). The area of an event is the area it was read in.
func (w *writer) events(ctx context.Context, candidates []*eventCandidate, area entity.ID) error {
	keys := make([]entity.EventKey, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, entity.EventKey{TaskID: w.objects[c.task].ID, Key: c.event.EventKey})
	}
	if len(keys) == 0 {
		return nil
	}
	current, err := w.u.d.Repo.CurrentTaskEvents(ctx, w.org, keys)
	if err != nil {
		return err
	}
	byKey := make(map[entity.EventKey]entity.TaskEvent, len(current))
	for i := range current {
		e := &current[i]
		byKey[entity.EventKey{TaskID: e.TaskID, Key: e.EventKey}] = *e
	}
	revisions, err := w.u.d.Repo.TaskEventRevisions(ctx, w.org, keys)
	if err != nil {
		return err
	}
	var (
		supersede []entity.ID
		insert    []entity.TaskEvent
	)
	for i, c := range candidates {
		key := keys[i]
		if w.objects[c.task].DeletedAt != nil || !w.admits(revisions[key], c.event.Revision, c.event.Record.RecordNo) {
			continue
		}
		if cur, has := byKey[key]; has {
			supersede = append(supersede, cur.ID)
		}
		e := c.event
		e.TaskID, e.AreaID, e.KnownSeq = key.TaskID, area, w.seq
		e.BeforeAccountID, e.AfterAccountID, e.ActorAccountID = w.account(c.before), w.account(c.after), w.account(c.actor)
		insert = append(insert, e)
		w.changes.add(area, _factTaskEvent, &e.EventTime)
	}
	if err := w.u.d.Repo.SupersedeTaskEvents(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertTaskEvents(ctx, w.org, insert)
}

// entries keeps the dictionaries of an area; an entry read again with the same content adds no version.
func (w *writer) entries(ctx context.Context, candidates []*entryCandidate, area entity.ID) error {
	keys := make([]entity.DictionaryKey, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, entity.DictionaryKey{AreaID: area, Kind: c.entry.Kind, NativeID: c.entry.NativeID})
	}
	if len(keys) == 0 {
		return nil
	}
	current, err := w.u.d.Repo.CurrentDictionaryEntries(ctx, w.org, keys)
	if err != nil {
		return err
	}
	byKey := make(map[entity.DictionaryKey]entity.DictionaryEntry, len(current))
	for i := range current {
		e := &current[i]
		byKey[entity.DictionaryKey{AreaID: e.AreaID, Kind: e.Kind, NativeID: e.NativeID}] = *e
	}
	revisions, err := w.u.d.Repo.DictionaryRevisions(ctx, w.org, keys)
	if err != nil {
		return err
	}
	var (
		supersede []entity.ID
		insert    []entity.DictionaryEntry
	)
	for i, c := range candidates {
		key := keys[i]
		if !w.admits(revisions[key], c.entry.Revision, c.entry.Record.RecordNo) {
			continue
		}
		cur, has := byKey[key]
		if has && bytes.Equal(cur.Revision.Hash, c.entry.Revision.Hash) {
			continue
		}
		if has {
			supersede = append(supersede, cur.ID)
		}
		e := c.entry
		e.AreaID, e.KnownSeq = area, w.seq
		insert = append(insert, e)
		w.changes.add(area, _factDictionary, nil)
	}
	if err := w.u.d.Repo.SupersedeDictionaryEntries(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertDictionaryEntries(ctx, w.org, insert)
}

// admits tells whether a fact may become a version: the same revision with the same content is already
// one, and the same revision with another content replaces it only by a conflict resolution; otherwise
// it is a newer schema of the same fact (TZ 12.6).
func (w *writer) admits(known []entity.Revision, r entity.Revision, recordNo int) bool {
	for _, k := range known {
		if k.Source == r.Source && (bytes.Equal(k.Hash, r.Hash) || recordNo != w.resolved) {
			return false
		}
	}
	return true
}

func (w *writer) account(native string) *entity.ID {
	id, ok := w.accounts[native]
	if native == "" || !ok {
		return nil
	}
	return &id
}

// changes collects what a transaction changed in each area, with the earliest event time to recount from.
type changes map[entity.ID]*entity.AreaChange

func (c changes) add(area entity.ID, kind string, at *time.Time) {
	ch, ok := c[area]
	if !ok {
		ch = &entity.AreaChange{AreaID: area}
		c[area] = ch
	}
	if !slices.Contains(ch.FactKinds, kind) {
		ch.FactKinds = append(ch.FactKinds, kind)
		slices.Sort(ch.FactKinds)
	}
	if at != nil && (ch.EarliestEventTime == nil || at.Before(*ch.EarliestEventTime)) {
		t := *at
		ch.EarliestEventTime = &t
	}
}

func (c changes) list() []entity.AreaChange {
	out := make([]entity.AreaChange, 0, len(c))
	for _, id := range slices.SortedFunc(maps.Keys(c), func(a, b entity.ID) int { return bytes.Compare(a[:], b[:]) }) {
		out = append(out, *c[id])
	}
	return out
}

func sortedAccounts(accounts map[string]entity.SourceAccount) []entity.SourceAccount {
	out := make([]entity.SourceAccount, 0, len(accounts))
	for _, native := range slices.Sorted(maps.Keys(accounts)) {
		out = append(out, accounts[native])
	}
	return out
}
