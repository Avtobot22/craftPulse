package sync

import (
	"context"
	"slices"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _dirtyBatch bounds the keys one run reads; more keys queue the next run.
const _dirtyBatch = 200

// dirtyGroup is the keys of one scope read with one data kind.
type dirtyGroup struct {
	scope    *entity.ConnectionScope
	dataKind string
	keys     []entity.DirtyKey
}

// RunDirty reads again the objects in the set of a connection under its lease, with every schema of their
// data kind, so a task comes with its history. The pages and the deletion of the keys read commit together;
// a key added after the read started stays for the next run (TZ 12.5.3).
func (u *UseCase) RunDirty(ctx context.Context, job entity.SyncDirtyJob, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.lease(ctx, org, conn, holder); err != nil {
		return err
	}
	started := u.d.Clock.Now()
	var keys []entity.DirtyKey
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		keys, err = u.d.Repo.DirtyKeys(ctx, org, conn, _dirtyBatch)
		return err
	})
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return u.finish(ctx, org, conn, holder, complete)
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	var pages []*entity.PreparedPackage
	for _, g := range groups(keys, scopes) {
		read, f, err := u.readGroup(ctx, org, conn, g)
		if err != nil {
			return err
		}
		if f != nil {
			return u.failDirty(ctx, job, f, holder, complete)
		}
		pages = append(pages, read...)
	}
	hashes := make([][]byte, len(keys))
	for i := range keys {
		hashes[i] = keys[i].Hash
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		for _, p := range pages {
			if _, err := u.d.Ingest.RecordPage(ctx, p); err != nil {
				return err
			}
		}
		if err := u.d.Repo.DeleteDirtyKeys(ctx, org, conn, hashes, started); err != nil {
			return err
		}
		if err := u.end(ctx, org, conn, holder, complete); err != nil {
			return err
		}
		if len(keys) < _dirtyBatch {
			return nil
		}
		// The next run is queued after this one is completed: an equal job is unique only among active ones.
		return u.d.Queue.Enqueue(ctx, entity.SyncDirtyJob{OrganizationID: org, ConnectionID: conn}, entity.QueueBulk)
	})
}

// groups puts the keys by scope and data kind; a key of a scope no longer collected goes alone and is
// deleted without a read.
func groups(keys []entity.DirtyKey, scopes []entity.ConnectionScope) []dirtyGroup {
	var out []dirtyGroup
	for i := range keys {
		k := keys[i]
		j := slices.IndexFunc(out, func(g dirtyGroup) bool { return g.scope != nil && g.scope.ID == k.ScopeID && g.dataKind == k.DataKind })
		if j >= 0 {
			out[j].keys = append(out[j].keys, k)
			continue
		}
		s := slices.IndexFunc(scopes, func(s entity.ConnectionScope) bool { return s.ID == k.ScopeID && s.Active })
		if s < 0 {
			continue
		}
		out = append(out, dirtyGroup{scope: &scopes[s], dataKind: k.DataKind, keys: []entity.DirtyKey{k}})
	}
	return out
}

func (u *UseCase) readGroup(ctx context.Context, org, conn entity.ID, g dirtyGroup) ([]*entity.PreparedPackage, *failure, error) {
	var pages []*entity.PreparedPackage
	for _, schema := range _schemas[g.dataKind] {
		for start := 0; start < len(g.keys); start += u.d.PageSize {
			batch := g.keys[start:min(start+u.d.PageSize, len(g.keys))]
			objects := make([]entity.ObjectKey, len(batch))
			for i := range batch {
				objects[i] = batch[i].Object
			}
			in := entity.FetchInput{
				SchemaID: schema, Scope: entity.ScopeRef{Kind: g.scope.Kind, NativeScopeID: g.scope.NativeScopeID},
				Limit: u.d.PageSize, ObjectKeys: objects,
			}
			page, _, f, err := u.fetch(ctx, org, conn, g.scope.ID, in)
			if err != nil || f != nil {
				return nil, f, err
			}
			page.ObservedVia = entity.ViaReconcile
			pages = append(pages, page)
		}
	}
	return pages, nil, nil
}

// failDirty keeps the keys and repeats the read later; auth and an unsupported adapter leave it to the
// next reconciliation.
func (u *UseCase) failDirty(ctx context.Context, job entity.SyncDirtyJob, f *failure, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	u.d.Metrics.CollectionFailed(f.class)
	zerolog.Ctx(ctx).Warn().Str("connection_id", conn.String()).Str("operation", "fetch").Str("reason", f.code).Int("retry", job.Retry).
		Bool("stopped", f.stop).Msg("read of the set of a connection failed")
	retry := job
	retry.Retry++
	at := f.at
	if at.IsZero() {
		at = u.d.Clock.Now().Add(Pause(job.Retry, u.d.Rand()))
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if !f.stop {
			if err := u.d.Queue.EnqueueAt(ctx, retry, entity.QueueBulk, at); err != nil {
				return err
			}
		}
		return u.end(ctx, org, conn, holder, complete)
	})
}
