package calc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _scopeDocumentation = "documentation"

type Documentation interface {
	CalculationInventoryAt(ctx context.Context, org entity.ID, scope entity.DocumentationScope, seq int64) (*entity.DocumentationInventory, error)
	AreaRoots(ctx context.Context, org entity.ID, areas []entity.ID) ([]entity.DocumentationRoot, error)
}

type DocumentationReports interface {
	DocumentationReportsAt(ctx context.Context, org entity.ID, root *entity.DocumentationRoot, sha string, seq int64) ([]entity.DocsReport, []entity.DocsFinding, error)
	ReceiptDocumentationRoots(ctx context.Context, org, receipt entity.ID) ([]entity.DocsReceiptRoot, error)
}

type documentationSeriesScope struct {
	Kind          string                    `json:"kind"`
	Documentation entity.DocumentationScope `json:"documentation"`
}

// DocumentationSeries finds or creates a scope at one exact Git commit.
// A zero root ID represents a service without a selected confirmed root.
func (u *UseCase) DocumentationSeries(ctx context.Context, org entity.ID, scope entity.DocumentationScope) (entity.CalcSeries, error) {
	if scope.ServiceID.IsZero() || scope.RepositoryID.IsZero() || !_reportSHA.MatchString(scope.SHA) {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	raw, err := json.Marshal(documentationSeriesScope{Kind: _scopeDocumentation, Documentation: scope})
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode documentation scope: %w", err)
	}
	return u.ensureScopedSeries(ctx, org, raw, entity.MetricSetDocumentation, entity.StepDay)
}

type documentationSource struct {
	series    *entity.CalcSeries
	window    entity.Window
	asOf      time.Time
	sha       string
	inventory *entity.DocumentationInventory
	reports   []entity.DocsReport
	findings  []entity.DocsFinding
}

func (u *UseCase) documentationSource(ctx context.Context, org entity.ID, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope entity.DocumentationScope,
) (*documentationSource, error) {
	src := &documentationSource{series: series, window: rev.Window, asOf: rev.AsOf, sha: scope.SHA}
	inv, err := u.d.Docs.CalculationInventoryAt(ctx, org, scope, rev.KnowledgeSeq)
	if err != nil || inv == nil {
		return src, err
	}
	src.inventory = inv
	src.reports, src.findings, err = u.d.DocReports.DocumentationReportsAt(ctx, org, &inv.Root, scope.SHA, rev.KnowledgeSeq)
	return src, err
}

func (s *documentationSource) periods() []period {
	return []period{{start: s.window.From, end: s.window.To}}
}

func (s *documentationSource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetDocumentation)
	d.bytes(s.series.ScopeHash)
	d.time(from)
	d.time(to)
	for i := range _documentationDefinitions {
		def := &_documentationDefinitions[i]
		d.text(def.ID)
		d.int(int64(def.Version))
	}
	d.bytes(s.factsHash())
	if len(s.findings) > 0 {
		d.time(s.asOf)
	}
	return d.sum()
}

func (s *documentationSource) factsHash() []byte {
	d := newDigest()
	if s.inventory == nil {
		return d.sum()
	}
	d.id(s.inventory.Root.ID)
	d.int(s.inventory.Root.KnownSeq)
	for i := range s.inventory.Documents {
		d.id(s.inventory.Documents[i].ID)
	}
	for i := range s.reports {
		r := &s.reports[i]
		d.id(r.ID)
		d.id(r.SARIFID)
	}
	for i := range s.findings {
		finding := &s.findings[i]
		d.id(finding.ID)
		d.time(finding.FirstSeenAt)
	}
	return d.sum()
}

func (s *documentationSource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetDocumentation, From: from, To: to}
	for _, p := range ps {
		out.Points = append(out.Points, s.points(p)...)
	}
	return out
}

func (s *documentationSource) inputs() []entity.RevisionInput {
	if s.inventory == nil {
		return nil
	}
	out := []entity.RevisionInput{{Kind: _inputDocsRoot, ID: s.inventory.Root.ID.String()}}
	for i := range s.reports {
		r := &s.reports[i]
		out = append(out, entity.RevisionInput{Kind: _inputDocsReport, ID: r.ID.String(), Completeness: r.Completeness})
		if !r.SARIFID.IsZero() {
			out = append(out, entity.RevisionInput{Kind: _inputDocsReport, ID: r.SARIFID.String(), Completeness: r.Completeness})
		}
	}
	return out
}

func (u *UseCase) recountDocumentation(ctx context.Context, job entity.CalcRecountJob, complete Complete) error {
	org := job.OrganizationID
	if u.d.Docs == nil || u.d.DocReports == nil {
		return u.d.Tx.InOrgTx(ctx, org, complete)
	}
	observed, err := u.d.DocReports.ReceiptDocumentationRoots(ctx, org, job.DocsReceiptID)
	if err != nil {
		return err
	}
	var areas []entity.ID
	for _, root := range observed {
		if !slices.Contains(areas, root.AreaID) {
			areas = append(areas, root.AreaID)
		}
	}
	roots, err := u.d.Docs.AreaRoots(ctx, org, areas)
	if err != nil {
		return err
	}
	now := u.d.Clock.Now().UTC()
	from := now.Truncate(24 * time.Hour)
	var plans []due
	for _, scope := range documentationScopes(observed, roots) {
		s, err := u.DocumentationSeries(ctx, org, scope)
		if err != nil {
			return err
		}
		plans = append(plans, due{at: now, job: entity.CalcSliceJob{
			OrganizationID: org, SeriesID: s.ID, From: from, To: from.AddDate(0, 0, 1), AsOf: now, CorrelationID: job.CorrelationID,
		}})
	}
	return u.queue(ctx, org, plans, complete)
}

func documentationScopes(observed []entity.DocsReceiptRoot, roots []entity.DocumentationRoot) []entity.DocumentationScope {
	var out []entity.DocumentationScope
	seen := map[entity.DocumentationScope]bool{}
	for _, o := range observed {
		for i := range roots {
			root := &roots[i]
			if o.AreaID != root.AreaID || o.PathPrefix != root.PathPrefix || o.ProfileID != root.ProfileID ||
				o.RootID != nil && *o.RootID != root.ID {
				continue
			}
			scope := entity.DocumentationScope{ServiceID: root.ServiceID, RootID: root.ID, RepositoryID: root.AreaID, SHA: o.SHA}
			if !seen[scope] {
				out = append(out, scope)
				seen[scope] = true
			}
		}
	}
	return out
}

func (u *UseCase) againDocumentation(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope entity.DocumentationScope, previous *documentationSource,
) error {
	org := job.OrganizationID
	seq, _, err := u.d.Tenants.Clock(ctx, org)
	if err != nil || seq <= rev.KnowledgeSeq {
		return err
	}
	latest, err := u.documentationSource(ctx, org, &entity.CalcRevision{KnowledgeSeq: seq}, series, scope)
	if err != nil || bytes.Equal(previous.factsHash(), latest.factsHash()) {
		return err
	}
	job.RevisionID, job.PartNo, job.AsOf = entity.ID{}, 0, u.d.Clock.Now()
	return u.d.Queue.Enqueue(ctx, job, entity.QueueBulk)
}
