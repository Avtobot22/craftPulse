package calc

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestAnalysisPoints_Completion(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	older := analysisRunForTest(1, 1, asOf.Add(-2*time.Hour), entity.AnalysisSAST)
	findings := analysisRunForTest(2, 2, asOf.Add(-time.Hour), entity.AnalysisSCA)
	findings.ExitClass = "findings"
	for _, tt := range []struct {
		name, exit, completeness, reason string
	}{
		{"timed out without a file", "timeout", "partial", "analysis_timeout"},
		{"partial exit", "partial", "partial", "analysis_partial"},
		{"partial coverage with findings", "findings", "partial", "analysis_partial"},
		{"failed", "failed", "unknown", "analysis_failed"},
		{"parser rejected", "success", "complete", "invalid_report"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			newer := analysisRunForTest(3, 2, asOf.Add(-time.Minute), entity.AnalysisSAST)
			newer.ExitClass, newer.Completeness = tt.exit, tt.completeness
			if tt.reason == "invalid_report" {
				newer.Reports = []entity.AnalysisReport{{ID: entity.ID{4}, Reason: tt.reason}}
			}
			facts := entity.AnalysisFacts{Runs: []entity.AnalysisRun{findings, older, newer}}
			points := analysisPoints(asOf, period{}, &facts)
			for _, id := range []string{"C03", "S07"} {
				p := requireAnalysisPoint(t, points, id, "")
				assert.Equal(t, num(.5), p.Value)
				assert.Equal(t, num(1), p.Numerator)
				assert.Equal(t, num(2), p.Denominator)
				assert.Equal(t, 1, p.MissingCount)
				assert.Contains(t, analysisReasons(p), tt.reason)
				assert.True(t, slices.ContainsFunc(p.Contributions, func(c entity.Contribution) bool {
					return c.ObjectID == findings.ID && c.Role == entity.RoleNumerator && c.ReasonCode == "" &&
						len(c.Evidence) != 0
				}))
			}
			last := requireAnalysisPoint(t, points, "S01", analysisEstimator(securityProfile(&older)))
			assert.Equal(t, num(timestampSeconds(*older.FinishedAt)), last.Value)
			assert.Contains(t, analysisReasons(last), tt.reason)
			age := requireAnalysisPoint(t, points, "S08", analysisEstimator(analysisScopeKey(&older)))
			assert.Equal(t, num(7200), age.Value)
			assert.Contains(t, analysisReasons(age), tt.reason)
			lastProfile := requireAnalysisPoint(t, points, "C35", older.Profile)
			assert.Equal(t, num(7200), lastProfile.Value, "a partially complete profile does not move the snapshot")
		})
	}
}

func TestAnalysisPoints_FileCoverage(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, kind, id string
		eligible       *int
		state          entity.DataState
		want           *float64
	}{
		{"code with stated denominator", entity.AnalysisCode, "C02", new(10), entity.StateAssessed, num(.7)},
		{"SAST with stated denominator", entity.AnalysisSAST, "S02", new(10), entity.StateAssessed, num(.7)},
		{"SAST producer has no denominator", entity.AnalysisSAST, "S02", nil, entity.StateUnknown, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := analysisRunForTest(1, 1, asOf, tt.kind)
			r.ScannedFiles, r.EligibleFiles = new(7), tt.eligible
			points := analysisPoints(asOf, period{}, &entity.AnalysisFacts{Runs: []entity.AnalysisRun{r}})
			p := requireAnalysisPoint(t, points, tt.id, analysisEstimator(fileScope(&r)))
			assert.Equal(t, tt.state, p.State)
			assert.Equal(t, tt.want, p.Value)
			if tt.eligible == nil {
				assert.Equal(t, "eligible_files_unknown", p.ReasonCode)
				assert.Nil(t, p.Denominator)
			}
		})
	}
}

func TestAnalysisPoints_InventoryHistoryAndAge(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	sca := analysisRunForTest(1, 1, asOf.Add(-time.Hour), entity.AnalysisSCA)
	sca.DBVersion, sca.DBPublished = "version", new(asOf.Add(-48*time.Hour))
	sca.QueueSeconds = num(13)
	sca.Reports = []entity.AnalysisReport{{ID: entity.ID{6}, Dependencies: &entity.DependencyInventory{Entries: 3, Resolved: 2}}}
	secret := analysisRunForTest(2, 1, asOf.Add(-time.Hour), entity.AnalysisSecrets)
	secret.History = &entity.ScanHistory{FromSHA: "from", ToSHA: "to", Truncated: true, FromAt: new(asOf.Add(-72 * time.Hour)), ToAt: new(asOf.Add(-3 * time.Hour))}
	secret.Completeness = "partial"
	doc := analysisRunForTest(3, 2, asOf.Add(-30*time.Minute), entity.AnalysisDocs)
	doc.Profile = "docs"
	firstDoc := asOf.Add(-96 * time.Hour)
	src := analysisSource{
		series: &entity.CalcSeries{}, asOf: asOf,
		facts: entity.AnalysisFacts{
			Runs:     []entity.AnalysisRun{sca, secret, doc},
			Versions: []entity.AnalysisDocumentVersion{{ID: entity.ID{7}, ObservedAt: firstDoc}},
		},
	}
	points := analysisPoints(asOf, period{}, &src.facts)
	testValues := []struct {
		id, estimator string
		value         float64
	}{
		{"S03", analysisEstimator(analysisScopeKey(&sca)), 2.0 / 3},
		{"S04", analysisEstimator(analysisScopeKey(&secret)), 1},
		{"S05", analysisEstimator(analysisScopeKey(&sca)), 172800},
		{"C35", "docs", 1800},
		{"C36", "p95/docs", 10},
		{"C36", "queue_p95/profile", 13},
		{"D27", "", 1},
		{"D27", "last_success", timestampSeconds(*doc.FinishedAt)},
		{"D27", "earliest_version", timestampSeconds(firstDoc)},
	}
	for _, tt := range testValues {
		p := requireAnalysisPoint(t, points, tt.id, tt.estimator)
		assert.Equal(t, num(tt.value), p.Value, tt.id)
	}
	history := requireAnalysisPoint(t, points, "S04", analysisEstimator(analysisScopeKey(&secret)))
	require.Len(t, history.Contributions, 1)
	var evidence entity.AnalysisRunMetadata
	require.NoError(t, json.Unmarshal(history.Contributions[0].Evidence, &evidence))
	assert.Equal(t, secret.History, evidence.History)
	assert.Equal(t, "history_truncated", history.ReasonCode)

	tomorrow := asOf.AddDate(0, 0, 1)
	firstHash := src.hash(asOf, tomorrow)
	src.asOf = tomorrow
	assert.NotEqual(t, firstHash, src.hash(asOf, tomorrow))
	points = analysisPoints(tomorrow, period{}, &src.facts)
	assert.Equal(t, num(259200), requireAnalysisPoint(t, points, "S05", analysisEstimator(analysisScopeKey(&sca))).Value)
	assert.Equal(t, num(88200), requireAnalysisPoint(t, points, "C35", "docs").Value)
}

func analysisRunForTest(id, receipt byte, finish time.Time, kind string) entity.AnalysisRun {
	return entity.AnalysisRun{
		ID: entity.ID{id}, ReceiptID: entity.ID{receipt}, KnownSeq: int64(receipt), ToolID: kind,
		AnalysisRunMetadata: entity.AnalysisRunMetadata{
			Kind: kind, Profile: "profile", Producer: "tool", Language: "go", ScopeHash: "scope",
			ExitClass: "success", Completeness: "complete", StartedAt: new(finish.Add(-10 * time.Second)), FinishedAt: &finish,
		},
	}
}

func requireAnalysisPoint(t *testing.T, points []entity.SeriesPoint, id, estimator string) entity.Point {
	t.Helper()
	for i := range points {
		p := &points[i]
		if p.MetricID == id && p.Estimator == estimator {
			return p.Point
		}
	}
	t.Fatalf("missing analysis point %s/%s", id, estimator)
	return entity.Point{}
}

func analysisReasons(p entity.Point) []string {
	var reasons []string
	for _, c := range p.Contributions {
		if c.Role == entity.RoleMissing {
			reasons = append(reasons, c.ReasonCode)
		}
	}
	return reasons
}

func TestAnalysisPoints_SonarCompletesOnlyItsScope(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	secret := analysisRunForTest(1, 2, asOf.Add(-time.Hour), entity.AnalysisSecrets)
	const sha = "abcdef0000000000000000000000000000000000"
	secret.Profile, secret.Producer, secret.Mode = "gitleaks-default", "gitleaks", entity.CheckModeBuiltin
	secret.SHA = sha
	secret.Reports = []entity.AnalysisReport{{ID: entity.ID{11}}}
	sca := analysisRunForTest(2, 2, asOf.Add(-time.Hour), entity.AnalysisSCA)
	sca.Profile, sca.Producer, sca.Mode = "trivy-default", "trivy", entity.CheckModeBuiltin
	sca.SHA = sha
	sca.Reports = []entity.AnalysisReport{{ID: entity.ID{12}}}
	partialSCA := sca
	partialSCA.Completeness = "partial"
	sonar := analysisRunForTest(3, 3, asOf, entity.AnalysisSonar)
	sonar.Profile, sonar.Producer, sonar.Mode = "sonar-import", "SonarQube", entity.CheckModeImport
	sonar.SHA = sha
	sonar.Reports = []entity.AnalysisReport{{ID: entity.ID{13}}}
	for _, tt := range []struct {
		name                 string
		builtin              []entity.AnalysisRun
		security, completion *float64
	}{
		{"Sonar alone", nil, nil, num(1)},
		{"Sonar and completed security", []entity.AnalysisRun{secret, sca}, num(1), num(1)},
		{"Sonar and incomplete SCA", []entity.AnalysisRun{secret, partialSCA}, num(.5), num(2.0 / 3)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before := analysisPoints(asOf, period{}, &entity.AnalysisFacts{Runs: tt.builtin})
			points := analysisPoints(asOf, period{}, &entity.AnalysisFacts{Runs: append(slices.Clone(tt.builtin), sonar)})
			s07 := requireAnalysisPoint(t, points, "S07", "")
			assert.Equal(t, tt.security, s07.Value)
			assert.Equal(t, requireAnalysisPoint(t, before, "S07", ""), s07)

			c03 := requireAnalysisPoint(t, points, "C03", "")
			assert.Equal(t, tt.completion, c03.Value)
			assert.Equal(t, num(float64(len(tt.builtin)+1)), c03.Denominator)
			for _, run := range tt.builtin {
				estimator := analysisEstimator(securityProfile(&run))
				assert.Equal(t, requireAnalysisPoint(t, before, "S01", estimator),
					requireAnalysisPoint(t, points, "S01", estimator))
				assert.True(t, slices.ContainsFunc(c03.Contributions, func(c entity.Contribution) bool {
					return c.ObjectID == run.ID && c.Role == entity.RoleDenominator
				}))
			}
			if len(tt.builtin) == 0 {
				assert.Equal(t, entity.StateUnknown, requireAnalysisPoint(t, points, "S01", "").State)
			}
		})
	}
}
