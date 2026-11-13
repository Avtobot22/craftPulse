package sync

import (
	"context"
	"errors"
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
var _manifestSchemas = map[string]string{"tasks": "tasks"}

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
		if !scopes[i].Active {
			continue
		}
		f, err := u.compare(ctx, org, conn, &scopes[i], job.DataKind, schema, t)
		if err != nil {
			return err
		}
		if f != nil {
			return u.closeRun(ctx, job, run, nil, f, holder, complete)
		}
	}
	return u.closeRun(ctx, job, run, t, nil, holder, complete)
}

// compare reads the manifest of one scope page by page and compares it with the current versions of its area.
func (u *UseCase) compare(ctx context.Context, org, conn entity.ID, scope *entity.ConnectionScope, dataKind, schema string, t *tally) (*failure, error) {
	area, err := u.d.Normalize.AreaOf(ctx, org, scope.InstanceID, entity.ObjectKind(scope.Kind), scope.NativeScopeID)
	if err != nil && !errors.Is(err, entity.ErrNotFound) {
		return nil, err
	}
	in := entity.ManifestInput{
		SchemaID: schema, Scope: entity.ScopeRef{Kind: scope.Kind, NativeScopeID: scope.NativeScopeID},
		Period: entity.Period{From: scope.HistoryFrom}, Limit: _manifestPage,
	}
	for {
		page, err := u.d.Connect.Manifest(ctx, org, conn, in)
		if err != nil {
			if f, ok := u.classify(err); ok {
				return f, nil
			}
			return nil, err
		}
		for start := 0; start < len(page.Items); start += _compareBatch {
			batch := page.Items[start:min(start+_compareBatch, len(page.Items))]
			if err := u.compareBatch(ctx, org, area, scope.ID, dataKind, batch, t); err != nil {
				return nil, err
			}
		}
		if page.NextCursor == "" {
			return nil, nil
		}
		in.Cursor = page.NextCursor
	}
}

// compareBatch counts the objects of a page of the manifest. A version of the source is compared with
// source_revision; without one, its time of change with source_updated_at, since the revision is a hash.
func (u *UseCase) compareBatch(ctx context.Context, org, area, scope entity.ID, dataKind string, items []entity.ManifestItem, t *tally) error {
	current := map[string]entity.TaskVersion{}
	if !area.IsZero() {
		ids := make([]string, 0, len(items))
		for i := range items {
			ids = append(ids, items[i].NativeID)
		}
		var err error
		if current, err = u.d.Normalize.CurrentTasks(ctx, org, area, ids); err != nil {
			return err
		}
	}
	for i := range items {
		it := &items[i]
		t.expected++
		cur, ok := current[it.NativeID]
		switch {
		case !ok:
			t.missing++
		case it.Version != "" && it.Version != cur.Revision.Source,
			it.Version == "" && it.UpdatedAt != nil && (cur.SourceUpdatedAt == nil || !it.UpdatedAt.Equal(*cur.SourceUpdatedAt)):
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
		if t != nil {
			run.ExpectedCount, run.ObservedCount, run.MissingCount, run.ChangedCount = &t.expected, &t.observed, &t.missing, &t.changed
		}
		if err := u.d.Repo.FinishReconcile(ctx, org, run); err != nil {
			return err
		}
		if err := u.d.Repo.SaveState(ctx, org, st); err != nil {
			return err
		}
		if recheck {
			if err := u.recheck(ctx, job, t, now); err != nil {
				return err
			}
		}
		zerolog.Ctx(ctx).Info().Str("connection_id", conn.String()).Str("data_kind", job.DataKind).Str("outcome", run.Outcome).
			Str("reason", run.ReasonCode).Str("correlation_id", job.CorrelationID.String()).Msg("reconciliation finished")
		return u.end(ctx, org, conn, holder, complete)
	})
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
