package calc

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	FindingNew           FindingDisposition = "new"
	FindingPersisting    FindingDisposition = "persisting"
	FindingResolved      FindingDisposition = "resolved"
	FindingUncertain     FindingDisposition = "uncertain"
	FindingNewlyObserved FindingDisposition = "newly_observed"

	FindingFromDatabase FindingFactor = "database_changed"
	FindingFromRules    FindingFactor = "rules_changed"
	FindingFromScope    FindingFactor = "scope_changed"

	_objectFinding       entity.ObjectKind = "finding"
	_objectException     entity.ObjectKind = "exception"
	_objectFindingReport entity.ObjectKind = "finding_report"
)

type FindingDisposition string

type FindingFactor string

// FindingScan is a report selected for one analysis scope, with the repository and commit it scanned.
type FindingScan struct {
	entity.ReportEvidence
	AreaID entity.ID
	SHA    string
}

// ClassifiedFinding is the result of comparing canonical occurrences. A resolved occurrence comes from the base.
type ClassifiedFinding struct {
	entity.FindingCoordinates
	ReportID       entity.ID
	OccurrenceID   entity.ID
	SourceID       entity.ID
	Class          string
	NativeSeverity string
	LineStart      *int
	Disposition    FindingDisposition
	Factor         FindingFactor
}

// FindingMetricsInput covers one component and finding kind of a selected analysis scope.
// SameArtifact can be set when matching artifact digests prove the same code at different SHAs.
type FindingMetricsInput struct {
	OrganizationID entity.ID
	AreaID         entity.ID
	ComponentID    entity.ID
	Kind           string
	AsOf           time.Time
	KnownSeq       int64
	HeadSHA        string
	BaseSHA        string
	Head           *FindingScan
	Base           *FindingScan
	// IncomparableReason is set by the comparison of tool, rules, configuration, database and scope.
	IncomparableReason string
	SameArtifact       bool
	Findings           []ClassifiedFinding
}

// FindingDecisions reads approvals at the same event and knowledge cut as the finding metrics.
type FindingDecisions interface {
	ActiveAndExpiredAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error)
}

type FindingMetricResult struct {
	Points     []entity.SeriesPoint
	CodeGroups []CodeFindingGroup
}

// CodeFindingGroup counts the applicable C11 identities with the same native classification.
type CodeFindingGroup struct {
	CodeFindingClass
	Count int
}

type CodeFindingClass struct {
	Class          string
	RuleID         string
	NativeSeverity string
}

// FindingMetricsAt evaluates one scope for change. The caller applies access rules before exposing evidence.
func (u *UseCase) FindingMetricsAt(ctx context.Context, in FindingMetricsInput) (FindingMetricResult, error) {
	versions, err := u.d.Decisions.ActiveAndExpiredAt(ctx, in.OrganizationID, in.AreaID, in.KnownSeq, in.AsOf)
	if err != nil {
		return FindingMetricResult{}, err
	}
	return findingMetrics(&in, versions), nil
}

type metricState struct {
	state  entity.DataState
	reason string
}

func matchingScan(scan *FindingScan, area entity.ID, sha string, seq int64) bool {
	return scan != nil && scan.AreaID == area && sha != "" && scan.SHA == sha && scan.KnownSeq <= seq
}

func scanState(scan *FindingScan) metricState {
	switch {
	case scan == nil:
		return metricState{entity.StateNotEvaluated, _reasonReportMissing}
	case scan.RejectedReason != "":
		return metricState{entity.StateUnknown, _reasonReportRejected}
	case scan.Completeness == "complete" && len(scan.SkippedTargets) == 0 &&
		(scan.ExitClass == "success" || scan.ExitClass == "findings"):
		return metricState{state: entity.StateAssessed}
	default:
		return metricState{entity.StateProvisional, _reasonReportPartial}
	}
}

func (s metricState) hasFacts() bool {
	return s.state == entity.StateAssessed || s.state == entity.StateProvisional
}

func (in *FindingMetricsInput) contains(f *ClassifiedFinding) bool {
	return f.AreaID == in.AreaID && f.Kind == in.Kind &&
		(in.ComponentID.IsZero() || f.ComponentID == in.ComponentID)
}

type findingSnapshot struct {
	series entity.SeriesPoint
	seen   map[entity.ID]int
}

func snapshot(id, estimator string, at time.Time, state metricState) findingSnapshot {
	return findingSnapshot{series: entity.SeriesPoint{
		MetricID: id, Estimator: estimator, Unit: UnitCount,
		Point: entity.Point{End: at, State: state.state, ReasonCode: state.reason},
	}, seen: make(map[entity.ID]int)}
}

func (s *findingSnapshot) add(c entity.Contribution) bool {
	if idx, exists := s.seen[c.ObjectID]; exists {
		// Another occurrence of this identity can remain applicable when only one occurrence is excepted.
		if c.Role == entity.RoleNumerator && s.series.Contributions[idx].Role != entity.RoleNumerator {
			s.series.Contributions[idx] = c
			return true
		}
		return false
	}
	s.seen[c.ObjectID] = len(s.series.Contributions)
	s.series.Contributions = append(s.series.Contributions, c)
	return true
}

func (s *findingSnapshot) result() entity.SeriesPoint {
	v := s.series
	var n int
	for _, c := range v.Contributions {
		switch c.Role {
		case entity.RoleNumerator:
			n++
		case entity.RoleExcluded:
			v.ExcludedCount++
		case entity.RoleMissing:
			v.MissingCount++
		case entity.RoleDenominator:
		}
	}
	if v.MissingCount > 0 && v.State == entity.StateAssessed {
		v.State, v.ReasonCode = entity.StateProvisional, "finding_identity_unknown"
	}
	if v.State == entity.StateAssessed || v.State == entity.StateProvisional && n > 0 {
		v.Value, v.Numerator = num(float64(n)), num(float64(n))
	}
	return v
}

type findingEvidence struct {
	ReportID       entity.ID          `json:"report_id"`
	SourceID       entity.ID          `json:"source_id"`
	OccurrenceID   entity.ID          `json:"occurrence_id"`
	Kind           string             `json:"kind"`
	Class          string             `json:"class,omitempty"`
	Rule           string             `json:"rule"`
	NativeSeverity string             `json:"native_severity,omitempty"`
	Path           string             `json:"path,omitempty"`
	PackagePURL    string             `json:"package_purl,omitempty"`
	Line           *int               `json:"line,omitempty"`
	Disposition    FindingDisposition `json:"disposition,omitempty"`
	Factor         FindingFactor      `json:"factor,omitempty"`
	ExceptionID    *entity.ID         `json:"exception_version_id,omitempty"`
}

func findingContribution(f *ClassifiedFinding, kind FindingDisposition, exception *entity.ExceptionVersion) entity.Contribution {
	c := entity.Contribution{ObjectID: f.FindingID, ObjectKind: _objectFinding, ItemNo: 1, Role: entity.RoleNumerator, Value: num(1)}
	e := findingEvidence{
		ReportID: f.ReportID, SourceID: f.SourceID, OccurrenceID: f.OccurrenceID, Kind: f.Kind, Class: f.Class,
		Rule: f.RuleID, NativeSeverity: f.NativeSeverity, Path: f.Path, PackagePURL: f.PackagePURL,
		Line: f.LineStart, Disposition: kind, Factor: f.Factor,
	}
	switch {
	case f.FindingID.IsZero():
		c.ObjectID, c.Role, c.Value, c.ReasonCode = f.OccurrenceID, entity.RoleMissing, nil, "finding_identity_unknown"
	case exception != nil:
		c.Role, c.Value, c.ReasonCode = entity.RoleExcluded, nil, "exception_active"
		e.ExceptionID = &exception.ID
	}
	c.Evidence, _ = json.Marshal(e)
	return c
}

func markScan(s *findingSnapshot, scan *FindingScan, fallback entity.ID, state metricState) {
	if state.state == entity.StateAssessed {
		return
	}
	id := fallback
	if scan != nil && state.reason != _reasonReportMissing {
		id = scan.ID
	}
	s.add(entity.Contribution{ObjectID: id, ObjectKind: _objectFindingReport, ItemNo: 1, Role: entity.RoleMissing, ReasonCode: state.reason})
}

type findingEvaluation struct {
	codeGroups                                       map[CodeFindingClass]int
	open, added, kept, observed, resolved, newData   findingSnapshot
	exceptions                                       exceptionIndex
	head                                             metricState
	headIDs                                          map[entity.ID]bool
	security, sameArtifact, comparable, newDataKnown bool
}

func findingMetrics(in *FindingMetricsInput, versions []entity.ExceptionVersion) FindingMetricResult {
	security := entity.SensitiveFinding(in.Kind)
	if !security && in.Kind != entity.FindingCode {
		return FindingMetricResult{}
	}
	deltaID, resolvedID, exceptionID := "C12", "C13", "C26"
	if security {
		deltaID, resolvedID, exceptionID = "S10", "S11", "S35"
	}
	e := newFindingEvaluation(in, versions, deltaID, resolvedID, security)
	e.calculate(in)
	result := []entity.SeriesPoint{e.added.result(), e.kept.result(), e.observed.result(), e.resolved.result()}
	if security {
		result = append(result, e.newData.result())
	} else {
		result = append(result, e.open.result())
	}
	return FindingMetricResult{Points: append(result, exceptionMetrics(in, versions, exceptionID)...), CodeGroups: e.classifiedCode()}
}

func newFindingEvaluation(in *FindingMetricsInput, versions []entity.ExceptionVersion, deltaID, resolvedID string, security bool) findingEvaluation {
	headReport, baseReport := in.scopedScan(in.Head, in.HeadSHA), in.scopedScan(in.Base, in.BaseSHA)
	head, base := scanState(headReport), scanState(baseReport)
	reason := in.IncomparableReason
	if reason == "" && !sameComparisonScope(headReport, baseReport) {
		reason = "report_scope_mismatch"
	}
	delta := deltaState(head, base, reason)
	e := findingEvaluation{
		open: snapshot("C11", "", in.AsOf, head), added: snapshot(deltaID, _estFindingNew, in.AsOf, delta),
		kept: snapshot(deltaID, _estFindingKept, in.AsOf, delta), resolved: snapshot(resolvedID, "", in.AsOf, delta),
		head: head, comparable: delta.state == entity.StateAssessed, security: security,
		sameArtifact: in.SameArtifact || in.HeadSHA != "" && in.HeadSHA == in.BaseSHA,
		exceptions:   indexExceptions(in, versions),
	}
	if !security {
		e.codeGroups = make(map[CodeFindingClass]int)
	}
	observed := head
	if e.comparable {
		observed = metricState{entity.StateNotApplicable, "baseline_comparable"}
	}
	e.observed = snapshot(deltaID, _estFindingObserved, in.AsOf, observed)
	// A provenance change can be the reason the scans are incomparable for code delta.
	dataReason := ""
	if headReport != nil && baseReport != nil && headReport.Kind != baseReport.Kind {
		dataReason = "report_scope_mismatch"
	}
	data := deltaState(head, base, dataReason)
	e.newDataKnown = data.state == entity.StateAssessed && e.sameArtifact
	e.newData = snapshot("S38", "", in.AsOf, data)
	if !head.hasFacts() || head.state == entity.StateProvisional {
		markScan(&e.open, in.Head, in.AreaID, head)
		markScan(&e.observed, in.Head, in.AreaID, head)
		markScan(&e.added, in.Head, in.AreaID, head)
		markScan(&e.kept, in.Head, in.AreaID, head)
		markScan(&e.resolved, in.Head, in.AreaID, head)
		markScan(&e.newData, in.Head, in.AreaID, head)
	} else if base.state != entity.StateAssessed {
		markScan(&e.added, in.Base, in.AreaID, base)
		markScan(&e.kept, in.Base, in.AreaID, base)
		markScan(&e.resolved, in.Base, in.AreaID, base)
		markScan(&e.newData, in.Base, in.AreaID, base)
	}
	return e
}

func (in *FindingMetricsInput) scopedScan(r *FindingScan, sha string) *FindingScan {
	if !matchingScan(r, in.AreaID, sha, in.KnownSeq) ||
		(!in.ComponentID.IsZero() && r.ComponentID != nil && *r.ComponentID != in.ComponentID) {
		return nil
	}
	return r
}

func sameComparisonScope(head, base *FindingScan) bool {
	if head == nil || base == nil {
		return true // The report state carries the reason.
	}
	return head.Kind == base.Kind && head.Language == base.Language &&
		(head.ComponentID == nil || base.ComponentID == nil || *head.ComponentID == *base.ComponentID) &&
		(head.ScopeHash == "" || base.ScopeHash == "" || head.ScopeHash == base.ScopeHash)
}

func deltaState(head, base metricState, reason string) metricState {
	if head.state != entity.StateAssessed {
		return head
	}
	if reason != "" {
		return metricState{entity.StateNotEvaluated, reason}
	}
	if base.state != entity.StateAssessed {
		return metricState{entity.StateNotEvaluated, "comparable_baseline_missing"}
	}
	return metricState{state: entity.StateAssessed}
}

func (e *findingEvaluation) calculate(in *FindingMetricsInput) {
	if !e.head.hasFacts() {
		return
	}
	e.headIDs = make(map[entity.ID]bool, len(in.Findings))
	for i := range in.Findings {
		f := &in.Findings[i]
		if in.contains(f) && f.ReportID == in.Head.ID && f.Disposition != FindingResolved {
			e.headIDs[f.FindingID] = true
			e.takeHead(f)
		}
	}
	if !e.comparable {
		return
	}
	for i := range in.Findings {
		f := &in.Findings[i]
		if in.contains(f) && f.ReportID == in.Base.ID && f.Disposition == FindingResolved && !e.headIDs[f.FindingID] {
			e.resolved.add(findingContribution(f, FindingResolved, e.exceptions.match(f.FindingCoordinates)))
		}
	}
}

func (e *findingEvaluation) takeHead(f *ClassifiedFinding) {
	exception := e.exceptions.match(f.FindingCoordinates)
	if !e.security {
		c := findingContribution(f, f.Disposition, exception)
		if e.open.add(c) && c.Role == entity.RoleNumerator {
			e.codeGroups[CodeFindingClass{Class: f.Class, RuleID: f.RuleID, NativeSeverity: f.NativeSeverity}]++
		}
	}
	if e.security && e.newDataKnown && (f.Disposition == FindingNew || f.Disposition == FindingNewlyObserved) && sourceFactor(f.Factor) {
		e.newData.add(findingContribution(f, f.Disposition, exception))
		return
	}
	if !e.comparable {
		e.observed.add(findingContribution(f, FindingNewlyObserved, exception))
		return
	}
	switch f.Disposition {
	case FindingNew:
		e.added.add(findingContribution(f, FindingNew, exception))
	case FindingPersisting:
		e.kept.add(findingContribution(f, FindingPersisting, exception))
	case FindingUncertain, FindingNewlyObserved:
		c := findingContribution(f, f.Disposition, exception)
		if c.Role != entity.RoleExcluded {
			c.Role, c.Value, c.ReasonCode = entity.RoleMissing, nil, "finding_identity_unknown"
		}
		e.added.add(c)
		e.kept.add(c)
		e.resolved.add(c)
	case FindingResolved:
	}
}

func (e *findingEvaluation) classifiedCode() []CodeFindingGroup {
	groups := make([]CodeFindingGroup, 0, len(e.codeGroups))
	for class, count := range e.codeGroups {
		groups = append(groups, CodeFindingGroup{CodeFindingClass: class, Count: count})
	}
	slices.SortFunc(groups, func(a, b CodeFindingGroup) int {
		return cmp.Or(cmp.Compare(a.Class, b.Class), cmp.Compare(a.RuleID, b.RuleID), cmp.Compare(a.NativeSeverity, b.NativeSeverity))
	})
	return groups
}

func sourceFactor(f FindingFactor) bool {
	return f == FindingFromDatabase || f == FindingFromRules || f == FindingFromScope
}
