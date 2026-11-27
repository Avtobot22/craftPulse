package sync

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_manifestPage = 500
	// _compareBatch bounds the native IDs of one read of current versions from normalize.
	_compareBatch = 500
	// _dirtyDelay lets keys of one reconciliation and of webhooks gather before they are read (TZ 10.6).
	_dirtyDelay = 10 * time.Second
	// _recheckAfter queues the next reconciliation after the objects found are read again and normalized.
	_recheckAfter = time.Minute
	// _maxRounds stops reconciliations in a row that keep finding the same differences, e.g. an object the
	// source lists but never returns.
	_maxRounds = 3
)

// _manifestSchemas are the schemas whose objects a reconciliation compares; normalize keeps their versions.
var _manifestSchemas = map[string]string{"tasks": "tasks", "merge_requests": "merge_requests", "ci": "ci_runs", "documents": "documents"}

// queueReconcile writes a running reconciliation of a data kind and queues its job in the caller's
// transaction; one already running absorbs the request.
func (u *UseCase) queueReconcile(ctx context.Context, org, conn entity.ID, dataKind string, corr entity.ID, round int, at time.Time) error {
	id, started, err := u.d.Repo.StartReconcile(ctx, org, conn, dataKind, u.d.Clock.Now())
	if err != nil || !started {
		return err
	}
	job := entity.SyncReconcileJob{OrganizationID: org, ConnectionID: conn, DataKind: dataKind, RunID: id, Round: round, CorrelationID: corr}
	return u.d.Queue.EnqueueAt(ctx, job, entity.QueueBulk, at)
}

// tally is what a reconciliation found.
type tally struct {
	expected, observed, missing, changed int
	dirty                                []entity.DirtyKey
	checks                               []entity.ActivityHistoryCheck
	wiki                                 []entity.WikiReconcileScope
	wikiSeen                             map[entity.ObjectKey]bool
}

// RunReconcile compares the identities and versions of the manifest of the source with the current versions
// of normalize (TZ 12.6, step 3) under the lease of the connection. Without differences the data kind gets
// its watermark, last success and completeness; otherwise the objects go to the set to read again, and
// the next reconciliation checks them.
func (u *UseCase) RunReconcile(ctx context.Context, job entity.SyncReconcileJob, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.lease(ctx, org, conn, holder); err != nil {
		return err
	}
	var run entity.ReconcileRun
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var (
			found bool
			err   error
		)
		run, found, err = u.d.Repo.Reconcile(ctx, org, job.RunID)
		if err == nil && (!found || run.Outcome != entity.ReconcileRunning) {
			run.Outcome = ""
		}
		return err
	})
	if err != nil {
		return err
	}
	if run.Outcome == "" {
		return u.finish(ctx, org, conn, holder, complete)
	}
	run.AsOf = u.d.Clock.Now()
	run.StartedAt = run.AsOf
	schema, ok := _manifestSchemas[job.DataKind]
	if !ok {
		return u.closeRun(ctx, job, run, nil, &failure{code: "not_supported", stop: true}, holder, complete)
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	t := &tally{}
	for i := range scopes {
		if !scopes[i].Active || (job.DataKind == "documents" && scopes[i].Kind != entity.ScopeWikiSpace) {
			continue
		}
		scope := &scopes[i]
		expected, read := t.expected, t.observed-t.changed
		if job.DataKind == "documents" {
			t.wikiSeen = make(map[entity.ObjectKey]bool)
		}
		f, err := u.compare(ctx, org, conn, scope, job.DataKind, schema, t)
		if err != nil {
			return u.releaseBeforeSnooze(ctx, org, conn, holder, err)
		}
		if f != nil {
			var failed *tally
			if job.DataKind == "documents" {
				failed = &tally{}
				for j := range scopes {
					if scopes[j].Active && scopes[j].Kind == entity.ScopeWikiSpace {
						failed.wiki = append(failed.wiki, entity.WikiReconcileScope{ScopeID: scopes[j].ID, Failed: true})
					}
				}
			}
			return u.closeRun(ctx, job, run, failed, f, holder, complete)
		}
		if job.DataKind == "documents" {
			t.wiki = append(t.wiki, entity.WikiReconcileScope{ScopeID: scope.ID, Expected: t.expected - expected, Read: t.observed - t.changed - read})
		}
	}
	if job.DataKind == "documents" && len(t.wiki) == 0 {
		return u.closeRun(ctx, job, run, nil, &failure{code: "not_supported", stop: true}, holder, complete)
	}
	return u.closeRun(ctx, job, run, t, nil, holder, complete)
}

// compare reads the manifest of one scope page by page and compares it with the current versions of its area.
func (u *UseCase) compare(ctx context.Context, org, conn entity.ID, scope *entity.ConnectionScope, dataKind, schema string, t *tally) (*failure, error) {
	area, err := u.d.Normalize.AreaOf(ctx, org, scope.InstanceID, entity.ObjectKind(scope.Kind), scope.NativeScopeID)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return nil, err
	}
	// The first reconciliation that finds the area binds it to the scope: access checks the area in this
	// connection, and listAreas offers it for projects.
	if !area.IsZero() && scope.AreaID == nil {
		if err := u.d.Connect.SetScopeArea(ctx, org, scope.ID, area); err != nil {
			return nil, err
		}
	}
	in := entity.ManifestInput{
		SchemaID: schema, Scope: entity.ScopeRef{Kind: scope.Kind, NativeScopeID: scope.NativeScopeID},
		Period: entity.Period{From: scope.HistoryFrom}, Limit: _manifestPage,
	}
	if dataKind == "documents" {
		in.Period = entity.Period{} // The corpus includes pages last edited before a history cutoff.
	}
	var total *int
	for {
		page, err := u.d.Connect.Manifest(ctx, org, conn, in)
		if err != nil {
			if f, ok := u.classify(err); ok {
				return f, nil
			}
			return nil, err
		}
		if dataKind == "documents" && page.TotalCount != nil {
			if total != nil && *total != *page.TotalCount {
				return &failure{code: "invalid_response", stop: true}, nil
			}
			total = page.TotalCount
		}
		for start := 0; start < len(page.Items); start += _compareBatch {
			batch := page.Items[start:min(start+_compareBatch, len(page.Items))]
			if err := u.compareBatch(ctx, org, area, scope.ID, dataKind, batch, t); err != nil {
				return nil, err
			}
		}
		if page.NextCursor == "" {
			if total != nil {
				missing := *total - len(t.wikiSeen)
				if missing < 0 {
					return &failure{code: "invalid_response", stop: true}, nil
				}
				t.expected += missing
				t.missing += missing
			}
			return nil, nil
		}
		in.Cursor = page.NextCursor
	}
}

// compareBatch counts the objects of a page of the manifest. A version of the source is compared with
// source_revision; without one, its time of change with source_updated_at, since the revision is a hash.
func (u *UseCase) compareBatch(ctx context.Context, org, area, scope entity.ID, dataKind string, items []entity.ManifestItem, t *tally) error {
	current := map[entity.ObjectKey]entity.SyncObjectVersion{}
	if !area.IsZero() {
		ids := make([]string, 0, len(items))
		for i := range items {
			ids = append(ids, items[i].NativeID)
		}
		if dataKind == "tasks" {
			tasks, err := u.d.Normalize.CurrentTasks(ctx, org, area, ids)
			if err != nil {
				return err
			}
			for _, item := range items {
				if v, ok := tasks[item.NativeID]; ok {
					current[entity.ObjectKey{ObjectKind: item.RecordKind, NativeNS: item.NativeNS, NativeID: item.NativeID}] = entity.SyncObjectVersion{Revision: v.Revision.Source, UpdatedAt: v.SourceUpdatedAt}
				}
			}
		} else {
			var err error
			if dataKind == "documents" {
				current, err = u.d.Normalize.CurrentWikiVersions(ctx, org, area, ids)
			} else {
				current, err = u.d.Normalize.CurrentSyncVersions(ctx, org, area, ids)
			}
			if err != nil {
				return err
			}
		}
	}
	for i := range items {
		it := &items[i]
		key := entity.ObjectKey{ObjectKind: it.RecordKind, NativeNS: it.NativeNS, NativeID: it.NativeID}
		if dataKind == "documents" {
			if key.ObjectKind != string(entity.ObjectDocument) || t.wikiSeen[key] {
				continue
			}
			t.wikiSeen[key] = true
		}
		t.expected++
		cur, ok := current[key]
		matches := ok && (it.Version != "" && it.Version == cur.Revision ||
			it.Version == "" && (it.UpdatedAt == nil || cur.UpdatedAt != nil && it.UpdatedAt.Equal(*cur.UpdatedAt)))
		if key.ObjectKind == "task" || key.ObjectKind == "merge_request" {
			t.checks = append(t.checks, entity.ActivityHistoryCheck{ScopeID: scope, Key: key, Matches: matches})
		}
		switch {
		case !ok:
			t.missing++
		case !matches:
			t.observed++
			t.changed++
		default:
			t.observed++
			continue
		}
		t.dirty = append(t.dirty, entity.DirtyKey{
			ScopeID: scope, DataKind: dataKind,
			Object: entity.ObjectKey{ObjectKind: it.RecordKind, NativeNS: it.NativeNS, NativeID: it.NativeID},
		})
	}
	return nil
}

// closeRun records the result of a reconciliation with the state of the data kind in one transaction. A
// refused call of the adapter repeats the reconciliation later, unless it stops the collection.
func (u *UseCase) closeRun(ctx context.Context, job entity.SyncReconcileJob, run entity.ReconcileRun, t *tally, f *failure, holder string,
	complete Complete,
) error {
	org, conn := job.OrganizationID, job.ConnectionID
	now := u.d.Clock.Now()
	run.FinishedAt = &now
	if f != nil && !f.stop {
		at := f.at
		if at.IsZero() {
			at = now.Add(Pause(0, u.d.Rand()))
		}
		u.d.Metrics.CollectionFailed(f.class)
		return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			if err := u.d.Queue.EnqueueAt(ctx, job, entity.QueueBulk, at); err != nil {
				return err
			}
			return u.end(ctx, org, conn, holder, complete)
		})
	}
	areas, err := u.reconciledAreas(ctx, org, conn, job.DataKind)
	if err != nil {
		return err
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		st, found, err := u.d.Repo.State(ctx, org, conn, job.DataKind)
		if err != nil {
			return err
		}
		if !found {
			st = entity.SyncState{ConnectionID: conn, DataKind: job.DataKind, Phase: entity.SyncIncremental, Completeness: completeUnknown}
		}
		recheck := false
		switch {
		case f != nil:
			run.Outcome, run.ReasonCode = entity.ReconcileFailed, f.code
		case t.missing+t.changed == 0:
			run.Outcome = entity.ReconcileComplete
			st.Watermark, st.LastSuccessAt, st.Completeness = &run.AsOf, &now, completeComplete
		default:
			run.Outcome, st.Completeness, recheck = entity.ReconcileIncomplete, completePartial, true
		}
		if f == nil {
			run.ExpectedCount, run.ObservedCount, run.MissingCount, run.ChangedCount = &t.expected, &t.observed, &t.missing, &t.changed
			if err := u.d.Repo.SaveActivityChecks(ctx, org, run.ID, t.checks); err != nil {
				return err
			}
		}
		if err := u.d.Repo.FinishReconcile(ctx, org, run); err != nil {
			return err
		}
		if t != nil && len(t.wiki) > 0 {
			seq, err := u.d.Tenants.NextSeq(ctx, org)
			if err != nil {
				return err
			}
			if err := u.d.Repo.SaveWikiReconcile(ctx, org, run.ID, seq, t.wiki); err != nil {
				return err
			}
		}
		if err := u.d.Repo.SaveState(ctx, org, st); err != nil {
			return err
		}
		if recheck {
			if err := u.recheck(ctx, job, t, now); err != nil {
				return err
			}
		}
		for _, area := range areas {
			recount := entity.CalcRecountJob{OrganizationID: org, AreaID: area, CorrelationID: job.CorrelationID}
			if err := u.d.Queue.Enqueue(ctx, recount, entity.QueueBulk); err != nil {
				return err
			}
		}
		if err := u.recountActivity(ctx, org, conn, job.CorrelationID); err != nil {
			return err
		}
		zerolog.Ctx(ctx).Info().Str("connection_id", conn.String()).Str("data_kind", job.DataKind).Str("outcome", run.Outcome).
			Str("reason", run.ReasonCode).Str("correlation_id", job.CorrelationID.String()).Msg("reconciliation finished")
		return u.end(ctx, org, conn, holder, complete)
	})
}

// reconciledAreas are the areas whose completeness of tasks a finished reconciliation decides: calc recounts their
// series, since the completeness of a collection is an input of T39-T42 (TZ 10.3, rule 6).
func (u *UseCase) reconciledAreas(ctx context.Context, org, conn entity.ID, dataKind string) ([]entity.ID, error) {
	if dataKind != _dataTasks && dataKind != "ci" && dataKind != "merge_requests" && dataKind != "documents" {
		return nil, nil
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return nil, err
	}
	var out []entity.ID
	for i := range scopes {
		if a := scopes[i].AreaID; scopes[i].Active && (dataKind != "documents" || scopes[i].Kind == entity.ScopeWikiSpace) && a != nil && !slices.Contains(out, *a) {
			out = append(out, *a)
		}
	}
	return out, nil
}

// recheck puts the differences into the set to read again and, after the finished run, queues the next
// reconciliation, up to _maxRounds in a row.
func (u *UseCase) recheck(ctx context.Context, job entity.SyncReconcileJob, t *tally, now time.Time) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.d.Repo.AddDirtyKeys(ctx, org, conn, t.dirty, now); err != nil {
		return err
	}
	dirty := entity.SyncDirtyJob{OrganizationID: org, ConnectionID: conn}
	if err := u.d.Queue.EnqueueAt(ctx, dirty, entity.QueueBulk, now.Add(_dirtyDelay)); err != nil {
		return err
	}
	if job.Round+1 >= _maxRounds {
		return nil
	}
	return u.queueReconcile(ctx, org, conn, job.DataKind, job.CorrelationID, job.Round+1, now.Add(_recheckAfter))
}
