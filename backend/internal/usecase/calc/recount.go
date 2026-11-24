package calc

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	// _recountDelay gathers the facts of 30 s into one chain (TZ 10.6).
	_recountDelay = 30 * time.Second
	// _loadingEvery is the most frequent chain of a series while its areas load their history.
	_loadingEvery = 5 * time.Minute
	// _windowWeeks and _windowMonths are the window of a series without a revision: ten months.
	_windowWeeks  = 44
	_windowMonths = 10
	// _windowDays is the window of a daily series without a revision.
	_windowDays = 90
	// _extendStart and _extendSpan are the night window 01:00-05:00 of the daily extension in the time zone of
	// the team; the minute of a series inside it comes from its ID.
	_extendStart = time.Hour
	_extendSpan  = 4 * time.Hour
	// _extendEvery is the period of calc.extend.tick: every hour queues the extensions due before the next one.
	_extendEvery = time.Hour
)

// due is a chain of a series to queue at a moment.
type due struct {
	job entity.CalcSliceJob
	at  time.Time
}

// RunRecount is the job calc.recount: every series of the team, or of the teams whose projects take the area, gets
// a chain over its extended window after the delay that gathers the facts; while the areas load their history a
// series gets one chain in 5 minutes at most (TZ 10.6).
func (u *UseCase) RunRecount(ctx context.Context, job entity.CalcRecountJob, complete Complete) error {
	if !job.CodeReceiptID.IsZero() {
		return u.recountCode(ctx, job, complete)
	}
	if !job.DocsReceiptID.IsZero() {
		return u.recountDocumentation(ctx, job, complete)
	}
	if !job.SecurityReceiptID.IsZero() {
		return u.recountSecurity(ctx, job, complete)
	}
	org := job.OrganizationID
	var connectionAreas []entity.ID
	if u.d.Activity != nil && (!job.TeamID.IsZero() || !job.ConnectionID.IsZero()) {
		scope := entity.ActivityScope{TeamID: job.TeamID, ConnectionID: job.ConnectionID}
		for _, step := range []entity.Step{entity.StepWeek, entity.StepMonth} {
			if _, err := u.ActivitySeries(ctx, org, scope, step); err != nil {
				return err
			}
		}
		if !job.ConnectionID.IsZero() {
			var err error
			connectionAreas, err = u.d.Activity.ActivityConnectionAreas(ctx, org, job.ConnectionID)
			if err != nil {
				return err
			}
		}
	}
	all, err := u.orgSeries(ctx, org)
	if err != nil {
		return err
	}
	at := u.d.Clock.Now().Add(_recountDelay)
	var plans []due
	for i := range all {
		s := &all[i]
		if s.MetricSet == entity.MetricSetCode || s.MetricSet == entity.MetricSetDocumentation || s.MetricSet == entity.MetricSetSecurity {
			continue
		}
		if !job.ConnectionID.IsZero() && s.MetricSet != entity.MetricSetActivity {
			continue
		}
		scope, ok, err := scopeOf(s.Scope)
		if err != nil {
			return err
		}
		if !ok || (!job.TeamID.IsZero() && scope.TeamID != job.TeamID) {
			continue
		}
		areas, err := u.scopeAreas(ctx, org, scope)
		if err != nil {
			return err
		}
		if !job.AreaID.IsZero() && !slices.Contains(areas, job.AreaID) {
			continue
		}
		if !job.ConnectionID.IsZero() && scope.ConnectionID != job.ConnectionID &&
			(!scope.ConnectionID.IsZero() || !slices.ContainsFunc(areas, func(a entity.ID) bool { return slices.Contains(connectionAreas, a) })) {
			continue
		}
		p, err := u.plan(ctx, org, s, scope.TeamID, areas, at, job.CorrelationID)
		if err != nil {
			return err
		}
		plans = append(plans, p)
	}
	return u.queue(ctx, org, plans, complete)
}

// TickExtend queues the daily extension of every active organization; each runs under its own organization.
func (u *UseCase) TickExtend(ctx context.Context) error {
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
			return u.d.Queue.Enqueue(ctx, entity.CalcExtendJob{OrganizationID: org}, entity.QueueBulk)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RunExtend is the job calc.extend: a series whose minute in the night window of today comes before the next tick
// and that has no revision started since, other than a failed one, gets a chain at that minute. After 05:00 the day is not caught up; the
// next night extends the window.
func (u *UseCase) RunExtend(ctx context.Context, job entity.CalcExtendJob, complete Complete) error {
	org := job.OrganizationID
	all, err := u.orgSeries(ctx, org)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now()
	var plans []due
	for i := range all {
		s := &all[i]
		if s.MetricSet == entity.MetricSetCode || s.MetricSet == entity.MetricSetDocumentation || s.MetricSet == entity.MetricSetSecurity {
			continue
		}
		scope, ok, err := scopeOf(s.Scope)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		team := scope.TeamID
		cur, last, err := u.revisions(ctx, org, s)
		if err != nil {
			return err
		}
		loc, err := u.location(ctx, org, team, cur)
		if err != nil {
			return err
		}
		slot, ok := extendSlot(s.ID, now, loc)
		if !ok || (last != nil && last.Status != entity.RevisionFailed && !last.StartedAt.Before(slot)) {
			continue
		}
		areas, err := u.scopeAreas(ctx, org, scope)
		if err != nil {
			return err
		}
		p, err := u.plan(ctx, org, s, team, areas, slot, entity.ID(logger.CorrelationID(ctx)))
		if err != nil {
			return err
		}
		plans = append(plans, p)
	}
	return u.queue(ctx, org, plans, complete)
}

// extendSlot is the minute of the series in the night window of the day of now in loc; false when it does not
// come before the next tick or the window of the day has passed.
func extendSlot(id entity.ID, now time.Time, loc *time.Location) (time.Time, bool) {
	local := now.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	offset := time.Duration(binary.BigEndian.Uint64(id[8:])%uint64(_extendSpan/time.Minute)) * time.Minute
	slot := day.Add(_extendStart + offset)
	if !slot.Before(now.Add(_extendEvery)) || !now.Before(day.Add(_extendStart+_extendSpan)) {
		return time.Time{}, false
	}
	return slot.UTC(), true
}

// SeriesTeam is the team of a series of the flow, for the check of access of a refresh.
func (u *UseCase) SeriesTeam(ctx context.Context, orgID, seriesID entity.ID) (entity.ID, error) {
	var s entity.CalcSeries
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		s, err = u.d.Repo.Series(ctx, orgID, seriesID)
		return err
	})
	if err != nil {
		return entity.ID{}, err
	}
	if s.MetricSet != entity.MetricSetFlow {
		return entity.ID{}, entity.ErrNotFound
	}
	return scopeTeam(s.Scope)
}

// RequestRefresh queues a new revision of the series on the current facts at once, in the transaction of the
// caller, with the same definitions as a planned chain. A chain the series builds already is not doubled: the
// queue drops the copy and the refresh is answered by that chain (TZ 10.6, FR-023).
func (u *UseCase) RequestRefresh(ctx context.Context, orgID, seriesID, corr entity.ID) (entity.SeriesRefresh, error) {
	s, err := u.d.Repo.Series(ctx, orgID, seriesID)
	if err != nil {
		return entity.SeriesRefresh{}, err
	}
	team, err := scopeTeam(s.Scope)
	if err != nil {
		return entity.SeriesRefresh{}, err
	}
	cur, last, err := u.revisions(ctx, orgID, &s)
	if err != nil {
		return entity.SeriesRefresh{}, err
	}
	now := u.d.Clock.Now().UTC()
	w, err := u.window(ctx, orgID, team, &s, cur, now)
	if err != nil {
		return entity.SeriesRefresh{}, err
	}
	// A building revision answers the refresh unless it stopped: the new chain fails that one and builds the next.
	out := entity.SeriesRefresh{SeriesID: s.ID}
	if last != nil {
		out.AfterRevisionNo = last.RevisionNo
		if last.Status == entity.RevisionBuilding && !last.ProgressAt.Before(now.Add(-_staleAfter)) {
			out.AfterRevisionNo--
		}
	}
	job := entity.CalcSliceJob{OrganizationID: orgID, SeriesID: s.ID, From: w.From, To: w.To, AsOf: now, CorrelationID: corr}
	return out, u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}

// RefreshStatus is where a refresh is: queued until the revision after its number exists, then running while it
// builds and succeeded or failed with it.
func (u *UseCase) RefreshStatus(ctx context.Context, orgID entity.ID, r entity.SeriesRefresh) (entity.RefreshStatus, error) {
	var out entity.RefreshStatus
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		rev, found, err := u.d.Repo.RevisionAfter(ctx, orgID, r.SeriesID, r.AfterRevisionNo)
		if err != nil || !found {
			out.Status = entity.OperationQueued
			return err
		}
		switch rev.Status {
		case entity.RevisionBuilding:
			out.Status = entity.OperationRunning
		case entity.RevisionPublished, entity.RevisionSuperseded:
			out.Status, out.RevisionID = entity.OperationSucceeded, &rev.ID
		case entity.RevisionFailed:
			out.Status = entity.OperationFailed
		}
		return nil
	})
	return out, err
}

// plan is the chain of a series at the moment at over its extended window; during the initial load of its areas it waits
// 5 minutes after the start of the last revision.
func (u *UseCase) plan(ctx context.Context, org entity.ID, s *entity.CalcSeries, team entity.ID, areas []entity.ID, at time.Time,
	corr entity.ID,
) (due, error) {
	cur, last, err := u.revisions(ctx, org, s)
	if err != nil {
		return due{}, err
	}
	scope, err := parseScope(s.Scope)
	if err != nil {
		return due{}, err
	}
	sources, err := u.seriesCollection(ctx, org, areas, s.MetricSet, scope.ConnectionID)
	if err != nil {
		return due{}, err
	}
	if last != nil && loading(sources) {
		at = later(at, last.StartedAt.Add(_loadingEvery))
	}
	at = at.UTC()
	w, err := u.window(ctx, org, team, s, cur, at)
	if err != nil {
		return due{}, err
	}
	return due{job: entity.CalcSliceJob{OrganizationID: org, SeriesID: s.ID, From: w.From, To: w.To, AsOf: at, CorrelationID: corr}, at: at}, nil
}

// window is the window of the current revision extended to the end of the step that holds at; a series without
// a revision gets ten months ending with that step. The start stays, so the parts keep their bounds and are reused.
func (u *UseCase) window(ctx context.Context, org, team entity.ID, s *entity.CalcSeries, cur *entity.CalcRevision, at time.Time) (entity.Window, error) {
	loc, err := u.location(ctx, org, team, cur)
	if err != nil {
		return entity.Window{}, err
	}
	w := entity.Window{Step: s.Step}
	if cur != nil {
		w.From, w.To = cur.Window.From, cur.Window.To
	} else {
		w.From = windowStart(at, loc, s.Step)
	}
	end := w.From.In(loc)
	for !end.After(at) {
		end = nextStep(end, s.Step)
	}
	if end.After(w.To) {
		w.To = end.UTC()
	}
	w.From = w.From.UTC()
	return w, nil
}

// location is the time zone of the current revision, or of the mapping in force for a series without one.
func (u *UseCase) location(ctx context.Context, org, team entity.ID, cur *entity.CalcRevision) (*time.Location, error) {
	name := "UTC"
	if cur != nil {
		name = cur.Timezone
	} else if !team.IsZero() && u.d.Settings != nil {
		mapping, ok, err := u.d.Settings.MappingAt(ctx, org, team, math.MaxInt64)
		if err != nil {
			return nil, err
		}
		if ok {
			name = mapping.Timezone
		}
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("calc: time zone %q: %w", name, err)
	}
	return loc, nil
}

// windowStart is the start of the step ten months, or 90 days for a daily series, before the step that holds at.
func windowStart(at time.Time, loc *time.Location, step entity.Step) time.Time {
	local := at.In(loc)
	if step == entity.StepMonth {
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1-_windowMonths, 0)
	}
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if step == entity.StepDay {
		return day.AddDate(0, 0, 1-_windowDays)
	}
	monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	return monday.AddDate(0, 0, -7*(_windowWeeks-1))
}

func nextStep(t time.Time, step entity.Step) time.Time {
	switch step {
	case entity.StepMonth:
		return t.AddDate(0, 1, 0)
	case entity.StepDay:
		return t.AddDate(0, 0, 1)
	case entity.StepWeek:
	}
	return t.AddDate(0, 0, 7)
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// revisions are the current revision of the series and the one with the highest number, nil when absent.
func (u *UseCase) revisions(ctx context.Context, org entity.ID, s *entity.CalcSeries) (cur, last *entity.CalcRevision, err error) {
	err = u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		cur, last = nil, nil
		if id := s.CurrentRevisionID; id != nil {
			rev, err := u.d.Repo.Revision(ctx, org, *id)
			if err != nil {
				return err
			}
			cur = &rev
		}
		rev, found, err := u.d.Repo.LastRevision(ctx, org, s.ID)
		if found {
			last = &rev
		}
		return err
	})
	return cur, last, err
}

func (u *UseCase) orgSeries(ctx context.Context, org entity.ID) ([]entity.CalcSeries, error) {
	var out []entity.CalcSeries
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		for _, set := range []string{
			entity.MetricSetFlow, entity.MetricSetReview, entity.MetricSetEvents, entity.MetricSetLinks,
			entity.MetricSetCI, entity.MetricSetActivity,
		} {
			series, err := u.d.Repo.OrgSeries(ctx, org, set)
			if err != nil {
				return err
			}
			out = append(out, series...)
		}
		return nil
	})
	return out, err
}

// queue puts the chains with the completion of the job in one transaction; a chain a series already waits for or
// builds over the same window is dropped by the queue.
func (u *UseCase) queue(ctx context.Context, org entity.ID, plans []due, complete Complete) error {
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		for i := range plans {
			if err := u.d.Queue.EnqueueAt(ctx, plans[i].job, entity.QueueBulk, plans[i].at); err != nil {
				return err
			}
		}
		return complete(ctx)
	})
}
