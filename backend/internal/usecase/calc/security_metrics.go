package calc

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// SecurityFacts supplies a repository and component snapshot at a knowledge number.
type SecurityFacts interface {
	SecurityFacts(context.Context, entity.ID, entity.SecurityScope) (entity.SecurityFacts, error)
	SecurityReports(context.Context, entity.ID, entity.SecurityScope) ([]entity.ReportEvidence, error)
	ReceiptSecurityScopes(context.Context, entity.ID, entity.ID) ([]entity.SecurityScope, error)
}

type securityScan struct {
	kind  string
	scope string
}

var (
	_sastScan = securityScan{kind: entity.AnalysisSAST}
	_scaScan  = securityScan{kind: entity.AnalysisSCA}
	_iacScan  = securityScan{kind: entity.AnalysisIaC}
	_headScan = securityScan{kind: entity.AnalysisSecrets, scope: entity.SecretScopeHead}
	_diffScan = securityScan{kind: entity.AnalysisSecrets, scope: entity.SecretScopeDiff}
)

type securityOccurrence struct {
	finding       entity.SecurityFinding
	evidence      entity.SecurityEvidence
	deltaComputed bool
}

type securitySnapshot struct {
	reports map[securityScan][]entity.ReportEvidence
	found   map[securityScan][]securityOccurrence
}

// SecurityAt reads a revision for an authorized repository, component and SHA.
func (u *UseCase) SecurityAt(ctx context.Context, org entity.ID, scope entity.SecurityScope) (entity.SecurityRevision, error) {
	if err := validateSecurityScope(scope); err != nil {
		return entity.SecurityRevision{}, err
	}
	facts, err := u.d.Security.SecurityFacts(ctx, org, scope)
	if err != nil {
		return entity.SecurityRevision{}, err
	}
	return u.CalculateSecurity(facts)
}

// CalculateSecurity derives the metrics and their contributions from one immutable report snapshot.
func (*UseCase) CalculateSecurity(facts entity.SecurityFacts) (entity.SecurityRevision, error) {
	snapshot, err := securitySnapshotOf(facts)
	if err != nil {
		return entity.SecurityRevision{}, err
	}
	metrics := map[string]entity.SecurityMetric{
		"S09": snapshot.openFindings(),
		"S12": snapshot.sast(),
		"S18": snapshot.iac(),
		"S19": snapshot.secrets(),
	}
	snapshot.dependencies(metrics)
	return entity.SecurityRevision{Scope: facts.Scope, Definitions: definitionInputs(_securityDefinitions), Reports: facts.Reports, Metrics: metrics}, nil
}

func securitySnapshotOf(facts entity.SecurityFacts) (*securitySnapshot, error) {
	s := &securitySnapshot{
		reports: make(map[securityScan][]entity.ReportEvidence),
		found:   make(map[securityScan][]securityOccurrence),
	}
	reports := slices.Clone(facts.Reports)
	slices.SortStableFunc(reports, func(a, b entity.ReportEvidence) int { return cmp.Compare(b.KnownSeq, a.KnownSeq) })
	byReport := make(map[entity.ID][]*entity.SecurityFinding)
	for i := range facts.Findings {
		f := &facts.Findings[i]
		byReport[f.ReportID] = append(byReport[f.ReportID], f)
	}
	type occurrenceKey struct {
		scan        securityScan
		fingerprint string
	}
	seen := make(map[occurrenceKey]bool)
	for i := range reports {
		r := &reports[i]
		scan := reportScan(r)
		s.reports[scan] = append(s.reports[scan], *r)
		if unusableSecurityReport(r) {
			continue
		}
		for _, f := range byReport[r.ID] {
			key := occurrenceKey{scan, string(f.Fingerprint)}
			if seen[key] {
				continue
			}
			seen[key] = true
			o := securityOccurrence{finding: *f, deltaComputed: r.DependencyDelta == entity.DependencyDeltaComputed}
			if len(f.Evidence) > 0 {
				if err := json.Unmarshal(f.Evidence, &o.evidence); err != nil {
					return nil, err
				}
			}
			if scan.kind == entity.AnalysisSecrets {
				o.finding.Evidence = nil
			}
			s.found[scan] = append(s.found[scan], o)
		}
	}
	return s, nil
}

func reportScan(r *entity.ReportEvidence) securityScan {
	scan := securityScan{kind: r.Kind}
	if r.Kind == entity.AnalysisSecrets {
		scan.scope = entity.SecretScopeHead
		if r.SecretScope == entity.SecretScopeDiff {
			scan.scope = entity.SecretScopeDiff
		}
	}
	return scan
}

func unusableSecurityReport(r *entity.ReportEvidence) bool {
	return r.RejectedReason != "" || r.ExitClass == "failed" || r.ExitClass == "timeout"
}

func securityScanState(reports []entity.ReportEvidence) entity.SecurityMetric {
	var usable, incomplete bool
	for i := range reports {
		r := &reports[i]
		if unusableSecurityReport(r) {
			incomplete = true
			continue
		}
		usable = true
		incomplete = incomplete || r.Partial() || r.Completeness != "complete"
	}
	switch {
	case usable && incomplete:
		return entity.SecurityMetric{State: entity.StateProvisional, ReasonCode: _reasonReportPartial}
	case usable:
		return entity.SecurityMetric{State: entity.StateAssessed}
	case incomplete:
		return entity.SecurityMetric{State: entity.StateUnknown, ReasonCode: _reasonReportRejected}
	default:
		return entity.SecurityMetric{State: entity.StateNotEvaluated, ReasonCode: _reasonReportMissing}
	}
}

func (s *securitySnapshot) openFindings() entity.SecurityMetric {
	var result entity.SecurityMetric
	count := 0
	for _, scan := range []securityScan{_sastScan, _scaScan, _iacScan, _headScan} {
		state := securityScanState(s.reports[scan])
		groups := make(map[string]int)
		for i := range s.found[scan] {
			o := &s.found[scan][i]
			c := o.contribution("")
			result.Contributions = append(result.Contributions, c)
			if c.Role == entity.RoleNumerator {
				groups[o.finding.NativeSeverity]++
				count++
			}
		}
		if len(groups) == 0 {
			result.Groups = append(result.Groups, securityGroup(scan, state, 0))
		}
		for _, severity := range slices.Sorted(maps.Keys(groups)) {
			group := securityGroup(scan, state, groups[severity])
			group.NativeSeverity = severity
			result.Groups = append(result.Groups, group)
		}
	}
	return combinedSecurityCount(result, count)
}

func (s *securitySnapshot) sast() entity.SecurityMetric {
	result := securityScanState(s.reports[_sastScan])
	type groupKey struct{ rule, cwe string }
	groups := make(map[groupKey]int)
	count := 0
	for i := range s.found[_sastScan] {
		o := &s.found[_sastScan][i]
		c := o.contribution("")
		result.Contributions = append(result.Contributions, c)
		if c.Role == entity.RoleExcluded {
			continue
		}
		count++
		weaknesses := o.evidence.CWE
		if len(weaknesses) == 0 {
			weaknesses = []string{""}
		}
		for _, cwe := range weaknesses {
			groups[groupKey{o.finding.RuleID, cwe}]++
		}
	}
	keys := slices.SortedFunc(maps.Keys(groups), func(a, b groupKey) int {
		return cmp.Or(cmp.Compare(a.rule, b.rule), cmp.Compare(a.cwe, b.cwe))
	})
	for _, key := range keys {
		group := securityGroup(_sastScan, result, groups[key])
		group.RuleID, group.CWE = key.rule, key.cwe
		result.Groups = append(result.Groups, group)
	}
	return securityCount(result, count)
}

func (s *securitySnapshot) iac() entity.SecurityMetric {
	result := securityScanState(s.reports[_iacScan])
	type resourceKey struct{ rule, path, resource string }
	seen := make(map[resourceKey]int)
	count := 0
	for i := range s.found[_iacScan] {
		o := &s.found[_iacScan][i]
		c := o.contribution("")
		if c.Role == entity.RoleExcluded {
			result.Contributions = append(result.Contributions, c)
			continue
		}
		key := resourceKey{o.finding.RuleID, o.finding.Path, o.evidence.Resource}
		if key.resource == "" || key.path == "" {
			key.resource = string(o.finding.Fingerprint)
		}
		if n, ok := seen[key]; ok {
			result.Contributions[n].Findings = append(result.Contributions[n].Findings, o.finding)
			continue
		}
		seen[key] = len(result.Contributions)
		result.Contributions = append(result.Contributions, c)
		count++
	}
	return securityCount(result, count)
}

func (s *securitySnapshot) secrets() entity.SecurityMetric {
	var result entity.SecurityMetric
	count := 0
	for _, scan := range []securityScan{_headScan, _diffScan} {
		n := 0
		for i := range s.found[scan] {
			c := s.found[scan][i].contribution(scan.scope)
			result.Contributions = append(result.Contributions, c)
			if c.Role == entity.RoleNumerator {
				n++
			}
		}
		state := securityScanState(s.reports[scan])
		result.Groups = append(result.Groups, securityGroup(scan, state, n))
		count += n
	}
	return combinedSecurityCount(result, count)
}

func (o *securityOccurrence) contribution(scope string) entity.SecurityContribution {
	c := entity.SecurityContribution{Role: entity.RoleNumerator, Findings: []entity.SecurityFinding{o.finding}, Scope: scope}
	if o.evidence.Suppressed {
		c.Role, c.ReasonCode = entity.RoleExcluded, "suppressed"
	}
	return c
}

func securityGroup(scan securityScan, state entity.SecurityMetric, count int) entity.SecurityGroup {
	group := entity.SecurityGroup{Kind: scan.kind, Scope: scan.scope, State: state.State, ReasonCode: state.ReasonCode}
	if group.State == entity.StateAssessed || group.State == entity.StateProvisional {
		group.Count = &count
	}
	return group
}

func combinedSecurityCount(result entity.SecurityMetric, count int) entity.SecurityMetric {
	var available, incomplete, failed bool
	for _, group := range result.Groups {
		available = available || group.Count != nil
		incomplete = incomplete || group.State != entity.StateAssessed
		failed = failed || group.State == entity.StateUnknown
	}
	switch {
	case available && !incomplete:
		result.State = entity.StateAssessed
	case available:
		result.State, result.ReasonCode = entity.StateProvisional, _reasonReportPartial
	case failed:
		result.State, result.ReasonCode = entity.StateUnknown, _reasonReportRejected
	default:
		result.State, result.ReasonCode = entity.StateNotEvaluated, _reasonReportMissing
	}
	return securityCount(result, count)
}

func securityCount(result entity.SecurityMetric, count int) entity.SecurityMetric {
	if result.State == entity.StateAssessed || result.State == entity.StateProvisional {
		value := float64(count)
		result.Value = &value
	}
	return result
}
