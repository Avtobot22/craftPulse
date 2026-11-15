package calc

import (
	"strconv"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// benchInput is a team of n tasks over ten months by weeks: each is taken to work, waits, goes back and is done.
func benchInput(n int) input {
	from := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	in := input{
		mapping: m1(), team: _team, window: entity.Window{From: from, To: from.AddDate(0, 0, 7*44), Step: entity.StepWeek},
		asOf: from.AddDate(0, 0, 7*44), location: time.UTC, followUp: _followUp,
	}
	for i := range n {
		created := from.Add(time.Duration(i%290) * 24 * time.Hour)
		day := func(k int) time.Time { return created.Add(time.Duration(k) * 24 * time.Hour) }
		in.tasks = append(in.tasks, task("T"+strconv.Itoa(i), created, _intake,
			status(day(1), _active), status(day(3), _waiting), status(day(4), _active), status(day(6), _done)).input())
	}
	return in
}

// BenchmarkPart is the work of one job of calc.slice after its facts are read: the timelines of the team, the hash
// of a part and, when no stored part has it, the points of ten weeks.
func BenchmarkPart(b *testing.B) {
	in := benchInput(1000)
	chunks := chunk(periods(in.window, in.asOf, in.location), _partPoints)
	c := chunks[len(chunks)/2]
	b.Run("prepare", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			prepare(in)
		}
	})
	pr := prepare(in)
	b.Run("hash", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			pr.partHash(nil, entity.StepWeek, c[0].start, c[len(c)-1].end)
		}
	})
	b.Run("calculate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			pr.calculate(c)
		}
	})
}
