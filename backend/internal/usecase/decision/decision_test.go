package decision

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func day(n int) time.Time {
	return time.Date(2026, 11, n, 12, 0, 0, 0, time.UTC)
}

func decisionFixture() entity.ExceptionRecord {
	first, renewal, actor := entity.ID{1}, entity.ID{2}, entity.ID{3}
	area := entity.ID{4}
	scope := entity.ExceptionScope{AreaID: area, PathPrefix: "src/billing", RuleID: "finding-rule"}
	return entity.ExceptionRecord{
		ID: entity.ID{9},
		Versions: []entity.ExceptionVersion{
			{
				ID: renewal, AuthorID: actor, FindingKind: "security", ApprovalRequired: true, Number: 2, Seq: 6,
				ExceptionDraft: entity.ExceptionDraft{Scope: scope, Type: entity.ExceptionAcceptedRisk, EffectiveFrom: day(4), ExpiresAt: day(9)},
			},
			{
				ID: first, AuthorID: actor, FindingKind: "security", ApprovalRequired: true, Number: 1, Seq: 2,
				ExceptionDraft: entity.ExceptionDraft{Scope: scope, Type: entity.ExceptionAcceptedRisk, EffectiveFrom: day(2), ExpiresAt: day(5)},
			},
		},
		Transitions: []entity.ExceptionTransition{
			{VersionID: first, Status: entity.ExceptionPending, At: day(1), Seq: 2},
			{VersionID: first, Status: entity.ExceptionApproved, At: day(3), Seq: 4},
			{VersionID: renewal, Status: entity.ExceptionPending, At: day(4), Seq: 6},
			{VersionID: renewal, Status: entity.ExceptionApproved, At: day(7), Seq: 8},
			{VersionID: renewal, Status: entity.ExceptionWithdrawn, At: day(8), Seq: 9},
		},
	}
}

func TestEffective_ScopeIntervalAndKnowledge(t *testing.T) {
	t.Parallel()
	r := decisionFixture()
	tests := []struct {
		name string
		at   time.Time
		seq  int64
		want int
	}{
		{"before required approval", day(2), 9, 0},
		{"approval not in knowledge cut", day(4), 3, 0},
		{"approval event inclusive", day(3), 4, 1},
		{"pending renewal keeps previous version", day(4), 6, 1},
		{"previous version expires exactly at boundary", day(5), 6, 0},
		{"new approval after previous expiry", day(7), 8, 2},
		{"old knowledge omits withdrawal", day(8), 8, 2},
		{"withdrawn at event time", day(8), 9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Effective(&r, tt.seq, tt.at)
			if tt.want == 0 {
				require.Nil(t, v)
				return
			}
			require.NotNil(t, v)
			assert.Equal(t, tt.want, v.Number)
			f := entity.FindingCoordinates{
				FindingAccess: entity.FindingAccess{AreaID: v.Scope.AreaID, Kind: "security"},
				RuleID:        "finding-rule", Path: "src/billing/a.go",
			}
			assert.True(t, v.Matches(f))
			f.Path = "src/billing-extra/a.go"
			assert.False(t, v.Matches(f), "a sibling path is outside the decision")
			f.Path, f.AreaID = "src/billing/a.go", entity.ID{5}
			assert.False(t, v.Matches(f))
		})
	}
}

func TestCheckDraft_NormalizesTextAndRejectsBroadenedPaths(t *testing.T) {
	t.Parallel()
	u := New(Deps{})
	d := entity.ExceptionDraft{
		Type: entity.ExceptionAcceptedRisk, Reason: "  reviewed  ", EffectiveFrom: day(1), ExpiresAt: day(2),
		Scope: entity.ExceptionScope{AreaID: entity.ID{1}, RuleID: " rule ", PathPrefix: "src/billing/"},
	}
	require.NoError(t, u.CheckDraft(&d))
	assert.Equal(t, "reviewed", d.Reason)
	assert.Equal(t, "rule", d.Scope.RuleID)
	assert.Equal(t, "src/billing", d.Scope.PathPrefix)

	d.Reason, d.ExpiresAt, d.Scope.PathPrefix, d.Scope.RuleID = strings.Repeat("я", 4001), d.EffectiveFrom, "../billing", ""
	var invalid *entity.ValidationError
	require.ErrorAs(t, u.CheckDraft(&d), &invalid)
	require.Len(t, invalid.Fields, 4)
}

func TestCheckTransition_ReviewerAndAuthor(t *testing.T) {
	t.Parallel()
	author, reviewer := entity.ID{1}, entity.ID{2}
	v := entity.ExceptionVersion{AuthorID: author, ApprovalRequired: true, ExceptionDraft: entity.ExceptionDraft{ExpiresAt: day(5)}}
	assert.ErrorIs(t, checkTransition(&v, author, entity.ExceptionApprove, entity.ExceptionPending, day(3)), entity.ErrForbidden)
	assert.NoError(t, checkTransition(&v, reviewer, entity.ExceptionApprove, entity.ExceptionPending, day(3)))
	assert.ErrorIs(t, checkTransition(&v, reviewer, entity.ExceptionApprove, entity.ExceptionRejected, day(3)), entity.ErrStateConflict)
	assert.ErrorIs(t, checkTransition(&v, reviewer, entity.ExceptionApprove, entity.ExceptionPending, day(5)), entity.ErrStateConflict)
	assert.ErrorIs(t, checkTransition(&v, reviewer, entity.ExceptionWithdraw, entity.ExceptionApproved, day(3)), entity.ErrForbidden)
	assert.NoError(t, checkTransition(&v, author, entity.ExceptionWithdraw, entity.ExceptionApproved, day(3)))
}

type expiryRepo struct {
	repo.DecisionRepo
	record entity.ExceptionRecord
}

func (r *expiryRepo) Decision(_ context.Context, _, _ entity.ID, _ bool) (entity.ExceptionRecord, error) {
	return r.record, nil
}

func (r *expiryRepo) SetWake(_ context.Context, _, _ entity.ID, wake *time.Time) error {
	r.record.WakeAt = wake
	return nil
}

type expiryTx struct{ repo.Transactor }

func (expiryTx) InOrgTx(ctx context.Context, _ entity.ID, work func(context.Context) error) error {
	return work(ctx)
}

type expiryQueue struct {
	recounts []string
	due      time.Time
}

func (q *expiryQueue) Enqueue(_ context.Context, job entity.Job, _ string) error {
	q.recounts = append(q.recounts, job.Kind())
	return nil
}

func (q *expiryQueue) EnqueueNearest(_ context.Context, _ entity.Job, _ string, wake time.Time) error {
	q.due = wake
	return nil
}

func TestRunExpire_QueuesBothRecountsOnce(t *testing.T) {
	t.Parallel()
	rec := decisionFixture()
	// The renewal is still pending at the first expiration.
	rec.Transitions = rec.Transitions[:3]
	rec.WakeAt = nextWake(&rec, day(4))
	require.NotNil(t, rec.WakeAt)
	assert.Equal(t, day(5), *rec.WakeAt)
	r, q := &expiryRepo{record: rec}, &expiryQueue{}
	clk := clock.NewManual(day(5))
	u := New(Deps{Repo: r, Tx: expiryTx{}, Queue: q, Clock: clk, NewID: func() (entity.ID, error) { return entity.ID{7}, nil }})
	job := entity.DecisionExpireJob{OrganizationID: entity.ID{8}, DecisionID: rec.ID}
	complete := func(context.Context) error { return nil }
	require.NoError(t, u.RunExpire(t.Context(), job, complete))
	assert.Equal(t, []string{entity.KindCalcRecount, entity.KindChangeRecount}, q.recounts)
	assert.Equal(t, day(9), q.due)
	assert.Nil(t, Effective(&r.record, math.MaxInt64, day(5)))

	require.NoError(t, u.RunExpire(t.Context(), job, complete))
	assert.Len(t, q.recounts, 2)
}
