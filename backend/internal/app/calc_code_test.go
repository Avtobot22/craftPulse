package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestCalcCode_ReceiptsPublishSeparateCommitAndProducerRevisions(t *testing.T) {
	t.Parallel()
	e := &reviewEnv{normalizeEnv: newNormalizeEnv(t)}
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.norm}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	seed := e.imported(t, e.admin, reportPackage(t, _reportSHA, sarifSpecs()[0]))
	e.drain(t)
	e.area = mrArea(t, e.normalizeEnv, seed, "42")
	component := uuid.Must(uuid.NewV7())
	var service uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(),
		"INSERT INTO org.service (organization_id, name, status, created_at) VALUES ($1, 'code', 'active', now()) RETURNING id",
		uuid.UUID(e.orgID)).Scan(&service))
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.component (organization_id, id, known_seq, service_id, repository_area_id, path_prefix, status, origin,
		confirmed_by, confirmed_at) VALUES ($1, $2, 1, $3, $4, '', 'confirmed', 'manual', $5, now())`,
		uuid.UUID(e.orgID), component, service, uuid.UUID(e.area), uuid.UUID(e.admin.UserID))
	require.NoError(t, err)

	sha := _reportSHA
	lizardScope := entity.CodeScope{
		RepositoryID: e.area, ComponentID: entity.ID(component), SHA: sha,
		Format: entity.CodeFormatLizard, Producer: "lizard", ProfileID: "go-default",
	}
	bare, err := e.calc.CodeSeries(t.Context(), e.orgID, lizardScope)
	require.NoError(t, err)
	now := e.clock.Now()
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, bare.ID, entity.Window{From: now.Add(-time.Hour), To: now, Step: entity.StepDay}, now))
	e.build(t)
	missing := codeRevision(t, e, lizardScope)
	points, err := e.calc.RevisionPoints(t.Context(), e.orgID, missing.ID)
	require.NoError(t, err)
	require.Len(t, points, 10)
	for _, point := range points {
		assert.Equal(t, entity.StateNotEvaluated, point.State)
	}

	for _, tt := range []struct {
		lang, sha                  string
		maxCC, maxNLOC, prod, test float64
		duplicate, analyzed        float64
		testTargets                []string
	}{
		{"go", sha, 13, 58, 5, 13, 56, 62, []string{"service_test.go", "testdata/sample.go"}},
		{"python", strings.Repeat("b", 40), 14, 59, 3, 6, 50, 50, []string{"test_service.py", "tests/helpers.py"}},
		{"js", strings.Repeat("c", 40), 14, 60, 4, 8, 46, 50, []string{"__tests__/helper.js", "service.test.ts"}},
	} {
		t.Run(tt.lang, func(t *testing.T) {
			checkCodeProfiles(t, e, lizardScope, component, tt.lang, tt.sha, tt.testTargets,
				tt.maxCC, tt.maxNLOC, tt.prod, tt.test, tt.duplicate, tt.analyzed)
		})
	}
	first := codeRevision(t, e, lizardScope)

	secondSHA := strings.Repeat("e", 40)
	clones := codeReportSpec(t, component, "go-clones.json", "jscpd", entity.CodeFormatJscpd)
	functions := codeReportSpec(t, component, "go-functions.csv", "lizard", entity.CodeFormatLizard)
	functions.thresholds = &envelope.ScanFunctionThresholds{CcnThreshold: 13, NlocThreshold: 58}
	ci := gitlabCI("5010", "7010")
	lint := sarifSpecs()[0]
	lint.component, lint.ci = functions.component, ci
	functions.ci, clones.ci = ci, ci
	alternative := functions
	alternative.name, alternative.producer, alternative.exit, alternative.completeness = "alternate.csv", "other-lizard", "partial", "partial"
	rc := e.imported(t, e.admin, reportPackage(t, secondSHA, lint, clones, functions, alternative))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	e.build(t)
	checkCodeFindingBaseline(t, e, rc.ID, entity.ID(component), ci.RefName)
	secondScope := lizardScope
	secondScope.SHA = secondSHA
	second := codeRevision(t, e, secondScope)
	assert.NotEqual(t, first.SeriesID, second.SeriesID)
	codeValue(t, e, second, "C05", "", 0, entity.StateAssessed)
	codeValue(t, e, second, "C06", "above_threshold", 0, entity.StateAssessed)
	secondScope.Format, secondScope.Producer = entity.CodeFormatJscpd, "jscpd"
	codeValue(t, e, codeRevision(t, e, secondScope), "C08", "", 56.0/62, entity.StateAssessed)
	secondScope.Format, secondScope.Producer = entity.CodeFormatLizard, alternative.producer
	partial := codeRevision(t, e, secondScope)
	assert.NotEqual(t, second.SeriesID, partial.SeriesID)
	pt := codeValue(t, e, partial, "C04", "max", 13, entity.StateProvisional)
	assert.Equal(t, 1, pt.MissingCount)
	codeCohort(t, e, partial.ID, pt, "app/generated.go", 4)
	assert.Equal(t, first.ID, codeRevision(t, e, lizardScope).ID)
	assert.Equal(t, 2, e.n(t, `SELECT count(*) FROM calc.revision_input
		WHERE revision_id = $1 AND input_kind = 'profile_threshold' AND (input_id, version) IN (('ccn_threshold','13'),('nloc_threshold','58'))`,
		uuid.UUID(second.ID)))
}

func checkCodeFindingBaseline(t *testing.T, e *reviewEnv, receipt, component entity.ID, branch string) {
	t.Helper()
	reports, err := e.norm.ReceiptReports(t.Context(), e.orgID, receipt)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.NotEmpty(t, reports[0].Occurrences)
	last, err := e.norm.LastBranchReports(t.Context(), e.orgID, []entity.BranchReportQuery{{
		ComponentID: component, Language: "go", Kind: entity.AnalysisCode, RefName: branch,
	}})
	require.NoError(t, err)
	require.Len(t, last, 1)
	assert.Equal(t, reports[0].ID, last[0].ID)
}

func checkCodeProfiles(t *testing.T, e *reviewEnv, lizard entity.CodeScope, component uuid.UUID, language, sha string,
	testTargets []string, maxCC, maxNLOC, prod, test, duplicate, analyzed float64,
) {
	t.Helper()
	lizard.SHA, lizard.ProfileID = sha, language+"-default"
	jscpd := lizard
	jscpd.Format, jscpd.Producer = entity.CodeFormatJscpd, "jscpd"
	functions := codeReportSpec(t, component, language+"-functions.csv", "lizard", entity.CodeFormatLizard)
	functions.thresholds = &envelope.ScanFunctionThresholds{CcnThreshold: 10, NlocThreshold: 50}
	clones := codeReportSpec(t, component, language+"-clones.json", "jscpd", entity.CodeFormatJscpd)
	rc := e.imported(t, e.admin, reportPackage(t, sha, functions, clones))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	e.build(t)
	f := codeRevision(t, e, lizard)
	codeValue(t, e, f, "C04", "p50", 1, entity.StateAssessed)
	pt := codeValue(t, e, f, "C04", "p90", maxCC, entity.StateAssessed)
	codeValue(t, e, f, "C05", "", 1.0/3, entity.StateAssessed)
	codeValue(t, e, f, "C06", "max", maxNLOC, entity.StateAssessed)
	codeValue(t, e, f, "C06", "above_threshold", 1, entity.StateAssessed)
	codeCohort(t, e, f.ID, pt, `"name"`, 3)
	c := codeRevision(t, e, jscpd)
	p := codeValue(t, e, c, "C08", "", duplicate/analyzed, entity.StateAssessed)
	assert.InDelta(t, duplicate, *p.Numerator, 0)
	p = codeValue(t, e, c, "C09", "", 1, entity.StateAssessed)
	codeCohort(t, e, c.ID, p, "members", 1)

	scope := codeReportSpec(t, component, language+"-scope.csv", "lizard", entity.CodeFormatLizard)
	scope.testTargets = &testTargets
	rc = e.imported(t, e.admin, reportPackage(t, sha, scope))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	e.build(t)
	v := codeRevision(t, e, lizard)
	require.NotEqual(t, f.ID, v.ID)
	codeValue(t, e, v, "C01", "production", prod, entity.StateAssessed)
	p = codeValue(t, e, v, "C01", "test", test, entity.StateAssessed)
	codeCohort(t, e, v.ID, p, testTargets[0], len(testTargets))
}

func codeReportSpec(t *testing.T, component uuid.UUID, name, producer, format string) reportSpec {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "usecase", "normalize", "testdata", "code", name))
	require.NoError(t, err)
	out := reportSpec{
		name: name, kind: entity.AnalysisCode, format: format, producer: producer, version: "5.4.2", content: content,
		exit: "success", completeness: "complete", component: &envelope.ScanComponent{Id: &component, PathPrefix: ""},
	}
	lang, _, _ := strings.Cut(name, "-")
	out.profileID, out.language = lang+"-default", lang
	if lang == "js" {
		out.language = "javascript"
	}
	if format == entity.CodeFormatLizard {
		out.version, out.formatVersion = "1.26.0", "2"
	}
	return out
}

func codeRevision(t *testing.T, e *reviewEnv, scope entity.CodeScope) entity.CalcRevision {
	t.Helper()
	series, err := e.calc.CodeSeries(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	r, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionPublished, r.Status)
	return r
}

func codeValue(t *testing.T, e *reviewEnv, rev entity.CalcRevision, metric, est string, want float64, state entity.DataState) entity.SeriesPoint {
	t.Helper()
	pt := e.point(t, rev.ID, metric, est, rev.Window.From)
	assert.Equal(t, state, pt.State)
	require.NotNil(t, pt.Value)
	assert.InDelta(t, want, *pt.Value, 1e-12)
	return pt
}

func codeCohort(t *testing.T, e *reviewEnv, rev entity.ID, pt entity.SeriesPoint, wantEvidence string, count int) {
	t.Helper()
	page, err := e.calc.PointContributions(t.Context(), e.orgID, rev, entity.ContributionQuery{
		MetricID: pt.MetricID, Estimator: pt.Estimator, PointStart: pt.Start,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, count)
	for _, item := range page.Items {
		if strings.Contains(string(item.Evidence), wantEvidence) {
			return
		}
	}
	t.Fatalf("no contribution with %q in %v", wantEvidence, page.Items)
}
