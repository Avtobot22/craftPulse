package app

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/adapter/sonarqube"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_sonarToken    = "fixture-sonar-secret"
	_sonarProject  = "orders:main"
	_sonarHost     = "sonar.example"
	_sonarRevision = "b111111111111111111111111111111111111111"
)

// The response shapes follow the MVP-09-T01 model and contain synthetic data.
//
//go:embed testdata/sonarqube/responses.json
var sonarResponses []byte

type sonarFixture struct {
	responses       map[string]json.RawMessage
	notProject      atomic.Bool
	failure         atomic.Int32
	missingRevision atomic.Bool
	depth           atomic.Bool
	advanced        atomic.Bool
}

func (f *sonarFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+_sonarToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/system/status":
		_, _ = w.Write(f.responses["status"])
	case "/api/components/show":
		if r.URL.Query().Get("component") != _sonarProject {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if f.notProject.Load() {
			_, _ = io.WriteString(w, `{"component":{"key":"orders:main","qualifier":"FIL"}}`)
			return
		}
		_, _ = w.Write(f.responses["project"])
	case "/api/project_analyses/search":
		f.analyses(w, r)
	case "/api/issues/search":
		f.issues(w, r)
	case "/api/measures/component":
		_, _ = w.Write(f.responses["measures"])
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *sonarFixture) analyses(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("project") != _sonarProject {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var analyses []map[string]any
	_ = json.Unmarshal(f.responses["analyses"], &analyses)
	if f.missingRevision.Load() {
		delete(analyses[0], "revision")
	}
	if f.advanced.Load() {
		analyses[0]["key"] = "new-analysis"
	}
	index, size := sonarPageArgs(r)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"paging":   sonarPaging(index, size, len(analyses)),
		"analyses": analyses[:min(size, len(analyses))],
	})
}

func (f *sonarFixture) issues(w http.ResponseWriter, r *http.Request) {
	index, size := sonarPageArgs(r)
	if r.URL.Query().Get("components") != _sonarProject {
		// SonarQube can hide an inaccessible project from issue search with HTTP 200.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"paging": sonarPaging(index, size, 0), "issues": []any{},
		})
		return
	}
	failure := f.failure.Load()
	switch {
	case failure == -1:
		<-r.Context().Done()
		return
	case failure == -2 && index == 2:
		_, _ = io.WriteString(w, `{"paging":`)
		return
	case failure == -3 && index == 2:
		w.WriteHeader(http.StatusNotFound)
		return
	case failure == -6 && index == 2:
		w.WriteHeader(http.StatusBadRequest)
		return
	case failure > 0:
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(int(failure))
		var errorsByCode map[string]json.RawMessage
		_ = json.Unmarshal(f.responses["errors"], &errorsByCode)
		_, _ = w.Write(errorsByCode[strconv.Itoa(int(failure))])
		return
	case failure == -4:
		f.advanced.Store(true)
	}
	total := 502
	if f.depth.Load() {
		total = 10001
		if index > 20 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	items := make([]map[string]any, 0, min(size, total))
	for i := (index - 1) * size; i < min(index*size, total); i++ {
		var item map[string]any
		_ = json.Unmarshal(f.responses["issue"], &item)
		if i != 0 {
			item["key"] = "issue-" + strconv.Itoa(i)
		}
		if failure == -5 && i == 500 && r.URL.Query().Get("additionalFields") == "" {
			item["status"] = "OPEN"
		}
		items = append(items, item)
	}
	components := f.responses["components"]
	if failure == -7 {
		components = json.RawMessage(`[{"key":"orders:main:internal/order/order.go","path":"C:/workspace/order.go","qualifier":"FIL"}]`)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"paging": sonarPaging(index, size, total), "issues": items,
		"rules": f.responses["rules"], "components": components,
	})
}

func sonarPageArgs(r *http.Request) (index int, size int) {
	index, _ = strconv.Atoi(r.URL.Query().Get("p"))
	size, _ = strconv.Atoi(r.URL.Query().Get("ps"))
	return index, size
}

func sonarPaging(index, size, total int) map[string]int {
	return map[string]int{"pageIndex": index, "pageSize": size, "total": total}
}

func newSonarFixture(t *testing.T) (gitLabTestEnv, *sonarFixture) {
	t.Helper()
	f := &sonarFixture{}
	require.NoError(t, json.Unmarshal(sonarResponses, &f.responses))
	return newSonarTestEnv(t, f, _sonarProject), f
}

func newSonarTestEnv(t *testing.T, upstreamHandler http.Handler, project string) gitLabTestEnv {
	t.Helper()
	ca := newTestCA(t, "Sonar fixtures")
	upstream := httptest.NewUnstartedServer(upstreamHandler)
	upstream.TLS, _ = ca.serverTLS(t, time.Now().Add(time.Hour), _sonarHost)
	upstream.StartTLS()
	t.Cleanup(upstream.Close)
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		RootCAs: ca.pool, Timeout: 3 * time.Second, Resolver: mapResolver{_sonarHost: _gitPublic},
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	db := newAdapterDB(t, true)
	logs := &lockedBuffer{}
	handler, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: sonarqube.New(client), Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)),
		ServiceKeys: []string{_adapterKey}, Log: zerolog.New(logs), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindAnalysisProject, NativeScopeId: project}
	area := adaptersdk.AccessItemAreaKindAnalysisProject
	return gitLabTestEnv{logs: logs, target: contracttest.Target{
		BaseURL: srv.URL, ServiceKey: _adapterKey, Organization: uuid.NewString(), Connection: uuid.NewString(),
		Secret:   `{"token":"` + _sonarToken + `"}`,
		Probe:    adaptersdk.ProbeInput{BaseUrl: "https://SONAR.example:443/", Scopes: []adaptersdk.ScopeRef{scope}},
		Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdFindings, Scope: scope, Limit: 1, ObjectKeys: []adaptersdk.ObjectKey{}},
		Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdFindings, Scope: scope, Limit: 1},
		CheckAccess: &adaptersdk.CheckAccessInput{UserId: uuid.New(), Items: []adaptersdk.AccessItem{
			{ItemKind: adaptersdk.AccessItemItemKindArea, AreaKind: &area, NativeId: project},
		}},
	}}
}

func TestContract_SonarQubeAdapter(t *testing.T) {
	t.Parallel()
	e, _ := newSonarFixture(t)
	contracttest.Run(t, e.target)
	assert.Contains(t, e.logs.String(), "SonarQube request")
	assert.NotContains(t, e.logs.String(), _sonarToken)
}

func TestSonarQubeReports(t *testing.T) {
	t.Parallel()
	e, f := newSonarFixture(t)
	probe := e.probe(t)
	require.Equal(t, "fixture-server", probe.InstanceKey)
	require.NotNil(t, probe.Edition)
	assert.Equal(t, "Community Build", *probe.Edition)
	require.NotNil(t, probe.Version)
	assert.Equal(t, "26.11.0.134175", *probe.Version)
	assert.Equal(t, new(adaptersdk.ProbeResultAccessModeUserToken), probe.AccessMode)
	require.Len(t, probe.Capabilities, 3)
	assert.Equal(t, adaptersdk.CapabilityStateVerified, probe.Capabilities[0].State)
	assert.Equal(t, adaptersdk.CapabilityCapabilityReadBranchAnalysis, probe.Capabilities[1].Capability)
	assert.Equal(t, adaptersdk.CapabilityStatePartial, probe.Capabilities[1].State)
	assert.Equal(t, new("edition_does_not_support_branch_analysis"), probe.Capabilities[1].ReasonCode)
	assert.Equal(t, adaptersdk.CapabilityCapabilityReadMrAnalysis, probe.Capabilities[2].Capability)
	assert.Equal(t, adaptersdk.CapabilityStatePartial, probe.Capabilities[2].State)
	assert.Equal(t, "edition_does_not_support_mr_analysis", *probe.Capabilities[2].ReasonCode)

	mixed := e.target.Probe
	mixed.Scopes = append([]adaptersdk.ScopeRef{}, mixed.Scopes...)
	mixed.Scopes = append(mixed.Scopes, adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindAnalysisProject, NativeScopeId: "private"})
	status, body := e.call(t, http.MethodPost, "probe", mixed)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.NoError(t, json.Unmarshal(body, &probe))
	assert.Equal(t, adaptersdk.CapabilityStatePartial, probe.Capabilities[0].State)
	require.Len(t, probe.Scopes, 2)
	assert.Equal(t, adaptersdk.ScopeCheckStateMissing, probe.Scopes[1].State)

	f.notProject.Store(true)
	probe = e.probe(t)
	assert.Equal(t, adaptersdk.CapabilityStateMissing, probe.Capabilities[0].State)
	require.Len(t, probe.Scopes, 1)
	assert.Equal(t, adaptersdk.ScopeCheckStateMissing, probe.Scopes[0].State)
	f.notProject.Store(false)

	in := e.target.Fetch
	_, older, _ := sonarRecords(t, e, in)
	require.Len(t, older, 1)
	require.Contains(t, older, "report_file/older")

	in.ObjectKeys = []adaptersdk.ObjectKey{{
		ObjectKind: adaptersdk.ObjectKeyObjectKindReportFile, NativeNs: _sonarProject, NativeId: "latest",
	}}
	spec, err := contracttest.GetSpec()
	require.NoError(t, err)
	keyKind := spec.Components.Schemas["ObjectKey"].Value.Properties["object_kind"].Value
	require.NoError(t, keyKind.VisitJSON(string(in.ObjectKeys[0].ObjectKind)))
	_, records, reports := sonarRecords(t, e, in)
	require.Len(t, records, 1)
	rec := records["report_file/latest"]
	checkSonarReport(t, reports, rec, envelope.SonarAnalysisFileIssuesStateComplete, new(_sonarRevision))
	report := reports[rec.File.Path]
	require.Len(t, report.Issues, 502)
	assert.True(t, slices.ContainsFunc(report.Issues, func(issue envelope.SonarIssue) bool { return issue.IssueId == "issue-501" }))
	issue := report.Issues[len(report.Issues)-1]
	assert.Equal(t, "issue-A", issue.IssueId)
	assert.Equal(t, new("FALSE-POSITIVE"), issue.Resolution)
	assert.Equal(t, "RESOLVED", issue.Status)
	assert.Equal(t, "MAJOR", issue.Severity)
	assert.Equal(t, envelope.SonarIssueTypeCODESMELL, issue.Type)
	assert.Equal(t, new("internal/order/order.go"), issue.Path)
	assert.Equal(t, new(8), issue.LineStart)
	assert.Equal(t, new(9), issue.LineEnd)
	assert.Equal(t, &envelope.SonarRuleInfo{Key: "go:S1481", Name: "Unused local variables", Language: "go", Repository: "go"}, issue.Rule)
	data, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "fixture-source-snippet")

	manifestIn := e.target.Manifest
	manifestIn.Limit = 10
	status, body = e.call(t, http.MethodPost, "manifest", manifestIn)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var manifest adaptersdk.ManifestPage
	require.NoError(t, json.Unmarshal(body, &manifest))
	require.Len(t, manifest.Items, 2)
	assert.Equal(t, "latest", manifest.Items[1].NativeId)
	assert.Equal(t, new(502), manifest.Items[1].IssueCount)
	assert.Nil(t, manifest.Items[0].IssueCount)

	status, body = e.call(t, http.MethodPost, "check-access", e.target.CheckAccess)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var access adaptersdk.CheckAccessResult
	require.NoError(t, json.Unmarshal(body, &access))
	require.Len(t, access.Decisions, 1)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, access.Decisions[0].Decision)
	assert.Equal(t, new(60), access.MaxAgeSeconds)

	f.missingRevision.Store(true)
	pkg, records, reports := sonarRecords(t, e, in)
	require.Len(t, records, 1)
	checkSonarReport(t, reports, records["report_file/latest"], envelope.SonarAnalysisFileIssuesStateMissingRevision, nil)
	assert.Equal(t, envelope.ManifestCompletenessPartial, pkg.Manifest.Completeness)
}

func TestSonarQubePartialWindowAndFailures(t *testing.T) {
	t.Parallel()
	e, f := newSonarFixture(t)
	e.probe(t)
	in := e.target.Fetch
	in.Limit = 2
	f.depth.Store(true)
	pkg, records, reports := sonarRecords(t, e, in)
	require.Len(t, records, 2)
	assert.Len(t, reports[records["report_file/latest"].File.Path].Issues, 10000)
	checkSonarReport(t, reports, records["report_file/latest"], envelope.SonarAnalysisFileIssuesStateIssueSearchWindowExceeded, new(_sonarRevision))
	assert.Equal(t, envelope.ManifestCompletenessPartial, pkg.Manifest.Completeness)
	f.depth.Store(false)

	for _, tc := range []struct {
		name     string
		upstream int32
		status   int
		code     adaptersdk.AdapterErrorCode
	}{
		{"invalid_token", 401, 403, adaptersdk.AdapterErrorCodeAuth},
		{"browse_denied", 403, 403, adaptersdk.AdapterErrorCodePermissionMissing},
		{"rate_limit", 429, 429, adaptersdk.AdapterErrorCodeRateLimited},
		{"source_unavailable", 503, 503, adaptersdk.AdapterErrorCodeUnavailable},
		{"timeout", -1, 504, adaptersdk.AdapterErrorCodeTimeout},
		{"truncated_page", -2, 503, adaptersdk.AdapterErrorCodeUnavailable},
		{"missing_page", -3, 503, adaptersdk.AdapterErrorCodeUnavailable},
		{"rejected_page", -6, 503, adaptersdk.AdapterErrorCodeUnavailable},
		{"issue_changed_with_same_count", -5, 503, adaptersdk.AdapterErrorCodeUnavailable},
		{"absolute_issue_path", -7, 502, adaptersdk.AdapterErrorCodeInvalidResponse},
		{"new_analysis", -4, 503, adaptersdk.AdapterErrorCodeUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.failure.Store(tc.upstream)
			status, body := e.call(t, http.MethodPost, "fetch", in)
			require.Equal(t, tc.status, status, "%s", body)
			var response adaptersdk.AdapterError
			require.NoError(t, json.Unmarshal(body, &response))
			assert.Equal(t, tc.code, response.Code)
			if tc.upstream == 429 {
				assert.NotNil(t, response.RetryAt)
			}
			assert.NotContains(t, string(body), _sonarToken)
		})
	}
}

func sonarRecords(t *testing.T, e gitLabTestEnv, in adaptersdk.FetchInput) (pkg *envelope.Package, records map[string]envelope.Record, reports map[string]envelope.SonarAnalysisFile) {
	t.Helper()
	status, body := e.call(t, http.MethodPost, "fetch", in)
	require.Equal(t, http.StatusOK, status, "%s", body)
	result := envelope.CheckPage(bytes.NewReader(body), envelope.DefaultLimits)
	require.Nil(t, result.Envelope)
	pkg, err := envelope.ReadCleaned(bytes.NewReader(body), envelope.DefaultLimits)
	require.NoError(t, err)
	records = make(map[string]envelope.Record, len(result.Records))
	var files []string
	for _, row := range result.Records {
		require.Nil(t, row.Rejection, "record %d: %v", row.No, row.Rejection)
		rec, ok, err := pkg.Record(row.No)
		require.NoError(t, err)
		require.True(t, ok)
		key := string(rec.RecordKind) + "/" + rec.NativeId
		require.NotContains(t, records, key)
		records[key] = *rec
		if rec.File != nil {
			files = append(files, rec.File.Path)
		}
	}
	reports = make(map[string]envelope.SonarAnalysisFile, len(files))
	require.NoError(t, envelope.Files(bytes.NewReader(body), files, func(name string, content io.Reader) error {
		var report envelope.SonarAnalysisFile
		if err := json.NewDecoder(content).Decode(&report); err != nil {
			return err
		}
		reports[name] = report
		return nil
	}))
	return pkg, records, reports
}

func checkSonarReport(t *testing.T, reports map[string]envelope.SonarAnalysisFile, rec envelope.Record, state envelope.SonarAnalysisFileIssuesState, revision *string) {
	t.Helper()
	require.NotNil(t, rec.File)
	var payload envelope.ReportFileV1
	require.NoError(t, json.Unmarshal(rec.Payload, &payload))
	assert.Equal(t, envelope.ReportFileV1ReportKindSonar, payload.ReportKind)
	assert.Equal(t, "26.11.0.134175", payload.ProducerVersion)
	assert.Equal(t, revision, payload.Sha)
	assert.Equal(t, time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), *payload.FinishedAt)
	report := reports[rec.File.Path]
	assert.Equal(t, envelope.ReportFileV1ReportFormatEnvelope, payload.ReportFormat)
	assert.Equal(t, envelope.SonarAnalysisFileVersionN1, report.Version)
	assert.Equal(t, &state, report.IssuesState)
	assert.Equal(t, map[string]string{"ncloc": "531", "coverage": "69.4"}, report.Measures)
}
