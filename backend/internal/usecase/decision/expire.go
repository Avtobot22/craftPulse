package decision

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

func nextWake(r *entity.ExceptionRecord, now time.Time) *time.Time {
	state := statuses(r, math.MaxInt64, now)
	var wake time.Time
	for i := range r.Versions {
		v := &r.Versions[i]
		kind := state[v.ID]
		if kind == entity.ExceptionRejected || kind == entity.ExceptionWithdrawn || !v.ExpiresAt.After(now) {
			continue
		}
		at := v.ExpiresAt
		if kind == entity.ExceptionApproved && v.EffectiveFrom.After(now) {
			at = v.EffectiveFrom
		}
		if wake.IsZero() || at.Before(wake) {
			wake = at
		}
	}
	if wake.IsZero() {
		return nil
	}
	return &wake
}

func (u *UseCase) onChange(ctx context.Context, org entity.ID, r *entity.ExceptionRecord, area entity.ID, now time.Time) error {
	job := entity.DecisionExpireJob{OrganizationID: org, DecisionID: r.ID, CorrelationID: entity.ID(logger.CorrelationID(ctx))}
	if err := u.recount(ctx, org, area, job.CorrelationID); err != nil {
		return err
	}
	return u.schedule(ctx, job, nextWake(r, now))
}

func (u *UseCase) schedule(ctx context.Context, job entity.DecisionExpireJob, at *time.Time) error {
	if err := u.d.Repo.SetWake(ctx, job.OrganizationID, job.DecisionID, at); err != nil {
		return err
	}
	if at == nil {
		return nil
	}
	return u.d.Queue.EnqueueNearest(ctx, job, entity.QueueBulk, *at)
}

func (u *UseCase) recount(ctx context.Context, org, area, corr entity.ID) error {
	if err := u.d.Queue.Enqueue(ctx, entity.CalcRecountJob{OrganizationID: org, AreaID: area, CorrelationID: corr}, entity.QueueBulk); err != nil {
		return err
	}
	cause, err := u.d.NewID()
	if err != nil {
		return err
	}
	return u.d.Queue.Enqueue(ctx, entity.ChangeRecountJob{OrganizationID: org, AreaID: area, CorrelationID: corr, CauseID: cause}, entity.QueueBulk)
}

// RunExpire consumes the scheduled boundary, queues both recalculations and schedules the next one atomically.
func (u *UseCase) RunExpire(ctx context.Context, job entity.DecisionExpireJob, complete func(context.Context) error) error {
	return u.d.Tx.InOrgTx(ctx, job.OrganizationID, func(ctx context.Context) error {
		r, err := u.d.Repo.Decision(ctx, job.OrganizationID, job.DecisionID, true)
		switch {
		case errors.Is(err, entity.ErrNotFound):
			return complete(ctx)
		case err != nil:
			return err
		case r.WakeAt == nil:
			return complete(ctx)
		}
		now := stamp(u.d.Clock.Now())
		if err := complete(ctx); err != nil {
			return err
		}
		if r.WakeAt.After(now) {
			return u.d.Queue.EnqueueNearest(ctx, job, entity.QueueBulk, *r.WakeAt)
		}
		if err := u.recount(ctx, job.OrganizationID, r.Versions[0].Scope.AreaID, job.CorrelationID); err != nil {
			return err
		}
		return u.schedule(ctx, job, nextWake(&r, now))
	})
}
