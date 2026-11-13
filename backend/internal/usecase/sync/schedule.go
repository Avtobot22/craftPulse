package sync

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _dueAfter starts an increment of an idle data kind; with the hourly tick the next collection begins no later
// than 24 h after the previous one (NFR-005). Each round ends with a reconciliation.
const _dueAfter = 23 * time.Hour

// Tick is the periodic job: it queues a plan for every organization, which runs under that organization
// (TZ 10.3, rule 4). The registry of organizations is the one table it reads outside row-level security.
func (u *UseCase) Tick(ctx context.Context) error {
	var orgs []entity.ID
	err := u.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		orgs, err = u.d.Tenants.ActiveTenants(ctx)
		return err
	})
	if err != nil {
		return err
	}
	for _, org := range orgs {
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			return u.d.Queue.Enqueue(ctx, entity.SyncScheduleJob{OrganizationID: org}, entity.QueueBulk)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Schedule starts the increment of every idle data kind of an active connection whose last attempt is
// older than _dueAfter, and reports how fresh each data kind is.
func (u *UseCase) Schedule(ctx context.Context, orgID entity.ID) error {
	now := u.d.Clock.Now()
	var states []entity.SyncState
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		states, err = u.d.Repo.States(ctx, orgID, entity.ID{})
		return err
	})
	if err != nil {
		return err
	}
	active := map[entity.ID]bool{}
	for i := range states {
		st := states[i]
		u.freshness(st, now)
		ok, seen := active[st.ConnectionID]
		if !seen {
			cs, err := u.d.Connect.State(ctx, orgID, st.ConnectionID)
			if err != nil {
				return err
			}
			ok = cs.Connection.Status == entity.ConnectionActive && !cs.Connection.SyncPaused
			active[st.ConnectionID] = ok
		}
		if !ok || st.Phase == entity.SyncStopped || (st.LastAttemptAt != nil && now.Sub(*st.LastAttemptAt) < _dueAfter) {
			continue
		}
		if err := u.startDue(ctx, orgID, st); err != nil {
			return err
		}
	}
	return nil
}

// startDue queues the increment of a data kind under the organization when no attempt of it is active.
func (u *UseCase) startDue(ctx context.Context, orgID entity.ID, st entity.SyncState) error {
	return u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		busy, err := u.busy(ctx, orgID, st.ConnectionID, st, true)
		if err != nil || busy {
			return err
		}
		job := entity.SyncPageJob{OrganizationID: orgID, ConnectionID: st.ConnectionID, DataKind: st.DataKind}
		return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
	})
}

func (u *UseCase) freshness(st entity.SyncState, now time.Time) {
	var since, lag *time.Duration
	if st.LastSuccessAt != nil {
		d := now.Sub(*st.LastSuccessAt)
		since = &d
	}
	if st.Watermark != nil {
		d := now.Sub(*st.Watermark)
		lag = &d
	}
	u.d.Metrics.Freshness(st.ConnectionID.String(), st.DataKind, since, lag)
}
