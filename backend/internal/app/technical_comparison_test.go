package app

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestTechnicalComparison_ReportsAndPermissions(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	project := uuid.UUID(e.project)
	plain := e.exceptionActor(t, "comparison-lead", []roleInput{{Role: "tech_lead", ProjectID: &project}})
	security := e.exceptionActor(t, "comparison-security", []roleInput{{Role: "tech_lead", ProjectID: &project}, {Role: "security_access", ProjectID: &project}})
	// The fixtures are package imports and do not have a live GitLab source.
	_, err := e.owner.Exec(t.Context(), "DELETE FROM connect.connection_scope WHERE organization_id=$1 AND area_id=$2", uuid.UUID(e.orgID), e.areas["42"])
	require.NoError(t, err)
	dir := t.TempDir()
	gitDiffCommand(t, dir, "init", "--quiet")
	base := diffCommit(t, dir, "example.go", "package fixture\n// base\n")
	head := diffCommit(t, dir, "example.go", "package fixture\n// head\n")
	other := diffCommit(t, dir, "example.go", "package fixture\n// other\n")
	call := func(user *browser, component uuid.UUID, from, to, kind string) openapi.TechnicalComparison {
		t.Helper()
		r := e.call(t, user, "", http.MethodGet, technicalComparisonURL(component, from, to, kind), nil, nil)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		return decode[openapi.TechnicalComparison](t, r)
	}
	baseReceipt := e.submit(t, e.codeReport(t, base, "feature", "8101", sarifOf(hit{rule: "old", path: "example.go", line: 2})))
	baseIDs := e.identities(t, baseReceipt)
	require.Len(t, baseIDs, 1)
	known := entity.ID(baseIDs[0].identity)
	state, err := e.identifier.CurrentFindingState(t.Context(), e.orgID, entity.ID(e.goComponent), known, entity.AnalysisCode)
	require.NoError(t, err)
	assert.Equal(t, entity.FindingNewlyObserved, state)
	missing := call(plain, e.goComponent, base, head, entity.AnalysisCode)
	assert.False(t, missing.Comparable)
	assert.Nil(t, missing.Head.ReportId)
	assert.Contains(t, missing.IncomparableReasons, openapi.TechnicalComparisonIncomparableReasonsHeadScanMissing)

	headReceipt := e.submit(t, e.codeReport(t, head, "feature", "8102", sarifOf(hit{rule: "new", path: "example.go", line: 2})))
	headIDs := e.identities(t, headReceipt)
	require.Len(t, headIDs, 1)
	result := call(plain, e.goComponent, base, head, entity.AnalysisCode)
	assert.True(t, result.Comparable, "%+v", result)
	assert.False(t, result.AccessLimited)
	require.Len(t, result.Factors, 1)
	assert.Equal(t, new(1), result.Factors[0].NewCount)
	assert.Equal(t, new(1), result.Factors[0].ResolvedCount)
	assert.ElementsMatch(t, []uuid.UUID{baseIDs[0].identity, headIDs[0].identity}, result.Factors[0].FindingIds)
	state, err = e.identifier.CurrentFindingState(t.Context(), e.orgID, entity.ID(e.goComponent), known, entity.AnalysisCode)
	require.NoError(t, err)
	assert.Equal(t, entity.FindingResolved, state)

	e.submit(t, e.codeReport(t, other, "feature", "8199", sarifOf(hit{rule: "old", path: "example.go", line: 2}), "partial", "partial"))
	partial := call(plain, e.goComponent, head, other, entity.AnalysisCode)
	assert.False(t, partial.Comparable)
	assert.Contains(t, partial.IncomparableReasons, openapi.TechnicalComparisonIncomparableReasonsScanIncomplete)
	state, err = e.identifier.CurrentFindingState(t.Context(), e.orgID, entity.ID(e.goComponent), known, entity.AnalysisCode)
	require.NoError(t, err)
	assert.Equal(t, entity.FindingResolved, state, "an incomplete observation does not reopen a finding")
	mr, err := e.identifier.CompareMergeRequestReports(t.Context(), e.orgID, entity.TechnicalComparisonQuery{
		ComponentID: entity.ID(e.goComponent), BaseSHA: other, HeadSHA: head, Kind: entity.AnalysisCode,
	}, "go", "go-default")
	require.NoError(t, err)
	assert.Contains(t, mr.IncomparableReasons, "base_scan_missing")
	assert.Nil(t, mr.Base.ReportID)
	assert.NotNil(t, mr.Head.ReportID)

	spec := reportSpec{
		name: "rules.sarif", kind: entity.AnalysisCode, format: "sarif", producer: "opengrep", version: "1.33.1", rulesHash: strings.Repeat("a", 64),
		ci: gitlabCI("58103", "8103"), component: &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"},
		content: sarifOf(), exit: "success", completeness: "complete",
	}
	e.submit(t, reportPackageOf(t, _checksInstance, other, spec))
	rules := call(plain, e.goComponent, head, other, entity.AnalysisCode)
	assert.False(t, rules.Comparable)
	assert.Contains(t, rules.IncomparableReasons, openapi.TechnicalComparisonIncomparableReasonsRulesChanged)
	assert.Contains(t, factorNames(&rules), openapi.TechnicalComparisonFactorsFactorRulesChanged)
	for _, f := range rules.Factors {
		assert.Nil(t, f.NewCount)
		assert.Nil(t, f.ResolvedCount)
		assert.Empty(t, f.FindingIds)
	}
	state, err = e.identifier.CurrentFindingState(t.Context(), e.orgID, entity.ID(e.goComponent), known, entity.AnalysisCode)
	require.NoError(t, err)
	assert.Equal(t, entity.FindingResolved, state, "a report under different rules does not replace the last comparable state")

	sca := reportSpec{
		name: "packages.json", kind: entity.AnalysisSCA, format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.70.0", profileID: "trivy-default",
		component: &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"},
		ci:        gitlabCI("58104", "8104"), dbVersion: "db-1", exit: "success", completeness: "complete",
		content: []byte(`{"SchemaVersion":2,"Results":[]}`),
	}
	e.submit(t, reportPackageOf(t, _checksInstance, head, sca))
	sca.ci, sca.dbVersion, sca.exit = gitlabCI("58105", "8105"), "db-2", "findings"
	sca.content = reportFixture(t, "trivy-sca.json")
	securityReceipt := e.submit(t, reportPackageOf(t, _checksInstance, head, sca))
	secureResult := call(security, e.goComponent, head, head, entity.AnalysisSCA)
	require.Len(t, secureResult.Factors, 1, "%+v", secureResult)
	assert.Equal(t, openapi.TechnicalComparisonFactorsFactorDbChanged, secureResult.Factors[0].Factor)
	assert.NotEmpty(t, e.identities(t, securityReceipt))
	assert.NotEmpty(t, secureResult.Factors[0].FindingIds)
	assert.False(t, secureResult.Comparable)
	assert.Nil(t, secureResult.Factors[0].NewCount)
	assert.NotEqual(t, secureResult.Base.ReportId, secureResult.Head.ReportId)

	masked := call(plain, e.goComponent, head, head, entity.AnalysisSCA)
	assert.True(t, masked.AccessLimited)
	assert.Empty(t, masked.Factors)
	assert.Nil(t, masked.Base.ReportId)
	assert.Nil(t, masked.Head.ReportId)
	require.Len(t, masked.AccessNotes, 1)
	assert.Equal(t, openapi.AccessNoteCode(entity.NoteRestricted), masked.AccessNotes[0].Code)
	assert.Equal(t, http.StatusNotFound, e.call(t, plain, "", http.MethodGet,
		technicalComparisonURL(e.otherComponent, base, head, entity.AnalysisCode), nil, nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.call(t, plain, "", http.MethodGet,
		technicalComparisonURL(e.goComponent, "invalid", head, entity.AnalysisCode), nil, nil).Code)
}

func technicalComparisonURL(component uuid.UUID, base, head, kind string) string {
	q := url.Values{"component_id": {component.String()}, "base_sha": {base}, "head_sha": {head}, "analysis_kind": {kind}}
	return "/api/v1/technical-comparisons?" + q.Encode()
}

func factorNames(c *openapi.TechnicalComparison) []openapi.TechnicalComparisonFactorsFactor {
	out := make([]openapi.TechnicalComparisonFactorsFactor, len(c.Factors))
	for i := range c.Factors {
		out[i] = c.Factors[i].Factor
	}
	return out
}
