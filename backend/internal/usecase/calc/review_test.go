package calc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var _d1 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func day(n int) time.Time { return _d1.AddDate(0, 0, n-1) }

func newID() entity.ID { return entity.ID(uuid.New()) }

type reviewCase struct {
	author, bot, human, other, reviewer entity.ID
	kinds                               map[entity.ID]entity.PersonKind
}

func newReviewCase() reviewCase {
	c := reviewCase{author: newID(), bot: newID(), human: newID(), other: newID(), reviewer: newID()}
	c.kinds = map[entity.ID]entity.PersonKind{c.author: entity.PersonHuman, c.bot: entity.PersonBot, c.human: entity.PersonHuman, c.other: entity.PersonUnknown}
	return c
}

func (c reviewCase) mr(ready *time.Time, field bool) entity.MRVersion {
	author := c.author
	v := entity.MRVersion{
		ID: newID(), MRID: newID(), State: "open", AuthorAccountID: &author, ReviewerField: field, OpenedAt: day(1).Add(-time.Hour), ReadyAt: ready,
	}
	if field {
		v.ReviewerAccountIDs = []entity.ID{c.reviewer}
	}
	return v
}

func event(kind string, at time.Time, actor *entity.ID) entity.MREvent {
	return entity.MREvent{ID: newID(), Kind: kind, EventTime: at, ActorAccountID: actor}
}

func TestReviewWaitOf(t *testing.T) {
	t.Parallel()
	c := newReviewCase()
	d1 := day(1)
	v15 := []entity.MREvent{event(_eventComment, day(2), &c.bot), event(_eventComment, day(3), &c.author), event(_eventReview, day(4), &c.human)}
	cases := []struct {
		name     string
		mr       entity.MRVersion
		events   []entity.MREvent
		reviews  bool
		complete bool
		at       time.Time
		want     entity.ReviewWait
	}{
		{
			name: "V-15 waits until the human review", mr: c.mr(&d1, true), events: v15, reviews: true, complete: true, at: day(4).Add(-time.Second),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: d1, WaitSeconds: int64((3*24*time.Hour - time.Second) / time.Second)},
		},
		{
			name: "V-15 human review on d4 ends the wait", mr: c.mr(&d1, true), events: v15, reviews: true, complete: true, at: day(6),
			want: entity.ReviewWait{Kind: entity.ReviewResponded, Since: day(4), WaitSeconds: 3 * 86400, ResponseKind: _eventReview},
		},
		{
			name: "an unconfirmed account does not respond", mr: c.mr(&d1, true), events: []entity.MREvent{event(_eventReview, day(2), &c.other)},
			reviews: true, complete: true, at: day(3),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: d1, WaitSeconds: 2 * 86400},
		},
		{
			name: "without the field of reviewers the MR waits for a response", mr: c.mr(&d1, false), reviews: true, complete: true, at: day(2),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: d1, WaitSeconds: 86400},
		},
		{
			name: "an incomplete history makes the wait unknown", mr: c.mr(&d1, false), reviews: true, at: day(2),
			want: entity.ReviewWait{Kind: entity.ReviewUnknown, Since: d1, WaitSeconds: 86400},
		},
		{
			name: "without a ready event the wait runs from the opening", mr: c.mr(nil, true), reviews: true, complete: true, at: day(2),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: day(1).Add(-time.Hour), WaitSeconds: 86400 + 3600, FallbackOpen: true},
		},
		{
			name: "a ready event stands for the ready time", mr: c.mr(nil, true), events: []entity.MREvent{event(_eventReady, day(2), &c.author)},
			reviews: true, complete: true, at: day(3),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: day(2), WaitSeconds: 86400},
		},
		{
			name: "with comments only a human comment is the first external response", mr: c.mr(&d1, true),
			events: []entity.MREvent{event(_eventComment, day(2), &c.author), event(_eventComment, day(3), &c.human)}, complete: true, at: day(5),
			want: entity.ReviewWait{Kind: entity.ReviewResponded, Since: day(3), WaitSeconds: 2 * 86400, ResponseKind: _eventComment},
		},
		{
			name: "removed reviewer leaves the MR without one", mr: c.mr(&d1, true),
			events: []entity.MREvent{
				{ID: newID(), Kind: _eventAssigned, EventTime: day(1), TargetAccountID: &c.reviewer},
				{ID: newID(), Kind: _eventRemoved, EventTime: day(2), TargetAccountID: &c.reviewer},
			},
			reviews: true, complete: true, at: day(3),
			want: entity.ReviewWait{Kind: entity.ReviewNoReviewer, Since: d1, WaitSeconds: 2 * 86400},
		},
		{
			name: "reassigned reviewer", mr: c.mr(&d1, true),
			events: []entity.MREvent{
				{ID: newID(), Kind: _eventRemoved, EventTime: day(2), TargetAccountID: &c.reviewer},
				{ID: newID(), Kind: _eventAssigned, EventTime: day(3), TargetAccountID: &c.human},
			},
			reviews: true, complete: true, at: day(4),
			want: entity.ReviewWait{Kind: entity.ReviewAwaitingFirst, Since: d1, WaitSeconds: 3 * 86400},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ReviewWaitOf(&tc.mr, tc.events, c.kinds, tc.reviews, tc.complete, tc.at)
			assert.Equal(t, tc.want, got)
		})
	}
}

func (c reviewCase) input(set string, mrs ...reviewMR) *reviewInput {
	for i := range mrs {
		mrs[i].kinds = c.kinds
	}
	return &reviewInput{set: set, mrs: mrs}
}

func TestReviewPoints_V15AndQueue(t *testing.T) {
	t.Parallel()
	c := newReviewCase()
	d1 := day(1)
	v15 := []entity.MREvent{event(_eventComment, day(2), &c.bot), event(_eventComment, day(3), &c.author), event(_eventReview, day(4), &c.human)}
	responded := reviewMR{version: c.mr(&d1, true), events: v15, reviews: true, complete: true}
	noField := reviewMR{version: c.mr(&d1, false), reviews: true, complete: true}
	noReviewer := reviewMR{version: c.mr(&d1, true), reviews: true, complete: true}
	noReviewer.version.ReviewerAccountIDs = nil
	unknown := reviewMR{version: c.mr(nil, true), reviews: true}
	in := c.input(entity.MetricSetReview, responded, noField, noReviewer, unknown)
	before := period{start: d1, end: day(4).Add(-time.Second)}

	a05 := waitingPoint(in, before)[0]
	assert.InDelta(t, 4, *a05.Value, 0)
	reasons := map[string]int{}
	for _, ct := range a05.Contributions {
		reasons[ct.ReasonCode]++
		assert.Equal(t, entity.ObjectMergeRequest, ct.ObjectKind)
	}
	assert.Equal(t, map[string]int{"awaiting_first_response": 2, "no_reviewer": 1, "unknown": 1}, reasons, "until d4 the V-15 MR waits")

	a06 := waitAgePoints(in, before)
	require.Len(t, a06, 2)
	assert.Equal(t, 3, *a06[0].N)
	assert.Equal(t, 1, a06[0].MissingCount, "the MR with an incomplete history is missing")
	assert.Equal(t, entity.StateProvisional, a06[0].State)

	a07 := firstReviewPoints(in, period{start: d1, end: day(8)})
	require.Len(t, a07, 2)
	assert.InDelta(t, 3*86400, *a07[0].Value, 0, "A07 of V-15 is three days")
	assert.Equal(t, 1, *a07[0].N)
	assert.True(t, a07[0].LowSample)
	assert.Empty(t, a07[0].ReasonCode)

	a10 := noReviewerPoint(in, before)[0]
	assert.InDelta(t, 3, *a10.Denominator, 0, "only MRs with the field are in the denominator")
	assert.InDelta(t, 1, *a10.Numerator, 0)
	assert.Equal(t, 1, a10.ExcludedCount)
	var excluded []string
	for _, ct := range a10.Contributions {
		if ct.Role == entity.RoleExcluded {
			excluded = append(excluded, ct.ReasonCode)
		}
	}
	assert.Equal(t, []string{entity.ReasonReviewerUnknown}, excluded)
}

func TestReviewPoints_CommentsOnlyAndFallback(t *testing.T) {
	t.Parallel()
	c := newReviewCase()
	opened := reviewMR{version: c.mr(nil, true), events: []entity.MREvent{event(_eventComment, day(2), &c.human)}, complete: true}
	in := c.input(entity.MetricSetReview, opened)
	a07 := firstReviewPoints(in, period{start: day(1), end: day(8)})
	assert.Equal(t, entity.ReasonExternalResponse, a07[0].ReasonCode)
	waiting := reviewMR{version: c.mr(nil, true), complete: true, reviews: true}
	a06 := waitAgePoints(c.input(entity.MetricSetReview, waiting), period{start: day(1), end: day(3)})
	require.Len(t, a06[0].Contributions, 1)
	assert.Equal(t, entity.ReasonFallbackOpen, a06[0].Contributions[0].ReasonCode)
	assert.InDelta(t, 2*86400+3600, *a06[0].Value, 0, "A06 runs from the opening")
}

// TestReviewPoints_V41 checks the nearest rank of [1,2,3,4] days and that a response at the end of the period
// is not in it.
func TestReviewPoints_V41(t *testing.T) {
	t.Parallel()
	c := newReviewCase()
	d1 := day(1)
	var mrs []reviewMR
	for i := 1; i <= 4; i++ {
		mrs = append(mrs, reviewMR{version: c.mr(&d1, true), events: []entity.MREvent{event(_eventReview, day(1+i), &c.human)}, reviews: true, complete: true})
	}
	in := c.input(entity.MetricSetReview, mrs...)
	pts := firstReviewPoints(in, period{start: d1, end: day(6).Add(time.Second)})
	assert.InDelta(t, 2*86400, *pts[0].Value, 0)
	assert.InDelta(t, 4*86400, *pts[1].Value, 0)
	assert.Equal(t, 4, *pts[0].N)
	assert.True(t, pts[0].LowSample)
	edge := firstReviewPoints(in, period{start: d1, end: day(5)})
	assert.Equal(t, 3, *edge[0].N, "a response exactly at the end belongs to the next period")
	empty := firstReviewPoints(in, period{start: day(10), end: day(11)})
	assert.Equal(t, entity.StateNotApplicable, empty[0].State)
	assert.Nil(t, empty[0].Value)
}

func TestEventPoints_ByKindOfParticipant(t *testing.T) {
	t.Parallel()
	task, mr := newID(), newID()
	in := &reviewInput{set: entity.MetricSetEvents, events: []activity{
		{id: newID(), object: mr, kind: entity.ObjectMergeRequest, reason: "mr:review", at: day(1), actor: ActorHuman},
		{id: newID(), object: mr, kind: entity.ObjectMergeRequest, reason: "mr:approval", at: day(1).Add(time.Minute), actor: ActorHuman},
		{id: newID(), object: mr, kind: entity.ObjectMergeRequest, reason: "mr:comment", at: day(1), actor: ActorBot},
		{id: newID(), object: task, kind: entity.ObjectTask, reason: "task:field_change", at: day(1), actor: ActorSystem},
		{id: newID(), object: task, kind: entity.ObjectTask, reason: "task:comment", at: day(1), actor: ActorUnknown},
		{id: newID(), object: task, kind: entity.ObjectTask, reason: "task:comment", at: day(2), actor: ActorHuman},
	}}
	pts := eventPoints(in, period{start: day(1), end: day(2)})
	require.Len(t, pts, 4)
	got := map[string]float64{}
	for i, est := range definition("A22").Estimators {
		got[est] = *pts[i].Value
	}
	assert.Equal(t, map[string]float64{ActorHuman: 2, ActorBot: 1, ActorSystem: 1, ActorUnknown: 1}, got)
	items := []int{pts[0].Contributions[0].ItemNo, pts[0].Contributions[1].ItemNo}
	assert.Equal(t, []int{1, 2}, items, "two events of one MR are two items")
}
