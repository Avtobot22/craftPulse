package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	docsrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/docs"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/docs"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestCalcDocumentation_ProfileReceiptsPublishCommitRevisions(t *testing.T) {
	// The link checker and package are real. Spectral replays the two results of its pinned rule fixture.
	pinSpectralFixture(t)
	e, docModule, scope := docsCalcEnv(t)
	checkout := documentationCheckout(t)
	profileDir, err := filepath.Abs(filepath.Join("..", "ci", "profiles", "git-markdown-openapi"))
	require.NoError(t, err)
	profile, err := ci.LoadDocsProfile(profileDir)
	require.NoError(t, err)

	var firstSeen time.Time
	var earliestVersion float64
	var firstAge int64
	var firstID entity.ID
	for i, stage := range []string{"broken", "date changed", "anchor fixed"} {
		t.Run(stage, func(t *testing.T) {
			scope.SHA = strings.Repeat(strconv.Itoa(i+1), 40)
			if i == 1 {
				updateDocFixture(t, checkout, "README.md", "owner: team-lead\n", "owner: team-lead\nupdated: 2026-11-20\n")
			}
			if i == 2 {
				updateDocFixture(t, checkout, "guide.md", "# Guide\n", "# Guide\n\n## Absent\n")
			}
			e.clock.Advance(24 * time.Hour)
			data := docsProfilePackage(t, profile, e.clock.Now().Add(-time.Hour), scope, checkout, i)
			rc := e.imported(t, e.admin, data)
			require.Equal(t, entity.ReceiptAccepted, rc.Status)
			e.drain(t)
			e.build(t)
			series, err := e.calc.DocumentationSeries(t.Context(), e.orgID, scope)
			require.NoError(t, err)
			rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
			require.NoError(t, err)
			require.Equal(t, entity.RevisionPublished, rev.Status)
			if i == 0 {
				firstID = rev.ID
			} else {
				assert.NotEqual(t, firstID, rev.ID)
			}
			checkDocsMetrics(t, e, rev, i)
			analysis, err := e.calc.AnalysisSeries(t.Context(), e.orgID, entity.AnalysisScope{RepositoryID: scope.RepositoryID, SHA: scope.SHA})
			require.NoError(t, err)
			analysisRev, err := e.calc.Revision(t.Context(), e.orgID, analysis.ID, nil)
			require.NoError(t, err)
			if i == 0 {
				at := *e.report(t, rc.ID, "docs").FinishedAt
				earliestVersion = float64(at.Unix())
				visible := e.areaScope(t, e.admin, e.area)
				events, err := e.norm.TimelineFacts(t.Context(), visible, e.orgID, []entity.ID{e.area}, entity.TimelineFactQuery{
					Kind: entity.ObjectDocument, Knowledge: rev.KnowledgeSeq, From: at.Add(-time.Minute), To: at.Add(time.Minute),
				})
				require.NoError(t, err)
				require.Len(t, events, 7)
				for j := range events {
					event := &events[j]
					assert.Equal(t, "version_created", event.Kind)
					assert.Equal(t, scope.SHA, event.Value)
					assert.Equal(t, entity.ViaImport, event.ObservedVia)
					assert.True(t, event.ObservedAt.After(event.EventTime))
				}
			}
			codeValue(t, e, analysisRev, "D27", "", 1, entity.StateAssessed)
			codeValue(t, e, analysisRev, "D27", "earliest_version", earliestVersion, entity.StateAssessed)
			finding := docsAnchorAt(t, e, rev)
			if i == 2 {
				assert.Nil(t, finding)
			} else {
				require.NotNil(t, finding)
				assert.Equal(t, "docs/guide.md", finding.Evidence.Link.TargetPath)
				assert.Equal(t, "absent", finding.Evidence.Link.Anchor)
				assert.Equal(t, scope.SHA, finding.VersionRef)
				assert.NotEmpty(t, finding.VersionID)
				if i == 0 {
					firstSeen, firstAge = finding.FirstSeenAt, finding.AgeSeconds
				} else {
					assert.Equal(t, firstSeen, finding.FirstSeenAt)
					assert.Greater(t, finding.AgeSeconds, firstAge)
				}
			}
			checkDocsInventory(t, docModule, e, scope, rev.KnowledgeSeq)
			assert.Equal(t, (i+1)*7, e.n(t, "SELECT count(*) FROM normalize.document_version"))
			assert.Equal(t, 7, e.n(t, "SELECT count(DISTINCT document_id) FROM normalize.document_version"))
			e.renormalize(t, rc.ID)
			e.drain(t)
			assert.Equal(t, (i+1)*7, e.n(t, "SELECT count(*) FROM normalize.document_version"))
			assert.Equal(t, (i+1)*2, e.n(t, "SELECT count(*) FROM normalize.docs_report"))
		})
	}

	scope.ServiceID, scope.RootID = newDocumentationService(t, e, "No docs"), entity.ID{}
	series, err := e.calc.DocumentationSeries(t.Context(), e.orgID, scope)
	require.NoError(t, err)
	now := e.clock.Now()
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, series.ID,
		entity.Window{From: now.Add(-time.Hour), To: now, Step: entity.StepDay}, now))
	e.build(t)
	rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	points, err := e.calc.RevisionPoints(t.Context(), e.orgID, rev.ID)
	require.NoError(t, err)
	require.Len(t, points, 9)
	for _, point := range points {
		assert.Equal(t, entity.StateUnknown, point.State)
		assert.Equal(t, "documentation_root_missing", point.ReasonCode)
		assert.Nil(t, point.Value)
	}
}

func docsCalcEnv(t *testing.T) (*reviewEnv, *docs.UseCase, entity.DocumentationScope) {
	t.Helper()
	e := &reviewEnv{normalizeEnv: newNormalizeEnv(t)}
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.norm}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	seed := e.imported(t, e.admin, reportPackage(t, _reportSHA, sarifSpecs()[0]))
	e.drain(t)
	e.area = mrArea(t, e.normalizeEnv, seed, "42")
	service := newDocumentationService(t, e, "Git docs")
	roles, err := orgs.UserRoles(t.Context(), e.admin, e.admin.UserID)
	require.NoError(t, err)
	_, err = orgs.ReplaceUserRoles(t.Context(), e.admin, e.admin.UserID, roles.Version,
		append(roles.Roles, entity.RoleAssignment{Role: entity.RoleServiceOwner, ServiceID: service}))
	require.NoError(t, err)
	tx := persistent.NewTransactor(e.app)
	docModule := docs.New(docs.Deps{
		Repo: docsrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Access: e.rights,
		References: orgs, Areas: e.norm, Documents: e.norm,
		Auditor: audit.New(auditrepo.NewRepo(e.app), tx, e.clock), Clock: e.clock, NewID: newID,
	})
	set, err := docModule.ReplaceRoots(t.Context(), e.admin, service, 0, entity.ID(uuid.New()), []entity.DocumentationRoot{{
		AreaID: e.area, PathPrefix: "docs", ProfileID: "git-markdown-openapi", Audience: "organization",
	}})
	require.NoError(t, err)
	require.Len(t, set.Roots, 1)
	return e, docModule, entity.DocumentationScope{ServiceID: service, RepositoryID: e.area, RootID: set.Roots[0].ID}
}

func newDocumentationService(t *testing.T, e *reviewEnv, name string) entity.ID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(),
		"INSERT INTO org.service (organization_id, name, status, created_at) VALUES ($1, $2, 'active', $3) RETURNING id",
		uuid.UUID(e.orgID), name, e.clock.Now()).Scan(&id))
	return entity.ID(id)
}

func pinSpectralFixture(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	const script = `#!/bin/sh
while [ "$1" != "--output" ] && [ "$#" -gt 1 ]; do shift; done
shift
cat > "$1" <<'JSON'
[
  {"code":"oas3-schema","severity":0,"source":"docs/invalid.openapi.yaml","range":{"start":{"line":21,"character":0}}},
  {"code":"operation-description","severity":1,"source":"docs/rules.openapi.yaml","range":{"start":{"line":15,"character":0}}}
]
JSON
exit 1
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "spectral"), []byte(script), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func documentationCheckout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, kind := range []string{"links", "openapi"} {
		source := filepath.Join("..", "ci", "profiles", "git-markdown-openapi", "fixtures", kind, "src")
		require.NoError(t, os.CopyFS(filepath.Join(root, "docs"), os.DirFS(source)))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# Links\n"), 0o600))
	return root
}

func updateDocFixture(t *testing.T, root, file, old, updated string) {
	t.Helper()
	name := filepath.Join(root, "docs", file)
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Contains(t, string(data), old)
	require.NoError(t, os.WriteFile(name, []byte(strings.Replace(string(data), old, updated, 1)), 0o600))
}

func docsProfilePackage(t *testing.T, profile *ci.DocsProfile, at time.Time, scope entity.DocumentationScope, root string, index int) []byte {
	t.Helper()
	job := &ci.Job{
		InstanceKey: _gitlabInstance, ProjectID: "42", ProjectPath: "demo/docs",
		JobID: strconv.Itoa(8100 + index), PipelineID: strconv.Itoa(8000 + index), SHA: scope.SHA, RefName: "main",
	}
	store := ci.NewStore(t.TempDir())
	now := func() time.Time { return at }
	runner := &ci.DocsRunner{
		Runner: &ci.Runner{Job: job, Store: store, Stdout: io.Discard, Stderr: io.Discard, Now: now}, Docs: profile, Root: root,
	}
	component := uuid.New()
	entries := make([]ci.Entry, 0, 2)
	roots := make([]ci.DocsRoot, 0, 2)
	for _, kind := range []string{entity.AnalysisDocs, entity.AnalysisOpenAPI} {
		entries = append(entries, ci.Entry{
			ComponentID: component, Kind: kind, Mode: "builtin", ProfileID: profile.ID, ImageDigest: new(_imageHash),
		})
		roots = append(roots, ci.DocsRoot{ID: uuid.UUID(scope.RootID), Kind: kind, PathPrefix: "docs"})
	}
	runs, err := runner.Run(t.Context(), entries, roots)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	for i := range runs {
		require.Len(t, runs[i].Reports, 2)
	}
	pack := &ci.Packer{Job: job, Store: store, Version: "test", Now: now}
	pkg, err := pack.Build(envelope.DefaultLimits)
	require.NoError(t, err)
	return pkg.Data
}

func checkDocsMetrics(t *testing.T, e *reviewEnv, rev entity.CalcRevision, stage int) {
	t.Helper()
	codeValue(t, e, rev, "D01", "roots", 1, entity.StateAssessed)
	codeValue(t, e, rev, "D01", "documents", 7, entity.StateAssessed)
	codeValue(t, e, rev, "D01", "coverage", 1, entity.StateAssessed)
	p := codeValue(t, e, rev, "D03", "root_owner", 0, entity.StateAssessed)
	assert.Equal(t, "documentation_root_owner_unassigned", p.ReasonCode)
	codeValue(t, e, rev, "D03", "document_owners", 0.5, entity.StateAssessed)
	broken := 4
	if stage == 2 {
		broken--
	}
	link := codeValue(t, e, rev, "D10", "", float64(broken)/14, entity.StateAssessed)
	assert.InDelta(t, 14, *link.Denominator, 0)
	codeCohort(t, e, rev.ID, link, "docs/missing.md", broken+1)
	spec := codeValue(t, e, rev, "D24", "", 2.0/3, entity.StateAssessed)
	assert.InDelta(t, 3, *spec.Denominator, 0)
	codeCohort(t, e, rev.ID, spec, "invalid.openapi.yaml", 3)
	codeValue(t, e, rev, "D24", "spectral_findings", 2, entity.StateAssessed)
	codeValue(t, e, rev, "D30", "", float64(broken+2), entity.StateAssessed)
}

type docAnchorEvidence struct {
	VersionRef  string    `json:"version_ref"`
	VersionID   entity.ID `json:"document_version_id"`
	Rule        string    `json:"rule"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	AgeSeconds  int64     `json:"age_seconds"`
	Evidence    struct {
		Link envelope.DocsLinkTarget `json:"link"`
	} `json:"evidence"`
}

func docsAnchorAt(t *testing.T, e *reviewEnv, rev entity.CalcRevision) *docAnchorEvidence {
	t.Helper()
	page, err := e.calc.PointContributions(t.Context(), e.orgID, rev.ID, entity.ContributionQuery{MetricID: "D30", PointStart: rev.Window.From})
	require.NoError(t, err)
	for _, item := range page.Items {
		var f docAnchorEvidence
		require.NoError(t, json.Unmarshal(item.Evidence, &f))
		if f.Rule == entity.DocsRuleMissingAnchor {
			return &f
		}
	}
	return nil
}

func checkDocsInventory(t *testing.T, module *docs.UseCase, e *reviewEnv, scope entity.DocumentationScope, seq int64) {
	t.Helper()
	access, err := e.rights.Authorize(t.Context(), e.admin, entity.AccessRequest{Action: "org.read"})
	require.NoError(t, err)
	access, err = e.rights.Decide(t.Context(), e.admin, access, entity.SourceQuery{Areas: []entity.ID{e.area}})
	require.NoError(t, err)
	out, err := module.InventoryAt(t.Context(), e.orgID, access, scope, seq)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Nil(t, out.Root.OwnerPersonID)
	require.Len(t, out.Documents, 7)
	for i := range out.Documents {
		d := &out.Documents[i]
		assert.Equal(t, scope.SHA, d.VersionRef)
		assert.Contains(t, d.Path, "docs/")
		assert.Equal(t, "known", d.ACLState)
		assert.Len(t, d.ContentHash, 32)
		if d.Path == "docs/README.md" {
			assert.Equal(t, "team-lead", d.Owner)
			assert.Equal(t, "Service documentation", d.Title)
		}
	}
	_, err = module.InventoryAt(t.Context(), e.orgID, entity.Scope{}, scope, seq)
	require.ErrorIs(t, err, entity.ErrNotFound)
}
