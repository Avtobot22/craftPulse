package sonarqube

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_metricKeys = "ncloc,bugs,vulnerabilities,code_smells,coverage,duplicated_lines_density"
	// Every analysis adds a file. Keep room in a package for the latest analysis and its findings.
	_reportBatch = 100
)

func (s source) measures(ctx context.Context, project string) (map[string]string, error) {
	var reply struct {
		Component struct {
			Key      string `json:"key"`
			Measures []struct {
				Metric string  `json:"metric"`
				Value  *string `json:"value"`
			} `json:"measures"`
		} `json:"component"`
	}
	query := url.Values{"component": {project}, "metricKeys": {_metricKeys}}
	if err := s.get(ctx, "/api/measures/component", query, &reply); err != nil {
		return nil, err
	}
	if reply.Component.Key != project || reply.Component.Measures == nil {
		return nil, adaptersdk.InvalidResponseError("invalid SonarQube measures response")
	}
	allowed := strings.Split(_metricKeys, ",")
	out := make(map[string]string, len(allowed))
	for _, m := range reply.Component.Measures {
		if !slices.Contains(allowed, m.Metric) || m.Value == nil || !textFits(*m.Value, 255) {
			return nil, adaptersdk.InvalidResponseError("invalid SonarQube measure")
		}
		if _, duplicate := out[m.Metric]; duplicate {
			return nil, adaptersdk.InvalidResponseError("duplicate SonarQube measure")
		}
		out[m.Metric] = *m.Value
	}
	return out, nil
}

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	if !validScope(in.Scope) || in.Limit < 1 || in.Limit > 1000 ||
		in.Cursor != nil && (in.Cursor.ModifiedAt.IsZero() || in.Cursor.NativeId == "" || !textFits(in.Cursor.NativeId, 255)) {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid SonarQube fetch scope, limit or cursor")
	}
	s, err := a.bound(ctx, call)
	if err != nil {
		return nil, err
	}
	project := in.Scope.NativeScopeId
	m := reportManifest(in, s.binding)
	switch in.SchemaId {
	case adaptersdk.FetchInputSchemaIdFindings:
	case adaptersdk.FetchInputSchemaIdJunit, adaptersdk.FetchInputSchemaIdCoverage:
		m.Selection.TotalCount = new(0)
		return m, nil
	default:
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "schema is not supported by SonarQube")
	}
	items, latest, err := s.readAnalyses(ctx, project, in.Period, in.ObjectKeys, in.Cursor)
	if err != nil {
		return nil, err
	}
	limit := min(in.Limit, len(items), _reportBatch)
	if in.Cursor == nil {
		m.Selection.TotalCount = new(len(items))
	}
	if len(items) > limit {
		last := items[limit-1]
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{NativeId: last.Key, ModifiedAt: last.at}
	}
	for _, item := range items[:limit] {
		if err := s.writeAnalysis(ctx, page, m, project, item, latest.Key); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func reportManifest(in adaptersdk.FetchInput, binding adaptersdk.SourceBinding) *envelope.Manifest {
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(in.SchemaId), SchemaVersion: "1",
		Source:    envelope.Source{Provider: envelope.SourceProviderSonarqube, InstanceKey: binding.InstanceKey, Edition: optional(binding.Edition), Version: &binding.Version},
		Area:      envelope.AreaRef{Kind: envelope.AreaRefKindAnalysisProject, NativeId: in.Scope.NativeScopeId},
		Period:    envelope.Period{From: in.Period.From, To: in.Period.To},
		CreatedAt: time.Now().UTC(), Completeness: envelope.ManifestCompletenessComplete,
		Producer:  envelope.Producer{Name: "adapter-sonarqube", Version: "1"},
		Selection: envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
	}
	if in.Cursor != nil {
		m.Selection.Cursor = &envelope.Cursor{NativeId: in.Cursor.NativeId, ModifiedAt: in.Cursor.ModifiedAt}
	}
	return m
}

func (s source) writeAnalysis(ctx context.Context, page *adaptersdk.Page, m *envelope.Manifest, project string, item analysis, latestKey string) error {
	report := envelope.SonarAnalysisFile{
		Version:  envelope.SonarAnalysisFileVersionN1,
		Issues:   []envelope.SonarIssue{},
		Measures: map[string]string{},
	}
	issuesState := envelope.SonarAnalysisFileIssuesStateHistorical
	if item.Key == latestKey {
		var err error
		report.Measures, err = s.measures(ctx, project)
		if err != nil {
			return err
		}
		if item.Revision != "" {
			current, err := s.currentIssues(ctx, project)
			if err != nil {
				return err
			}
			issuesState = envelope.SonarAnalysisFileIssuesStateComplete
			keys := slices.Sorted(maps.Keys(current.issues))
			report.Issues = make([]envelope.SonarIssue, 0, len(keys))
			for _, key := range keys {
				issue := current.issues[key]
				report.Issues = append(report.Issues, issue.reportIssue(current.rules[issue.issue.Rule]))
			}
			if current.total >= _issueWindow {
				issuesState = envelope.SonarAnalysisFileIssuesStateIssueSearchWindowExceeded
			}
		}
		if err := s.confirmLatest(ctx, project, item); err != nil {
			return err
		}
	}
	if item.Revision == "" {
		issuesState = envelope.SonarAnalysisFileIssuesStateMissingRevision
	}
	state := envelope.ReportFileV1CompletenessComplete
	switch issuesState {
	case envelope.SonarAnalysisFileIssuesStateComplete:
	case envelope.SonarAnalysisFileIssuesStateHistorical:
		state = envelope.ReportFileV1CompletenessUnknown
	case envelope.SonarAnalysisFileIssuesStateMissingRevision, envelope.SonarAnalysisFileIssuesStateIssueSearchWindowExceeded:
		state = envelope.ReportFileV1CompletenessPartial
		m.Completeness = envelope.ManifestCompletenessPartial
	}
	report.IssuesState = &issuesState
	return s.addReport(page, project, item, report, state, m.CreatedAt)
}

func (s source) addReport(page *adaptersdk.Page, project string, item analysis, report envelope.SonarAnalysisFile, state envelope.ReportFileV1Completeness, observed time.Time) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	name := sha256.Sum256([]byte(item.Key))
	ref, err := page.AddFile("files/sonar/"+hex.EncodeToString(name[:])+".json", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	payload, err := json.Marshal(envelope.ReportFileV1{
		ReportKind: envelope.ReportFileV1ReportKindSonar, ReportFormat: envelope.ReportFileV1ReportFormatEnvelope,
		Sha: optional(item.Revision), ArtifactDigest: &ref.Sha256, Producer: "sonarqube", ProducerVersion: s.binding.Version,
		FinishedAt: &item.at, Completeness: &state,
	})
	if err != nil {
		return err
	}
	return page.AddRecord(&envelope.Record{
		RecordKind: envelope.RecordRecordKindReportFile, RecordClass: envelope.RecordRecordClassFile,
		PayloadSchema: "report_file/1", NativeId: item.Key, NativeNs: project,
		ObservedTime: &observed, Payload: payload, File: &ref,
	})
}
