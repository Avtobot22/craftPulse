package app

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _sonarInstance = "sonar.example"

func sonarPage(t *testing.T, project, analysis, revision string, at time.Time) []byte {
	t.Helper()
	m := manifestOf(envelope.ManifestSchemaIdFindings, envelope.SourceProviderSonarqube, _sonarInstance, project)
	m.Area.Kind = envelope.AreaRefKindAnalysisProject
	var data bytes.Buffer
	w, err := envelope.NewWriter(&data, m)
	require.NoError(t, err)
	ref, err := w.AddFile("files/sonar.json", "application/json", []byte(`{
		"version":"1",
		"measures":{"coverage":"87.40","ncloc":"125"},
		"issues":[
			{"key":"I-1","type":"VULNERABILITY","rule_id":"go:S1","path":"cmd/main.go","line_start":3,"line_end":5,
			 "severity":"CRITICAL","status":"RESOLVED","resolution":"FALSE-POSITIVE","rule_description":"Validate the input."},
			{"key":"I-2","type":"BUG","rule_id":"go:S2","path":"cmd/main.go","line_start":7,"severity":"MAJOR","status":"OPEN"},
			{"key":"I-3","type":"CODE_SMELL","rule_id":"go:S3","path":"cmd/other.go","line_start":8,"severity":"MINOR","status":"OPEN"}
		]
	}`))
	require.NoError(t, err)
	p := envelope.ReportFileV1{
		ReportKind: envelope.ReportFileV1ReportKindSonar, ReportFormat: envelope.ReportFileV1ReportFormatEnvelope,
		Producer: "SonarQube", ProducerVersion: "26.9.0", FinishedAt: &at, Completeness: new(envelope.ReportFileV1Completeness("complete")),
	}
	if revision != "" {
		p.Sha = &revision
	}
	r := mrRecord(t, "report_file", analysis, project, analysis, p)
	r.RecordClass, r.File = envelope.RecordRecordClassFile, &ref
	require.NoError(t, w.AddRecord(r))
	require.NoError(t, w.Close())
	check := envelope.CheckPage(bytes.NewReader(data.Bytes()), envelope.DefaultLimits)
	require.Nil(t, check.Envelope)
	require.Zero(t, check.Rejected())
	return data.Bytes()
}

func TestNormalizeSonar_AnalysisReports(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	const project = "orders"
	repository, component := entity.ID(e.areas["43"]), entity.ID(e.otherComponent)
	var repoScopeID uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id FROM connect.connection_scope WHERE area_id = $1`,
		uuid.UUID(repository)).Scan(&repoScopeID))
	repoScope := entity.ID(repoScopeID)
	conn, scope := e.sourceScope(t, entity.ProviderSonarQube, _sonarInstance, entity.ScopeAnalysisProject, project)
	connections := connectrepo.NewRepo(e.app)
	capability := entity.Capability{Name: entity.CapabilityReadMRAnalysis, State: entity.CapabilityVerified, CheckedAt: e.clock.Now()}
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		return connections.ReplaceCapabilities(ctx, e.orgID, conn, []entity.Capability{capability})
	}))

	at := e.clock.Now()
	first := sonarPage(t, project, "analysis-1", _reportSHA, at)
	rc := e.page(t, conn, scope, first)
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	firstRecords := e.records(t, rc.ID)
	require.Len(t, firstRecords, 1)
	require.Equal(t, entity.RecordAccepted, firstRecords[0].Outcome, firstRecords[0].ReasonCode)
	state, err := e.connect.State(t.Context(), e.orgID, conn)
	require.NoError(t, err)
	assert.Equal(t, "repository_binding_missing", state.Capabilities[0].ReasonCode)
	assert.Equal(t, entity.CapabilityMissing, state.Capabilities[0].State)
	var unbound bool
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT repository_id IS NULL FROM normalize.report WHERE receipt_id=$1`,
		uuid.UUID(rc.ID)).Scan(&unbound))
	assert.True(t, unbound)

	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		return connections.SetScopeOptions(ctx, e.orgID, scope, nil, &repoScope)
	}))
	state, err = e.connect.State(t.Context(), e.orgID, conn)
	require.NoError(t, err)
	assert.Equal(t, entity.CapabilityVerified, state.Capabilities[0].State)

	candidate := sonarPage(t, project, "analysis-2", _reportSHA, at.Add(time.Second))
	accepted := e.page(t, conn, scope, candidate)
	e.drain(t)
	var reportID, repoID uuid.UUID
	var kind, mode, producer, version, sha, parser, completeness string
	var finished time.Time
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id, repository_id, report_kind, mode, producer, producer_version, sha,
		parser_version, finished_at, completeness FROM normalize.report WHERE receipt_id=$1`, uuid.UUID(accepted.ID)).
		Scan(&reportID, &repoID, &kind, &mode, &producer, &version, &sha, &parser, &finished, &completeness))
	assert.Equal(t, repository, entity.ID(repoID))
	assert.Equal(t, []string{"sonar", "import", "SonarQube", "26.9.0", _reportSHA, "sonar-envelope/1", "complete"},
		[]string{kind, mode, producer, version, sha, parser, completeness})
	assert.WithinDuration(t, at.Add(time.Second), finished, time.Microsecond)
	metrics, err := e.norm.SonarMeasures(t.Context(), e.orgID, entity.ID(reportID))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"coverage": "87.40", "ncloc": "125"}, metrics)

	findings := e.normalized.findings(t, accepted.ID, "sonar")
	require.Len(t, findings, 3)
	assert.Equal(t, []string{"security", "code", "code"}, []string{findings[0].kind, findings[1].kind, findings[2].kind})
	assert.Equal(t, "go:S1", findings[0].rule)
	assert.Equal(t, "cmd/main.go", findings[0].path)
	require.NotNil(t, findings[0].finish)
	assert.Equal(t, 3, findings[0].start)
	assert.EqualValues(t, 5, *findings[0].finish)
	assert.Equal(t, "CRITICAL", findings[0].severity)
	assert.JSONEq(t, `{"status":"RESOLVED","resolution":"FALSE-POSITIVE","rule_description":"Validate the input."}`,
		string(findings[0].evidence))
	assert.Equal(t, 3, e.normalized.n(t, `SELECT count(*) FROM change.finding_identity WHERE rule_id IN ('go:S1','go:S2','go:S3')`))

	// Replay both the page and its normalize job, then send an analysis missing revision.
	e.page(t, conn, scope, candidate)
	e.normalized.renormalize(t, accepted.ID)
	broken := e.page(t, conn, scope, sonarPage(t, project, "analysis-3", "", at.Add(2*time.Second)))
	e.drain(t)
	records := e.records(t, broken.ID)
	require.Len(t, records, 1)
	assert.Equal(t, entity.RecordRejected, records[0].Outcome)
	assert.Equal(t, entity.RecordSonarRevisionMissing, records[0].ReasonCode)
	assert.Equal(t, 2, e.normalized.n(t, `SELECT count(*) FROM normalize.report WHERE report_kind='sonar'`))
	assert.Equal(t, 6, e.normalized.n(t, `SELECT count(*) FROM normalize.finding_occurrence`))
	assert.Equal(t, 1, e.normalized.n(t, `SELECT count(*) FROM normalize.area_change WHERE area_id=$1 AND 'report'=ANY(fact_kinds)`, repoID))
	scopes, err := e.norm.ReceiptAnalysisScopes(t.Context(), e.orgID, accepted.ID)
	require.NoError(t, err)
	analysisScope := entity.AnalysisScope{RepositoryID: repository, SHA: _reportSHA}
	require.Contains(t, scopes, analysisScope)
	analysisScope.ComponentID = component
	require.Contains(t, scopes, analysisScope)
	facts, err := e.norm.AnalysisFacts(t.Context(), e.orgID, analysisScope, 1<<62, at.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, facts.Runs, 1)
	require.Len(t, facts.Runs[0].Reports, 1)
	assert.Equal(t, entity.ID(reportID), facts.Runs[0].Reports[0].ID)

	verifySonarSummaryReports(t, e, repository, component, entity.ID(reportID))
	verifySonarSummaryAudience(t, e)

	capability.State, capability.ReasonCode = entity.CapabilityMissing, "edition_unsupported"
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		return connections.ReplaceCapabilities(ctx, e.orgID, conn, []entity.Capability{capability})
	}))
	state, err = e.connect.State(t.Context(), e.orgID, conn)
	require.NoError(t, err)
	assert.Equal(t, capability.ReasonCode, state.Capabilities[0].ReasonCode)
	empty := manifestOf(envelope.ManifestSchemaIdFindings, envelope.SourceProviderSonarqube, _sonarInstance, project)
	empty.Area.Kind = envelope.AreaRefKindAnalysisProject
	e.page(t, conn, scope, pack(t, empty))
	e.drain(t)
	assert.Equal(t, 2, e.normalized.n(t, `SELECT count(*) FROM normalize.report WHERE report_kind='sonar'`))
}

func verifySonarSummaryReports(t *testing.T, e *identityEnv, repository, component, report entity.ID) {
	t.Helper()
	repo := normalizerepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		mr := entity.SummaryMR{AreaID: repository, HeadSHA: _reportSHA, KnowledgeSeq: 1 << 62}
		entry := entity.ProfileEntry{ComponentID: component, AnalysisKind: "code", Mode: "import", ProfileID: "sonar-import", Language: "go"}
		reports, err := repo.SummaryReports(ctx, e.orgID, mr, entry, nil)
		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Equal(t, report, reports[0].ID)

		reports, err = repo.SummaryReports(ctx, e.orgID, mr, entry, []entity.ID{entity.ID(uuid.New())})
		require.NoError(t, err)
		assert.Empty(t, reports, "a Sonar analysis is not an artifact of a CI run")

		mr.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		reports, err = repo.SummaryReports(ctx, e.orgID, mr, entry, nil)
		require.NoError(t, err)
		assert.Empty(t, reports)
		mr.HeadSHA = _reportSHA

		for _, kind := range []string{"secrets", "sca"} {
			entry.AnalysisKind = kind
			reports, err = repo.SummaryReports(ctx, e.orgID, mr, entry, nil)
			require.NoError(t, err)
			assert.Empty(t, reports)
		}
		return nil
	}))
}

func verifySonarSummaryAudience(t *testing.T, e *identityEnv) {
	t.Helper()
	profile := e.confirm(t, e.other, `"0"`, uuid.New(),
		row(e.otherComponent, "go", "code", "import", "sonar-import"),
		row(e.otherComponent, "", "secrets", "builtin", "gitleaks-default"),
		row(e.otherComponent, "", "sca", "builtin", "trivy-default"),
	)
	require.Equal(t, http.StatusCreated, profile.Code, profile.Body.String())
	e.clock.Advance(time.Second)

	manifest := mrManifest("43")
	manifest.Source.InstanceKey = _checksInstance
	const nativeID = "9700"
	page := pack(t, manifest, mrRecord(t, "merge_request", nativeID, "", "v1", envelope.MergeRequestV1{
		ProjectNativeId: "43", Iid: new(7), State: "open", HeadSha: _reportSHA,
		SourceBranch: "feature", TargetBranch: "main", OpenedAt: e.clock.Now(), UpdatedAt: new(e.clock.Now()),
	}))
	receipt := e.submit(t, page)
	require.NotNil(t, receipt.SourceInstanceID)
	mr := mrID(t, e.normalized, nativeID)
	source := &summaryTestAdapter{
		source: entity.AreaSource{
			InstanceID: *receipt.SourceInstanceID, Kind: entity.ScopeRepository, NativeScopeID: "43",
			Active: true, ConnectionStatus: entity.ConnectionActive,
		},
		fetch: func(entity.FetchInput) []byte { return page },
	}
	deps, err := newSummaryDeps(e.app, source, e.norm, new(calc.UseCase), e.store, prometheus.NewRegistry())
	require.NoError(t, err)
	u := newSummarizer(e.app, e.norm, e.orgs, e.clock, e.decisions, deps, e.catalog)
	var jobID int64
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT id FROM river_job
		WHERE kind = 'change.summary' AND state = 'available' AND args->>'mr_id' = $1`, mr.String()).Scan(&jobID))
	require.NoError(t, u.RunSummary(t.Context(), entity.ChangeSummaryJob{OrganizationID: e.orgID, MRID: mr}, completeJob(jobID)))

	summary, err := u.ReadSummary(t.Context(), e.orgID, mr, nil)
	require.NoError(t, err)
	var missing []string
	var observed bool
	for i := range summary.Items {
		item := &summary.Items[i]
		if item.Kind == "not_evaluated" {
			missing = append(missing, item.AnalysisKind)
		}
		observed = observed || item.OccurrenceID != nil
	}
	assert.ElementsMatch(t, []string{"secrets", "sca"}, missing)
	assert.True(t, observed, "the MR summary includes Sonar occurrences")
	repo := changerepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		areas, err := repo.SummarySourceAreas(ctx, e.orgID, summary.ID)
		require.NoError(t, err)
		require.Len(t, areas, 1)
		items, err := repo.PublicationItems(ctx, e.orgID, summary.ID)
		require.NoError(t, err)
		for i := range items {
			assert.Nil(t, items[i].OccurrenceID)
		}
		return nil
	}))

	summaryReadAPI(t, e, *receipt.SourceInstanceID, entity.ID(e.userID(t, e.orgID, _otherSub)))
	path := "/api/v1/merge-requests/" + mr.String() + "/summary"
	// The user has repository access but has no linked SonarQube account.
	require.Equal(t, http.StatusNotFound, e.do(t, e.lead, http.MethodGet, path, nil).Code)
}
