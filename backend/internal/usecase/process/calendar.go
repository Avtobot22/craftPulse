package process

import (
	"context"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _maxHolidays = 400

// ListCalendarVersions pages the versions of the calendar of a team newest first; Latest is for If-Match.
func (u *UseCase) ListCalendarVersions(ctx context.Context, s entity.Subject, teamID entity.ID, p entity.VersionPageRequest) (entity.CalendarVersionPage, error) {
	org := s.OrganizationID
	var page entity.CalendarVersionPage
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if page.Latest, err = u.d.Repo.LatestCalendarVersionNo(ctx, org, teamID); err != nil {
			return err
		}
		items, err := u.d.Repo.PageCalendarVersions(ctx, org, teamID, p)
		if err != nil {
			return err
		}
		if len(items) > p.Limit {
			items = items[:p.Limit]
			next := items[len(items)-1].VersionNo
			page.Next = &next
		}
		page.Items = items
		return nil
	})
	return page, err
}

// CreateCalendarVersion confirms the calendar of a team as version baseVersion+1 by the rules of
// CreateMappingVersion: a repeat of the key first, then If-Match, then a draft equal to the current version.
func (u *UseCase) CreateCalendarVersion(ctx context.Context, s entity.Subject, teamID entity.ID, baseVersion int, key entity.ID,
	d entity.CalendarDraft,
) (v entity.CalendarVersion, created bool, err error) {
	org := s.OrganizationID
	d.Holidays = slices.SortedFunc(slices.Values(d.Holidays), time.Time.Compare)
	if err := u.checkCalendar(ctx, org, d); err != nil {
		return entity.CalendarVersion{}, false, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.CalendarVersion{}, false, errorf("new calendar version id: %w", err)
	}
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		created = false
		seq, err := u.d.Tenants.NextSeq(ctx, org)
		if err != nil {
			return err
		}
		saved, err := u.d.Repo.CalendarVersionByConfirmation(ctx, org, key)
		if err == nil {
			if saved.TeamID != teamID || saved.ConfirmedBy != s.UserID || !sameCalendar(&saved.CalendarDraft, &d) {
				return entity.ErrIdempotencyKeyReused
			}
			v, created = saved, true
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		latest, err := u.d.Repo.LatestCalendarVersionNo(ctx, org, teamID)
		if err != nil {
			return err
		}
		if latest != baseVersion {
			return entity.ErrVersionConflict
		}
		if latest > 0 {
			current, err := u.d.Repo.CalendarVersionAt(ctx, org, teamID, math.MaxInt64)
			if err != nil {
				return err
			}
			if sameCalendar(&current.CalendarDraft, &d) {
				v = current
				return nil
			}
		}
		v = entity.CalendarVersion{
			ID: id, TeamID: teamID, VersionNo: latest + 1, BaseVersionNo: baseOf(latest), CalendarDraft: d, KnownSeq: seq,
			Confirmation: entity.Confirmation{ConfirmedBy: s.UserID, ConfirmedAt: u.d.Clock.Now(), ConfirmationID: key},
		}
		if err := u.d.Repo.InsertCalendarVersion(ctx, org, v); err != nil {
			return err
		}
		created = true
		if err := u.record(ctx, s, _operationConfirmCalendar, "calendar_version", v.ID, teamID, latest, v.Confirmation); err != nil {
			return err
		}
		return u.recount(ctx, org, teamID)
	})
	if err != nil {
		return entity.CalendarVersion{}, false, err
	}
	return v, created, nil
}

// CalendarAt is the version of the calendar of a team in force at the knowledge number seq; false without one.
func (u *UseCase) CalendarAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.CalendarVersion, bool, error) {
	var out entity.CalendarVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CalendarVersionAt(ctx, orgID, teamID, seq)
		return err
	})
	if isNotFound(err) {
		return entity.CalendarVersion{}, false, nil
	}
	return out, err == nil, err
}

// checkCalendar checks the form; Holidays come sorted, so a repeated date is next to its first one.
func (u *UseCase) checkCalendar(ctx context.Context, orgID entity.ID, d entity.CalendarDraft) error {
	verr := &entity.ValidationError{}
	var zoneKnown bool
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		zoneKnown, err = u.timezoneKnown(ctx, d.Timezone)
		return err
	})
	if err != nil {
		return err
	}
	if !zoneKnown {
		verr.Add("/timezone", entity.FieldNotAllowedValue, "not an IANA time zone")
	}
	checkTimezone(verr, d.Timezone)
	if d.Workdays > entity.AllWorkdays {
		verr.Add("/workdays", entity.FieldOutOfRange, "")
	}
	switch {
	case (d.WorkStart == nil) != (d.WorkEnd == nil):
		verr.Add("/work_end", entity.FieldRequired, "working hours have both a start and an end, or neither")
	case d.WorkStart != nil && *d.WorkStart >= *d.WorkEnd:
		verr.Add("/work_end", entity.FieldOutOfRange, "the end comes after the start")
	}
	if len(d.Holidays) > _maxHolidays {
		verr.Add("/holidays", entity.FieldOutOfRange, "at most "+strconv.Itoa(_maxHolidays)+" dates")
	} else {
		for i := 1; i < len(d.Holidays); i++ {
			if d.Holidays[i].Equal(d.Holidays[i-1]) {
				verr.Add("/holidays", entity.FieldDuplicate, d.Holidays[i].Format(time.DateOnly))
			}
		}
	}
	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}

func sameCalendar(a, b *entity.CalendarDraft) bool {
	sameTime := func(x, y *entity.TimeOfDay) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	return a.Timezone == b.Timezone && a.Workdays == b.Workdays && sameTime(a.WorkStart, b.WorkStart) &&
		sameTime(a.WorkEnd, b.WorkEnd) && slices.EqualFunc(a.Holidays, b.Holidays, time.Time.Equal)
}
