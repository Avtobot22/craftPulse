package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestCalcAnalysis_ManifestReportsAndTimeoutPublishRevisions(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	reference := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	e.clock.Advance(reference.Sub(e.clock.Now()))
	calculator, err := newCalc(e.app, calcModules{facts: e.norm}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	sca := reportSpec{
		fixture: "trivy-sca.json", kind: "sca", format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.76.2",
		profileID: "trivy-default", exit: "findings", completeness: "complete",
	}
	sast := reportSpec{
		fixture: "opengrep.sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1",
		profileID: "opengrep-default", exit: "findings", completeness: "complete", mode: envelope.ScanToolRunModeImport,
	}
	report := e.imported(t, e.admin, reportPackage(t, _reportSHA, sca, sast))
	require.Equal(t, entity.ReceiptAccepted, report.Status)
	e.drain(t)
	area := mrArea(t, e, report, "42")
	buildCalcJobs(t, e.owner, e.clock, calculator)
	scope := entity.AnalysisScope{RepositoryID: area, SHA: _reportSHA}
	series, err := calculator.AnalysisSeries(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	first, err := calculator.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.Equal(t, entity.RevisionPublished, first.Status)
	points := analysisRevisionPoints(t, calculator, e.orgID, first.ID)
	assert.Equal(t, ptr(1.0), points["C03"].Value)
	assert.Equal(t, ptr(1.0), points["S07"].Value)
	assert.Equal(t, entity.StateUnknown, points["S02"].State)
	assert.Equal(t, "eligible_files_unknown", points["S02"].ReasonCode)
	assert.Equal(t, entity.StateProvisional, points["S03"].State)
	assert.Equal(t, ptr(1.0), points["S03"].Numerator)
	assert.Equal(t, ptr(1.0), points["S03"].Denominator)
	assert.Equal(t, ptr(first.AsOf.Sub(time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)).Seconds()), points["S05"].Value)

	e.clock.Advance(time.Hour)
	timeout := e.imported(t, e.admin, analysisTimeoutPackage(t, e.clock.Now().Add(-time.Minute)))
	require.Equal(t, entity.ReceiptAccepted, timeout.Status)
	assert.Zero(t, timeout.AcceptedCount)
	e.drain(t)
	buildCalcJobs(t, e.owner, e.clock, calculator)
	require.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.analysis_run WHERE receipt_id = $1", uuid.UUID(timeout.ID)))
	second, err := calculator.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	points = analysisRevisionPoints(t, calculator, e.orgID, second.ID)
	for _, id := range []string{"C03", "S07"} {
		assert.Equal(t, ptr(0.0), points[id].Value)
		assert.Equal(t, ptr(1.0), points[id].Denominator)
		page, err := calculator.PointContributions(t.Context(), e.orgID, second.ID, entity.ContributionQuery{MetricID: id, PointStart: second.Window.From})
		require.NoError(t, err)
		require.Len(t, page.Items, 2)
		assert.Equal(t, "analysis_timeout", page.Items[0].ReasonCode)
		var evidence entity.AnalysisRunMetadata
		require.NoError(t, json.Unmarshal(page.Items[0].Evidence, &evidence))
		assert.Equal(t, "timeout", evidence.ExitClass)
		assert.Equal(t, "partial", evidence.Completeness)
	}
	assert.Equal(t, ptr(second.AsOf.Sub(time.Date(2026, 11, 17, 10, 1, 10, 0, time.UTC)).Seconds()), points["profile/opengrep-default"].Value)

	tomorrow := second.AsOf.AddDate(0, 0, 1)
	e.clock.Advance(tomorrow.Sub(e.clock.Now()))
	require.NoError(t, calculator.Refresh(t.Context(), e.orgID, series.ID,
		entity.Window{From: tomorrow.Truncate(24 * time.Hour), To: tomorrow.Truncate(24*time.Hour).AddDate(0, 0, 1), Step: entity.StepDay}, tomorrow))
	buildCalcJobs(t, e.owner, e.clock, calculator)
	third, err := calculator.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.NotEqual(t, second.ID, third.ID)
	assert.Equal(t, second.KnowledgeSeq, third.KnowledgeSeq)
	next := analysisRevisionPoints(t, calculator, e.orgID, third.ID)
	for _, id := range []string{"S05", "profile/opengrep-default"} {
		require.NotNil(t, next[id].Value, id)
		require.NotNil(t, points[id].Value, id)
		assert.Equal(t, 86400.0, *next[id].Value-*points[id].Value, id)
	}
}

func analysisTimeoutPackage(t *testing.T, finished time.Time) []byte {
	t.Helper()
	m := manifestOf(envelope.ManifestSchemaIdCiRuns, envelope.SourceProviderGitlab, _gitlabInstance, "42")
	m.Area.Kind = envelope.AreaRefKindRepository
	m.Completeness = envelope.ManifestCompletenessPartial
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	require.NoError(t, w.AddScanManifest(&envelope.ScanManifestV1{SchemaVersion: "1", Tools: []envelope.ScanToolRun{{
		Id: "opengrep-timeout", Producer: "opengrep", ProducerVersion: "1.33.1", Mode: envelope.ScanToolRunModeImport,
		ProfileId: ptr("opengrep-default"), ReportKind: envelope.ScanToolRunReportKindSast, Language: ptr("go"),
		ScopeHash: _scopeHash, Sha: ptr(_reportSHA), StartedAt: finished.Add(-time.Minute), FinishedAt: finished,
		ExitClass: envelope.ScanToolRunExitClassTimeout, Completeness: envelope.ScanToolRunCompletenessPartial,
		SkippedTargets: []envelope.ScanSkippedTarget{}, Reports: []envelope.ScanReport{},
	}}}))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func analysisRevisionPoints(t *testing.T, calculator *calc.UseCase, org, revision entity.ID) map[string]entity.Point {
	t.Helper()
	points, err := calculator.RevisionPoints(t.Context(), org, revision)
	require.NoError(t, err)
	out := make(map[string]entity.Point, len(points))
	for i := range points {
		p := &points[i]
		if p.MetricID == "C35" {
			out["profile/"+p.Estimator] = p.Point
		} else if !strings.HasPrefix(p.Estimator, "queue_") {
			out[p.MetricID] = p.Point
		}
	}
	return out
}
