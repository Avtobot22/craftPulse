package calc

import (
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type affectedInstance struct {
	packageInfo entity.AffectedPackage
	findings    []entity.SecurityFinding
	deltaKnown  bool
}

func (s *securitySnapshot) dependencies(metrics map[string]entity.SecurityMetric) {
	state := securityScanState(s.reports[_scaScan])
	occurrences := s.found[_scaScan]
	packages, withoutPackage := affectedInstances(occurrences)
	packageCount := len(packages)
	packageState := securityMetadataState(state, packageCount, withoutPackage, "dependency_identity_missing")
	for i := range packages {
		packageState.Contributions = append(packageState.Contributions, packages[i].contribution(entity.RoleNumerator, ""))
	}
	metrics["S13"] = securityCount(packageState, packageCount)
	metrics["S14"] = advisoryCount(state, occurrences)
	metrics["S15"] = fixablePackages(packageState, packages)
	metrics["S16"] = introducedPackages(s.reports[_scaScan], packageState, packages)
}

func securityMetadataState(state entity.SecurityMetric, count int, missing []entity.SecurityContribution, reason string) entity.SecurityMetric {
	if len(missing) > 0 {
		state.ReasonCode = reason
		state.State = entity.StateProvisional
		if count == 0 {
			state.State = entity.StateUnknown
		}
	}
	state.Contributions = missing
	return state
}

func affectedInstances(occurrences []securityOccurrence) ([]affectedInstance, []entity.SecurityContribution) {
	type packageKey struct {
		identity, version, ecosystem, lockfile, context, path string
	}
	index := make(map[packageKey]int)
	var affected []affectedInstance
	var missing []entity.SecurityContribution
	for i := range occurrences {
		o := &occurrences[i]
		if o.evidence.Suppressed {
			continue
		}
		pkg := o.evidence.Package
		if pkg == nil || pkg.Version == "" || o.finding.Path == "" {
			c := o.contribution("")
			c.Role, c.ReasonCode = entity.RoleMissing, "dependency_identity_missing"
			missing = append(missing, c)
			continue
		}
		identity := o.finding.PackagePURL
		if identity == "" {
			identity = pkg.Name
		}
		key := packageKey{identity, pkg.Version, pkg.Ecosystem, o.finding.Path, pkg.Context, pkg.PackagePath}
		n, exists := index[key]
		if !exists {
			n = len(affected)
			index[key] = n
			info := entity.AffectedPackage{DependencyEvidence: *pkg, PURL: o.finding.PackagePURL, Lockfile: o.finding.Path}
			info.FixState, info.FixedVersion, info.Delta = entity.DependencyNoFix, "", ""
			affected = append(affected, affectedInstance{packageInfo: info})
		}
		affected[n].add(o)
	}
	return affected, missing
}

func (p *affectedInstance) add(o *securityOccurrence) {
	p.findings = append(p.findings, o.finding)
	pkg := o.evidence.Package
	switch {
	case pkg.FixState == entity.DependencyFixable:
		p.packageInfo.FixState = entity.DependencyFixable
	case pkg.FixState != entity.DependencyNoFix && p.packageInfo.FixState != entity.DependencyFixable:
		p.packageInfo.FixState = entity.DependencyFixUnknown
	}
	if o.deltaComputed {
		p.deltaKnown = true
		if pkg.Delta == entity.DependencyAdded || pkg.Delta == entity.DependencyChanged {
			p.packageInfo.Delta = pkg.Delta
		}
	}
}

func (p *affectedInstance) contribution(role entity.ContributionRole, reason string) entity.SecurityContribution {
	return entity.SecurityContribution{Role: role, ReasonCode: reason, Package: &p.packageInfo, Findings: p.findings}
}

func fixablePackages(state entity.SecurityMetric, packages []affectedInstance) entity.SecurityMetric {
	result := dependencyMetricState(state)
	if state.State != entity.StateAssessed && state.State != entity.StateProvisional {
		return result
	}
	counts := make(map[string]int, 3)
	for i := range packages {
		p := &packages[i]
		fix := p.packageInfo.FixState
		counts[fix]++
		switch fix {
		case entity.DependencyFixable:
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleNumerator, ""))
		case entity.DependencyNoFix:
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleDenominator, entity.DependencyNoFix))
		default:
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleMissing, "fix_metadata_missing"))
		}
	}
	for _, fix := range []string{entity.DependencyFixable, entity.DependencyNoFix, entity.DependencyFixUnknown} {
		group := securityGroup(_scaScan, state, counts[fix])
		group.Scope = fix
		result.Groups = append(result.Groups, group)
	}
	numerator := counts[entity.DependencyFixable]
	denominator := numerator + counts[entity.DependencyNoFix]
	result.Numerator, result.Denominator = &numerator, &denominator
	switch {
	case denominator > 0:
		ratio := float64(numerator) / float64(denominator)
		result.Value = &ratio
	case len(packages) > 0:
		result.State, result.ReasonCode = entity.StateUnknown, "fix_metadata_missing"
	default:
		result.State, result.ReasonCode = entity.StateNotApplicable, "no_affected_packages"
	}
	return result
}

func introducedPackages(reports []entity.ReportEvidence, state entity.SecurityMetric, packages []affectedInstance) entity.SecurityMetric {
	result := dependencyMetricState(state)
	if state.State != entity.StateAssessed && state.State != entity.StateProvisional {
		return result
	}
	var computed, unavailable, unchecked bool
	for i := range reports {
		r := &reports[i]
		if unusableSecurityReport(r) {
			continue
		}
		computed = computed || r.DependencyDelta == entity.DependencyDeltaComputed
		unavailable = unavailable || r.DependencyDelta == entity.DependencyDeltaUnavailable
		unchecked = unchecked || r.DependencyDelta != entity.DependencyDeltaComputed
	}
	count := 0
	for i := range packages {
		p := &packages[i]
		switch {
		case !p.deltaKnown:
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleMissing, "lockfile_diff_missing"))
		case p.packageInfo.Delta != "":
			count++
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleNumerator, p.packageInfo.Delta))
		default:
			result.Contributions = append(result.Contributions, p.contribution(entity.RoleExcluded, "unchanged_dependency"))
		}
	}
	switch {
	case !computed && unavailable:
		result.State, result.ReasonCode = entity.StateUnknown, "lockfile_diff_unavailable"
	case !computed:
		result.State, result.ReasonCode = entity.StateNotEvaluated, "lockfile_diff_missing"
	case unchecked:
		result.State, result.ReasonCode = entity.StateProvisional, "lockfile_diff_incomplete"
	}
	return securityCount(result, count)
}

func dependencyMetricState(state entity.SecurityMetric) entity.SecurityMetric {
	result := entity.SecurityMetric{State: state.State, ReasonCode: state.ReasonCode}
	for i := range state.Contributions {
		if c := &state.Contributions[i]; c.Role == entity.RoleMissing {
			result.Contributions = append(result.Contributions, *c)
		}
	}
	return result
}

func advisoryCount(state entity.SecurityMetric, occurrences []securityOccurrence) entity.SecurityMetric {
	aliases := make(advisoryAliases)
	var missing []entity.SecurityContribution
	for i := range occurrences {
		o := &occurrences[i]
		if o.evidence.Suppressed {
			continue
		}
		if a := o.evidence.Advisory; a != nil && a.ID != "" {
			aliases.root(a.ID)
			for _, alias := range a.Aliases {
				if alias != "" {
					aliases.join(a.ID, alias)
				}
			}
		} else {
			c := o.contribution("")
			c.Role, c.ReasonCode = entity.RoleMissing, "advisory_identity_missing"
			missing = append(missing, c)
		}
	}
	index := make(map[string]int)
	var known []entity.SecurityContribution
	for i := range occurrences {
		o := &occurrences[i]
		a := o.evidence.Advisory
		if o.evidence.Suppressed || a == nil || a.ID == "" {
			continue
		}
		root := aliases.root(a.ID)
		n, ok := index[root]
		if !ok {
			n = len(known)
			index[root] = n
			known = append(known, entity.SecurityContribution{AdvisoryID: root, Role: entity.RoleNumerator})
		}
		known[n].Findings = append(known[n].Findings, o.finding)
	}
	result := securityMetadataState(state, len(known), missing, "advisory_identity_missing")
	result.Contributions = slices.Concat(result.Contributions, known)
	return securityCount(result, len(known))
}

type advisoryAliases map[string]string

func (a advisoryAliases) root(id string) string {
	if parent := a[id]; parent == "" {
		a[id] = id
	}
	root := id
	for a[root] != root {
		root = a[root]
	}
	for a[id] != root {
		parent := a[id]
		a[id] = root
		id = parent
	}
	return root
}

func (a advisoryAliases) join(id, alias string) {
	x, y := a.root(id), a.root(alias)
	xcve, ycve := strings.HasPrefix(x, "CVE-"), strings.HasPrefix(y, "CVE-")
	if ycve && !xcve || xcve == ycve && y < x {
		x, y = y, x
	}
	a[y] = x
}
