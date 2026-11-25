package change

import (
	"context"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_varToken = "CRAFTPULSE_UPLOAD_TOKEN" //nolint:gosec // the name of the variable, not its value
	_varURL   = "CRAFTPULSE_URL"
	// _reportVar names the variable of the path of a ready report of a kind the client points the import to.
	_reportVar = "CRAFTPULSE_REPORT_"
)

// matrix is the matrix in force of a project with its components and areas, read in one transaction.
type matrix struct {
	version    entity.CheckProfileVersion
	components map[entity.ID]entity.Component
	roots      []entity.DocumentationRoot
}

func (u *UseCase) matrix(ctx context.Context, orgID, projectID entity.ID, roots bool) (matrix, error) {
	var m matrix
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if m.version, err = u.d.Repo.LatestProfileVersion(ctx, orgID, projectID); err != nil {
			return err
		}
		now := u.d.Clock.Now()
		components, err := u.d.Components.ProjectComponents(ctx, orgID, projectID, now)
		if err != nil {
			return err
		}
		m.components = make(map[entity.ID]entity.Component, len(components))
		for _, c := range components {
			m.components[c.ID] = c
		}
		if !roots {
			return nil
		}
		links, err := u.d.Components.ProjectAreas(ctx, orgID, now)
		if err != nil {
			return err
		}
		var areas []entity.ID
		for _, l := range links {
			if l.From == projectID && !slices.Contains(areas, l.To) {
				areas = append(areas, l.To)
			}
		}
		m.roots, err = u.d.Roots.AreaRoots(ctx, orgID, areas)
		return err
	})
	return m, err
}

// CICheckProfile is the profile the step of CI reads by the upload token of its project: the rows of components
// in force with the images of builtin profiles by digest, the reports the step sends, the documentation roots the
// matrix checks and the limits of a package.
func (u *UseCase) CICheckProfile(ctx context.Context, s entity.Subject) (entity.CICheckProfile, error) {
	if s.UploadTokenID.IsZero() {
		return entity.CICheckProfile{}, entity.ErrAuthRequired
	}
	m, err := u.matrix(ctx, s.OrganizationID, s.ProjectID, true)
	if err != nil {
		return entity.CICheckProfile{}, err
	}
	limits := envelope.DefaultLimits
	out := entity.CICheckProfile{
		ProjectID: s.ProjectID, VersionNo: m.version.VersionNo, Entries: []entity.CIProfileEntry{}, ExpectedReports: []string{},
		DocumentationRoots: []entity.CIDocumentationRoot{},
		Limits:             entity.PackageLimits{MaxPackageBytes: limits.MaxPackageBytes, MaxUnpackedBytes: limits.MaxUnpackedBytes, MaxFiles: limits.MaxFiles},
	}
	docKinds := map[entity.ID][]string{}
	for _, e := range m.version.Entries {
		c, ok := m.components[e.ComponentID]
		p, known := u.profiles[e.ProfileID]
		if !ok || !known {
			continue
		}
		row := entity.CIProfileEntry{
			ComponentID: e.ComponentID, PathPrefix: c.PathPrefix, Language: e.Language, AnalysisKind: e.AnalysisKind, Mode: e.Mode, ProfileID: e.ProfileID,
		}
		if e.Mode == entity.CheckModeBuiltin {
			row.ImageDigest = p.Digest
		}
		out.Entries = append(out.Entries, row)
		if !slices.Contains(out.ExpectedReports, e.AnalysisKind) {
			out.ExpectedReports = append(out.ExpectedReports, e.AnalysisKind)
		}
		if (e.AnalysisKind == entity.AnalysisDocs || e.AnalysisKind == entity.AnalysisOpenAPI) && !slices.Contains(docKinds[c.RepositoryAreaID], e.AnalysisKind) {
			docKinds[c.RepositoryAreaID] = append(docKinds[c.RepositoryAreaID], e.AnalysisKind)
		}
	}
	slices.Sort(out.ExpectedReports)
	for i := range m.roots {
		r := &m.roots[i]
		kinds := slices.Sorted(slices.Values(docKinds[r.AreaID]))
		for _, kind := range kinds {
			out.DocumentationRoots = append(out.DocumentationRoots, entity.CIDocumentationRoot{RootID: r.ID, Kind: kind, PathPrefix: r.PathPrefix})
		}
	}
	return out, nil
}

// gitlabJob is a job of the snippet; the order of fields is the order a reader of .gitlab-ci.yml expects.
type gitlabJob struct {
	Stage        string   `yaml:"stage"`
	Image        string   `yaml:"image"`
	AllowFailure bool     `yaml:"allow_failure"`
	Script       []string `yaml:"script"`
}

// CIConfiguration generates a CI snippet in advisory mode: a job per builtin
// profile of the matrix and a job that imports the ready reports, each uploading its package. Images go by digest;
// the token is a masked variable of the CI of the client and its value is never in the snippet.
func (u *UseCase) CIConfiguration(ctx context.Context, s entity.Subject, projectID entity.ID, system string) (entity.CIConfiguration, error) {
	switch system {
	case entity.CIGitLab, entity.CIGitHubActions, entity.CIJenkins:
	default:
		return entity.CIConfiguration{}, &entity.ValidationError{Fields: []entity.FieldError{{
			Field: "ci_system", Code: entity.FieldNotAllowedValue, Detail: "expected gitlab_ci, github_actions or jenkins",
		}}}
	}
	m, err := u.matrix(ctx, s.OrganizationID, projectID, false)
	if err != nil {
		return entity.CIConfiguration{}, err
	}
	jobs := map[string]gitlabJob{}
	var imports []string
	vars := []entity.CIVariable{{Name: _varToken, Masked: true}, {Name: _varURL}}
	for _, e := range m.version.Entries {
		p, ok := u.profiles[e.ProfileID]
		if _, live := m.components[e.ComponentID]; !ok || !live {
			continue
		}
		if e.Mode == entity.CheckModeBuiltin {
			jobs["craftpulse-"+p.ID] = gitlabJob{
				Stage: "test", Image: p.Image + "@" + p.Digest, AllowFailure: true, Script: append(slices.Clone(p.Script), "craftpulse-ci upload"),
			}
			continue
		}
		name := _reportVar + strings.ToUpper(e.AnalysisKind)
		report := e.AnalysisKind + ":" + p.ReportFormat + ":${" + name + "}"
		if p.ReportFormat == "" || slices.Contains(imports, report) {
			continue
		}
		imports = append(imports, report)
		if !slices.ContainsFunc(vars, func(v entity.CIVariable) bool { return v.Name == name }) {
			vars = append(vars, entity.CIVariable{Name: name})
		}
	}
	if len(imports) > 0 {
		slices.Sort(imports)
		args := make([]string, 0, 2*len(imports)+2)
		args = append(args, "craftpulse-ci", "import")
		for _, r := range imports {
			if system != entity.CIGitLab {
				r = "\"" + r + "\""
			}
			args = append(args, "--report", r)
		}
		jobs["craftpulse-import"] = gitlabJob{
			Stage: "test", Image: u.d.Catalog.StepImage + "@" + u.d.Catalog.StepDigest, AllowFailure: true,
			Script: []string{strings.Join(args, " "), "craftpulse-ci upload"},
		}
	}
	var raw []byte
	switch system {
	case entity.CIGitHubActions:
		raw, err = actionsSnippet(jobs, vars)
	case entity.CIJenkins:
		raw = []byte(jenkinsSnippet(jobs))
	default:
		raw, err = yaml.Marshal(jobs)
	}
	if err != nil {
		return entity.CIConfiguration{}, errorf("encode the snippet: %w", err)
	}
	slices.SortStableFunc(vars[2:], func(a, b entity.CIVariable) int { return strings.Compare(a.Name, b.Name) })
	return entity.CIConfiguration{System: system, VersionNo: m.version.VersionNo, Snippet: string(raw), Variables: vars}, nil
}
