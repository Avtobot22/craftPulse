package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_findingsPath  = "/api/v1/findings"
	_findingMarker = "fixture-only-secret-marker"
)

func TestFindingsAPI_VisibilityEvidenceAndOverview(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	ids := e.importOverviewFindings(t)
	e.adminBrowser = newBrowser()
	e.lead = newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, e.adminBrowser, _adminSub).Code)
	require.Equal(t, http.StatusFound, e.login(t, e.lead, _otherSub).Code)
	project := uuid.UUID(e.project)
	dev := e.exceptionActor(t, "findings-dev", []roleInput{{Role: "developer", ProjectID: &project}, {Role: "security_access", ProjectID: &project}})
	pm := e.exceptionActor(t, "findings-pm", []roleInput{{Role: "project_manager", ProjectID: &project}})
	outsider := e.exceptionActor(t, "findings-outsider", []roleInput{{Role: "developer", ProjectID: &e.other}})
	overviewPath := "/api/v1/components/" + e.goComponent.String() + "/overview"
	get := func(b *browser, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		rec := e.call(t, b, "", http.MethodGet, path, nil, nil)
		require.Equal(t, want, rec.Code, rec.Body.String())
		for _, fragment := range []string{_findingMarker, "fixture-only", "secret-marker"} {
			assert.NotContains(t, rec.Body.String(), fragment)
		}
		return rec
	}
	secretPath := _findingsPath + "/" + ids["AWS"].String()
	card := decode[openapi.FindingDetail](t, get(dev, secretPath, http.StatusOK))
	assert.Equal(t, "AWS", card.RuleId)
	require.NotNil(t, card.Location)
	assert.Equal(t, "config.env", card.Location.Path)
	assert.Equal(t, new("high"), card.Severity.Native)
	assert.Nil(t, card.Confidence)
	assert.Equal(t, "secrets", card.Report.Producer)
	assert.Equal(t, "6.4.0", card.Report.ProducerVersion)
	assert.Equal(t, new("gitleaks-default"), card.Report.ProfileId)
	assert.Equal(t, new(_rulesHash), card.Report.RulesVersion)
	assert.Contains(t, card.Actions, openapi.CreateException)
	assert.NotContains(t, card.Actions, openapi.ReviewException)
	assert.NotContains(t, card.Actions, openapi.CreateTechDebtTask)
	get(pm, secretPath, http.StatusNotFound)
	get(outsider, secretPath, http.StatusNotFound)
	get(outsider, overviewPath, http.StatusNotFound)
	sastPath := _findingsPath + "/" + ids["sast-rule"].String()
	imported := decode[openapi.FindingDetail](t, get(dev, sastPath, http.StatusOK))
	require.Len(t, imported.ExternalTriage, 1)
	assert.Equal(t, "opengrep", imported.ExternalTriage[0].Authority)
	assert.Equal(t, "accepted", imported.ExternalTriage[0].State)
	assert.Equal(t, openapi.FindingStateNewlyObserved, imported.State)
	assert.Empty(t, imported.Exceptions)

	docsPage := decode[openapi.FindingPage](t, get(dev, _findingsPath+"?finding_kind=docs", http.StatusOK))
	require.Len(t, docsPage.Data, 1)
	doc := decode[openapi.FindingDetail](t, get(dev, _findingsPath+"/"+docsPage.Data[0].Id.String(), http.StatusOK))
	require.NotNil(t, doc.Location)
	assert.Equal(t, "docs/index.md", doc.Location.Path)
	assert.Equal(t, "docs/guide.md", doc.Evidence["target_path"])
	assert.Equal(t, "go-here", doc.Evidence["anchor"])

	componentList := _findingsPath + "?component_id=" + e.goComponent.String()
	plain := decode[openapi.FindingPage](t, get(pm, componentList, http.StatusOK))
	assert.True(t, plain.AccessLimited)
	require.Len(t, plain.Data, 1)
	assert.Equal(t, openapi.FindingKindCode, plain.Data[0].FindingKind)
	assert.Empty(t, decode[openapi.FindingPage](t, get(outsider, componentList, http.StatusOK)).Data)
	first := decode[openapi.FindingPage](t, get(dev, componentList+"&limit=1&sort=-severity", http.StatusOK))
	require.Len(t, first.Data, 1)
	require.NotNil(t, first.NextCursor)
	next := "&cursor=" + url.QueryEscape(*first.NextCursor)
	page2 := decode[openapi.FindingPage](t, get(dev, componentList+"&limit=1&sort=-severity"+next, http.StatusOK))
	require.Len(t, page2.Data, 1)
	assert.NotEqual(t, first.Data[0].Id, page2.Data[0].Id)
	get(dev, componentList+"&sort=first_seen_at"+next, http.StatusBadRequest)

	overview := decode[openapi.ComponentOverview](t, get(dev, overviewPath, http.StatusOK))
	assert.False(t, overview.AccessLimited)
	assert.Empty(t, overview.Hotspots)
	metrics := make([]string, 0, len(overview.Facts))
	for i := range overview.Facts {
		fact := &overview.Facts[i]
		metrics = append(metrics, fact.MetricId)
		assert.Equal(t, "builtin", string(fact.Source))
		assert.True(t, fact.Window.From.Before(fact.Window.To))
		switch fact.MetricId {
		case "C05":
			require.NotNil(t, fact.Value.Value)
			assert.InDelta(t, 1.0/3, *fact.Value.Value, 1e-6)
		case "C08":
			require.NotNil(t, fact.Value.Value)
			assert.InDelta(t, 56.0/62, *fact.Value.Value, 1e-6)
		}
	}
	assert.ElementsMatch(t, []string{"C01", "C01", "C04", "C04", "C04", "C05", "C06", "C06", "C06", "C06", "C08", "C09"}, metrics)
	masked := decode[openapi.ComponentOverview](t, get(pm, overviewPath, http.StatusOK))
	assert.True(t, masked.AccessLimited)
	require.Len(t, masked.FindingsByState, 1)
	assert.Equal(t, openapi.FindingKindCode, masked.FindingsByState[0].FindingKind)
	assert.Equal(t, overview.Facts, masked.Facts)

	projectList := _findingsPath + "?project_id=" + project.String() + "&service_id=" + overview.Component.ServiceId.String()
	page := decode[openapi.FindingPage](t, get(dev, projectList+"&finding_kind=secret&severity=high", http.StatusOK))
	require.Len(t, page.Data, 1)
	assert.Equal(t, card.Id, page.Data[0].Id)
	e.checkFindingExpiry(t, dev, secretPath, card.Id)
}

func (e *identityEnv) importOverviewFindings(t *testing.T) map[string]uuid.UUID {
	t.Helper()
	project := uuid.UUID(e.project)
	profile := e.confirm(t, project, `"0"`, uuid.New(),
		row(e.goComponent, "go", "code", "builtin", "go-default"),
		row(e.goComponent, "", "secrets", "builtin", "gitleaks-default"),
		row(e.goComponent, "", "sca", "builtin", "trivy-default"),
	)
	require.Equal(t, http.StatusCreated, profile.Code, profile.Body.String())
	_, err := e.owner.Exec(t.Context(), "DELETE FROM connect.connection_scope WHERE organization_id=$1 AND area_id=$2", uuid.UUID(e.orgID), e.areas["42"])
	require.NoError(t, err)
	component := &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"}
	ci := gitlabCI("58200", "8200")
	lint := reportSpec{
		name: "lint.sarif", kind: "code", format: "sarif", producer: "golangci-lint", version: "2.15.0",
		content: sarifOf(hit{rule: "lint-rule", path: "services/billing/app.go", line: 2}),
	}
	sast := reportSpec{
		name: "sast.sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1",
		profileID: "sarif-import", mode: envelope.ScanToolRunModeImport,
		content: []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"opengrep"}},"results":[{
			"ruleId":"sast-rule","level":"error","suppressions":[{"kind":"external","status":"accepted"}],
			"locations":[{"physicalLocation":{"artifactLocation":{"uri":"services/billing/app.go"},"region":{"startLine":3}}}]}]}]}`),
	}
	secret := reportSpec{
		name: "secrets.json", kind: "secrets", format: "gitlab_security", producer: "secrets", version: "6.4.0",
		profileID: "gitleaks-default", content: []byte(strings.NewReplacer("Critical", "high", "demo-secret-extract", _findingMarker).
				Replace(string(reportFixture(t, "gl-secret-detection-report.json")))),
	}
	sca := reportSpec{
		name: "dependencies.json", kind: "sca", format: "trivy_json", formatVersion: "2", producer: "trivy", version: "0.70.0",
		profileID: "trivy-default", content: reportFixture(t, "trivy-sca.json"),
	}
	for _, s := range []*reportSpec{&lint, &sast, &secret, &sca} {
		s.ci, s.component, s.exit, s.completeness = ci, component, "findings", "complete"
	}
	docs := reportSpec{
		name: "links.sarif", kind: "docs", format: "sarif", formatVersion: "2.1.0", producer: "craftpulse-docs", version: "1", profileID: "docs-default",
		ci: ci, component: &envelope.ScanComponent{Id: &e.docs, PathPrefix: "docs"}, exit: "findings", completeness: "complete",
		content: []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"craftpulse-docs"}},"results":[{
			"ruleId":"missing-anchor","properties":{"target_path":"docs/guide.md","anchor":"go-here"},
			"locations":[{"physicalLocation":{"artifactLocation":{"uri":"docs/index.md"},"region":{"startLine":4}}}]}]}]}`),
	}
	functions := codeReportSpec(t, e.goComponent, "go-functions.csv", "lizard", entity.CodeFormatLizard)
	functions.thresholds = &envelope.ScanFunctionThresholds{CcnThreshold: 10, NlocThreshold: 50}
	clones := codeReportSpec(t, e.goComponent, "go-clones.json", "jscpd", entity.CodeFormatJscpd)
	functions.component, clones.component = component, component
	other := clones
	other.name, other.profileID, other.mode = "unselected-clones.json", "sonar-import", envelope.ScanToolRunModeImport
	rec := e.submit(t, reportPackageOf(t, _checksInstance, _reportSHA, lint, sast, secret, sca, docs, functions, clones, other))
	buildCalcJobs(t, e.owner, e.clock, e.calculator)

	pythonClones := codeReportSpec(t, e.goComponent, "python-clones.json", "jscpd", entity.CodeFormatJscpd)
	pythonFunctions := codeReportSpec(t, e.goComponent, "python-functions.csv", "lizard", entity.CodeFormatLizard)
	pythonClones.component, pythonClones.profileID = component, clones.profileID
	pythonFunctions.component, pythonFunctions.profileID = component, functions.profileID
	pythonFunctions.thresholds = functions.thresholds
	importedClones := clones
	importedClones.name, importedClones.mode = "imported-go-clones.json", envelope.ScanToolRunModeImport
	e.submit(t, reportPackageOf(t, _checksInstance, _reportSHA, pythonClones, pythonFunctions, importedClones))
	buildCalcJobs(t, e.owner, e.clock, e.calculator)
	out := map[string]uuid.UUID{}
	for _, f := range e.identities(t, rec) {
		out[f.rule] = f.identity
	}
	require.Contains(t, out, "AWS")
	require.Contains(t, out, "lint-rule")
	require.Contains(t, out, "sast-rule")
	require.Contains(t, out, "missing-anchor")
	return out
}

func (e *identityEnv) checkFindingExpiry(t *testing.T, dev *browser, path string, id uuid.UUID) {
	t.Helper()
	project := uuid.UUID(e.project)
	e.setExceptionRoles(t, _otherSub, []roleInput{{Role: "tech_lead", ProjectID: &project}, {Role: "security_access", ProjectID: &project}})
	now := e.clock.Now().UTC()
	expires := now.Add(10 * time.Minute)
	draft := openapi.ExceptionDraft{
		DecisionType: openapi.ExceptionDraftDecisionType(entity.ExceptionAcceptedRisk), Reason: "Reviewed in fixture",
		EffectiveFrom: now, ExpiresAt: expires,
		Scope: openapi.ExceptionScope{AreaId: e.areas["42"], ComponentId: &e.goComponent, FindingId: &id},
	}
	header := _changing.Clone()
	header.Set("Idempotency-Key", uuid.NewString())
	created := e.call(t, dev, "", http.MethodPost, _exceptionsPath, header, mustMarshal(t, draft))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	exception := decode[openapi.ExceptionDetail](t, created)
	header.Set("Idempotency-Key", uuid.NewString())
	header.Set("If-Match", created.Header().Get("ETag"))
	approval := e.call(t, e.lead, "", http.MethodPost, _exceptionsPath+"/"+exception.Id.String()+"/decisions", header, []byte(`{"decision":"approve"}`))
	require.Equal(t, http.StatusOK, approval.Code, approval.Body.String())

	check := func(state openapi.FindingState, decisionState openapi.FindingDetailExceptionsState) {
		t.Helper()
		rec := e.call(t, dev, "", http.MethodGet, path, nil, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		card := decode[openapi.FindingDetail](t, rec)
		assert.Equal(t, state, card.State)
		require.Len(t, card.Exceptions, 1)
		assert.Equal(t, decisionState, card.Exceptions[0].State)
		p := _findingsPath + "?finding_kind=secret&state=" + string(state)
		page := decode[openapi.FindingPage](t, e.call(t, dev, "", http.MethodGet, p, nil, nil))
		require.Len(t, page.Data, 1)
		assert.Equal(t, id, page.Data[0].Id)
		overviewPath := "/api/v1/components/" + e.goComponent.String() + "/overview"
		rec = e.call(t, dev, "", http.MethodGet, overviewPath, nil, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		overview := decode[openapi.ComponentOverview](t, rec)
		for i := range overview.FindingsByState {
			bucket := &overview.FindingsByState[i]
			if bucket.FindingKind == openapi.FindingKindSecret {
				assert.Equal(t, state, bucket.State)
				assert.Equal(t, new(float32(1)), bucket.Count.Value)
				return
			}
		}
		t.Fatal("missing secret bucket")
	}
	check(openapi.FindingStateAcceptedRisk, openapi.FindingDetailExceptionsStateActive)
	e.clock.Advance(expires.Sub(e.clock.Now()))
	check(openapi.FindingStateOpen, openapi.FindingDetailExceptionsStateExpired)
}
