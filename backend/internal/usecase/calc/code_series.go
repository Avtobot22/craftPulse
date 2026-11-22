package calc

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _scopeCode = "code"

var _codeSHA = regexp.MustCompile("^(?:[0-9a-f]{40}|[0-9a-f]{64})$")

type CodeReports interface {
	CodeReportAt(ctx context.Context, org entity.ID, scope entity.CodeScope, seq int64) (*entity.CodeReport, error)
	ReceiptCodeScopes(ctx context.Context, org, receipt entity.ID) ([]entity.CodeScope, error)
}

type codeSeriesScope struct {
	Kind string `json:"kind"`
	entity.CodeScope
}

// CodeSeries finds or creates the series of one code-report stream at an exact commit.
func (u *UseCase) CodeSeries(ctx context.Context, org entity.ID, scope entity.CodeScope) (entity.CalcSeries, error) {
	if scope.RepositoryID.IsZero() || !_codeSHA.MatchString(scope.SHA) ||
		(scope.Format != entity.CodeFormatLizard && scope.Format != entity.CodeFormatJscpd) {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	raw, err := json.Marshal(codeSeriesScope{Kind: _scopeCode, CodeScope: scope})
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode code scope: %w", err)
	}
	return u.ensureScopedSeries(ctx, org, raw, entity.MetricSetCode, entity.StepDay)
}

type codeSource struct {
	report *entity.CodeReport
	series *entity.CalcSeries
	window entity.Window
	format string
}

func (u *UseCase) codeSource(ctx context.Context, org entity.ID, rev *entity.CalcRevision, series *entity.CalcSeries, scope entity.CodeScope) (*codeSource, error) {
	report, err := u.d.Code.CodeReportAt(ctx, org, scope, rev.KnowledgeSeq)
	return &codeSource{report: report, series: series, window: rev.Window, format: scope.Format}, err
}

func (s *codeSource) periods() []period {
	return []period{{start: s.window.From, end: s.window.To}}
}

func (s *codeSource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetCode)
	d.bytes(s.series.ScopeHash)
	d.time(from)
	d.time(to)
	defs := codeDefinitions(s.format)
	for i := range defs {
		def := &defs[i]
		d.text(def.ID)
		d.int(int64(def.Version))
	}
	if s.report != nil {
		d.id(s.report.ID)
	}
	return d.sum()
}

func (s *codeSource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetCode, From: from, To: to}
	for _, p := range ps {
		out.Points = append(out.Points, s.points(p)...)
	}
	return out
}

func codeInputs(report *entity.CodeReport) []entity.RevisionInput {
	if report == nil {
		return nil
	}
	inputs := []entity.RevisionInput{{Kind: _inputCodeReport, ID: report.ID.String(), Completeness: report.Completeness}}
	if t := report.Facts.Thresholds; t != nil {
		inputs = append(inputs,
			entity.RevisionInput{Kind: _inputThreshold, ID: "ccn_threshold", Version: strconv.Itoa(t.CCN)},
			entity.RevisionInput{Kind: _inputThreshold, ID: "nloc_threshold", Version: strconv.Itoa(t.NLOC)})
	}
	return inputs
}

// recountCode starts a scope-specific chain after the receipt has been normalized in full.
func (u *UseCase) recountCode(ctx context.Context, job entity.CalcRecountJob, complete Complete) error {
	if u.d.Code == nil {
		return u.d.Tx.InOrgTx(ctx, job.OrganizationID, complete)
	}
	scopes, err := u.d.Code.ReceiptCodeScopes(ctx, job.OrganizationID, job.CodeReceiptID)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now().UTC()
	from := now.Truncate(24 * time.Hour)
	var plans []due
	for _, scope := range scopes {
		s, err := u.CodeSeries(ctx, job.OrganizationID, scope)
		if err != nil {
			return err
		}
		plans = append(plans, due{at: now, job: entity.CalcSliceJob{
			OrganizationID: job.OrganizationID, SeriesID: s.ID, From: from, To: from.AddDate(0, 0, 1), AsOf: now,
			CorrelationID: job.CorrelationID,
		}})
	}
	return u.queue(ctx, job.OrganizationID, plans, complete)
}

func (u *UseCase) againCode(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, scope entity.CodeScope, previous *entity.CodeReport) error {
	seq, _, err := u.d.Tenants.Clock(ctx, job.OrganizationID)
	if err != nil || seq <= rev.KnowledgeSeq {
		return err
	}
	latest, err := u.d.Code.CodeReportAt(ctx, job.OrganizationID, scope, seq)
	if err != nil || latest == nil || (previous != nil && latest.ID == previous.ID) {
		return err
	}
	job.RevisionID, job.PartNo, job.AsOf = entity.ID{}, 0, u.d.Clock.Now()
	return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}
