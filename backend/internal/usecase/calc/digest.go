package calc

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"maps"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// digest writes fields of fixed or prefixed length, so two different inputs never give the same stream.
type digest struct {
	h hash.Hash
}

func newDigest() *digest { return &digest{h: sha256.New()} }

func (d *digest) bytes(b []byte) {
	_ = binary.Write(d.h, binary.BigEndian, uint32(len(b))) //nolint:gosec // a field of a hash is far below 4 GiB
	_, _ = d.h.Write(b)
}

func (d *digest) text(s string) { d.bytes([]byte(s)) }

func (d *digest) id(id entity.ID) { _, _ = d.h.Write(id[:]) }

func (d *digest) int(v int64) { _ = binary.Write(d.h, binary.BigEndian, v) }

func (d *digest) flag(v bool) {
	if v {
		d.int(1)
		return
	}
	d.int(0)
}

func (d *digest) time(t time.Time) { d.int(t.UnixNano()) }

func (d *digest) optTime(t *time.Time) {
	d.flag(t != nil)
	if t != nil {
		d.time(*t)
	}
}

func (d *digest) optText(s *string) {
	d.flag(s != nil)
	if s != nil {
		d.text(*s)
	}
}

func (d *digest) sum() []byte { return d.h.Sum(nil) }

// partHash is SHA-256 of what the part [from, to) reads (TZ 12.7): the definitions and their versions, the
// mapping version, the targets in force and the window H, the scope and the step, and for each task the state
// its earlier history leaves at from with the changes inside the part by ID and known_seq, whether its area is
// fully collected and, for a task first completed in the part, whether its follow-up ended and saw a reopen. The
// last part also reads the state of the collection at as_of. The knowledge number of the slice is not in it, so a
// fact of another week leaves the part as it was, and a fact that changes the state carried into a later part
// changes that part too.
func (pr *prepared) partHash(scopeHash []byte, step entity.Step, from, to time.Time) []byte {
	d := newDigest()
	for i := range _definitions {
		d.text(_definitions[i].ID)
		d.int(int64(_definitions[i].Version))
	}
	d.bytes(scopeHash)
	d.text(string(step))
	d.time(from)
	d.time(to)
	d.flag(pr.in.mapping != nil)
	if pr.in.mapping == nil {
		return d.sum()
	}
	d.id(pr.in.mapping.ID)
	x := &pr.ctx
	d.int(int64(x.followUp))
	groups := slices.SortedFunc(maps.Keys(x.targets), func(a, b entity.TargetGroup) int {
		return cmp.Or(cmp.Compare(a.TaskType, b.TaskType), cmp.Compare(a.SizeLabel, b.SizeLabel))
	})
	d.int(int64(len(groups)))
	for _, g := range groups {
		d.text(g.TaskType)
		d.text(g.SizeLabel)
		d.int(x.targets[g])
	}
	last := to.Equal(x.last)
	d.flag(last)
	if last {
		x.digestCollection(d)
	}
	tls := slices.Clone(pr.tls)
	slices.SortFunc(tls, func(a, b *timeline) int { return bytes.Compare(a.slice.TaskID[:], b.slice.TaskID[:]) })
	for _, tl := range tls {
		if tl.slice.Created.After(to) {
			continue
		}
		tl.digestPart(d, x, from, to)
	}
	return d.sum()
}

// digestCollection writes the state of the connections the last point of T40 and T42 reads at as_of.
func (x *pointContext) digestCollection(d *digest) {
	d.time(x.asOf)
	conns := x.connections()
	d.int(int64(len(conns)))
	for _, c := range conns {
		d.id(c.ConnectionID)
		d.text(c.Completeness)
		d.optTime(c.Watermark)
		d.optTime(c.LastSuccessAt)
		d.flag(c.Reconcile != nil)
		if r := c.Reconcile; r != nil {
			d.id(r.ID)
			d.optTime(r.FinishedAt)
			for _, n := range []*int{r.ExpectedCount, r.MissingCount, r.ChangedCount} {
				d.flag(n != nil)
				if n != nil {
					d.int(int64(*n))
				}
			}
		}
	}
}

func (tl *timeline) digestPart(d *digest, x *pointContext, from, to time.Time) {
	s := &tl.slice
	d.id(s.TaskID)
	d.id(s.ProjectID)
	d.time(s.Created)
	d.optText(s.TaskType)
	d.optText(s.SizeLabel)
	d.optTime(s.HistoryFrom)
	d.flag(s.Conflict)
	d.flag(tl.preStart)
	d.flag(tl.preCompletion)
	if s.FirstStart != nil && s.FirstStart.Before(from) {
		d.optTime(s.FirstStart)
	} else {
		d.optTime(nil)
	}
	before := 0
	for _, c := range tl.completions {
		if c.Before(from) {
			before++
		}
	}
	d.int(int64(before))
	if before > 0 {
		d.time(tl.completions[0])
	}
	if ep := tl.episodeAt(from); ep != nil {
		d.flag(true)
		d.time(ep.start)
		d.flag(ep.startKnown)
		d.text(string(ep.classAt(from)))
	} else {
		d.flag(false)
	}
	for i := range tl.events {
		e := &tl.events[i]
		if e.EventTime.Before(from) || e.EventTime.After(to) {
			continue
		}
		d.id(e.ID)
		d.int(e.KnownSeq)
	}
	d.flag(x.incomplete[tl.area])
	entered, mapped := 0, 0
	for _, e := range tl.entries {
		if !e.at.Before(from) {
			break
		}
		entered++
		if e.mapped {
			mapped++
		}
	}
	d.int(int64(entered))
	d.int(int64(mapped))
	if first := s.FirstCompletion; first != nil && !first.Before(from) && first.Before(to) {
		end := first.Add(x.followUp)
		d.flag(end.After(x.asOf))
		d.flag(tl.reopenedIn(*first, end))
	}
}
