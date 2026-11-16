package dataset

import "time"

// Background tasks live in the working day the whole team shares: 07:00-14:00 UTC is 10:00-17:00 in +03:00,
// 12:00-19:00 in +05:00 and 07:00-14:00 in UTC, Monday to Friday. Durations of work are counted in this time, so
// a share of the work always lands inside a working day and keeps the order of the events.
const (
	_workOpen  = 7 * time.Hour
	_workClose = 14 * time.Hour
)

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func workday(day time.Time) bool {
	return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
}

// toWork is the first working moment at or after t.
func toWork(t time.Time) time.Time {
	t = t.UTC()
	for {
		day := dayOf(t)
		switch {
		case !workday(day) || !t.Before(day.Add(_workClose)):
			t = day.AddDate(0, 0, 1)
		case t.Before(day.Add(_workOpen)):
			return day.Add(_workOpen)
		default:
			return t
		}
	}
}

// workAdd is the working moment d of working time after t.
func workAdd(t time.Time, d time.Duration) time.Time {
	t = toWork(t)
	for {
		left := dayOf(t).Add(_workClose).Sub(t)
		if d < left {
			return t.Add(d)
		}
		d -= left
		t = toWork(dayOf(t).Add(_workClose))
	}
}

// workBetween is the working time inside [a, b).
func workBetween(a, b time.Time) time.Duration {
	var total time.Duration
	for t := toWork(a); t.Before(b); {
		closing := dayOf(t).Add(_workClose)
		total += minTime(closing, b).Sub(t)
		t = toWork(closing)
	}
	return total
}

// workShare is the moment a share in [0, 1) of the working time of [a, b) after a, in whole seconds.
func workShare(a, b time.Time, share float64) time.Time {
	return workAdd(a, time.Duration(float64(workBetween(a, b))*share)).Truncate(time.Second)
}
