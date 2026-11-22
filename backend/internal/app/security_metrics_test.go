package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestSecurityMetrics_FromImportedReports(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	seed := sarifSpecs()[0]
	seed.ci = nil
	receipt := e.imported(t, e.admin, reportPackage(t, _reportSHA, seed))
	e.drain(t)
	area := mrArea(t, e, receipt, "42")
	component := uuid.Must(uuid.NewV7())
	var service uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "INSERT INTO org.service (organization_id, name, status, created_at) VALUES ($1, 'security', 'active', now()) RETURNING id",
		uuid.UUID(e.orgID)).Scan(&service))
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.component (organization_id, id, known_seq, service_id, repository_area_id, path_prefix, status, origin,
		confirmed_by, confirmed_at) VALUES ($1, $2, 1, $3, $4, '.', 'confirmed', 'manual', $5, now())`,
		uuid.UUID(e.orgID), component, service, uuid.UUID(area), uuid.UUID(e.admin.UserID))
	require.NoError(t, err)

	const marker = "synthetic-secret-marker-176"
	gitleaks := []byte(`[{"RuleID":"synthetic-token","File":"old.env","StartLine":5,"Secret":"` + marker +
		`","Match":"` + marker + `","Description":"` + marker + `"}]`)
	specs := []reportSpec{
		{fixture: "opengrep.sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1"},
		{fixture: "trivy-sca.json", kind: "sca", format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.76.2"},
		{fixture: "trivy-iac.json", kind: "iac", format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.76.2"},
		{fixture: "gitleaks.sarif", kind: "secrets", format: "sarif", producer: "gitleaks", version: "8.30.1"},
		{
			name: "history.json", content: gitleaks, kind: "secrets", format: "gitleaks_json", producer: "gitleaks", version: "8.30.1",
			history: &envelope.ScanHistory{FromSha: strings.Repeat("b", 40), ToSha: _reportSHA},
		},
	}
	for i := range specs {
		specs[i].component = &envelope.ScanComponent{Id: &component, PathPrefix: "."}
		specs[i].exit, specs[i].completeness = "findings", "complete"
	}
	seqBefore, err := e.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	imported := e.imported(t, e.admin, reportPackage(t, _reportSHA, specs...))
	require.Equal(t, entity.ReceiptAccepted, imported.Status)
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, imported.ID).Status)
	seq, err := e.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	calculator, err := newCalc(e.app, calcModules{facts: e.norm, security: e.norm}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	buildCalcJobs(t, e.owner, e.clock, calculator)
	scope := entity.SecurityScope{AreaID: area, ComponentID: ptr(entity.ID(component)), SHA: _reportSHA, KnownSeq: seq}
	revision, err := calculator.SecurityAt(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	require.Len(t, revision.Definitions, 8)
	require.Len(t, revision.Reports, 5)
	for id, value := range map[string]float64{"S09": 8, "S12": 1, "S13": 3, "S14": 2, "S15": 0.5, "S16": 2, "S18": 1, "S19": 2} {
		m := revision.Metrics[id]
		assert.Equal(t, entity.StateAssessed, m.State, id)
		require.NotNil(t, m.Value, id)
		assert.Equal(t, value, *m.Value, id)
	}
	m := revision.Metrics
	var scaHigh *int
	for _, group := range m["S09"].Groups {
		if group.Kind == entity.AnalysisSCA && group.NativeSeverity == "HIGH" {
			scaHigh = group.Count
		}
	}
	assert.Equal(t, ptr(2), scaHigh)
	require.Len(t, m["S12"].Groups, 1)
	assert.Equal(t, "CWE-89", m["S12"].Groups[0].CWE)
	assert.Equal(t, "MEDIUM", m["S12"].Contributions[0].Findings[0].NativeConfidence)
	assert.Len(t, m["S13"].Contributions, 3)
	require.Len(t, m["S15"].Groups, 3)
	assert.Equal(t, ptr(1), m["S15"].Groups[2].Count)
	require.Len(t, m["S19"].Groups, 2)
	assert.Equal(t, entity.SecretScopeHead, m["S19"].Groups[0].Scope)
	assert.Equal(t, entity.SecretScopeDiff, m["S19"].Groups[1].Scope)
	assert.Equal(t, ptr(1), m["S19"].Groups[0].Count)
	assert.Equal(t, ptr(1), m["S19"].Groups[1].Count)
	raw, err := json.Marshal(revision)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), marker)
	assert.NotContains(t, string(raw), "demo-secret-snippet")
	for _, f := range e.findings(t, imported.ID, entity.AnalysisSecrets) {
		assert.NotContains(t, f.evidence, marker)
		assert.NotContains(t, f.evidence, "demo-secret-snippet")
	}

	series, err := calculator.SecuritySeries(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	stored, err := calculator.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionPublished, stored.Status)
	inputs, err := calculator.RevisionInputs(t.Context(), e.orgID, stored.ID)
	require.NoError(t, err)
	assert.Len(t, inputs, 13)
	points, err := calculator.RevisionPoints(t.Context(), e.orgID, stored.ID)
	require.NoError(t, err)
	require.Len(t, points, 10)
	for _, p := range points {
		if p.Estimator != "" {
			assert.Equal(t, ptr(1.0), p.Value)
			continue
		}
		assert.Equal(t, m[p.MetricID].Value, p.Value, p.MetricID)
	}
	s13, err := calculator.PointContributions(t.Context(), e.orgID, stored.ID, entity.ContributionQuery{MetricID: "S13", PointStart: stored.Window.From})
	require.NoError(t, err)
	require.Len(t, s13.Items, 3)
	for _, item := range s13.Items {
		assert.Equal(t, entity.ObjectAffectedPackage, item.ObjectKind)
		assert.Equal(t, entity.RoleNumerator, item.Role)
		assert.Contains(t, string(item.Evidence), "package-lock.json")
	}
	s19, err := calculator.PointContributions(t.Context(), e.orgID, stored.ID, entity.ContributionQuery{MetricID: "S19", PointStart: stored.Window.From})
	require.NoError(t, err)
	require.Len(t, s19.Items, 2)
	for _, item := range s19.Items {
		assert.NotContains(t, string(item.Evidence), marker)
		assert.NotContains(t, string(item.Evidence), "demo-secret-snippet")
	}

	for _, other := range []entity.SecurityScope{
		{AreaID: area, SHA: _reportSHA, KnownSeq: seq},
		{AreaID: area, ComponentID: scope.ComponentID, SHA: _reportSHA, KnownSeq: seqBefore},
		{AreaID: area, ComponentID: scope.ComponentID, SHA: strings.Repeat("f", 40), KnownSeq: seq},
	} {
		missing, err := calculator.SecurityAt(t.Context(), e.orgID, other)
		require.NoError(t, err)
		assert.Equal(t, entity.StateNotEvaluated, missing.Metrics["S13"].State)
		assert.Nil(t, missing.Metrics["S13"].Value)
	}

	failed := specs[1]
	failed.exit, failed.completeness = "failed", "unknown"
	failedSHA := strings.Repeat("e", 40)
	emptyDiff := specs[4]
	emptyDiff.content = []byte(`[]`)
	emptyDiff.profileID, emptyDiff.exit = "gitleaks-default", "success"
	emptyDiff.history = &envelope.ScanHistory{FromSha: failedSHA, ToSha: failedSHA}
	emptyDiff.omitScannedCount = true
	e.imported(t, e.admin, reportPackage(t, failedSHA, failed, emptyDiff))
	e.drain(t)
	scope.KnownSeq, err = e.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	scope.SHA = failedSHA
	failedRevision, err := calculator.SecurityAt(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	assert.Equal(t, entity.StateUnknown, failedRevision.Metrics["S13"].State)
	assert.Nil(t, failedRevision.Metrics["S13"].Value)
	secretGroups := failedRevision.Metrics["S19"].Groups
	require.Len(t, secretGroups, 2)
	assert.Equal(t, entity.StateNotEvaluated, secretGroups[0].State)
	assert.Equal(t, entity.StateAssessed, secretGroups[1].State)
	assert.Equal(t, ptr(0), secretGroups[1].Count)
}
