package calc

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// TargetBaselines is the historical p85 of the cycle time T08 of every group «type, size» of a team over the
// weeks of the period, built from the stored contributions of the current revision of its weekly series
// (FR-019). It is a baseline, not a target: below 20 tasks it is a small sample. Tasks without a type are in no
// group; tasks without a size make the group with a nil size and size_unknown.
func (u *UseCase) TargetBaselines(ctx context.Context, s entity.Subject, teamID entity.ID, w entity.Window) (entity.TargetBaselines, error) {
	if !w.From.Before(w.To) {
		return entity.TargetBaselines{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "to", Code: entity.FieldOutOfRange, Detail: "to is after from"}}}
	}
	org := s.OrganizationID
	var out entity.TargetBaselines
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		rev, err := u.weeklyRevision(ctx, org, teamID)
		if err != nil {
			return err
		}
		samples, err := u.d.Repo.PointSamples(ctx, org, rev, "T08", EstimatorP85, w)
		if err != nil {
			return err
		}
		out = entity.TargetBaselines{RevisionID: rev, Items: baselines(samples, w)}
		return nil
	})
	return out, err
}

// CreateTargetVersion confirms the target of a group (UC-09, FR-019). A historical p85 is read from the revision
// and the period of its baseline first; then process saves the version in one transaction with the
// recalculation of the series of the team (TZ 10.3, rule 2). created is false for a form equal to the version in
// force.
func (u *UseCase) CreateTargetVersion(ctx context.Context, s entity.Subject, teamID entity.ID, baseVersion int, key entity.ID,
	d entity.TargetDraft,
) (v entity.TargetVersion, created bool, err error) {
	if err := u.d.Settings.CheckTargetDraft(d); err != nil {
		return entity.TargetVersion{}, false, err
	}
	org := s.OrganizationID
	var basis *entity.TargetBasis
	if d.Basis == entity.TargetHistoricalP85 {
		b, err := u.basis(ctx, org, teamID, *d.BasisRevisionID, *d.BasisPeriod, d.TargetGroup)
		if err != nil {
			return entity.TargetVersion{}, false, err
		}
		basis = &b
	}
	var res entity.SaveResult
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		v, res, err = u.d.Settings.ConfirmTargetVersion(ctx, s, teamID, baseVersion, key, d, basis)
		if err != nil || res != entity.SaveInserted {
			return err
		}
		return u.recount(ctx, org, teamID)
	})
	if err != nil {
		return entity.TargetVersion{}, false, err
	}
	return v, res != entity.SaveUnchanged, nil
}

// weeklyRevision is the current revision of the weekly series of the team; ErrNotFound before the first one.
func (u *UseCase) weeklyRevision(ctx context.Context, org, teamID entity.ID) (entity.ID, error) {
	_, hash, err := teamScopeOf(teamID)
	if err != nil {
		return entity.ID{}, err
	}
	all, err := u.d.Repo.ScopeSeries(ctx, org, hash, entity.MetricSetFlow)
	if err != nil {
		return entity.ID{}, err
	}
	i := slices.IndexFunc(all, func(s entity.CalcSeries) bool { return s.Step == entity.StepWeek && s.CurrentRevisionID != nil })
	if i < 0 {
		return entity.ID{}, entity.ErrNotFound
	}
	return *all[i].CurrentRevisionID, nil
}

// basis is the historical p85 of the group over the period within the window of a published revision of the team.
func (u *UseCase) basis(ctx context.Context, org, teamID, revisionID entity.ID, period entity.Window, g entity.TargetGroup) (entity.TargetBasis, error) {
	unknown := &entity.ValidationError{Fields: []entity.FieldError{{
		Field: "/basis_revision_id", Code: entity.FieldUnknownReference, Detail: "no published revision of the team with this ID",
	}}}
	var out entity.TargetBasis
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		rev, err := u.d.Repo.Revision(ctx, org, revisionID)
		if errors.Is(err, entity.ErrNotFound) {
			return unknown
		}
		if err != nil {
			return err
		}
		series, err := u.d.Repo.Series(ctx, org, rev.SeriesID)
		if err != nil {
			return err
		}
		team, err := scopeTeam(series.Scope)
		if err != nil || team != teamID || series.MetricSet != entity.MetricSetFlow || rev.PublishedAt == nil {
			return unknown
		}
		w := entity.Window{From: period.From.UTC(), To: period.To.UTC()}
		if w.From.Before(rev.Window.From) {
			w.From = rev.Window.From
		}
		if w.To.After(rev.Window.To) {
			w.To = rev.Window.To
		}
		samples, err := u.d.Repo.PointSamples(ctx, org, rev.ID, "T08", EstimatorP85, w)
		if err != nil {
			return err
		}
		items := baselines(samples, w)
		i := slices.IndexFunc(items, func(b entity.TargetBaseline) bool {
			return b.TaskType == g.TaskType && b.SizeLabel != nil && *b.SizeLabel == g.SizeLabel
		})
		if i < 0 || items[i].P85.Value == nil {
			return &entity.ValidationError{Fields: []entity.FieldError{{
				Field: "/basis_period", Code: entity.FieldOutOfRange, Detail: "the group has no p85 in the period of the revision",
			}}}
		}
		out = entity.TargetBasis{RevisionID: rev.ID, ValueSeconds: int64(math.Round(*items[i].P85.Value)), N: *items[i].P85.N, Window: w}
		return nil
	})
	return out, err
}

// recount queues a new revision of every series of the team with a current one, over its window at now, in
// the transaction of the caller; the earlier revisions keep the settings they were computed with.
func (u *UseCase) recount(ctx context.Context, org, teamID entity.ID) error {
	_, hash, err := teamScopeOf(teamID)
	if err != nil {
		return err
	}
	all, err := u.d.Repo.ScopeSeries(ctx, org, hash, entity.MetricSetFlow)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now().UTC()
	for _, s := range all {
		if s.CurrentRevisionID == nil {
			continue
		}
		rev, err := u.d.Repo.Revision(ctx, org, *s.CurrentRevisionID)
		if err != nil {
			return err
		}
		job := entity.CalcSliceJob{
			OrganizationID: org, SeriesID: s.ID, From: rev.Window.From, To: rev.Window.To, AsOf: now,
			CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		}
		if err := u.d.Queue.Enqueue(ctx, job, entity.QueueBulk); err != nil {
			return err
		}
	}
	return nil
}

// baselines group the contributions of T08 p85 by type and size: every task counts once, its cycle when it is a
// numerator, and a point of p85 is built over the period as the definition builds it (BR-006).
func baselines(samples []entity.PointSample, w entity.Window) []entity.TargetBaseline {
	type key struct {
		taskType string
		size     string
		sized    bool
	}
	type group struct {
		key
		tally  *tally
		seen   map[entity.ID]bool
		sample []float64
	}
	var groups []*group
	byKey := map[key]*group{}
	for _, s := range samples {
		if s.TaskType == nil {
			continue
		}
		k := key{taskType: *s.TaskType, sized: s.SizeLabel != nil}
		if k.sized {
			k.size = *s.SizeLabel
		}
		g, ok := byKey[k]
		if !ok {
			g = &group{key: k, tally: newTally(period{start: w.From, end: w.To}), seen: map[entity.ID]bool{}}
			byKey[k] = g
			groups = append(groups, g)
		}
		if g.seen[s.TaskID] {
			continue
		}
		g.seen[s.TaskID] = true
		g.tally.add(s.TaskID, 1, s.Role, s.Value, s.ReasonCode)
		if s.Role == entity.RoleNumerator && s.Value != nil {
			g.sample = append(g.sample, *s.Value)
		}
	}
	slices.SortFunc(groups, func(a, b *group) int {
		return cmp.Or(cmp.Compare(a.taskType, b.taskType), compareLabels(label(a.sized, a.size), label(b.sized, b.size)))
	})
	out := make([]entity.TargetBaseline, len(groups))
	for i, g := range groups {
		p := g.tally.distribution(g.sample, []string{EstimatorP85})[0]
		p.Contributions = nil
		if !g.sized {
			p.ReasonCode = entity.ReasonSizeUnknown
		}
		out[i] = entity.TargetBaseline{TaskType: g.taskType, SizeLabel: label(g.sized, g.size), Unit: UnitSeconds, P85: p}
	}
	return out
}

func label(ok bool, s string) *string {
	if !ok {
		return nil
	}
	return &s
}
