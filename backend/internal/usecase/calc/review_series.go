package calc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_scopeTeam       = "team"
	_scopeRepository = "repository"
)

// seriesScope is a scope of a series as calc.series.scope keeps it; a team or a repository.
type seriesScope struct {
	Kind         string    `json:"kind"`
	TeamID       entity.ID `json:"team_id"`
	RepositoryID entity.ID `json:"repository_id"`
}

type repositoryScope struct {
	Kind         string    `json:"kind"`
	RepositoryID entity.ID `json:"repository_id"`
}

func parseScope(raw []byte) (seriesScope, error) {
	s, ok, err := scopeOf(raw)
	if err == nil && !ok {
		err = fmt.Errorf("calc: scope of the series: %s", raw)
	}
	return s, err
}

// scopeOf is the scope of a series; false for a scope of another kind, which the recount leaves alone.
func scopeOf(raw []byte) (seriesScope, bool, error) {
	var s seriesScope
	if err := json.Unmarshal(raw, &s); err != nil {
		return seriesScope{}, false, fmt.Errorf("calc: scope of the series: %w", err)
	}
	return s, s.Kind == _scopeTeam || s.Kind == _scopeRepository, nil
}

// ReviewSeries finds the series of a set of the review of MRs, of the events or of the coverage of links of a team or
// a repository and creates it on the first request: the review and the links take weeks or months, the events take
// days. The coverage of links reads the mapping of a team, so it has no series of a repository.
func (u *UseCase) ReviewSeries(ctx context.Context, orgID entity.ID, scope entity.ReviewScope, set string, step entity.Step) (entity.CalcSeries, error) {
	if !validReviewStep(set, step) || (set == entity.MetricSetLinks && scope.TeamID.IsZero()) {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/step", Code: entity.FieldNotAllowedValue}}}
	}
	return u.scopedSeries(ctx, orgID, scope, set, step)
}

func (u *UseCase) scopedSeries(ctx context.Context, orgID entity.ID, scope entity.ReviewScope, set string, step entity.Step) (entity.CalcSeries, error) {
	if scope.TeamID.IsZero() == scope.RepositoryID.IsZero() {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	var (
		raw []byte
		err error
	)
	if scope.TeamID.IsZero() {
		raw, err = json.Marshal(repositoryScope{Kind: _scopeRepository, RepositoryID: scope.RepositoryID})
	} else {
		raw, err = json.Marshal(teamScope{Kind: _scopeTeam, TeamID: scope.TeamID})
	}
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode scope: %w", err)
	}
	sum := sha256.Sum256(raw)
	id, err := u.d.NewID()
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: new series id: %w", err)
	}
	var out entity.CalcSeries
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		out, err = u.d.Repo.EnsureSeries(ctx, orgID, entity.CalcSeries{ID: id, Scope: raw, ScopeHash: sum[:], MetricSet: set, Step: step})
		return err
	})
	return out, err
}

func validReviewStep(set string, step entity.Step) bool {
	switch set {
	case entity.MetricSetReview, entity.MetricSetLinks:
		return step == entity.StepWeek || step == entity.StepMonth
	case entity.MetricSetEvents:
		return step == entity.StepDay
	}
	return false
}

// scopeAreas are the source areas of a scope: of the projects of a team by the current structure, or the repository.
func (u *UseCase) scopeAreas(ctx context.Context, org entity.ID, s seriesScope) ([]entity.ID, error) {
	if s.Kind == _scopeRepository {
		return []entity.ID{s.RepositoryID}, nil
	}
	return u.teamAreas(ctx, org, s.TeamID)
}

// partSource computes the parts of a revision of one set of metrics.
type partSource interface {
	periods() []period
	hash(from, to time.Time) []byte
	stored(c []period, hash []byte, from, to time.Time) *entity.StoredPart
}

type flowSource struct {
	in     input
	pr     *prepared
	series *entity.CalcSeries
}

func (f *flowSource) periods() []period { return periods(f.in.window, f.in.asOf, f.in.location) }

func (f *flowSource) hash(from, to time.Time) []byte {
	if f.pr == nil {
		f.pr = prepare(f.in)
	}
	return f.pr.partHash(f.series.ScopeHash, f.series.Step, from, to)
}

func (f *flowSource) stored(c []period, hash []byte, from, to time.Time) *entity.StoredPart {
	if f.pr == nil {
		f.pr = prepare(f.in)
	}
	return storedPart(f.pr.calculate(c), hash, from, to)
}

type reviewSource struct {
	in     reviewInput
	series *entity.CalcSeries
}

func (r *reviewSource) periods() []period { return periods(r.in.window, r.in.asOf, r.in.location) }

// hash is SHA-256 of the definitions of the set, the scope, the step, the part and every MR and event it reads with
// the kinds of their participants and the completeness of their areas.
func (r *reviewSource) hash(from, to time.Time) []byte {
	d := newDigest()
	defs := definitionsOf(r.in.set)
	for i := range defs {
		d.text(defs[i].ID)
		d.int(int64(defs[i].Version))
	}
	d.bytes(r.series.ScopeHash)
	d.text(r.in.set)
	d.text(string(r.series.Step))
	d.time(from)
	d.time(to)
	for i := range r.in.mrs {
		m := &r.in.mrs[i]
		d.id(m.version.ID)
		d.flag(m.reviews)
		d.flag(m.complete)
		for j := range m.events {
			e := &m.events[j]
			d.id(e.ID)
			d.text(actorOf(m.kinds, e.ActorAccountID))
		}
		if m.version.AuthorAccountID != nil {
			d.text(actorOf(m.kinds, m.version.AuthorAccountID))
		}
	}
	for _, a := range r.in.events {
		d.id(a.id)
		d.text(a.actor)
	}
	return d.sum()
}

func (r *reviewSource) stored(c []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: r.in.set, From: from, To: to}
	for _, s := range r.in.series(c) {
		for j := range s.Points {
			out.Points = append(out.Points, entity.SeriesPoint{MetricID: s.MetricID, Estimator: s.Estimator, Unit: s.Unit, Point: s.Points[j]})
		}
	}
	return out
}

// source reads what the parts of a revision compute from at its knowledge cutoff, with the team whose settings and
// the areas whose facts make the chain run again.
func (u *UseCase) source(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, series *entity.CalcSeries) (partSource, seriesScope, error) {
	scope, err := parseScope(series.Scope)
	if err != nil {
		return nil, scope, err
	}
	w := entity.Window{From: rev.Window.From, To: rev.Window.To, Step: series.Step}
	if series.MetricSet == entity.MetricSetFlow {
		in, err := u.input(ctx, entity.CalcRequest{
			OrganizationID: job.OrganizationID, TeamID: scope.TeamID, KnowledgeSeq: rev.KnowledgeSeq, Window: w, AsOf: rev.AsOf, Sources: job.Sources,
		})
		return &flowSource{in: in, series: series}, scope, err
	}
	if series.MetricSet == entity.MetricSetLinks {
		src, err := u.linkSourceOf(ctx, job, rev, series, scope)
		return src, scope, err
	}
	loc, err := time.LoadLocation(rev.Timezone)
	if err != nil {
		return nil, scope, fmt.Errorf("calc: time zone %q: %w", rev.Timezone, err)
	}
	areas, err := u.scopeAreas(ctx, job.OrganizationID, scope)
	if err != nil {
		return nil, scope, err
	}
	if series.MetricSet == entity.MetricSetCI {
		src, err := u.ciSource(ctx, job, rev, series, areas, loc)
		return src, scope, err
	}
	facts := make([]entity.ReviewFacts, 0, len(areas))
	for _, area := range areas {
		f, err := u.d.Facts.ReviewFacts(ctx, job.OrganizationID, area, rev.KnowledgeSeq)
		if err != nil {
			return nil, scope, err
		}
		facts = append(facts, f)
	}
	var collection []entity.AreaCollection
	if job.Sources != nil {
		collection = *job.Sources
	}
	incomplete := incompleteAreas(collection)
	in := reviewInput{set: series.MetricSet, window: w, asOf: rev.AsOf, location: loc}
	if series.MetricSet == entity.MetricSetEvents {
		in.events = activitiesOf(facts)
	} else {
		in.mrs = reviewInputOf(facts, incomplete)
	}
	return &reviewSource{in: in, series: series}, scope, nil
}
