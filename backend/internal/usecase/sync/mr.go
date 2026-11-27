package sync

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) RunMR(ctx context.Context, job entity.SyncMRJob, holder string, complete Complete) error {
	org, conn := job.OrganizationID, job.ConnectionID
	if err := u.lease(ctx, org, conn, holder); err != nil {
		return err
	}
	scopes, err := u.d.Connect.Scopes(ctx, org, conn)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(scopes, func(s entity.ConnectionScope) bool { return s.Active && s.ID == job.ScopeID })
	if i < 0 {
		return u.finish(ctx, org, conn, holder, complete)
	}
	if job.SchemaID == "" {
		// The GitLab merge_requests page includes the current revision's diff for an open MR.
		job.SchemaID = "merge_requests"
	}
	in := entity.FetchInput{
		SchemaID: job.SchemaID, Scope: entity.ScopeRef{Kind: scopes[i].Kind, NativeScopeID: scopes[i].NativeScopeID},
		Cursor: job.Cursor, Limit: u.d.PageSize, ObjectKeys: []entity.ObjectKey{{ObjectKind: "merge_request", NativeNS: job.NativeNS, NativeID: job.NativeID}},
	}
	page, sel, f, err := u.fetch(ctx, org, conn, job.ScopeID, holder, in)
	if err != nil {
		return err
	}
	at := u.d.Clock.Now()
	next, more := job, false
	switch {
	case f != nil:
		u.d.Metrics.CollectionFailed(f.class)
		if !f.stop {
			more, next.Retry, at = true, job.Retry+1, f.at
			if at.IsZero() {
				at = u.d.Clock.Now().Add(Pause(job.Retry, u.d.Rand()))
			}
		}
	case !sel.End:
		next.Cursor, next.Retry, more = sourceCursor(sel.NextCursor), 0, true
	case job.SchemaID == "merge_requests":
		next.SchemaID, next.Cursor, next.Retry, more = "ci_runs", nil, 0, true
	default:
		atScope := slices.Index(job.ScopeIDs, job.ScopeID)
		if atScope >= 0 && atScope+1 < len(job.ScopeIDs) {
			next.ScopeID, next.SchemaID, next.Cursor, next.Retry, more = job.ScopeIDs[atScope+1], "merge_requests", nil, 0, true
		}
	}
	return u.withPageIntake(ctx, org, func(ctx context.Context, record pageRecorder) error {
		if page != nil {
			page.ObservedVia = entity.ViaWebhook
			if _, err := record(page); err != nil {
				return err
			}
		}
		if err := u.end(ctx, org, conn, holder, complete); err != nil {
			return err
		}
		if f == nil && job.SchemaID == "merge_requests" {
			scope := step{scope: scopes[i], schema: job.SchemaID}
			for _, history := range histories(entity.SyncPageJob{OrganizationID: org, ConnectionID: conn}, scope, sel) {
				if err := u.d.Queue.Enqueue(ctx, history, entity.QueueBulk); err != nil {
					return err
				}
			}
		}
		if more {
			return u.d.Queue.EnqueueAt(ctx, next, entity.QueueInteractive, at)
		}
		return nil
	})
}
