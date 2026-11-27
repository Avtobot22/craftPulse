package calc

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonDocsRootMissing         = "documentation_root_missing"
	_reasonDocsReportMissing       = "documentation_report_missing"
	_reasonDocsReportPartial       = "documentation_report_partial"
	_reasonDocsInventoryIncomplete = "documentation_inventory_incomplete"
	_reasonDocsFindingMismatch     = "documentation_finding_count_mismatch"
	_reasonDocsOwnerMissing        = "documentation_root_owner_unassigned"
	_reasonDocsOwnerRuleMissing    = "documentation_owner_rule_missing"
	_reasonDocsCheckDisabled       = "documentation_check_disabled"
)

type documentationPoints struct {
	roots, documents, coverage          entity.Point
	rootOwner, docOwners                entity.Point
	links, validity, spectral, findings entity.Point
}

func (s *documentationSource) points(p period) []entity.SeriesPoint {
	if s.sha == "" {
		return wikiPoints(p, s.wiki)
	}
	unknown := entity.Point{Start: p.start, End: p.end, State: entity.StateUnknown, ReasonCode: _reasonDocsRootMissing}
	if s.inventory != nil {
		unknown.ReasonCode = _reasonDocsReportMissing
	}
	values := documentationPoints{unknown, unknown, unknown, unknown, unknown, unknown, unknown, unknown, unknown}
	if s.inventory != nil {
		checks := s.enabledChecks()
		var docs, openapi *entity.DocsReport
		if slices.Contains(checks, entity.AnalysisDocs) {
			docs = s.reportOf(entity.AnalysisDocs)
		}
		if slices.Contains(checks, entity.AnalysisOpenAPI) {
			openapi = s.reportOf(entity.AnalysisOpenAPI)
		}
		s.coverageAndOwners(&values, docs, openapi)
		s.linkPoint(&values.links, docs)
		s.specPoints(&values, openapi)
		s.debtPoint(&values.findings, checks)
	}
	return []entity.SeriesPoint{
		{MetricID: "D01", Estimator: _estDocRoots, Unit: UnitCount, Point: values.roots},
		{MetricID: "D01", Estimator: _estDocRead, Unit: UnitCount, Point: values.documents},
		{MetricID: "D01", Estimator: _estDocCoverage, Unit: UnitRatio, Point: values.coverage},
		{MetricID: "D03", Estimator: _estRootOwner, Unit: UnitRatio, Point: values.rootOwner},
		{MetricID: "D03", Estimator: _estDocOwners, Unit: UnitRatio, Point: values.docOwners},
		{MetricID: "D10", Unit: UnitRatio, Point: values.links},
		{MetricID: "D24", Unit: UnitRatio, Point: values.validity},
		{MetricID: "D24", Estimator: _estSpectral, Unit: UnitCount, Point: values.spectral},
		{MetricID: "D30", Unit: UnitCount, Point: values.findings},
	}
}

func (s *documentationSource) reportOf(kind string) *entity.DocsReport {
	i := slices.IndexFunc(s.reports, func(r entity.DocsReport) bool { return r.Kind == kind })
	if i < 0 {
		return nil
	}
	return &s.reports[i]
}

func (s *documentationSource) coverageAndOwners(p *documentationPoints, docs, openapi *entity.DocsReport) {
	root := &s.inventory.Root
	rootEvidence := struct {
		Scope      entity.DocumentationScope `json:"scope"`
		PathPrefix string                    `json:"path_prefix"`
		Owner      *entity.ID                `json:"owner_person_id"`
	}{
		Scope:      entity.DocumentationScope{ServiceID: root.ServiceID, RootID: root.ID, RepositoryID: root.AreaID, SHA: s.sha},
		PathPrefix: root.PathPrefix, Owner: root.OwnerPersonID,
	}
	docsValue(&p.roots, 1)
	p.roots.Contributions = []entity.Contribution{docsContribution(root.ID, entity.ObjectDocumentationRoot, entity.RoleNumerator, num(1), rootEvidence)}
	var owner float64
	role := entity.RoleDenominator
	if root.OwnerPersonID != nil {
		owner, role = 1, entity.RoleNumerator
	}
	docsValue(&p.rootOwner, owner)
	if owner == 0 {
		p.rootOwner.ReasonCode = _reasonDocsOwnerMissing
	}
	p.rootOwner.Contributions = []entity.Contribution{docsContribution(root.ID, entity.ObjectDocumentationRoot, role, num(owner), rootEvidence)}

	basis := docs
	if basis == nil {
		basis = openapi
	}
	if basis == nil {
		return
	}
	members := make(map[entity.ID]bool, len(basis.Documents))
	for i := range basis.Documents {
		members[basis.Documents[i].ID] = true
	}
	var read, eligible, owners int
	for i := range s.inventory.Documents {
		doc := &s.inventory.Documents[i]
		if !members[doc.ID] {
			continue
		}
		read++
		p.documents.Contributions = append(p.documents.Contributions, documentContribution(doc, entity.RoleNumerator))
		if doc.Format != "markdown" || docs == nil || docs.OwnerRule == "" {
			continue
		}
		eligible++
		role := entity.RoleDenominator
		if doc.Owner != "" {
			owners++
			role = entity.RoleNumerator
		}
		p.docOwners.Contributions = append(p.docOwners.Contributions, documentContribution(doc, role))
	}
	docsValue(&p.documents, float64(read))
	docsCompleteness(&p.documents, basis)
	if docs == nil || !docsComplete(docs) || read != len(basis.Documents) {
		p.coverage.ReasonCode, p.docOwners.ReasonCode = _reasonDocsInventoryIncomplete, _reasonDocsInventoryIncomplete
		return
	}
	docsRatio(&p.coverage, read, read, "no_eligible_documents", docs)
	p.coverage.Contributions = p.documents.Contributions
	if docs.OwnerRule == "" {
		p.docOwners.ReasonCode = _reasonDocsOwnerRuleMissing
		return
	}
	docsRatio(&p.docOwners, owners, eligible, "no_documents_eligible_for_owner_rule", docs)
}

func (s *documentationSource) linkPoint(p *entity.Point, r *entity.DocsReport) {
	if r == nil {
		s.checkUnavailable(p, entity.AnalysisDocs)
		return
	}
	if r.Links == nil || r.SARIFID.IsZero() {
		return
	}
	n := 0
	for i := range s.findings {
		f := &s.findings[i]
		if f.ReportID != r.SARIFID || !f.HasLinkTarget || (f.Rule != entity.DocsRuleMissingFile && f.Rule != entity.DocsRuleMissingAnchor) {
			continue
		}
		n++
		p.Contributions = append(p.Contributions, s.findingContribution(f))
	}
	if n > r.Links.Checked {
		p.ReasonCode = _reasonDocsFindingMismatch
		return
	}
	docsRatio(p, n, r.Links.Checked, "no_checked_local_links", r)
	p.Contributions = append(p.Contributions, docsContribution(r.ID, entity.ObjectDocumentationReport, entity.RoleDenominator, num(float64(r.Links.Checked)), r.Links))
	if p.Value != nil && n != r.Links.Broken {
		p.State, p.ReasonCode = entity.StateProvisional, _reasonDocsFindingMismatch
	}
}

func (s *documentationSource) specPoints(p *documentationPoints, r *entity.DocsReport) {
	if r == nil {
		s.checkUnavailable(&p.validity, entity.AnalysisOpenAPI)
		s.checkUnavailable(&p.spectral, entity.AnalysisOpenAPI)
		return
	}
	var valid, checked int
	for i := range r.Documents {
		d := &r.Documents[i]
		if d.SpecValid == nil {
			continue
		}
		checked++
		role := entity.RoleDenominator
		if *d.SpecValid {
			valid++
			role = entity.RoleNumerator
		}
		p.validity.Contributions = append(p.validity.Contributions, docsContribution(d.ID, entity.ObjectDocument, role, num(1), d))
	}
	docsRatio(&p.validity, valid, checked, "no_checked_specs", r)
	if !r.SARIFID.IsZero() {
		n := s.findingCohort(&p.spectral, r.SARIFID)
		docsValue(&p.spectral, float64(n))
		docsCompleteness(&p.spectral, r)
	}
}

func (s *documentationSource) debtPoint(p *entity.Point, checks []string) {
	var count, complete, observed int
	for _, kind := range checks {
		r := s.reportOf(kind)
		if r == nil || r.SARIFID.IsZero() {
			continue
		}
		observed++
		if docsComplete(r) {
			complete++
		}
		count += s.findingCohort(p, r.SARIFID)
	}
	if observed == 0 {
		return
	}
	docsValue(p, float64(count))
	if complete != len(checks) {
		p.State, p.ReasonCode = entity.StateProvisional, _reasonDocsReportPartial
		p.MissingCount = len(checks) - complete
	}
}

func (s *documentationSource) enabledChecks() []string {
	var latest *entity.DocsReport
	for i := range s.reports {
		r := &s.reports[i]
		if len(r.Checks) != 0 && (latest == nil || r.ObservedAt.After(latest.ObservedAt) ||
			r.ObservedAt.Equal(latest.ObservedAt) && r.KnownSeq > latest.KnownSeq) {
			latest = r
		}
	}
	if latest != nil {
		return latest.Checks
	}
	out := make([]string, 0, len(s.reports))
	for i := range s.reports {
		out = append(out, s.reports[i].Kind)
	}
	return out
}

func (s *documentationSource) checkUnavailable(p *entity.Point, kind string) {
	if slices.ContainsFunc(s.reports, func(r entity.DocsReport) bool { return len(r.Checks) != 0 }) &&
		!slices.Contains(s.enabledChecks(), kind) {
		p.State, p.ReasonCode = entity.StateNotApplicable, _reasonDocsCheckDisabled
	}
}

func (s *documentationSource) findingCohort(p *entity.Point, report entity.ID) int {
	n := 0
	for i := range s.findings {
		f := &s.findings[i]
		if f.ReportID == report {
			n++
			p.Contributions = append(p.Contributions, s.findingContribution(f))
		}
	}
	return n
}

func (s *documentationSource) findingContribution(f *entity.DocsFinding) entity.Contribution {
	evidence := struct {
		*entity.DocsFinding
		VersionRef        string     `json:"version_ref"`
		DocumentVersionID *entity.ID `json:"document_version_id,omitempty"`
		AgeSeconds        int64      `json:"age_seconds"`
	}{DocsFinding: f, VersionRef: s.sha, AgeSeconds: max(0, int64(s.asOf.Sub(f.FirstSeenAt)/time.Second))}
	at, found := slices.BinarySearchFunc(s.inventory.Documents, f.Path, func(d entity.DocumentVersion, p string) int {
		return strings.Compare(d.Path, p)
	})
	if found {
		evidence.DocumentVersionID = &s.inventory.Documents[at].ID
	}
	return docsContribution(f.ID, entity.ObjectDocumentationFinding, entity.RoleNumerator, num(1), evidence)
}

func documentContribution(doc *entity.DocumentVersion, role entity.ContributionRole) entity.Contribution {
	return docsContribution(doc.ID, entity.ObjectDocument, role, num(1), doc)
}

func docsContribution(id entity.ID, kind entity.ObjectKind, role entity.ContributionRole, value *float64, details any) entity.Contribution {
	raw, _ := json.Marshal(details) // values consist of validated coordinates and database JSON
	return entity.Contribution{ObjectID: id, ObjectKind: kind, ItemNo: 1, Role: role, Value: value, Evidence: raw}
}

func docsValue(p *entity.Point, value float64) {
	p.State, p.ReasonCode, p.Value = entity.StateAssessed, "", num(value)
}

func docsComplete(r *entity.DocsReport) bool {
	return r.Completeness == "complete" && r.Skipped == 0
}

func docsCompleteness(p *entity.Point, r *entity.DocsReport) {
	if !docsComplete(r) && p.Value != nil {
		p.State, p.ReasonCode, p.MissingCount = entity.StateProvisional, _reasonDocsReportPartial, max(r.Skipped, 1)
	}
}

func docsRatio(p *entity.Point, numerator, denominator int, empty string, r *entity.DocsReport) {
	p.Numerator, p.Denominator = num(float64(numerator)), num(float64(denominator))
	if denominator == 0 {
		if docsComplete(r) {
			p.State, p.ReasonCode = entity.StateNotApplicable, empty
		} else {
			p.ReasonCode = _reasonDocsReportPartial
		}
		return
	}
	docsValue(p, float64(numerator)/float64(denominator))
	docsCompleteness(p, r)
}
