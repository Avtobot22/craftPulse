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
	receipt       entity.ID
	instance      entity.ID
	provider      string
	area          entity.SourceKey
	truncated     map[string]bool
	accounts      map[string]entity.SourceAccount
	tasks         []*taskCandidate
	events        []*eventCandidate
	entries       []*entryCandidate
	repositories  []*repositoryCandidate
	mrs           []*mrCandidate
	mrEvents      []*mrEventCandidate
	mrRevisions   []*mrRevisionCandidate
	mrDiffs       []mrDiffCandidate
	ciRuns        []*ciRunCandidate
	ciAttempts    []*ciAttemptCandidate
	reports       []*reportCandidate
	analysis      []analysisCandidate
	importProject *entity.ID
	manifests     []*manifestCandidate
	// buildAttempts are the attempts that stand for a whole Jenkins build of the part without its manifest.
	buildAttempts map[entity.SourceKey]*ciAttemptCandidate
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
func prepare(rc entity.Receipt, pkg *envelope.Package, part []entity.IngestRecord, via entity.ObservedVia, first bool) (*input, error) {
	m := pkg.Manifest
	in := &input{
		receipt:   rc.ID,
		instance:  *rc.SourceInstanceID,
		provider:  string(m.Source.Provider),
		area:      entity.SourceKey{InstanceID: *rc.SourceInstanceID, Kind: entity.ObjectKind(m.Area.Kind), NativeID: m.Area.NativeId},
		truncated: truncatedRuns(m.Selection),
		accounts:  map[string]entity.SourceAccount{},
	}
	if rc.Channel == entity.ChannelImport && rc.ProjectID != nil &&
		m.Area.Kind == envelope.AreaRefKindAnalysisProject && m.Area.NativeId == rc.ProjectID.String() {
		in.importProject = rc.ProjectID
	}
	taskNS := ""
	if m.Source.Provider == _providerUniversal {
		taskNS = m.Area.NativeId
	}
	tasks := map[entity.SourceKey]int{}
	events := map[[2]string]int{}
	entries := map[entity.DictionaryKey]int{}
	repositories := map[entity.SourceKey]int{}
	mrs := map[entity.SourceKey]int{}
	mrEvents := map[entity.SourceKey]int{}
	mrRevisions := map[entity.SourceKey]int{}
	ciRuns := map[entity.SourceKey]int{}
	ciAttempts := map[entity.SourceKey]int{}
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
		case kind == "repository":
			c, err := in.repository(rec, base)
			if err != nil {
				return nil, err
			}
			keep(&in.repositories, repositories, c.key, c, func(old *repositoryCandidate) bool {
				return !olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, old.version.SourceUpdatedAt, old.version.ObservedAt, c.snapshot)
			})
		case kind == "merge_request":
			c, err := in.mergeRequest(rec, base)
			if err != nil {
				return nil, err
			}
			keep(&in.mrs, mrs, c.key, c, func(old *mrCandidate) bool {
				return !olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, old.version.SourceUpdatedAt, old.version.ObservedAt, c.snapshot)
			})
		case kind == "mr_event":
			c, err := in.mrEvent(rec, base, via)
			if err != nil {
				return nil, err
			}
			if c == nil {
				continue
			}
			approval := in.githubReview(c)
			for _, e := range []*mrEventCandidate{c, approval} {
				if e != nil {
					keep(&in.mrEvents, mrEvents, entity.SourceKey{NativeNS: e.mr.NativeID, NativeID: e.event.EventKey}, e, func(*mrEventCandidate) bool { return true })
				}
			}
		case kind == "mr_revision":
			c, err := in.mrRevision(rec, base)
			if err != nil {
				return nil, err
			}
			keep(&in.mrRevisions, mrRevisions, entity.SourceKey{NativeNS: c.mr.NativeID, NativeID: c.revision.RevisionKey}, c, func(*mrRevisionCandidate) bool { return true })
		case kind == "mr_diff":
			c, err := in.mrDiff(rec, base)
			if err != nil {
				return nil, err
			}
			in.mrDiffs = append(in.mrDiffs, c)
		case kind == "ci_run":
			c, err := in.ciRun(rec, base, entity.ObjectKind(m.Area.Kind))
			if err != nil {
				return nil, err
			}
			keep(&in.ciRuns, ciRuns, c.key, c, func(old *ciRunCandidate) bool {
				return !olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, old.version.SourceUpdatedAt, old.version.ObservedAt, false)
			})
		case kind == "ci_attempt":
			c, err := in.ciAttempt(rec, base)
			if err != nil {
				return nil, err
			}
			keep(&in.ciAttempts, ciAttempts, c.key, c, func(old *ciAttemptCandidate) bool { return !olderAttempt(c.version, old.version) })
		case kind == _reportFileRecord:
			c, err := in.reportFile(rec, base, pkg.ScanManifest)
			if err != nil {
				return nil, err
			}
			in.reports = append(in.reports, c)
		case kind == _recordExecutionManifest:
			c, err := in.manifestRecord(rec, base)
			if err != nil {
				return nil, err
			}
			in.manifests = append(in.manifests, c)
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
	if err := in.jenkinsBuilds(rc, pkg, part, first); err != nil {
		return nil, err
	}
	return in, nil
}

func (in *input) hasImportedReport() bool {
	return in.importProject != nil && slices.ContainsFunc(in.reports, func(c *reportCandidate) bool {
		return c.parser != nil && c.rejected == ""
	})
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
