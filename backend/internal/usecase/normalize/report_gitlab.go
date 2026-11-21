package normalize

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _gitlabScanTypes is the scan.type of a GitLab security report by the kind of report it is sent as.
var _gitlabScanTypes = map[string]string{
	entity.AnalysisSAST:    "sast",
	entity.AnalysisSecrets: "secret_detection",
	entity.AnalysisSCA:     "dependency_scanning",
}

type gitlabScanner struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type gitlabScan struct {
	Type    string        `json:"type"`
	Scanner gitlabScanner `json:"scanner"`
}

// gitlabVulnerability takes coordinates and identities only: raw_source_code_extract, the commit message and its
// author are read past.
type gitlabVulnerability struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Severity    string       `json:"severity"`
	Identifiers []identifier `json:"identifiers"`
	Location    struct {
		File       string       `json:"file"`
		StartLine  *json.Number `json:"start_line"`
		EndLine    *json.Number `json:"end_line"`
		Dependency *struct {
			Package struct {
				Name string `json:"name"`
			} `json:"package"`
			Version string `json:"version"`
		} `json:"dependency"`
	} `json:"location"`
}

// parseGitLabSecurity reads a GitLab security report of schema 15: SAST, secret detection or dependency scanning.
// Schema 15 has no confidence of a finding, so it stays unknown (BR-017).
func parseGitLabSecurity(r io.Reader, in parseInput) (*parsedReport, error) {
	kind := in.kind
	dec := json.NewDecoder(r)
	out := &parsedReport{}
	var (
		scan  *gitlabScan
		vulns []gitlabVulnerability
	)
	err := object(dec, func(key string) error {
		switch key {
		case "version":
			return dec.Decode(&out.formatVersion)
		case "scan":
			return dec.Decode(&scan)
		case "vulnerabilities":
			return array(dec, func() error {
				var v gitlabVulnerability
				if err := dec.Decode(&v); err != nil {
					return err
				}
				vulns = append(vulns, v)
				return nil
			})
		}
		return skip(dec)
	})
	if err != nil {
		return nil, err
	}
	if err := end(dec); err != nil {
		return nil, err
	}
	switch {
	case out.formatVersion == "":
		return nil, invalidReport("gitlab_security: no version")
	case scan == nil:
		return nil, invalidReport("gitlab_security: no scan")
	case scan.Type != _gitlabScanTypes[kind]:
		return nil, invalidReport("gitlab_security: scan type %.32q is not a %s report", scan.Type, kind)
	}
	tool := &toolRef{Name: scan.Scanner.Name, Version: scan.Scanner.Version}
	for i := range vulns {
		f, err := gitlabFinding(kind, &vulns[i], tool)
		if err != nil {
			return nil, err
		}
		out.findings = append(out.findings, f)
	}
	return out, nil
}

func gitlabFinding(kind string, v *gitlabVulnerability, tool *toolRef) (entity.FindingOccurrence, error) {
	if len(v.Identifiers) == 0 {
		return entity.FindingOccurrence{}, invalidReport("gitlab_security: vulnerability without identifiers")
	}
	f, err := newFinding(kind, v.Identifiers[0].Value, v.ID)
	if err != nil {
		return entity.FindingOccurrence{}, err
	}
	f.NativeSeverity = bounded(v.Severity, _maxSeverity)
	f.Path = repoPath(v.Location.File)
	start, err := lineNumber(v.Location.StartLine)
	if err != nil {
		return entity.FindingOccurrence{}, err
	}
	finish, err := lineNumber(v.Location.EndLine)
	if err != nil {
		return entity.FindingOccurrence{}, err
	}
	f.LineStart, f.LineEnd = lines(start, finish)
	ev := evidence{Tool: tool, RuleName: v.Name, RuleDescription: short(v.Description)}
	var weaknesses []string
	for _, id := range v.Identifiers {
		ev.Identifiers = append(ev.Identifiers, identifier{Type: id.Type, Name: id.Name, Value: id.Value})
		if id.Type == "cwe" {
			weaknesses = append(weaknesses, "CWE-"+id.Value)
		}
	}
	ev.CWE = cwes(weaknesses...)
	pkg, version := "", ""
	if d := v.Location.Dependency; d != nil && d.Package.Name != "" {
		pkg, version = d.Package.Name, d.Version
		ev.Package = &packageRef{Name: pkg, Version: version}
	}
	if f.Evidence, err = ev.encode(); err != nil {
		return entity.FindingOccurrence{}, err
	}
	f.Fingerprint = fingerprint("gitlab/1", kind, f.RuleID, f.Path, lineText(f.LineStart), lineText(f.LineEnd), pkg, version)
	return f, nil
}

// lineNumber reads a line of schema 15, a JSON number; a fraction is no line.
func lineNumber(n *json.Number) (int, error) {
	if n == nil {
		return 0, nil
	}
	v, err := strconv.Atoi(n.String())
	if err != nil {
		return 0, invalidReport("gitlab_security: line %.16q is not a whole number", n.String())
	}
	return v, nil
}
