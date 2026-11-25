package calc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _scopeAnalysis = "analysis"

type AnalysisReports interface {
	AnalysisFacts(context.Context, entity.ID, entity.AnalysisScope, int64, time.Time) (entity.AnalysisFacts, error)
	ReceiptAnalysisScopes(context.Context, entity.ID, entity.ID) ([]entity.AnalysisScope, error)
}

type analysisSeriesScope struct {
	Kind string `json:"kind"`
	entity.AnalysisScope
}

func (u *UseCase) AnalysisSeries(ctx context.Context, org entity.ID, scope entity.AnalysisScope) (entity.CalcSeries, error) {
	if scope.RepositoryID.IsZero() || !_reportSHA.MatchString(scope.SHA) {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	raw, err := json.Marshal(analysisSeriesScope{Kind: _scopeAnalysis, AnalysisScope: scope})
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode analysis scope: %w", err)
	}
	return u.ensureScopedSeries(ctx, org, raw, entity.MetricSetAnalysis, entity.StepDay)
}

func (s seriesScope) analysisScope() entity.AnalysisScope {
	return entity.AnalysisScope{RepositoryID: s.RepositoryID, ComponentID: s.ComponentID, SHA: s.SHA}
}

type analysisSource struct {
	series *entity.CalcSeries
	window entity.Window
	asOf   time.Time
	facts  entity.AnalysisFacts
}

func (u *UseCase) analysisSource(ctx context.Context, org entity.ID, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope entity.AnalysisScope,
) (*analysisSource, error) {
	facts, err := u.d.Analysis.AnalysisFacts(ctx, org, scope, rev.KnowledgeSeq, rev.AsOf)
	return &analysisSource{series: series, window: rev.Window, asOf: rev.AsOf, facts: facts}, err
}

func (s *analysisSource) periods() []period {
	return []period{{start: s.window.From, end: s.window.To}}
}

func (s *analysisSource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetAnalysis)
	d.bytes(s.series.ScopeHash)
	d.time(from)
	d.time(to)
	d.time(s.asOf)
	for i := range _analysisDefinitions {
		def := &_analysisDefinitions[i]
		d.text(def.ID)
		d.int(int64(def.Version))
	}
	d.bytes(s.factsHash())
	return d.sum()
}

func (s *analysisSource) factsHash() []byte {
	d := newDigest()
	for i := range s.facts.Runs {
		run := &s.facts.Runs[i]
		d.id(run.ID)
		for _, report := range run.Reports {
			d.id(report.ID)
			d.text(report.Reason)
		}
	}
	for _, v := range s.facts.Versions {
		d.id(v.ID)
		d.id(v.ReportID)
		d.time(v.ObservedAt)
	}
	return d.sum()
}

func (s *analysisSource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetAnalysis, From: from, To: to}
	for _, p := range ps {
		out.Points = append(out.Points, analysisPoints(s.asOf, p, &s.facts)...)
	}
	return out
}

func (s *analysisSource) inputs() []entity.RevisionInput {
	out := make([]entity.RevisionInput, 0, len(s.facts.Runs))
	for i := range s.facts.Runs {
		r := &s.facts.Runs[i]
		out = append(out, entity.RevisionInput{
			Kind: entity.InputAnalysisRun, ID: r.ID.String(), Version: strconv.FormatInt(r.KnownSeq, 10), Completeness: r.Completeness,
		})
	}
	return out
}

func analysisPlan(org, series, corr entity.ID, at time.Time) due {
	at = at.UTC()
	day := at.Truncate(24 * time.Hour)
	return due{at: at, job: entity.CalcSliceJob{
		OrganizationID: org, SeriesID: series, From: day, To: day.AddDate(0, 0, 1), AsOf: at, CorrelationID: corr,
	}}
}

func (u *UseCase) recountAnalysis(ctx context.Context, job entity.CalcRecountJob, complete Complete) error {
	org := job.OrganizationID
	if u.d.Analysis == nil {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	scopes, err := u.d.Analysis.ReceiptAnalysisScopes(ctx, org, job.AnalysisReceiptID)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now()
	plans := make([]due, 0, len(scopes))
	for _, scope := range scopes {
		s, err := u.AnalysisSeries(ctx, org, scope)
		if err != nil {
			return err
		}
		plans = append(plans, analysisPlan(org, s.ID, job.CorrelationID, now))
	}
	return u.queue(ctx, org, plans, complete)
}

func (u *UseCase) againAnalysis(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, scope entity.AnalysisScope,
	previous *analysisSource,
) error {
	org := job.OrganizationID
	seq, _, err := u.d.Tenants.Clock(ctx, org)
	if err != nil || seq <= rev.KnowledgeSeq {
		return err
	}
	now := u.d.Clock.Now()
	latest, err := u.analysisSource(ctx, org, &entity.CalcRevision{KnowledgeSeq: seq, AsOf: now}, previous.series, scope)
	if err != nil || bytes.Equal(previous.factsHash(), latest.factsHash()) {
		return err
	}
	return u.d.Queue.Enqueue(ctx, analysisPlan(org, job.SeriesID, job.CorrelationID, now).job, entity.QueueBulk)
}
