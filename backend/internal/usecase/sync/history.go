package sync

import (
	"context"
	"slices"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// RunHistory reads on the cut history of one object under the lease of its connection, page by page,
// until the source reports the end; failures repeat it like a page of collection.
func (u *UseCase) RunHistory(ctx context.Context, job entity.SyncHistoryJob, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.lease(ctx, org, conn, holder); err != nil {
		return err
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(scopes, func(s entity.ConnectionScope) bool { return s.ID == job.ScopeID && s.Active })
	if i < 0 {
		return u.finish(ctx, org, conn, holder, complete)
	}
	in := entity.FetchInput{
		SchemaID: job.SchemaID, Scope: entity.ScopeRef{Kind: scopes[i].Kind, NativeScopeID: scopes[i].NativeScopeID},
		Cursor: job.Cursor, Limit: u.d.PageSize,
		ObjectKeys: []entity.ObjectKey{{ObjectKind: job.ObjectKind, NativeNS: job.NativeNS, NativeID: job.NativeID}},
	}
	page, sel, f, err := u.fetch(ctx, org, conn, job.ScopeID, in)
	if err != nil {
		return err
	}
	if page != nil {
		page.ObservedVia = entity.ViaBackfill
	}
	var next []entity.Job
	at := u.d.Clock.Now()
	switch {
	case f != nil:
		retry := job
		if f.backoff {
			retry.Retry++
			f.at = at.Add(Pause(job.Retry, u.d.Rand()))
		}
		u.d.Metrics.CollectionFailed(f.class)
		zerolog.Ctx(ctx).Warn().Str("connection_id", conn.String()).Str("operation", "fetch").Str("object", job.NativeNS+"/"+job.NativeID).
			Str("reason", f.code).Int("retry", job.Retry).Bool("stopped", f.stop).Str("correlation_id", job.CorrelationID.String()).
			Msg("history of an object failed")
		if !f.stop {
			at, next = f.at, []entity.Job{retry}
		}
	case !sel.End:
		more := job
		more.Retry = 0
		more.Cursor = sourceCursor(sel.NextCursor)
		next = []entity.Job{more}
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if page != nil {
			if _, err := u.d.Ingest.RecordPage(ctx, page); err != nil {
				return err
			}
		}
		for _, j := range next {
			if err := u.d.Queue.EnqueueAt(ctx, j, entity.QueueBulk, at); err != nil {
				return err
			}
		}
		return u.end(ctx, org, conn, holder, complete)
	})
}
