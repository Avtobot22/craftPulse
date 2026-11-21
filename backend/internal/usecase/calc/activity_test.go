package calc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func activityTestPoint(t *testing.T, points []entity.SeriesPoint, estimator string) entity.Point {
	t.Helper()
	for i := range points {
		point := &points[i]
		if point.Estimator == estimator {
			return point.Point
		}
	}
	t.Fatalf("missing estimator %q", estimator)
	return entity.Point{}
}

func TestActivityDeliveryLag(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	observed := start.Add(time.Hour)
	p := period{start: start, end: start.AddDate(0, 0, 7)}
	var s activitySource
	for i, tc := range []struct {
		mode  entity.ObservedVia
		delay time.Duration
	}{
		{entity.ViaWebhook, 2 * time.Second},
		{entity.ViaWebhook, 4 * time.Second},
		{entity.ViaWebhook, -time.Second},
		{entity.ViaIncremental, 25 * time.Second},
		{entity.ViaBackfill, 15 * 24 * time.Hour},
	} {
		s.events = append(s.events, activityEvent{
			id: entity.ID{byte(i + 1)}, object: entity.ID{1}, kind: entity.ObjectMergeRequest,
			via: tc.mode, observed: observed, at: observed.Add(-tc.delay),
		})
	}
	points := s.deliveryPoints(p)
	for _, tc := range []struct {
		estimator   string
		want        float64
		n, excluded int
	}{
		{"webhook:p50", 2, 2, 1},
		{"webhook:p95", 4, 2, 1},
		{"incremental:p95", 25, 1, 0},
		{"backfill:p95", 15 * 86400, 1, 0},
	} {
		point := activityTestPoint(t, points, tc.estimator)
		require.NotNil(t, point.Value)
		assert.InDelta(t, tc.want, *point.Value, 0)
		assert.Equal(t, tc.n, *point.N)
		assert.Equal(t, tc.excluded, point.ExcludedCount)
	}
}

func TestActivityActorKindsV15(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	responseAt := start.Add(time.Hour)
	end := start.AddDate(0, 0, 7)
	mr, actor, sameName := entity.ID{1}, entity.ID{2}, entity.ID{3}
	response := entity.MREvent{ID: entity.ID{4}, MRID: mr, ActorAccountID: &actor, Kind: "review", EventTime: responseAt}
	for _, tc := range []struct {
		name      string
		kind      entity.PersonKind
		wait      entity.ReviewWaitKind
		estimator string
		coverage  float64
	}{
		{"unconfirmed account", entity.PersonUnknown, entity.ReviewAwaitingFirst, ActorUnknown, 0},
		{"confirmed human", entity.PersonHuman, entity.ReviewResponded, ActorHuman, .5},
		{"confirmed bot", entity.PersonBot, entity.ReviewAwaitingFirst, ActorBot, .5},
		{"confirmed system", entity.PersonSystem, entity.ReviewAwaitingFirst, ActorSystem, .5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kinds := map[entity.ID]entity.PersonKind{actor: tc.kind}
			wait := ReviewWaitOf(&entity.MRVersion{MRID: mr, OpenedAt: start}, []entity.MREvent{response}, kinds, true, true, end)
			assert.Equal(t, tc.wait, wait.Kind)
			s := activitySource{events: []activityEvent{
				{id: response.ID, object: mr, kind: entity.ObjectMergeRequest, account: &actor, actor: actorOf(kinds, &actor), at: responseAt},
				{id: entity.ID{5}, object: mr, kind: entity.ObjectMergeRequest, account: &sameName, actor: actorOf(kinds, &sameName), at: responseAt},
				{id: entity.ID{6}, object: mr, kind: entity.ObjectMergeRequest, at: responseAt},
			}}
			points := s.actorPoints(period{start: start, end: end})
			coverage := activityTestPoint(t, points, "")
			require.NotNil(t, coverage.Value)
			assert.InDelta(t, tc.coverage, *coverage.Value, 0)
			assert.InDelta(t, 2, *coverage.Denominator, 0)
			assert.Equal(t, 1, coverage.ExcludedCount)
			point := activityTestPoint(t, points, tc.estimator)
			require.NotEmpty(t, point.Contributions)
			var evidence eventEvidence
			require.NoError(t, json.Unmarshal(point.Contributions[0].Evidence, &evidence))
			assert.Equal(t, &actor, evidence.AccountID)
			unknown := activityTestPoint(t, points, ActorUnknown)
			require.NotEmpty(t, unknown.Contributions)
			require.NoError(t, json.Unmarshal(unknown.Contributions[len(unknown.Contributions)-1].Evidence, &evidence))
			assert.Equal(t, &sameName, evidence.AccountID)
		})
	}
}

func TestActivityLinkAmbiguityUsesDistinctTasks(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	first, second, third := entity.ID{1}, entity.ID{2}, entity.ID{3}
	a, b, c := entity.ID{4}, entity.ID{5}, entity.ID{6}
	edges := []entity.LinkEdge{
		{Type: entity.LinkTaskMR, From: a, To: first, Status: entity.LinkCandidate, Method: entity.LinkBranchKey},
		{Type: entity.LinkTaskMR, From: a, To: first, Status: entity.LinkCandidate, Method: entity.LinkTitleKey},
		{Type: entity.LinkTaskMR, From: b, To: first, Status: entity.LinkCandidate},
		{Type: entity.LinkTaskMR, From: a, To: second, Status: entity.LinkCandidate},
		{Type: entity.LinkTaskMR, From: b, To: second, Status: entity.LinkRejected},
		{Type: entity.LinkTaskMR, From: c, To: third, Status: entity.LinkCandidate},
		{Type: entity.LinkTaskMR, From: c, To: third, Status: entity.LinkConfirmed},
		{Type: entity.LinkMRRun, From: second, To: entity.ID{7}, Status: entity.LinkConfirmed},
	}
	mrs := map[entity.ID]time.Time{first: start, second: start, third: start}
	s := activitySource{links: activityLinksOf(mrs, []entity.ID{first, second, third}, edges)}
	points := s.linkPoints(period{start: start, end: start.AddDate(0, 0, 7)})
	for est, want := range map[string]float64{"confirmed": 2, "proposed": 3, "rejected": 1, _ambiguous: 2, _unambiguous: 1, "ambiguous_share": 2.0 / 3} {
		p := activityTestPoint(t, points, est)
		require.NotNil(t, p.Value)
		assert.InDelta(t, want, *p.Value, 1e-12, est)
	}
}
