package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
)

func TestCodeAPI_V40TestExecutionsAndCoverage(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	runs := append(v28(t), ciRun(t, runSpec{id: "5002", project: "42", sha: _headA, status: "success", revision: "p1", updated: _t1}))
	receipt := e.imported(t, e.admin, pack(t, ciManifest("42"), runs...))
	e.drain(t)
	area := mrArea(t, e, receipt, "42")
	seqBefore, err := e.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	calculator := calc.New(calc.Deps{TestCoverage: e.norm})

	junit := func(name, id, testcase string) reportSpec {
		return reportSpec{
			name: name, kind: "junit", format: "junit_xml", producer: "gotestsum", version: "1.12.0",
			content: []byte(`<testsuite name="math"><testcase name="add" time="0.125">` + testcase + `</testcase></testsuite>`),
			exit:    "success", completeness: "complete", ci: gitlabCI("5001", id),
		}
	}
	line := reportSpec{
		name: "math.info", kind: "coverage", format: "lcov", producer: "lcov", version: "2.0",
		content: []byte("SF:src/math.go\nDA:1,1\nDA:2,1\nDA:3,0\nDA:4,0\nend_of_record\n"),
		exit:    "success", completeness: "complete", ci: gitlabCI("5001", "7001"),
	}
	rc := e.imported(t, e.admin, reportPackage(t, _headA, junit("linux.xml", "7001", `<failure/>`), junit("windows.xml", "7003", ""), line))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	wrongSHA := line
	wrongSHA.name = "other-sha.info"
	wrongSHA.content = []byte("SF:src/other.go\nDA:1,1\nend_of_record\n")
	e.imported(t, e.admin, reportPackage(t, _headB, wrongSHA))
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, rc.ID).Status)

	path := "/api/v1/pipelines/" + runID(t, e, "5001").String()
	pipeline := decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, path, "Pipeline"))
	assert.Equal(t, openapi.DataState(entity.StateAssessed), pipeline.Tests.State)
	assert.Equal(t, ptr(1), pipeline.Tests.Passed)
	assert.Equal(t, ptr(1), pipeline.Tests.Failed)
	assert.Equal(t, ptr(1), pipeline.Tests.DistinctCases)
	assert.Equal(t, ptr(2), pipeline.Tests.Executions)
	require.Len(t, pipeline.Reports, 2)
	assert.Equal(t, openapi.AnalysisKindCoverage, pipeline.Reports[0].AnalysisKind)
	assert.Equal(t, openapi.PipelineReportsStateReceived, pipeline.Reports[0].State)
	require.NotNil(t, pipeline.Coverage)
	assert.Equal(t, openapi.DataState(entity.StateAssessed), pipeline.Coverage.Line.State)
	assert.Equal(t, ptr(float32(0.5)), pipeline.Coverage.Line.Value)
	assert.Equal(t, ptr(float32(2)), pipeline.Coverage.Line.Numerator)
	assert.Equal(t, ptr(float32(4)), pipeline.Coverage.Line.Denominator)
	assert.Equal(t, openapi.DataState(entity.StateNotEvaluated), pipeline.Coverage.Branch.State)
	assert.Nil(t, pipeline.Coverage.Branch.Value)
	otherPath := "/api/v1/pipelines/" + runID(t, e, "5002").String()
	other := decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, otherPath, "Pipeline"))
	assert.Empty(t, other.Reports)
	assert.Equal(t, openapi.DataState(entity.StateNotEvaluated), other.Tests.State)
	require.NotNil(t, other.Coverage)
	assert.Equal(t, openapi.DataState(entity.StateNotEvaluated), other.Coverage.Line.State)

	seq, err := e.norm.ReportKnowledge(t.Context(), e.orgID, nil)
	require.NoError(t, err)
	scope := entity.ReportScope{AreaID: area, SHA: _headA, KnownSeq: seq}
	revision, err := calculator.TestCoverageAt(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	require.Len(t, revision.Definitions, 3)
	require.Len(t, revision.Tests.Contributions, 2)
	require.Len(t, revision.Tests.Groups, 2)
	assert.Equal(t, ptr("math"), revision.Tests.Groups[0].Suite)
	require.Len(t, revision.LineCoverage.Contributions, 1)
	assert.Equal(t, "src/math.go", revision.LineCoverage.Contributions[0].File.Path)
	scope.KnownSeq = seqBefore
	earlier, err := calculator.TestCoverageAt(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	assert.Equal(t, entity.StateNotEvaluated, earlier.LineCoverage.State)
	assert.Equal(t, entity.StateNotEvaluated, earlier.Tests.State)

	path += "/test-executions"
	failed := decode[openapi.TestExecutionPage](t, codeRead(t, e.loginEnv, b, path+"?outcome=failed", "TestExecutionPage"))
	require.Len(t, failed.Data, 1)
	assert.Equal(t, "math::add", failed.Data[0].TestCase.Key)
	assert.Equal(t, _linux, failed.Data[0].ConfigFingerprint)
	assert.Equal(t, ptr(objectID(t, e, "ci_attempt", "7001")), failed.Data[0].AttemptId)
	assert.Equal(t, ptr(125), failed.Data[0].DurationMs)

	first := decode[openapi.TestExecutionPage](t, codeRead(t, e.loginEnv, b, path+"?limit=1", "TestExecutionPage"))
	require.Len(t, first.Data, 1)
	require.NotNil(t, first.NextCursor)
	assert.Equal(t, http.StatusBadRequest, e.do(t, b, http.MethodGet, path+"?outcome=failed&cursor="+*first.NextCursor, nil).Code)

	corrected := e.imported(t, e.admin, reportPackage(t, _headA, junit("rerun.xml", "7001", "")))
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, corrected.ID).Status)
	second := decode[openapi.TestExecutionPage](t, codeRead(t, e.loginEnv, b, path+"?limit=1&cursor="+*first.NextCursor, "TestExecutionPage"))
	require.Len(t, second.Data, 1)
	assert.Nil(t, second.NextCursor)
	assert.Equal(t, first.Data[0].TestCase.Id, second.Data[0].TestCase.Id)
	assert.ElementsMatch(t, []string{_linux, _windows}, []string{first.Data[0].ConfigFingerprint, second.Data[0].ConfigFingerprint})
	assert.ElementsMatch(t, []openapi.TestExecutionOutcome{"failed", "passed"}, []openapi.TestExecutionOutcome{first.Data[0].Outcome, second.Data[0].Outcome})
	assert.Empty(t, decode[openapi.TestExecutionPage](t, codeRead(t, e.loginEnv, b, path+"?outcome=failed", "TestExecutionPage")).Data)
}

func TestCodeAPI_ExpectedAndMalformedPipelineReports(t *testing.T) {
	t.Parallel()
	e, b := codeEnv(t)
	rec := e.imported(t, e.admin, pack(t, ciManifest("42"), v28(t)...))
	e.drain(t)
	area := mrArea(t, e, rec, "42")
	seedPipelineMatrix(t, e, area)

	path := "/api/v1/pipelines/" + runID(t, e, "5001").String()
	get := func() openapi.Pipeline {
		return decode[openapi.Pipeline](t, codeRead(t, e.loginEnv, b, path, "Pipeline"))
	}
	missing := get()
	require.Len(t, missing.Reports, 2)
	assert.Equal(t, openapi.AnalysisKindJunit, missing.Reports[1].AnalysisKind)
	assert.Equal(t, openapi.PipelineReportsStateMissing, missing.Reports[1].State)
	assert.Equal(t, openapi.DataState(entity.StateNotEvaluated), missing.Tests.State)
	assert.Nil(t, missing.Tests.Passed)
	assert.Equal(t, openapi.PipelineReportsStateMissing, missing.Reports[0].State)

	spec := reportSpec{
		name: "broken.info", kind: "coverage", format: "lcov", producer: "lcov", version: "2.0",
		content: []byte("DA:1,1\nend_of_record\n"), ci: gitlabCI("5001", "7001"),
	}
	e.imported(t, e.admin, reportPackage(t, _headB, spec))
	e.drain(t)
	assert.Equal(t, openapi.PipelineReportsStateMissing, get().Reports[0].State, "the other SHA does not measure this pipeline")
	bad := e.imported(t, e.admin, reportPackage(t, _headA, spec))
	e.drain(t)
	require.Equal(t, entity.ReceiptPartial, e.receipt(t, bad.ID).Status)
	broken := get()
	assert.Equal(t, openapi.AnalysisKindCoverage, broken.Reports[0].AnalysisKind)
	assert.Equal(t, openapi.PipelineReportsStateMalformed, broken.Reports[0].State)
	assert.Equal(t, openapi.PipelineReportsStateMissing, broken.Reports[1].State)
	require.NotNil(t, broken.Coverage)
	assert.Equal(t, openapi.DataState(entity.StateUnknown), broken.Coverage.Line.State)
	assert.Nil(t, broken.Coverage.Line.Value)

	badJUnit := e.imported(t, e.admin, reportPackage(t, _headA, reportSpec{
		name: "broken.xml", kind: "junit", format: "junit_xml", producer: "gotestsum", version: "1.12.0",
		content: []byte("<testsuite>"), ci: gitlabCI("5001", "7001"),
	}))
	e.drain(t)
	require.Equal(t, entity.ReceiptPartial, e.receipt(t, badJUnit.ID).Status)
	broken = get()
	assert.Equal(t, openapi.PipelineReportsStateMalformed, broken.Reports[1].State)
	assert.Equal(t, openapi.DataState(entity.StateUnknown), broken.Tests.State)
	assert.Nil(t, broken.Tests.Passed)
}

func seedPipelineMatrix(t *testing.T, e *normalizeEnv, area entity.ID) {
	t.Helper()
	org, project, user := uuid.UUID(e.orgID), uuid.UUID(e.project), uuid.UUID(e.admin.UserID)
	service, component, first, later := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	before, after := _t1.Add(-time.Hour), _t1.Add(time.Hour)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id, project_id, area_id, valid, known_seq)
		VALUES ($1, $2, $3, tstzrange($4, NULL, '[)'), 1)`, org, project, uuid.UUID(area), before)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.service (organization_id, id, name, status, created_at)
		VALUES ($1, $2, 'CI service', 'active', $3)`, org, service, before)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO org.component (organization_id, id, known_seq, service_id, repository_area_id,
		path_prefix, status, origin, confirmed_by, confirmed_at) VALUES ($1, $2, 1, $3, $4, '', 'confirmed', 'manual', $5, $6)`,
		org, component, service, uuid.UUID(area), user, before)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO change.profile_version (organization_id, id, project_id, version_no,
		base_version_no, content_hash, known_seq, confirmed_by, confirmed_at, confirmation_id)
		VALUES ($1, $2, $3, 1, NULL, decode(repeat('00',32),'hex'), 1, $4, $5, uuidv7()),
		       ($1, $6, $3, 2, 1, decode(repeat('11',32),'hex'), 2, $4, $7, uuidv7())`,
		org, first, project, user, before, later, after)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO change.profile_entry (organization_id, profile_version_id, component_id,
		language, analysis_kind, mode, profile_id, tool_versions)
		VALUES ($1, $2, $3, 'go', 'junit', 'import', 'junit-import', '{}'),
		       ($1, $2, $3, 'go', 'coverage', 'import', 'lcov-import', '{}'),
		       ($1, $4, $3, 'go', 'code', 'import', 'code-import', '{}')`, org, first, component, later)
	require.NoError(t, err)
}
