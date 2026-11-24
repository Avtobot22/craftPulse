package normalize

import (
	"encoding/json"
	"io"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// The field names below mirror the Trivy JSON v2 wire format.
type trivyVulnerability struct {
	VulnerabilityID  string
	VendorIDs        []string
	Aliases          []string
	PkgID            string
	PkgName          string
	PkgPath          string
	PkgIdentifier    trivyPackageIdentifier
	InstalledVersion string
	FixedVersion     string
	Status           string
	Severity         string
	CweIDs           []string
	CraftPulseDelta  string
}

type trivyPackageIdentifier struct {
	PURL string
}

type trivyPackage struct {
	ID           string
	Version      string
	Identifier   trivyPackageIdentifier
	Relationship string
	Dev          *bool
	Locations    []struct{ StartLine, EndLine int }
}

type trivyMisconfiguration struct {
	ID, AVDID, Severity, Status string
	CauseMetadata               struct {
		Resource           string
		StartLine, EndLine int
	}
}

type trivyResult struct {
	Target, Type, Class string
	Packages            []trivyPackage
	Vulnerabilities     []trivyVulnerability
	Misconfigurations   []trivyMisconfiguration
}

func parseTrivy(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	out := &parsedReport{}
	var inventory entity.DependencyInventory
	var hasInventory bool
	err := object(dec, func(key string) error {
		switch key {
		case "SchemaVersion":
			var version int
			err := dec.Decode(&version)
			out.formatVersion = strconv.Itoa(version)
			return err
		case "CraftPulse":
			var meta struct{ Delta, BaseSHA string }
			if err := dec.Decode(&meta); err != nil {
				return err
			}
			switch meta.Delta {
			case entity.DependencyDeltaComputed, entity.DependencyDeltaNoBase, entity.DependencyDeltaUnavailable:
				out.dependencyDelta = meta.Delta
			}
			if meta.Delta == entity.DependencyDeltaComputed && entity.ValidGitSHA(meta.BaseSHA) {
				out.dependencyBaseSHA = meta.BaseSHA
			}
			return nil
		case "Results":
			return array(dec, func() error {
				var result trivyResult
				if err := dec.Decode(&result); err != nil {
					return err
				}
				if in.kind == entity.AnalysisSCA && (result.Class == "lang-pkgs" || result.Packages != nil) {
					hasInventory = hasInventory || result.Packages != nil
					addDependencyInventory(&inventory, result.Packages)
				}
				return result.findings(in.kind, out)
			})
		default:
			return skip(dec)
		}
	})
	if err != nil {
		return nil, err
	}
	if out.formatVersion == "" {
		return nil, invalidReport("trivy: no schema version")
	}
	if hasInventory {
		out.dependencies = &inventory
	}
	return out, end(dec)
}

func addDependencyInventory(inventory *entity.DependencyInventory, packages []trivyPackage) {
	inventory.Partial = inventory.Partial || packages == nil
	for i := range packages {
		p := &packages[i]
		if p.Relationship == "root" {
			continue
		}
		inventory.Entries++
		if exactPackageVersion(p.Version) {
			inventory.Resolved++
		}
	}
}

func exactPackageVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" || strings.ContainsAny(version, "<>=~^*|, \t\r\n") {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if strings.EqualFold(part, "x") {
			return false
		}
	}
	return strings.ContainsAny(version, "0123456789")
}

func (r *trivyResult) findings(kind string, out *parsedReport) error {
	file := securityPath(r.Target)
	if kind == entity.AnalysisIaC {
		for i := range r.Misconfigurations {
			m := &r.Misconfigurations[i]
			if m.Status == "PASS" {
				continue
			}
			f, err := m.finding(file)
			if err != nil {
				return err
			}
			out.findings = append(out.findings, f)
		}
		return nil
	}
	packages := make(map[string]*trivyPackage, len(r.Packages))
	for i := range r.Packages {
		p := &r.Packages[i]
		if p.ID != "" {
			packages[p.ID] = p
		}
	}
	for i := range r.Vulnerabilities {
		v := &r.Vulnerabilities[i]
		if v.Status == "not_affected" {
			continue
		}
		f, err := v.finding(file, r.Type, packages[v.PkgID])
		if err != nil {
			return err
		}
		out.findings = append(out.findings, f)
	}
	return nil
}

func (v *trivyVulnerability) finding(file, ecosystem string, pkg *trivyPackage) (entity.FindingOccurrence, error) {
	f, err := newFinding(entity.AnalysisSCA, v.VulnerabilityID, v.VulnerabilityID)
	if err != nil {
		return f, err
	}
	if v.PkgName == "" || v.InstalledVersion == "" {
		return f, invalidReport("trivy: affected package is missing its name or installed version")
	}
	f.Path = file
	f.NativeSeverity = bounded(v.Severity, _maxSeverity)
	mapSecuritySeverity(&f)
	f.PackagePURL = bounded(v.PkgIdentifier.PURL, 1024)
	dep := &entity.DependencyEvidence{
		Name: v.PkgName, Version: v.InstalledVersion, FixedVersion: strings.TrimSpace(v.FixedVersion),
		FixState: v.fixState(), Ecosystem: ecosystem, Context: v.PkgID, PackagePath: securityPath(v.PkgPath),
	}
	if pkg != nil {
		if f.PackagePURL == "" {
			f.PackagePURL = bounded(pkg.Identifier.PURL, 1024)
		}
		dep.Relationship, dep.Dev = pkg.Relationship, pkg.Dev
		if len(pkg.Locations) > 0 {
			loc := pkg.Locations[0]
			f.LineStart, f.LineEnd = trivyLines(loc.StartLine, loc.EndLine)
		}
	}
	if f.PackagePURL == "" {
		f.PackagePURL = trivyPURL(ecosystem, dep.Name, dep.Version)
	}
	switch v.CraftPulseDelta {
	case entity.DependencyAdded, entity.DependencyChanged:
		dep.Delta = v.CraftPulseDelta
	}
	aliases := slices.Clone(v.Aliases)
	// VendorIDs can also contain bulletins that cover several independent vulnerabilities.
	for _, id := range v.VendorIDs {
		prefix, _, _ := strings.Cut(id, "-")
		switch prefix {
		case "CVE", "GHSA", "GO", "PYSEC", "OSV", "RUSTSEC", "MAL", "GMS", "SNYK":
			aliases = append(aliases, id)
		}
	}
	aliases = slices.DeleteFunc(aliases, func(s string) bool { return s == "" || s == v.VulnerabilityID })
	slices.Sort(aliases)
	ev := evidence{Tool: &toolRef{Name: "trivy"}}
	ev.Package, ev.CWE = dep, cwes(v.CweIDs...)
	ev.Advisory = &entity.AdvisoryEvidence{ID: v.VulnerabilityID, Aliases: slices.Compact(aliases)}
	f.Evidence, err = ev.encode()
	f.Fingerprint = fingerprint("trivy/sca/1", f.RuleID, file, f.PackagePURL, ecosystem, dep.Name, dep.Version, dep.Context, dep.PackagePath)
	f.ContentFingerprint = fingerprint("trivy/sca-content/1", f.RuleID, f.PackagePURL, ecosystem, dep.Name, dep.Version)
	return f, err
}

func (v *trivyVulnerability) fixState() string {
	if strings.TrimSpace(v.FixedVersion) != "" {
		return entity.DependencyFixable
	}
	switch v.Status {
	case "affected", "will_not_fix", "fix_deferred":
		return entity.DependencyNoFix
	default:
		return entity.DependencyFixUnknown
	}
}

func trivyPURL(ecosystem, name, version string) string {
	switch ecosystem {
	case "npm":
	case "gomod", "gobinary":
		ecosystem = "golang"
	case "pip", "pipenv", "poetry", "uv", "python-pkg":
		ecosystem = "pypi"
	default:
		return ""
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(parts[i]), "@", "%40")
	}
	return bounded("pkg:"+ecosystem+"/"+strings.Join(parts, "/")+"@"+strings.ReplaceAll(url.PathEscape(version), "+", "%2B"), 1024)
}

func (m *trivyMisconfiguration) finding(file string) (entity.FindingOccurrence, error) {
	rule := m.ID
	if rule == "" {
		rule = m.AVDID
	}
	f, err := newFinding(entity.AnalysisIaC, rule, rule)
	if err != nil {
		return f, err
	}
	f.Path = file
	f.LineStart, f.LineEnd = trivyLines(m.CauseMetadata.StartLine, m.CauseMetadata.EndLine)
	f.NativeSeverity = bounded(m.Severity, _maxSeverity)
	mapSecuritySeverity(&f)
	ev := evidence{Tool: &toolRef{Name: "trivy"}}
	ev.Resource, ev.Suppressed = bounded(m.CauseMetadata.Resource, _maxPath), m.Status == "EXCEPTION"
	f.Evidence, err = ev.encode()
	f.Fingerprint = fingerprint("trivy/iac/1", f.RuleID, file, ev.Resource, lineText(f.LineStart), lineText(f.LineEnd))
	return f, err
}

func trivyLines(start, finish int) (first, last *int) {
	if start > math.MaxInt32 {
		return nil, nil
	}
	if finish > math.MaxInt32 {
		finish = 0
	}
	return lines(start, finish)
}

func mapSecuritySeverity(f *entity.FindingOccurrence) {
	severity := strings.ToLower(strings.TrimSpace(f.NativeSeverity))
	switch severity {
	case "critical", "high", "medium", "low", "info":
		f.MappedSeverity, f.SeverityMappingVersion = severity, 1
	}
}

func securityPath(p string) string {
	if strings.ContainsRune(p, 0) || len(p) > 1 && p[1] == ':' && (p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') {
		return ""
	}
	return repoPath(p)
}
