package normalize

import (
	"encoding/json"
	"io"
	"math"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type codeQualityFinding struct {
	Description string `json:"description"`
	CheckName   string `json:"check_name"`
	Fingerprint string `json:"fingerprint"`
	Severity    string `json:"severity"`
	Location    struct {
		Path  string `json:"path"`
		Lines struct {
			Begin int `json:"begin"`
			End   int `json:"end"`
		} `json:"lines"`
		Positions struct {
			Begin struct {
				Line int `json:"line"`
			} `json:"begin"`
			End struct {
				Line int `json:"line"`
			} `json:"end"`
		} `json:"positions"`
	} `json:"location"`
}

func parseGitLabCodeQuality(r io.Reader, in parseInput) (*parsedReport, error) {
	return readFindingArray(r, func(dec *json.Decoder) (entity.FindingOccurrence, error) {
		var p codeQualityFinding
		if err := dec.Decode(&p); err != nil {
			return entity.FindingOccurrence{}, err
		}
		f, err := newFinding(in.kind, p.CheckName, p.Fingerprint)
		if err != nil {
			return f, err
		}
		start, finish := p.Location.Lines.Begin, p.Location.Lines.End
		if start == 0 {
			start, finish = p.Location.Positions.Begin.Line, p.Location.Positions.End.Line
		}
		f.Path = repoPath(p.Location.Path)
		if p.Description == "" || f.NativeID == "" || f.Path == "" || start < 1 || start > math.MaxInt32 || finish > math.MaxInt32 ||
			!slices.Contains([]string{"info", "minor", "major", "critical", "blocker"}, p.Severity) {
			return f, invalidReport("gitlab_code_quality: missing or invalid finding fields")
		}
		f.LineStart, f.LineEnd = lines(start, finish)
		f.NativeSeverity = p.Severity
		f.Fingerprint = fingerprint("gitlab_code_quality", f.RuleID, p.Fingerprint, f.Path)
		return f, nil
	})
}

func parseEnvelopeFindings(r io.Reader, in parseInput) (*parsedReport, error) {
	return readFindingArray(r, func(dec *json.Decoder) (entity.FindingOccurrence, error) {
		var rec envelope.Record
		if err := dec.Decode(&rec); err != nil {
			return entity.FindingOccurrence{}, err
		}
		if rec.RecordKind != "finding" || rec.RecordClass != "object" || rec.PayloadSchema != "finding/1" ||
			rec.NativeId == "" || len(rec.NativeId) > _maxNativeID || rec.NativeNs != in.sha || rec.File != nil {
			return entity.FindingOccurrence{}, invalidReport("envelope: expected a finding record for the report commit")
		}
		var p envelope.FindingV1
		if err := json.Unmarshal(rec.Payload, &p); err != nil {
			return entity.FindingOccurrence{}, err
		}
		f, err := newFinding(in.kind, p.RuleId, rec.NativeId)
		if err != nil {
			return f, err
		}
		kind := string(p.FindingKind)
		if in.sha == "" || p.Sha != in.sha || p.Producer == "" || len(p.Producer) > 128 ||
			p.ProducerVersion == "" || len(p.ProducerVersion) > 64 || !slices.Contains([]string{
			entity.FindingCode, entity.FindingSecurity, entity.FindingSecret, entity.FindingDependency,
			entity.FindingIaC, entity.FindingDocs, entity.FindingOpenAPI,
		}, kind) {
			return f, invalidReport("envelope: invalid provenance or finding kind")
		}
		f.Kind, f.Path = kind, repoPath(deref(p.Path))
		if p.LineStart != nil {
			finish := 0
			if p.LineEnd != nil {
				finish = *p.LineEnd
			}
			if *p.LineStart < 1 || *p.LineStart > math.MaxInt32 || p.LineEnd != nil && (finish < *p.LineStart || finish > math.MaxInt32) {
				return f, invalidReport("envelope: invalid finding lines")
			}
			f.LineStart, f.LineEnd = lines(*p.LineStart, finish)
		}
		f.NativeSeverity = bounded(deref(p.NativeSeverity), _maxSeverity)
		f.NativeConfidence = bounded(deref(p.NativeConfidence), _maxSeverity)
		f.PackagePURL = bounded(deref(p.PackagePurl), 1024)
		f.Fingerprint = fingerprint("envelope", p.Producer, f.Kind, f.RuleID, rec.NativeId)
		f.Evidence, err = (&evidence{Tool: &toolRef{Name: p.Producer, Version: p.ProducerVersion}}).encode()
		return f, err
	})
}

func readFindingArray(r io.Reader, read func(*json.Decoder) (entity.FindingOccurrence, error)) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	if err := delim(dec, '['); err != nil {
		return nil, err
	}
	out := &parsedReport{formatVersion: "1"}
	for dec.More() {
		f, err := read(dec)
		if err != nil {
			return nil, err
		}
		out.findings = append(out.findings, f)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, end(dec)
}
