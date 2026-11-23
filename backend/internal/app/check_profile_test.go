package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	decisionuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/decision"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/org"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

// checksEnv is the CI env with the test catalog of profiles, a second project, a service with a Go component in
// the repository of each project and a tech lead of both projects.
type checksEnv struct {
	*ciEnv
	lead              *browser
	other             uuid.UUID
	otherToken        uploadTokenBody
	goComponent, docs uuid.UUID
	otherComponent    uuid.UUID
	areas             map[string]uuid.UUID
	orgs              *org.UseCase
	norm              *normalize.UseCase
	decisions         *decisionuc.UseCase
}

func newChecksEnv(t *testing.T) *checksEnv {
	t.Helper()
	e := &checksEnv{ciEnv: newCIEnv(t), areas: map[string]uuid.UUID{}}
	catalog, err := change.ReadCatalog("../usecase/change/testdata/catalog.yaml")
	require.NoError(t, err)
	orgs, _, err := newOrgAccess(e.app, e.clock, e.connect, nil)
	require.NoError(t, err)
	e.orgs = orgs
	e.norm, err = newNormalize(e.app, e.store, e.uc, orgs, nil, e.clock)
	require.NoError(t, err)
	e.decisions, err = newDecision(e.app, e.rights, e.clock)
	require.NoError(t, err)
	reg := prometheus.NewRegistry()
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg,
		apiModules{orgs: orgs, access: e.rights, connections: e.connect, intake: e.uc, catalog: catalog, findingReports: e.norm, decisions: e.decisions})
	e.router = httpctrl.NewRouter(zerolog.New(zerolog.SyncWriter(e.logs)), reg, health.Probes{Ready: &health.Readiness{}}, api)

	e.other = e.tokenProject(t, e.orgID)
	e.bindRepository(t, entity.ID(e.other), "43")
	e.otherToken = e.issueUpload(t, e.adminBrowser, e.other)
	for _, native := range []string{"42", "43"} {
		var id uuid.UUID
		require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind='repository' AND native_id=$1", native).Scan(&id))
		e.areas[native] = id
	}

	st, tag := e.structure(t, e.adminBrowser)
	in := st.input()
	in.Services = append(in.Services, named{Name: "billing", Status: "active"})
	rec := e.replace(t, e.adminBrowser, tag, in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	st = decode[structureBody](t, rec)
	service := *st.Services[0].ID
	in = st.input()
	component := func(area uuid.UUID, prefix, status string) map[string]any {
		return map[string]any{"id": nil, "service_id": service, "repository_area_id": area, "path_prefix": prefix, "status": status}
	}
	in.Components = []any{
		component(e.areas["42"], "services/billing", "confirmed"), component(e.areas["42"], "docs", "confirmed"),
		component(e.areas["42"], "legacy", "rejected"), component(e.areas["43"], "", "confirmed"),
	}
	rec = e.replace(t, e.adminBrowser, rec.Header().Get("ETag"), in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	st = decode[structureBody](t, rec)
	require.Len(t, st.Components, 4)
	for _, raw := range st.Components {
		c := raw.(map[string]any)
		id := uuid.MustParse(c["id"].(string))
		assert.Equal(t, "manual", c["origin"])
		switch {
		case c["path_prefix"] == "services/billing":
			e.goComponent = id
		case c["path_prefix"] == "docs":
			e.docs = id
		case c["repository_area_id"] == e.areas["43"].String():
			e.otherComponent = id
		}
	}

	e.lead = newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, e.lead, _otherSub).Code)
	user := e.userID(t, e.orgID, _otherSub)
	project := uuid.UUID(e.project)
	require.Equal(t, http.StatusOK, e.roles(t, e.adminBrowser, user, `"0"`, []roleInput{{Role: "tech_lead", ProjectID: &project}, {Role: "tech_lead", ProjectID: &e.other}}).Code)
	return e
}

// call sends a request of a browser or of an upload token and checks the response against the contract.
func (e *checksEnv) call(t *testing.T, b *browser, token, method, target string, header http.Header, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader = http.NoBody
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, target, rd)
	for k, v := range header {
		req.Header[k] = v
	}
	if b != nil {
		for name, value := range b.cookies {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	e.validateTemplate(t, req, rec)
	return rec
}

// validateTemplate validates a response of a path with parameters against the operation of its template.
func (e *checksEnv) validateTemplate(t *testing.T, req *http.Request, out *httptest.ResponseRecorder) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/api/v1"), "/")
	for template, item := range e.contract.Paths.Map() {
		tparts := strings.Split(template, "/")
		if len(tparts) != len(parts) {
			continue
		}
		params := map[string]string{}
		match := true
		for i, p := range tparts {
			switch {
			case strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}"):
				params[strings.Trim(p, "{}")] = parts[i]
			case p != parts[i]:
				match = false
			}
		}
		if !match {
			continue
		}
		input := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request: req, PathParams: params,
				Route: &routers.Route{Spec: e.contract, Path: template, PathItem: item, Method: req.Method, Operation: item.GetOperation(req.Method)},
			},
			Status: out.Code, Header: out.Header(), Body: io.NopCloser(bytes.NewReader(out.Body.Bytes())),
		}
		require.NoError(t, openapi3filter.ValidateResponse(t.Context(), input), out.Body.String())
		return
	}
	t.Fatalf("missing contract path %s", req.URL.Path)
}

func profilePath(project uuid.UUID) string {
	return "/api/v1/projects/" + project.String() + "/check-profiles"
}

func (e *checksEnv) confirm(t *testing.T, project uuid.UUID, ifMatch string, key uuid.UUID, entries ...map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	h := http.Header{"Origin": {_site}, "Content-Type": {"application/json"}, "Idempotency-Key": {key.String()}}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	raw, err := json.Marshal(map[string]any{"entries": entries})
	require.NoError(t, err)
	return e.call(t, e.lead, "", http.MethodPost, profilePath(project), h, raw)
}

func row(component uuid.UUID, language, kind, mode, profile string) map[string]any {
	return map[string]any{"component_id": component, "language": language, "analysis_kind": kind, "mode": mode, "profile_id": profile}
}

type profileBody struct {
	VersionNo     int  `json:"version_no"`
	BaseVersionNo *int `json:"base_version_no"`
	Entries       []struct {
		ComponentID  uuid.UUID         `json:"component_id"`
		Language     string            `json:"language"`
		AnalysisKind string            `json:"analysis_kind"`
		Mode         string            `json:"mode"`
		ProfileID    string            `json:"profile_id"`
		ToolVersions map[string]string `json:"tool_versions"`
	} `json:"entries"`
	Uncovered []struct {
		ComponentID uuid.UUID `json:"component_id"`
		ReasonCode  string    `json:"reason_code"`
	} `json:"uncovered"`
}

func TestCheckProfiles_MatrixVersionsAndValidation(t *testing.T) {
	t.Parallel()
	e := newChecksEnv(t)
	project := uuid.UUID(e.project)
	assert.Equal(t, http.StatusNotFound, e.call(t, e.lead, "", http.MethodGet, profilePath(project), nil, nil).Code, "no matrix yet")
	v24 := make([]map[string]any, 0, 4)
	v24 = append(v24,
		row(e.goComponent, "go", "code", "import", "sarif-import"),
		row(e.goComponent, "", "secrets", "builtin", "gitleaks-default"),
		row(e.goComponent, "", "sca", "builtin", "trivy-default"),
	)
	assert.Equal(t, http.StatusPreconditionRequired, e.confirm(t, project, "", uuid.New(), v24...).Code)

	key := uuid.New()
	rec := e.confirm(t, project, `"0"`, key, v24...)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, `"1"`, rec.Header().Get("ETag"))
	first := decode[profileBody](t, rec)
	assert.Equal(t, 1, first.VersionNo)
	assert.Nil(t, first.BaseVersionNo)
	require.Len(t, first.Entries, 3)
	kinds := map[string]string{}
	for _, en := range first.Entries {
		kinds[en.AnalysisKind] = en.Mode
	}
	assert.Equal(t, map[string]string{"code": "import", "secrets": "builtin", "sca": "builtin"}, kinds, "V-24: an import of code keeps the builtin secrets and sca")
	uncovered := map[uuid.UUID]string{}
	for _, u := range first.Uncovered {
		uncovered[u.ComponentID] = u.ReasonCode
	}
	assert.Equal(t, map[uuid.UUID]string{e.docs: entity.ReasonProfileMissing}, uncovered, "a component without a profile is uncovered with a reason")

	again := e.confirm(t, project, `"0"`, key, v24...)
	assert.Equal(t, http.StatusCreated, again.Code, "a repeated key answers what it saved")
	same := e.confirm(t, project, `"1"`, uuid.New(), v24...)
	assert.Equal(t, http.StatusOK, same.Code, "the same matrix creates no version")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM change.profile_version"))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM audit.event WHERE operation = 'check_profile.confirm'"))

	cases := []struct {
		name    string
		entries []map[string]any
		field   string
	}{
		{"without secrets", []map[string]any{row(e.goComponent, "go", "code", "builtin", "go-default"), row(e.goComponent, "", "sca", "builtin", "trivy-default")}, "/entries"},
		{"unsupported language", []map[string]any{row(e.goComponent, "kotlin", "code", "builtin", "go-default")}, "/entries/0/language"},
		{"unsupported analysis kind", []map[string]any{row(e.goComponent, "", "iac", "builtin", "gitleaks-default")}, "/entries/0/analysis_kind"},
		{"mode of another profile", []map[string]any{row(e.goComponent, "go", "code", "builtin", "sarif-import")}, "/entries/0/profile_id"},
		{"component of another project", []map[string]any{row(e.otherComponent, "", "secrets", "builtin", "gitleaks-default")}, "/entries/0/component_id"},
		{"repeated row", []map[string]any{row(e.docs, "", "docs", "builtin", "docs-default"), row(e.docs, "", "docs", "builtin", "docs-default")}, "/entries/1"},
	}
	for _, tc := range cases {
		rec := e.confirm(t, project, `"1"`, uuid.New(), tc.entries...)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, tc.name)
		assert.Contains(t, rec.Body.String(), `"`+tc.field+`"`, tc.name)
	}
	stale := e.confirm(t, project, `"0"`, uuid.New(), append(v24, row(e.docs, "", "docs", "builtin", "docs-default"))...)
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM change.profile_version"), "a stale version writes nothing")

	read := e.call(t, e.lead, "", http.MethodGet, profilePath(project), nil, nil)
	require.Equal(t, http.StatusOK, read.Code)
	assert.Equal(t, `"1"`, read.Header().Get("ETag"))
	for _, en := range decode[profileBody](t, read).Entries {
		if en.ProfileID == "gitleaks-default" {
			assert.Equal(t, map[string]string{"gitleaks": "8.28.0"}, en.ToolVersions)
		}
	}
	assert.Equal(t, http.StatusForbidden, e.call(t, e.adminBrowser, "", http.MethodGet, profilePath(project), nil, nil).Code,
		"the matrix belongs to the tech lead of the project")
}

type ciProfileBody struct {
	ProjectID        uuid.UUID `json:"project_id"`
	ProfileVersionNo int       `json:"profile_version_no"`
	Entries          []struct {
		ComponentID uuid.UUID `json:"component_id"`
		PathPrefix  string    `json:"path_prefix"`
		Kind        string    `json:"analysis_kind"`
		Mode        string    `json:"mode"`
		ImageDigest *string   `json:"image_digest"`
	} `json:"entries"`
	ExpectedReports    []string `json:"expected_reports"`
	DocumentationRoots []struct {
		RootID     uuid.UUID `json:"root_id"`
		Kind       string    `json:"kind"`
		PathPrefix string    `json:"path_prefix"`
	} `json:"documentation_roots"`
	Limits struct {
		MaxPackageBytes  int `json:"max_package_bytes"`
		MaxUnpackedBytes int `json:"max_unpacked_bytes"`
		MaxFiles         int `json:"max_files"`
	} `json:"limits"`
}

func TestCheckProfiles_ProfileOfTheStepAndSnippet(t *testing.T) {
	t.Parallel()
	e := newChecksEnv(t)
	project := uuid.UUID(e.project)
	rec := e.confirm(t, project, `"0"`, uuid.New(),
		row(e.goComponent, "go", "code", "builtin", "go-default"), row(e.goComponent, "go", "sast", "import", "sarif-import"),
		row(e.goComponent, "", "secrets", "builtin", "gitleaks-default"), row(e.goComponent, "", "sca", "import", "sca-import"),
		row(e.docs, "", "docs", "builtin", "docs-default"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = e.confirm(t, e.other, `"0"`, uuid.New(), row(e.otherComponent, "", "secrets", "builtin", "gitleaks-default"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	root, confirmation := uuid.New(), uuid.New()
	_, err := e.owner.Exec(t.Context(), `INSERT INTO docs.root_set (organization_id, service_id, version_no, known_seq, content_hash, confirmed_by,
  confirmed_at, confirmation_id) SELECT $1, service_id, 1, 1, sha256('roots'), $2, now(), $3 FROM org.component WHERE id = $4`,
		uuid.UUID(e.orgID), uuid.UUID(e.admin.UserID), confirmation, e.docs)
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), `INSERT INTO docs.root (organization_id, id, service_id, area_id, path_prefix, profile_id, audience, known_seq,
  confirmed_by, confirmed_at, confirmation_id) SELECT $1, $2, service_id, $3, 'docs', 'git-markdown-openapi', 'team', 1, $4, now(), $5
  FROM org.component WHERE id = $6`, uuid.UUID(e.orgID), root, e.areas["42"], uuid.UUID(e.admin.UserID), confirmation, e.docs)
	require.NoError(t, err)

	got := e.call(t, nil, e.token.Value, http.MethodGet, "/api/v1/ci/check-profile", nil, nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	p := decode[ciProfileBody](t, got)
	assert.Equal(t, project, p.ProjectID)
	require.Len(t, p.Entries, 5)
	for _, en := range p.Entries {
		assert.NotEqual(t, e.otherComponent, en.ComponentID, "the token of project A reads only the matrix of A")
		if en.Mode == "builtin" {
			require.NotNil(t, en.ImageDigest)
			assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, *en.ImageDigest)
		} else {
			assert.Nil(t, en.ImageDigest, "an import has no image")
		}
		if en.ComponentID == e.goComponent {
			assert.Equal(t, "services/billing", en.PathPrefix)
		}
	}
	assert.Equal(t, []string{"code", "docs", "sast", "sca", "secrets"}, p.ExpectedReports)
	require.Len(t, p.DocumentationRoots, 1)
	assert.Equal(t, root, p.DocumentationRoots[0].RootID)
	assert.Equal(t, "docs", p.DocumentationRoots[0].Kind)
	assert.Equal(t, [3]int{25 << 20, 100 << 20, 1000}, [3]int{p.Limits.MaxPackageBytes, p.Limits.MaxUnpackedBytes, p.Limits.MaxFiles})
	other := decode[ciProfileBody](t, e.call(t, nil, e.otherToken.Value, http.MethodGet, "/api/v1/ci/check-profile", nil, nil))
	require.Len(t, other.Entries, 1)
	assert.Equal(t, e.otherComponent, other.Entries[0].ComponentID)
	assert.Equal(t, http.StatusUnauthorized, e.call(t, nil, "", http.MethodGet, "/api/v1/ci/check-profile", nil, nil).Code)

	cfg := e.call(t, e.lead, "", http.MethodGet, "/api/v1/projects/"+project.String()+"/ci-configuration?ci_system=gitlab_ci", nil, nil)
	require.Equal(t, http.StatusOK, cfg.Code, cfg.Body.String())
	var body struct {
		Snippet   string `json:"snippet"`
		Variables []struct {
			Name   string `json:"name"`
			Masked bool   `json:"masked"`
		} `json:"variables"`
	}
	require.NoError(t, json.Unmarshal(cfg.Body.Bytes(), &body))
	assert.NotContains(t, body.Snippet, e.token.Value)
	assert.NotContains(t, body.Snippet, "cpt_")
	var jobs map[string]struct {
		Stage        string   `yaml:"stage"`
		Image        string   `yaml:"image"`
		AllowFailure bool     `yaml:"allow_failure"`
		Script       []string `yaml:"script"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(body.Snippet), &jobs))
	names := make([]string, 0, len(jobs))
	for name, job := range jobs {
		names = append(names, name)
		assert.Regexp(t, `@sha256:[0-9a-f]{64}$`, job.Image, name)
		assert.True(t, job.AllowFailure, name)
		assert.Equal(t, "test", job.Stage, name)
		assert.Equal(t, "craftpulse-ci upload", job.Script[len(job.Script)-1], name)
	}
	assert.ElementsMatch(t, []string{"craftpulse-go-default", "craftpulse-gitleaks-default", "craftpulse-docs-default", "craftpulse-import"}, names)
	assert.Equal(t, "craftpulse-ci import --report sast:sarif:${CRAFTPULSE_REPORT_SAST} --report sca:sarif:${CRAFTPULSE_REPORT_SCA}", jobs["craftpulse-import"].Script[0])
	require.GreaterOrEqual(t, len(body.Variables), 2)
	assert.Equal(t, "CRAFTPULSE_UPLOAD_TOKEN", body.Variables[0].Name)
	assert.True(t, body.Variables[0].Masked)
	assert.Equal(t, "CRAFTPULSE_URL", body.Variables[1].Name)
	assert.False(t, body.Variables[1].Masked)
	jenkins := e.call(t, e.lead, "", http.MethodGet, "/api/v1/projects/"+project.String()+"/ci-configuration?ci_system=jenkins", nil, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, jenkins.Code)
}

func TestCheckProfiles_ComponentsOfTheStructure(t *testing.T) {
	t.Parallel()
	e := newChecksEnv(t)
	st, tag := e.structure(t, e.adminBrowser)
	in := st.input()
	in.Components = in.Components[:len(in.Components)-1]
	rec := e.replace(t, e.adminBrowser, tag, in)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "a component left out of the body is an error")
	in = st.input()
	for _, raw := range in.Components {
		c := raw.(map[string]any)
		if c["id"] == e.docs.String() {
			c["status"] = "rejected"
		}
	}
	rec = e.replace(t, e.adminBrowser, tag, in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.component WHERE id = '"+e.docs.String()+"' AND superseded_seq IS NOT NULL"), "a change is a new version")
	assert.Equal(t, 4, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.component WHERE superseded_seq IS NULL"))
	in = st.input()
	in.Components = append(in.Components, map[string]any{
		"id": nil, "service_id": *st.Services[0].ID, "repository_area_id": e.areas["42"], "path_prefix": "services/billing", "status": "confirmed",
	})
	rec = e.replace(t, e.adminBrowser, rec.Header().Get("ETag"), in)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "two components take one path")
	for _, prefix := range []string{"/abs", "a/../b", "a//b"} {
		in = st.input()
		in.Components = append(in.Components, map[string]any{
			"id": nil, "service_id": *st.Services[0].ID, "repository_area_id": e.areas["42"], "path_prefix": prefix, "status": "confirmed",
		})
		_, tag = e.structure(t, e.adminBrowser)
		rec = e.replace(t, e.adminBrowser, tag, in)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, prefix)
	}
}
