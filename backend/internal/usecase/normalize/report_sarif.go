package normalize

import (
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _sarifDefaultLevel is the level SARIF 2.1.0 gives a result whose result and rule name none (3.27.10).
const _sarifDefaultLevel = "warning"

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name"`
	ShortDescription     *sarifText              `json:"shortDescription"`     //nolint:tagliatelle // SARIF wire format.
	FullDescription      *sarifText              `json:"fullDescription"`      //nolint:tagliatelle // SARIF wire format.
	DefaultConfiguration *struct{ Level string } `json:"defaultConfiguration"` //nolint:tagliatelle // SARIF wire format.
	Properties           sarifProperties         `json:"properties"`
}

type sarifTool struct {
	Driver struct {
		Name            string      `json:"name"`
		Version         string      `json:"version"`
		SemanticVersion string      `json:"semanticVersion"` //nolint:tagliatelle // SARIF wire format.
		Rules           []sarifRule `json:"rules"`
	} `json:"driver"`
}

// sarifResult takes only coordinates and identities of a result: its message, snippets and code flows may carry
// code or the value of a secret and are read past.
type sarifResult struct {
	RuleID    string `json:"ruleId"`    //nolint:tagliatelle // SARIF wire format.
	RuleIndex *int   `json:"ruleIndex"` //nolint:tagliatelle // SARIF wire format.
	Rule      *struct {
		ID    string `json:"id"`
		Index *int   `json:"index"`
	} `json:"rule"`
	Kind      string `json:"kind"`
	Level     string `json:"level"`
	GUID      string `json:"guid"`
	Locations []struct {
		PhysicalLocation *struct {
			ArtifactLocation *struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"` //nolint:tagliatelle // SARIF wire format.
			Region *struct {
				StartLine int `json:"startLine"` //nolint:tagliatelle // SARIF wire format.
				EndLine   int `json:"endLine"`   //nolint:tagliatelle // SARIF wire format.
			} `json:"region"`
		} `json:"physicalLocation"` //nolint:tagliatelle // SARIF wire format.
	} `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"` //nolint:tagliatelle // SARIF wire format.
	Suppressions        []json.RawMessage `json:"suppressions"`
	Properties          sarifProperties   `json:"properties"`
}

// sarifProperties are the property bags SARIF tools put tags, CWE, confidence and security-severity in.
type sarifProperties map[string]json.RawMessage

func (p sarifProperties) text(key string) string {
	raw, ok := p[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

func (p sarifProperties) texts(key string) []string {
	raw, ok := p[key]
	if !ok {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	if s := p.text(key); s != "" {
		return []string{s}
	}
	return nil
}

type sarifRun struct {
	tool    sarifTool
	roots   []string
	results []sarifResult
}

// parseSARIF reads a SARIF log: runs, the rules of tool.driver and results. Members of a run may come in any order,
// so its results are resolved against its rules once the run is read.
func parseSARIF(r io.Reader, kind string) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	out := &parsedReport{}
	err := object(dec, func(key string) error {
		switch key {
		case "version":
			return dec.Decode(&out.formatVersion)
		case "runs":
			return array(dec, func() error {
				run, err := readSARIFRun(dec)
				if err != nil {
					return err
				}
				return run.findings(kind, &out.findings)
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
	if out.formatVersion == "" {
		return nil, invalidReport("sarif: no version")
	}
	return out, nil
}

func readSARIFRun(dec *json.Decoder) (*sarifRun, error) {
	run := &sarifRun{}
	err := object(dec, func(key string) error {
		switch key {
		case "tool":
			return dec.Decode(&run.tool)
		case "originalUriBaseIds":
			var bases map[string]struct {
				URI string `json:"uri"`
			}
			if err := dec.Decode(&bases); err != nil {
				return err
			}
			for _, k := range slices.Sorted(maps.Keys(bases)) {
				run.roots = append(run.roots, bases[k].URI)
			}
			return nil
		case "results":
			return array(dec, func() error {
				var res sarifResult
				if err := dec.Decode(&res); err != nil {
					return err
				}
				run.results = append(run.results, res)
				return nil
			})
		}
		return skip(dec)
	})
	return run, err
}

func (run *sarifRun) findings(kind string, out *[]entity.FindingOccurrence) error {
	d := &run.tool.Driver
	tool := &toolRef{Name: d.Name, Version: d.Version}
	if tool.Version == "" {
		tool.Version = d.SemanticVersion
	}
	byID := make(map[string]int, len(d.Rules))
	for i := range d.Rules {
		if _, ok := byID[d.Rules[i].ID]; !ok {
			byID[d.Rules[i].ID] = i
		}
	}
	for i := range run.results {
		res := &run.results[i]
		if res.Kind == "pass" || res.Kind == "notApplicable" || res.Kind == "informational" {
			continue
		}
		rule := run.rule(res, byID)
		id := res.RuleID
		if id == "" && res.Rule != nil {
			id = res.Rule.ID
		}
		if id == "" && rule != nil {
			id = rule.ID
		}
		f, err := newFinding(kind, id, res.GUID)
		if err != nil {
			return err
		}
		f.NativeSeverity = bounded(res.level(rule), _maxSeverity)
		f.NativeConfidence = bounded(res.Properties.text("confidence"), _maxSeverity)
		if len(res.Locations) > 0 && res.Locations[0].PhysicalLocation != nil {
			loc := res.Locations[0].PhysicalLocation
			if loc.ArtifactLocation != nil {
				f.Path = uriPath(loc.ArtifactLocation.URI, run.roots)
			}
			if loc.Region != nil {
				f.LineStart, f.LineEnd = lines(loc.Region.StartLine, loc.Region.EndLine)
			}
		}
		ev := evidence{Tool: tool, Suppressed: len(res.Suppressions) > 0}
		tags := res.Properties.texts("tags")
		if rule != nil {
			if f.NativeConfidence == "" {
				f.NativeConfidence = bounded(rule.Properties.text("confidence"), _maxSeverity)
			}
			ev.RuleName = rule.Name
			if rule.ShortDescription != nil && rule.ShortDescription.Text != "" {
				ev.RuleDescription = short(rule.ShortDescription.Text)
			} else if rule.FullDescription != nil {
				ev.RuleDescription = short(rule.FullDescription.Text)
			}
			ev.SecuritySeverity = rule.Properties.text("security-severity")
			tags = append(tags, rule.Properties.texts("tags")...)
			tags = append(tags, rule.Properties.texts("cwe")...)
		}
		if s := res.Properties.text("security-severity"); s != "" {
			ev.SecuritySeverity = s
		}
		ev.SecuritySeverity = bounded(ev.SecuritySeverity, _maxSeverity)
		ev.CWE = cwes(append(tags, res.Properties.texts("cwe")...)...)
		if f.Evidence, err = ev.encode(); err != nil {
			return err
		}
		f.Fingerprint = sarifFingerprint(kind, &f, res.PartialFingerprints)
		*out = append(*out, f)
	}
	return nil
}

// rule finds the rule of a result by its index, then by its ID.
func (run *sarifRun) rule(res *sarifResult, byID map[string]int) *sarifRule {
	rules := run.tool.Driver.Rules
	index := res.RuleIndex
	if index == nil && res.Rule != nil {
		index = res.Rule.Index
	}
	if index != nil && *index >= 0 && *index < len(rules) {
		return &rules[*index]
	}
	id := res.RuleID
	if id == "" && res.Rule != nil {
		id = res.Rule.ID
	}
	if i, ok := byID[id]; ok {
		return &rules[i]
	}
	return nil
}

// level is the level of a result, of its rule without one, and the default of SARIF without both.
func (res *sarifResult) level(rule *sarifRule) string {
	if res.Level != "" {
		return res.Level
	}
	if rule != nil && rule.DefaultConfiguration != nil && rule.DefaultConfiguration.Level != "" {
		return rule.DefaultConfiguration.Level
	}
	return _sarifDefaultLevel
}

// sarifFingerprint identifies a result by the partial fingerprints of its tool, which survive moved lines, and by
// its lines without them. An empty fingerprint value is no fingerprint.
func sarifFingerprint(kind string, f *entity.FindingOccurrence, partial map[string]string) []byte {
	parts := []string{"sarif/1", kind, f.RuleID, f.Path}
	var named []string
	for _, k := range slices.Sorted(maps.Keys(partial)) {
		if v := strings.TrimSpace(partial[k]); v != "" {
			named = append(named, k+"="+v)
		}
	}
	if len(named) == 0 {
		return fingerprint(append(parts, "lines", lineText(f.LineStart), lineText(f.LineEnd))...)
	}
	return fingerprint(append(append(parts, "partial"), named...)...)
}
