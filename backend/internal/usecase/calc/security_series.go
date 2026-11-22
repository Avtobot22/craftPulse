package calc

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_scopeSecurity       = "security"
	_inputSecurityReport = "security_report"
)

type securitySeriesScope struct {
	Kind         string    `json:"kind"`
	RepositoryID entity.ID `json:"repository_id"`
	ComponentID  entity.ID `json:"component_id"`
	SHA          string    `json:"sha"`
}

func validateSecurityScope(scope entity.SecurityScope) error {
	if scope.AreaID.IsZero() || !_reportSHA.MatchString(scope.SHA) {
		return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	return nil
}

// SecuritySeries identifies the scope of published security revisions at an exact commit.
func (u *UseCase) SecuritySeries(ctx context.Context, org entity.ID, scope entity.SecurityScope) (entity.CalcSeries, error) {
	if err := validateSecurityScope(scope); err != nil {
		return entity.CalcSeries{}, err
	}
	s := securitySeriesScope{Kind: _scopeSecurity, RepositoryID: scope.AreaID, SHA: scope.SHA}
	if scope.ComponentID != nil {
		s.ComponentID = *scope.ComponentID
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode security scope: %w", err)
	}
	return u.ensureScopedSeries(ctx, org, raw, entity.MetricSetSecurity, entity.StepDay)
}

func (s seriesScope) securityAt(seq int64) entity.SecurityScope {
	scope := entity.SecurityScope{AreaID: s.RepositoryID, SHA: s.SHA, KnownSeq: seq}
	if !s.ComponentID.IsZero() {
		scope.ComponentID = &s.ComponentID
	}
	return scope
}

type securitySource struct {
	series   *entity.CalcSeries
	window   entity.Window
	revision entity.SecurityRevision
}

func (u *UseCase) securitySource(ctx context.Context, org entity.ID, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope entity.SecurityScope,
) (*securitySource, error) {
	r, err := u.SecurityAt(ctx, org, scope)
	return &securitySource{series: series, window: rev.Window, revision: r}, err
}

func (s *securitySource) periods() []period {
	return []period{{start: s.window.From, end: s.window.To}}
}

func (s *securitySource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetSecurity)
	d.bytes(s.series.ScopeHash)
	d.time(from)
	d.time(to)
	for _, in := range s.revision.Definitions {
		d.text(in.ID)
		d.text(in.Version)
	}
	for i := range s.revision.Reports {
		d.id(s.revision.Reports[i].ID)
	}
	return d.sum()
}

func (s *securitySource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetSecurity, From: from, To: to}
	for _, p := range ps {
		for i := range _securityDefinitions {
			def := &_securityDefinitions[i]
			m := s.revision.Metrics[def.ID]
			point := s.point(p, &m)
			out.Points = append(out.Points, entity.SeriesPoint{MetricID: def.ID, Unit: def.Unit, Point: point})
			if def.ID == "S19" {
				out.Points = append(out.Points, s.secretPoints(p, &m, def)...)
			}
		}
	}
	return out
}

func (s *securitySource) point(p period, m *entity.SecurityMetric) entity.Point {
	out := entity.Point{
		Start: p.start, End: p.end, State: m.State, ReasonCode: m.ReasonCode, Value: m.Value,
		Contributions: make([]entity.Contribution, 0, len(m.Contributions)),
	}
	if m.Numerator != nil {
		out.Numerator = num(float64(*m.Numerator))
	}
	if m.Denominator != nil {
		out.Denominator = num(float64(*m.Denominator))
	}
	for i := range m.Contributions {
		c := &m.Contributions[i]
		out.Contributions = append(out.Contributions, s.contribution(c))
		switch c.Role {
		case entity.RoleNumerator, entity.RoleDenominator:
		case entity.RoleExcluded:
			out.ExcludedCount++
		case entity.RoleMissing:
			out.MissingCount++
		}
	}
	return out
}

func (s *securitySource) secretPoints(p period, m *entity.SecurityMetric, def *Definition) []entity.SeriesPoint {
	out := make([]entity.SeriesPoint, 0, len(m.Groups))
	for _, g := range m.Groups {
		scoped := entity.SecurityMetric{State: g.State, ReasonCode: g.ReasonCode}
		if g.Count != nil {
			scoped.Value = num(float64(*g.Count))
		}
		for i := range m.Contributions {
			if c := &m.Contributions[i]; c.Scope == g.Scope {
				scoped.Contributions = append(scoped.Contributions, *c)
			}
		}
		out = append(out, entity.SeriesPoint{MetricID: def.ID, Unit: def.Unit, Estimator: g.Scope, Point: s.point(p, &scoped)})
	}
	return out
}

func (s *securitySource) contribution(c *entity.SecurityContribution) entity.Contribution {
	id, kind := c.Findings[0].ID, entity.ObjectSecurityFinding
	if c.Package != nil || c.AdvisoryID != "" {
		d := newDigest()
		d.bytes(s.series.ScopeHash)
		if p := c.Package; p != nil {
			kind = entity.ObjectAffectedPackage
			d.text(string(kind))
			for _, value := range []string{p.PURL, p.Name, p.Version, p.Ecosystem, p.Lockfile, p.Context, p.PackagePath} {
				d.text(value)
			}
		} else {
			kind = entity.ObjectSecurityAdvisory
			d.text(string(kind))
			d.text(c.AdvisoryID)
		}
		id = entity.ID(d.sum()[:16])
	}
	raw, _ := json.Marshal(c) // Canonical evidence is validated JSON from the report parser or NULL.
	out := entity.Contribution{ObjectID: id, ObjectKind: kind, ItemNo: 1, Role: c.Role, ReasonCode: c.ReasonCode, Evidence: raw}
	if c.Role == entity.RoleNumerator || c.Role == entity.RoleDenominator {
		out.Value = num(1)
	}
	return out
}

func securityInputs(reports []entity.ReportEvidence) []entity.RevisionInput {
	inputs := make([]entity.RevisionInput, 0, len(reports))
	for i := range reports {
		r := &reports[i]
		inputs = append(inputs, entity.RevisionInput{
			Kind: _inputSecurityReport, ID: r.ID.String(), Version: strconv.FormatInt(r.KnownSeq, 10),
			Completeness: r.Completeness,
		})
	}
	return inputs
}

func (u *UseCase) recountSecurity(ctx context.Context, job entity.CalcRecountJob, complete Complete) error {
	org := job.OrganizationID
	if u.d.Security == nil {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	scopes, err := u.d.Security.ReceiptSecurityScopes(ctx, org, job.SecurityReceiptID)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now().UTC()
	from := now.Truncate(24 * time.Hour)
	plans := make([]due, 0, len(scopes))
	for _, scope := range scopes {
		s, err := u.SecuritySeries(ctx, org, scope)
		if err != nil {
			return err
		}
		plans = append(plans, due{at: now, job: entity.CalcSliceJob{
			OrganizationID: org, SeriesID: s.ID,
			From: from, To: from.AddDate(0, 0, 1), AsOf: now, CorrelationID: job.CorrelationID,
		}})
	}
	return u.queue(ctx, org, plans, complete)
}

func (u *UseCase) againSecurity(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, scope seriesScope,
	previous []entity.ReportEvidence,
) error {
	seq, _, err := u.d.Tenants.Clock(ctx, job.OrganizationID)
	if err != nil || seq <= rev.KnowledgeSeq {
		return err
	}
	reports, err := u.d.Security.SecurityReports(ctx, job.OrganizationID, scope.securityAt(seq))
	if err != nil || slices.EqualFunc(previous, reports, func(a, b entity.ReportEvidence) bool { return a.ID == b.ID }) {
		return err
	}
	job.RevisionID, job.PartNo, job.AsOf = entity.ID{}, 0, u.d.Clock.Now()
	return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}
