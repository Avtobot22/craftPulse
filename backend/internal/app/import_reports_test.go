package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func reportForm(project uuid.UUID, schema, format string) map[string]string {
	return map[string]string{
		"schema_id": schema, "schema_version": "1", "report_format": format, "commit_sha": _reportSHA,
		"project_id": project.String(), "source_provider": "gitlab", "source_instance_key": "gitlab.example",
	}
}

func reportNormalization(t *testing.T, env *ingestEnv) *normalizeEnv {
	t.Helper()
	orgs, _, err := newOrgAccess(env.app, env.clock, env.connect, nil)
	require.NoError(t, err)
	norm, err := newNormalize(env.app, env.store, env.uc, orgs, nil, env.clock)
	require.NoError(t, err)
	return &normalizeEnv{ingestEnv: env, norm: norm}
}

func TestImports_ReportsMatchCIPackage(t *testing.T) {
	t.Parallel()
	ci, imports := newCIEnv(t), newCIEnv(t)
	ciFacts, importFacts := reportNormalization(t, ci.ingestEnv), reportNormalization(t, imports.ingestEnv)
	specs := []struct {
		schema, media string
		spec          reportSpec
	}{
		{"findings", "application/sarif+json", sarifSpecs()[0]},
		{"junit", "application/xml", v40()[0]},
		{"coverage", "text/plain", v40()[3]},
	}
	for _, item := range specs {
		t.Run(item.schema, func(t *testing.T) {
			spec := item.spec
			input := reportFixture(t, spec.fixture)
			c := ci.uploadCI(t, ci.token.Value, reportPackageOf(t, "gitlab.example", _reportSHA, spec))
			require.Equal(t, http.StatusAccepted, c.Code, c.Body.String())
			ciID := entity.ID(decode[acceptedBody](t, c).ReceiptID)
			require.NoError(t, ci.uc.ProcessPackage(t.Context(), ci.orgID, ciID))
			ciFacts.drain(t)
			require.Equal(t, entity.ReceiptAccepted, ciFacts.receipt(t, ciID).Status)

			form := reportForm(uuid.UUID(imports.project), item.schema, spec.format)
			i := imports.upload(t, imports.adminBrowser, form, item.media, input)
			require.Equal(t, http.StatusAccepted, i.Code, i.Body.String())
			imp := decode[acceptedBody](t, i)
			require.Equal(t, "quarantined", imp.Status, i.Body.String())
			id := entity.ID(imp.ReceiptID)
			require.NoError(t, imports.uc.ProcessPackage(t.Context(), imports.orgID, id))
			importFacts.drain(t)
			rc := importFacts.receipt(t, id)
			require.Equal(t, entity.ChannelImport, rc.Channel)
			require.Equal(t, entity.ReceiptAccepted, rc.Status)
			require.Equal(t, 1, rc.AcceptedCount)
			var m envelope.Manifest
			require.NoError(t, json.Unmarshal(rc.Manifest, &m))
			assert.Equal(t, imports.project.String(), m.Area.NativeId)
			assert.Equal(t, envelope.AreaRefKindAnalysisProject, m.Area.Kind)
			r := importFacts.report(t, id, spec.kind)
			assert.Equal(t, "import", r.Mode)
			assert.Equal(t, &_reportSHA, r.SHA)
			assert.Equal(t, ciFacts.report(t, ciID, spec.kind).ParserVersion, r.ParserVersion)
			assert.Equal(t, 1, importFacts.n(t, `SELECT count(*) FROM org.project_area
				WHERE project_id = $1 AND area_id = $2 AND superseded_seq IS NULL`, uuid.UUID(imports.project), r.AreaID))

			switch item.schema {
			case "findings":
				want := ciFacts.findings(t, ciID, spec.kind)
				require.NotEmpty(t, want)
				assert.Equal(t, want, importFacts.findings(t, id, spec.kind))
			case "junit":
				want, got := ciFacts.executions(t, ciID), importFacts.executions(t, id)
				require.NotEmpty(t, want)
				require.Len(t, got, len(want))
				for i := range want {
					assert.NotNil(t, want[i].attempt)
					assert.Nil(t, got[i].attempt)
					want[i].attempt = nil
					want[i].fingerprint, got[i].fingerprint = "", ""
				}
				assert.Equal(t, want, got)
			case "coverage":
				want, got := ciFacts.coverage(t, ciID), importFacts.coverage(t, id)
				require.NotEmpty(t, want)
				require.Len(t, got, len(want))
				ciMap, err := ci.store.Get(t.Context(), want[0].lineMap)
				require.NoError(t, err)
				importMap, err := imports.store.Get(t.Context(), got[0].lineMap)
				require.NoError(t, err)
				assert.Equal(t, lineMaps(t, ciMap), lineMaps(t, importMap))
				for i := range want {
					want[i].lineMap, got[i].lineMap = "", ""
				}
				assert.Equal(t, want, got)
			}

			imports.clock.Advance(time.Minute)
			repeat := imports.upload(t, imports.adminBrowser, form, item.media, input)
			require.Equal(t, http.StatusAccepted, repeat.Code, repeat.Body.String())
			duplicate := decode[acceptedBody](t, repeat)
			assert.Equal(t, "duplicate", duplicate.Status)
			assert.Equal(t, &imp.ReceiptID, duplicate.DuplicateOf)
		})
	}
}

func TestImports_AdditionalReportFormats(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	norm := reportNormalization(t, e.ingestEnv)
	rec := mrRecord(t, "finding", "finding-1", _reportSHA, "1", envelope.FindingV1{
		FindingKind: "code", Sha: _reportSHA, Producer: "linter", ProducerVersion: "1",
		RuleId: "rule-1", Path: ptr("main.go"), LineStart: ptr(4), LineEnd: ptr(6),
	})
	cases := []struct {
		schema, format, kind, media string
		file                        []byte
	}{
		{"findings", "envelope", "code", "application/json", mustMarshal(t, []*envelope.Record{rec})},
		{"findings", "gitlab_code_quality", "code", "application/json", []byte(`[{
			"description":"Unused variable", "check_name":"unused", "fingerprint":"item-1", "severity":"minor",
			"location":{"path":"src/main.js","positions":{"begin":{"line":4},"end":{"line":5}}}
		}]`)},
		{"findings", "gitlab_security", "secrets", "application/json", reportFixture(t, "gl-secret-detection-report.json")},
		{"findings", "gitlab_security", "sca", "application/json", reportFixture(t, "gl-dependency-scanning-report.json")},
		{"findings", "sarif", "sast", "application/sarif+json", reportFixture(t, "opengrep.sarif")},
		{"coverage", "cobertura", "coverage", "text/xml", reportFixture(t, "coverage-py.xml")},
		{"coverage", "jacoco", "coverage", "application/xml", reportFixture(t, "jacoco.xml")},
	}
	for _, tt := range cases {
		t.Run(tt.format+"/"+tt.kind, func(t *testing.T) {
			form := reportForm(uuid.UUID(e.project), tt.schema, tt.format)
			if tt.format == "sarif" {
				form["report_kind"] = tt.kind
			}
			if tt.format == "cobertura" {
				form["source_root"] = "/builds/demo/app"
			}
			resp := e.upload(t, e.adminBrowser, form, tt.media, tt.file)
			require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
			id := entity.ID(decode[acceptedBody](t, resp).ReceiptID)
			require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, id))
			norm.drain(t)
			require.Equal(t, entity.ReceiptAccepted, norm.receipt(t, id).Status, norm.records(t, id))
			r := norm.report(t, id, tt.kind)
			assert.Equal(t, tt.format, r.Format)
			assert.Equal(t, "import", r.Mode)
			if tt.schema == "findings" {
				assert.NotEmpty(t, norm.findings(t, id, tt.kind))
			} else {
				assert.NotEmpty(t, norm.coverage(t, id))
			}
		})
	}
}

func TestImports_ReportsRejectInvalidInputAndOutsideScope(t *testing.T) {
	t.Parallel()
	e := newCIEnv(t)
	facts := reportNormalization(t, e.ingestEnv)
	form := reportForm(uuid.UUID(e.project), "junit", "junit_xml")
	content := reportFixture(t, "junit-gotestsum.xml")

	delete(form, "commit_sha")
	missing := e.upload(t, e.adminBrowser, form, "application/xml", content)
	require.Equal(t, http.StatusUnprocessableEntity, missing.Code, missing.Body.String())
	assert.Contains(t, missing.Body.String(), `"field":"/commit_sha"`)
	assert.Contains(t, missing.Body.String(), `"code":"required"`)
	form["commit_sha"] = _reportSHA
	delete(form, "report_format")
	missingFormat := e.upload(t, e.adminBrowser, form, "application/xml", content)
	require.Equal(t, http.StatusUnprocessableEntity, missingFormat.Code, missingFormat.Body.String())
	assert.Contains(t, missingFormat.Body.String(), `"field":"/report_format"`)
	form["report_format"] = "lcov"
	wrongFormat := e.upload(t, e.adminBrowser, form, "application/xml", content)
	require.Equal(t, http.StatusUnprocessableEntity, wrongFormat.Code, wrongFormat.Body.String())
	assert.Contains(t, wrongFormat.Body.String(), `"field":"/report_format"`)
	form["report_format"] = "junit_xml"

	first := e.upload(t, e.adminBrowser, form, "application/xml", content)
	require.Equal(t, http.StatusAccepted, first.Code, first.Body.String())
	firstID := entity.ID(decode[acceptedBody](t, first).ReceiptID)
	require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, firstID))
	facts.drain(t)
	former := facts.executions(t, firstID)
	require.NotEmpty(t, former)

	for _, file := range [][]byte{content[:len(content)/2], append(bytes.Clone(content), "<testsuite/>"...)} {
		bad := e.upload(t, e.adminBrowser, form, "application/xml", file)
		require.Equal(t, http.StatusAccepted, bad.Code, bad.Body.String())
		badID := decode[acceptedBody](t, bad).ReceiptID
		require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(badID)))
		facts.drain(t)
		path := "/api/v1/receipts/" + badID.String()
		body := e.do(t, e.adminBrowser, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, body.Code, body.Body.String())
		rc := decode[receiptBody](t, body)
		assert.Equal(t, "import", rc.Channel)
		assert.Equal(t, "partial", rc.Status)
		assert.Zero(t, rc.AcceptedCount)
		assert.Equal(t, 1, rc.RejectedCount)
		records := e.do(t, e.adminBrowser, http.MethodGet, path+"/records?outcome=rejected", nil)
		require.Equal(t, http.StatusOK, records.Code, records.Body.String())
		assert.Contains(t, records.Body.String(), `"reason_code":"invalid_report"`)
	}
	assert.Equal(t, former, facts.executions(t, firstID))
	assert.Equal(t, 1, facts.n(t, "SELECT count(*) FROM normalize.report"))

	other := e.tokenProject(t, e.orgID)
	lead := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, lead, _otherSub).Code)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment (organization_id, user_id, role, project_id, assigned_by, assigned_at)
		VALUES ($1, $2, 'tech_lead', $3, $2, now())`, uuid.UUID(e.orgID), e.userID(t, e.orgID, _otherSub), other)
	require.NoError(t, err)
	denied := e.upload(t, lead, form, "application/xml", content)
	require.Equal(t, http.StatusAccepted, denied.Code, denied.Body.String())
	rejected := decode[acceptedBody](t, denied)
	assert.Equal(t, "rejected", rejected.Status)
	assert.Nil(t, rejected.DuplicateOf)
	path := "/api/v1/receipts/" + rejected.ReceiptID.String()
	body := e.do(t, e.adminBrowser, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, body.Code, body.Body.String())
	rc := decode[receiptBody](t, body)
	require.NotNil(t, rc.ReasonCode)
	assert.Equal(t, "outside_scope", *rc.ReasonCode)
	assert.Equal(t, http.StatusNotFound, e.do(t, lead, http.MethodGet, path, nil).Code)
	assert.Equal(t, 1, facts.n(t, "SELECT count(*) FROM normalize.report"))
}
