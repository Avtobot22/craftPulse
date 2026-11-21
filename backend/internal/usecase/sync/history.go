package sync

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func completeHistoryPage(p *entity.PreparedPackage) bool {
	var m envelope.Manifest
	return json.Unmarshal(p.Receipt.Manifest, &m) == nil && m.Completeness == envelope.ManifestCompletenessComplete &&
		!slices.ContainsFunc(p.Records, func(r entity.IngestRecord) bool { return r.Outcome == entity.RecordRejected })
}

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
	key := in.ObjectKeys[0]
	hasHistory := job.HasHistory || f == nil && slices.ContainsFunc(page.Records, func(r entity.IngestRecord) bool {
		return r.Kind == key.ObjectKind || key.ObjectKind == "task" && r.Kind == "task_event" ||
			key.ObjectKind == "merge_request" && r.Kind == "mr_event"
	})
	incomplete := job.Incomplete || job.Cursor != nil && len(job.PageReceiptIDs) == 0 || f == nil && (!completeHistoryPage(page) ||
		slices.ContainsFunc(sel.Truncated, func(t envelope.TruncatedHistory) bool {
			return string(t.RecordKind) == key.ObjectKind && t.NativeNs == key.NativeNS && t.NativeId == key.NativeID
		}))
	var next *entity.SyncHistoryJob
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
			at, next = f.at, &retry
		}
	case !sel.End:
		more := job
		more.Retry = 0
		more.Cursor = sourceCursor(sel.NextCursor)
		more.Incomplete = incomplete
		more.HasHistory = hasHistory
		next = &more
	}
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if page != nil {
			rc, err := u.d.Ingest.RecordPage(ctx, page)
			if err != nil {
				return err
			}
			if f == nil && (key.ObjectKind == "task" || key.ObjectKind == "merge_request") {
				pages := slices.Concat(job.PageReceiptIDs, []entity.ID{rc.ID})
				switch {
				case next != nil:
					next.PageReceiptIDs = pages
				case hasHistory && !incomplete:
					recovery := entity.ActivityHistoryRecovery{
						ActivityHistoryNotice: entity.ActivityHistoryNotice{ScopeID: job.ScopeID, Key: key, ReceiptID: rc.ID, ObservedAt: rc.ReceivedAt},
						PageReceiptIDs:        pages,
					}
					if err := u.saveHistoryRecovery(ctx, org, &recovery, scopes[i].AreaID, job.CorrelationID); err != nil {
						return err
					}
				}
			}
		}
		// The job ends first: a repeat after a pause has the same arguments, and an active equal job absorbs it.
		if err := u.end(ctx, org, conn, holder, complete); err != nil {
			return err
		}
		if next != nil {
			return u.d.Queue.EnqueueAt(ctx, *next, entity.QueueBulk, at)
		}
		return nil
	})
}

func (u *UseCase) saveHistoryRecovery(ctx context.Context, org entity.ID, recovery *entity.ActivityHistoryRecovery, area *entity.ID, corr entity.ID) error {
	complete, err := u.d.Ingest.ActivityCompleteReceipts(ctx, org, recovery.PageReceiptIDs)
	if err != nil || !historyReceiptsComplete(recovery.PageReceiptIDs, complete) {
		return err
	}
	if err := u.d.Repo.SaveActivityRecovery(ctx, org, recovery); err != nil {
		return err
	}
	if _, err := u.d.Tenants.NextSeq(ctx, org); err != nil {
		return err
	}
	if area != nil {
		return u.d.Queue.Enqueue(ctx, entity.CalcRecountJob{OrganizationID: org, AreaID: *area, CorrelationID: corr}, entity.QueueBulk)
	}
	return nil
}

func historyReceiptsComplete(ids []entity.ID, complete map[entity.ID]bool) bool {
	return len(ids) > 0 && !slices.ContainsFunc(ids, func(id entity.ID) bool { return !complete[id] })
}
