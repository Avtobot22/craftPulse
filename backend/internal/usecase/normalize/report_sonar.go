package normalize

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_sonarParserKey = "sonar-envelope"
	_sonarProfile   = "sonar-import"
)

func (in *input) sonarReport(c *reportCandidate, rec *envelope.Record, p *envelope.ReportFileV1,
	scan *envelope.ScanManifestV1,
) *reportCandidate {
	switch {
	case in.provider != string(entity.ProviderSonarQube) || in.area.Kind != entity.ObjectKind(entity.ScopeAnalysisProject) ||
		rec.NativeNs != in.area.NativeID || len(rec.NativeId) > _maxNativeID || scan != nil:
		c.rejected = entity.RecordReportInvalid
	case c.report.SHA == "":
		c.rejected = entity.RecordSonarRevisionMissing
	case c.report.Format != "envelope":
		c.rejected = entity.RecordReportFormatUnsupported
	case p.FinishedAt == nil || p.FinishedAt.IsZero() || p.StartedAt != nil && p.StartedAt.After(*p.FinishedAt),
		bounded(p.Producer, 128) == "" || bounded(p.ProducerVersion, 64) == "",
		sonarTextHasNUL(rec.NativeId, p.Producer, p.ProducerVersion):
		c.rejected = entity.RecordReportInvalid
	default:
		rules, rulesErr := hexHash(p.RulesHash)
		config, configErr := hexHash(p.ConfigHash)
		if rulesErr != nil || configErr != nil {
			c.rejected = entity.RecordReportInvalid
			return c
		}
		c.parser, _ = parserOf(_sonarParserKey)
		r := &c.report
		r.RulesHash, r.ConfigHash = rules, config
		r.SonarAnalysisID = rec.NativeId
		r.Producer, r.ProducerVersion, r.ProfileID = p.Producer, p.ProducerVersion, _sonarProfile
		r.ScopeHash = fingerprint("sonar-project/1", in.instance.String(), in.area.NativeID)
		if p.Completeness != nil {
			r.Completeness = string(*p.Completeness)
		}
		r.ExitClass = ""
		if p.ExitClass != nil {
			r.ExitClass = string(*p.ExitClass)
		}
	}
	return c
}

func parseSonar(r io.Reader, _ parseInput) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	out := &parsedReport{}
	fields := make(map[string]bool, 4)
	err := object(dec, func(key string) error {
		if fields[key] {
			return invalidReport("sonar: repeated analysis field")
		}
		fields[key] = true
		switch key {
		case "version":
			return dec.Decode(&out.formatVersion)
		case "issues":
			return readSonarIssues(dec, out)
		case "issues_state":
			var state envelope.SonarAnalysisFileIssuesState
			if err := dec.Decode(&state); err != nil {
				return err
			}
			if !state.Valid() {
				return invalidReport("sonar: invalid issues state")
			}
			return nil
		case "measures":
			return readSonarMeasures(dec, out)
		default:
			return invalidReport("sonar: unknown analysis field")
		}
	})
	if err != nil {
		return nil, err
	}
	if out.formatVersion == "" || !fields["issues"] || out.sonarMeasures == nil {
		return nil, invalidReport("sonar: version, issues and measures are required")
	}
	return out, end(dec)
}

func readSonarMeasures(dec *json.Decoder, out *parsedReport) error {
	out.sonarMeasures = make(map[string]string)
	return object(dec, func(metric string) error {
		if _, exists := out.sonarMeasures[metric]; exists {
			return invalidReport("sonar: repeated measure")
		}
		var value *string
		if err := dec.Decode(&value); err != nil {
			return err
		}
		if value == nil || bounded(metric, 255) == "" || len(*value) > 255 || sonarTextHasNUL(metric, *value) {
			return invalidReport("sonar: invalid measure")
		}
		out.sonarMeasures[metric] = *value
		return nil
	})
}

func readSonarIssues(dec *json.Decoder, out *parsedReport) error {
	if err := delim(dec, '['); err != nil {
		return err
	}
	seen := map[string]bool{}
	for dec.More() {
		var issue envelope.SonarIssue
		if err := dec.Decode(&issue); err != nil {
			return err
		}
		f, err := sonarFinding(issue)
		if err != nil {
			return err
		}
		if seen[f.NativeID] {
			return invalidReport("sonar: repeated issue key")
		}
		seen[f.NativeID] = true
		out.findings = append(out.findings, f)
	}
	_, err := dec.Token()
	return err
}

func sonarFinding(issue envelope.SonarIssue) (entity.FindingOccurrence, error) {
	kind := entity.AnalysisCode
	switch issue.Type {
	case envelope.SonarIssueTypeVULNERABILITY:
		kind = entity.AnalysisSAST
	case envelope.SonarIssueTypeBUG, envelope.SonarIssueTypeCODESMELL:
	default:
		return entity.FindingOccurrence{}, invalidReport("sonar: unknown issue type")
	}
	if issue.IssueId != "" && issue.Key != "" && issue.IssueId != issue.Key {
		return entity.FindingOccurrence{}, invalidReport("sonar: conflicting issue IDs")
	}
	key := cmp.Or(issue.IssueId, issue.Key)
	f, err := newFinding(kind, issue.RuleId, key)
	if err != nil {
		return f, err
	}
	path, resolution, description := deref(issue.Path), deref(issue.Resolution), deref(issue.RuleDescription)
	if sonarTextHasNUL(key, issue.RuleId, issue.Severity, issue.Status, path, resolution, description) {
		return f, invalidReport("sonar: invalid issue text")
	}
	f.Path = repoPath(path)
	var start, finish int
	if issue.LineStart != nil {
		start = *issue.LineStart
	}
	if issue.LineEnd != nil {
		finish = *issue.LineEnd
	}
	if f.NativeID == "" || bounded(issue.Severity, _maxSeverity) == "" || bounded(issue.Status, _maxSeverity) == "" ||
		len(resolution) > _maxSeverity || path != "" && f.Path == "" ||
		issue.LineStart != nil && (start < 1 || start > math.MaxInt32) ||
		issue.LineEnd != nil && (start < 1 || finish < start || finish > math.MaxInt32) {
		return f, invalidReport("sonar: invalid issue fields")
	}
	f.LineStart, f.LineEnd = lines(start, finish)
	f.NativeSeverity = issue.Severity
	// The source issue key survives line changes. The area of the identity is the Sonar project.
	f.Fingerprint = fingerprint("sonar-issue/1", f.RuleID, f.NativeID)
	if rule := issue.Rule; rule != nil &&
		(rule.Key != issue.RuleId || bounded(rule.Name, 1024) == "" || bounded(rule.Language, 64) == "" ||
			bounded(rule.Repository, 255) == "" || sonarTextHasNUL(rule.Key, rule.Name, rule.Language, rule.Repository)) {
		return f, invalidReport("sonar: invalid rule metadata")
	}
	f.Evidence, err = json.Marshal(struct {
		Status      string                  `json:"status"`
		Resolution  *string                 `json:"resolution,omitempty"`
		Description string                  `json:"rule_description,omitempty"`
		Rule        *envelope.SonarRuleInfo `json:"rule,omitempty"`
	}{issue.Status, issue.Resolution, short(description), issue.Rule})
	return f, err
}

// PostgreSQL cannot store NUL in text or jsonb strings.
func sonarTextHasNUL(values ...string) bool {
	return slices.ContainsFunc(values, func(v string) bool { return strings.ContainsRune(v, 0) })
}

func (w *writer) recordSonarRun(ctx context.Context, in *input, r *entity.ScanReport) error {
	area := r.AreaID
	if r.RepositoryID != nil {
		area = *r.RepositoryID
		w.changes.add(area, _factReport, r.FinishedAt)
	}
	if err := w.u.d.Repo.InsertAnalysisRuns(ctx, w.org, []entity.AnalysisRun{{
		AnalysisScope: entity.AnalysisScope{RepositoryID: area, SHA: r.SHA},
		ReceiptID:     r.Record.ReceiptID, KnownSeq: w.seq, ToolID: r.SonarAnalysisID, RecordNos: []int{r.Record.RecordNo},
		AnalysisRunMetadata: entity.AnalysisRunMetadata{
			Kind: r.Kind, Profile: r.ProfileID, Mode: r.Mode, Producer: r.Producer,
			StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, ExitClass: r.ExitClass, Completeness: r.Completeness,
			ScopeHash: hex.EncodeToString(r.ScopeHash),
		},
	}}); err != nil {
		return err
	}
	in.sonarRuns = true
	return nil
}
