package app

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// The captured local Web API responses retain only fields read by the adapter.
//
//go:embed testdata/sonarqube/local-responses.json
var sonarLocalResponses []byte

func TestSonarQubeLocalEvidence(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../deploy/sonarqube/fixture-analyses.local.json")
	require.NoError(t, err)
	var evidence struct {
		Project  string
		Analyses []struct {
			Key, Revision, Date string
			Issues              []struct {
				Key, Rule, Type, Severity, Status string
				Resolution, Path                  *string
				Start                             *int `json:"line_start"`
				End                               *int `json:"line_end"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(data, &evidence))
	require.Len(t, evidence.Analyses, 2)

	var responses struct {
		Status, Measures, Components, Rules json.RawMessage
		Analyses, Issues                    []json.RawMessage
	}
	require.NoError(t, json.Unmarshal(sonarLocalResponses, &responses))
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+_sonarToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/system/status":
			_, _ = w.Write(responses.Status)
		case "/api/components/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"component": map[string]string{"key": evidence.Project, "qualifier": "TRK"},
			})
		case "/api/measures/component":
			_, _ = w.Write(responses.Measures)
		case "/api/project_analyses/search":
			writeSonarEvidencePage(w, r, "analyses", responses.Analyses, map[string]any{})
		case "/api/issues/search":
			writeSonarEvidencePage(w, r, "issues", responses.Issues, map[string]any{
				"components": responses.Components, "rules": responses.Rules,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	e := newSonarTestEnv(t, upstream, evidence.Project)
	probe := e.probe(t)
	require.Len(t, probe.Scopes, 1)
	require.Equal(t, adaptersdk.ScopeCheckStateVerified, probe.Scopes[0].State)
	in := e.target.Fetch
	in.Limit = 2
	_, records, reports := sonarRecords(t, e, in)

	latest := evidence.Analyses[1]
	require.NotEmpty(t, latest.Issues)
	require.Len(t, reports, 2)
	require.Len(t, records, 2)
	byKey := make(map[string]envelope.SonarIssue, len(latest.Issues))
	for _, analysis := range evidence.Analyses {
		rec := records["report_file/"+analysis.Key]
		require.NotNil(t, rec.File)
		var payload envelope.ReportFileV1
		require.NoError(t, json.Unmarshal(rec.Payload, &payload))
		date, err := time.Parse("2006-01-02T15:04:05-0700", analysis.Date)
		require.NoError(t, err)
		require.NotNil(t, payload.FinishedAt)
		require.True(t, date.Equal(*payload.FinishedAt))
		require.Equal(t, &analysis.Revision, payload.Sha)
		if analysis.Key == latest.Key {
			report := reports[rec.File.Path]
			require.Len(t, report.Measures, 6)
			require.Len(t, report.Issues, len(latest.Issues))
			for _, issue := range report.Issues {
				byKey[issue.IssueId] = issue
			}
		} else {
			require.Empty(t, reports[rec.File.Path].Issues)
		}
	}
	for _, issue := range latest.Issues {
		payload, ok := byKey[issue.Key]
		require.True(t, ok, issue.Key)
		require.Equal(t, issue.Rule, payload.RuleId)
		require.Equal(t, &envelope.SonarRuleInfo{
			Key: issue.Rule, Language: "go", Repository: "go",
			Name: "Cognitive Complexity of functions should not be too high",
		}, payload.Rule)
		require.Equal(t, envelope.SonarIssueType(issue.Type), payload.Type)
		require.Equal(t, issue.Severity, payload.Severity)
		require.Equal(t, issue.Status, payload.Status)
		require.Equal(t, issue.Resolution, payload.Resolution)
		require.Equal(t, issue.Path, payload.Path)
		require.Equal(t, issue.Start, payload.LineStart)
		require.Equal(t, issue.End, payload.LineEnd)
	}
	status, archive := e.call(t, http.MethodPost, "fetch", in)
	require.Equal(t, http.StatusOK, status)
	normalized := newIdentityEnv(t)
	conn, scope := normalized.sourceScope(t, entity.ProviderSonarQube, probe.InstanceKey, entity.ScopeAnalysisProject, evidence.Project)
	receipt := normalized.page(t, conn, scope, archive)
	require.Equal(t, entity.ReceiptAccepted, receipt.Status)
	normalized.drain(t)
	for _, record := range normalized.records(t, receipt.ID) {
		require.Equal(t, entity.RecordAccepted, record.Outcome, "report rejected: %s", record.ReasonCode)
	}

	var reportID uuid.UUID
	require.NoError(t, normalized.owner.QueryRow(t.Context(), `
		SELECT id FROM normalize.report WHERE receipt_id=$1 AND sonar_analysis_id=$2 AND sha=$3`,
		uuid.UUID(receipt.ID), latest.Key, latest.Revision).Scan(&reportID))
	measures, err := normalized.norm.SonarMeasures(t.Context(), normalized.orgID, entity.ID(reportID))
	require.NoError(t, err)
	require.Len(t, measures, 6)
	var issueIDs []string
	require.NoError(t, normalized.owner.QueryRow(t.Context(), `
		SELECT array_agg(native_id) FROM normalize.finding_occurrence WHERE report_id=$1 AND evidence->'rule'->>'language'='go'`,
		reportID).Scan(&issueIDs))
	expectedIDs := make([]string, 0, len(latest.Issues))
	for _, issue := range latest.Issues {
		expectedIDs = append(expectedIDs, issue.Key)
	}
	require.ElementsMatch(t, expectedIDs, issueIDs)
}

func writeSonarEvidencePage(w http.ResponseWriter, r *http.Request, collection string, rows []json.RawMessage, body map[string]any) {
	index, size := sonarPageArgs(r)
	if index != 1 || size < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body["paging"] = sonarPaging(index, size, len(rows))
	body[collection] = rows[:min(size, len(rows))]
	_ = json.NewEncoder(w).Encode(body)
}
