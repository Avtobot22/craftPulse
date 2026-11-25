package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_jenkinsInstance = "jenkins.demo.invalid"
	_jenkinsSHA      = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"
	_fastConfig      = "819cceaa52adb92b320363b3cad8e884d97a27d14150bbf2d6167880717b9a89"
	_fullConfig      = "1d5d296f9033e53b70a20357e71a05c9ac061c1a2467d18dfebbc4d98bee40b6"
)

var _buildStart = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// sourceScope is a connection that collects one area of a source.
func (e *ingestEnv) sourceScope(t *testing.T, provider entity.Provider, instance string, kind entity.ScopeKind, native string) (conn, scope entity.ID) {
	t.Helper()
	inst, err := e.connect.EnsureInstance(t.Context(), e.orgID, provider, instance)
	require.NoError(t, err)
	conn = entity.ID(uuid.Must(uuid.NewV7()))
	r := connectrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		err := r.CreateConnection(ctx, entity.Connection{
			ID: conn, OrganizationID: e.orgID, InstanceID: inst.ID, Name: string(provider), AdapterKind: entity.AdapterBuiltin,
			ContractVersion: "1", AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionActive, RowVersion: 1,
			CreatedBy: e.admin.UserID, CreatedAt: e.clock.Now(),
		})
		if err != nil {
			return err
		}
		scope, err = r.AddScope(ctx, e.orgID, conn, inst.ID, entity.NewScope{Kind: kind, NativeScopeID: native, DisplayName: native}, e.clock.Now())
		return err
	}))
	return conn, scope
}

func jenkinsManifest(job string, truncated ...int) *envelope.Manifest {
	m := manifestOf(envelope.ManifestSchemaIdCiRuns, envelope.SourceProviderJenkins, _jenkinsInstance, job)
	m.Area.Kind = envelope.AreaRefKindCiProject
	for _, n := range truncated {
		m.Selection.Truncated = append(m.Selection.Truncated, envelope.TruncatedHistory{RecordKind: "ci_run", NativeId: strconv.Itoa(n)})
	}
	return m
}

type buildSpec struct {
	job, result, sha string
	number           int
	running          bool
}

func jenkinsBuild(t *testing.T, b buildSpec) *envelope.Record {
	t.Helper()
	p := map[string]any{
		"project_native_id": b.job, "ref_name": "main", "sha": b.sha, "native_status": b.result, "outcome": "unknown",
		"started_at": _buildStart, "updated_at": _buildStart.Add(time.Minute),
	}
	if !b.running {
		p["finished_at"] = _buildStart.Add(time.Minute)
	}
	return mrRecord(t, "ci_run", strconv.Itoa(b.number), b.job, b.result+"-"+strconv.Itoa(b.number), p)
}

func jenkinsRetry(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "pkg", "envelope", "testdata", "ci", "jenkins-retry", name))
	require.NoError(t, err)
	return data
}

// jenkinsPage is a page of the jenkins adapter: builds of a job and the execution manifests read from their
// artifacts, by build number.
func jenkinsPage(t *testing.T, m *envelope.Manifest, builds []*envelope.Record, manifests map[int][]byte, shas map[int]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	for _, b := range builds {
		require.NoError(t, w.AddRecord(b))
	}
	for number, content := range manifests {
		ref, err := w.AddFile("files/artifacts/"+strconv.Itoa(number)+"/execution-manifest.json", "application/json", content)
		require.NoError(t, err)
		rec := mrRecord(t, "execution_manifest", strconv.Itoa(number), m.Area.NativeId, ref.Sha256,
			envelope.ExecutionManifestFileV1{Job: m.Area.NativeId, BuildNumber: number, Sha: shas[number]})
		rec.RecordClass, rec.File = envelope.RecordRecordClassFile, &ref
		require.NoError(t, w.AddRecord(rec))
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// jenkinsStepPackage is the package of the CI step of build demo #7 with its execution manifest.
func jenkinsStepPackage(t *testing.T, edits ...func(*envelope.Manifest, *envelope.ScanManifestV1)) []byte {
	t.Helper()
	var m envelope.Manifest
	require.NoError(t, json.Unmarshal(jenkinsRetry(t, envelope.ManifestName), &m))
	var scan envelope.ScanManifestV1
	require.NoError(t, json.Unmarshal(jenkinsRetry(t, envelope.ScanManifestName), &scan))
	var execution envelope.JenkinsExecutionManifestV1
	require.NoError(t, json.Unmarshal(jenkinsRetry(t, envelope.ExecutionManifestName), &execution))
	for _, edit := range edits {
		edit(&m, &scan)
	}
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, &m)
	require.NoError(t, err)
	require.NoError(t, w.AddScanManifest(&scan))
	require.NoError(t, w.AddExecutionManifest(&execution))
	root := filepath.Join("..", "..", "pkg", "envelope", "testdata", "ci", "jenkins-retry")
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "files"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, err = w.AddFile(filepath.ToSlash(rel), "application/xml", content)
		return err
	}))
	records := jenkinsRetry(t, envelope.RecordsName)
	require.NoError(t, w.AddRecordsFrom(bytes.NewReader(records), int64(len(records))))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

type attemptRow struct {
	Job, Config, Status, Outcome, Runner string
	No                                   int
	Seconds                              *int64
}

// jenkinsAttempts are the current attempts of a build, in the order of job, configuration and number.
func jenkinsAttempts(t *testing.T, e *normalizeEnv, job string, build int) []attemptRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT a.job_key, encode(a.config_fingerprint, 'hex'), a.native_status, a.outcome,
		COALESCE(a.runner_class, ''), a.attempt_no, a.duration_s
		FROM normalize.ci_attempt_version a JOIN normalize.source_object r ON r.organization_id = a.organization_id AND r.id = a.run_id
		WHERE r.native_ns = $1 AND r.native_id = $2 AND a.superseded_seq IS NULL ORDER BY 1, 2, 6`, job, strconv.Itoa(build))
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (attemptRow, error) {
		var a attemptRow
		return a, row.Scan(&a.Job, &a.Config, &a.Status, &a.Outcome, &a.Runner, &a.No, &a.Seconds)
	})
	require.NoError(t, err)
	return out
}

func buildState(t *testing.T, e *normalizeEnv, job string, build int) (native, outcome, history string, area uuid.UUID) {
	t.Helper()
	require.NoError(t, e.owner.QueryRow(t.Context(), `SELECT v.native_status, v.outcome, v.attempt_history, v.area_id
		FROM normalize.ci_run_version v JOIN normalize.source_object r ON r.organization_id = v.organization_id AND r.id = v.run_id
		WHERE r.native_ns = $1 AND r.native_id = $2 AND v.superseded_seq IS NULL`, job, strconv.Itoa(build)).Scan(&native, &outcome, &history, &area))
	return native, outcome, history, area
}

func jobArea(t *testing.T, e *normalizeEnv, job string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind = 'ci_project' AND native_id = $1", job).Scan(&id))
	return id
}

var _demoAttempts = []attemptRow{
	{Job: "test/test", Config: _fullConfig, Status: "success", Outcome: "success", Runner: "demo-go", No: 1, Seconds: ptr(int64(10))},
	{Job: "test/test", Config: _fastConfig, Status: "failure", Outcome: "failed", Runner: "demo-go", No: 1, Seconds: ptr(int64(10))},
	{Job: "test/test", Config: _fastConfig, Status: "success", Outcome: "success", Runner: "demo-go", No: 2, Seconds: ptr(int64(10))},
}

func sortedAttempts(rows []attemptRow) []attemptRow {
	out := make([]attemptRow, 0, len(rows))
	for _, config := range []string{_fullConfig, _fastConfig} {
		for _, r := range rows {
			if r.Config == config {
				out = append(out, r)
			}
		}
	}
	return append(out, func() []attemptRow {
		var rest []attemptRow
		for _, r := range rows {
			if r.Config != _fullConfig && r.Config != _fastConfig {
				rest = append(rest, r)
			}
		}
		return rest
	}()...)
}

func TestNormalizeJenkins_ManifestFromAdapterAndStep(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	conn, scope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "demo")
	page := jenkinsPage(t, jenkinsManifest("demo"), []*envelope.Record{
		jenkinsBuild(t, buildSpec{job: "demo", result: "SUCCESS", sha: _jenkinsSHA, number: 7}),
	}, map[int][]byte{7: jenkinsRetry(t, envelope.ExecutionManifestName)}, map[int]string{7: _jenkinsSHA})
	rc := e.page(t, conn, scope, page)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)

	assert.Equal(t, _demoAttempts, sortedAttempts(jenkinsAttempts(t, e, "demo", 7)),
		"fail→pass gives attempts 1 and 2 of one step, and the full configuration is another logical check")
	native, outcome, history, area := buildState(t, e, "demo", 7)
	assert.Equal(t, []string{"SUCCESS", "success", "complete"}, []string{native, outcome, history})
	assert.Equal(t, jobArea(t, e, "demo"), area, "a build belongs to the ci_project area of its job")

	step := jenkinsStepPackage(t)
	first := e.imported(t, e.admin, step)
	require.Equal(t, entity.ReceiptAccepted, first.Status)
	e.drain(t)
	for range 3 {
		assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, step).Status)
		e.renormalize(t, first.ID)
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.ci_run_version"))
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"), "the manifest of the adapter and of the step gives the same attempts")
	assert.Equal(t, _demoAttempts, sortedAttempts(jenkinsAttempts(t, e, "demo", 7)))

	scopeOf := e.areaScope(t, e.admin, entity.ID(area))
	run, err := e.norm.RunAt(t.Context(), scopeOf, e.orgID, entity.ID(objectID(t, e, "ci_run", "7")), math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.AttemptHistoryComplete, run.History)
	assert.Len(t, run.Attempts, 3)
}

func TestNormalizeJenkins_StepBeforeTheBuild(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, jenkinsStepPackage(t))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	assert.Len(t, jenkinsAttempts(t, e, "demo", 7), 3)

	conn, scope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "demo")
	e.page(t, conn, scope, jenkinsPage(t, jenkinsManifest("demo", 7), []*envelope.Record{
		jenkinsBuild(t, buildSpec{job: "demo", result: "SUCCESS", sha: _jenkinsSHA, number: 7}),
	}, nil, nil))
	e.drain(t)
	_, _, history, area := buildState(t, e, "demo", 7)
	assert.Equal(t, "complete", history, "the attempts of the step manifest are known")
	assert.Equal(t, jobArea(t, e, "demo"), area)
	assert.Equal(t, _demoAttempts, sortedAttempts(jenkinsAttempts(t, e, "demo", 7)), "no attempt stands for the build")
}

func TestNormalizeJenkins_BuildsWithoutManifestAndForeignSHA(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	conn, scope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "legacy")
	results := []string{"SUCCESS", "UNSTABLE", "ABORTED", "NOT_BUILT", "FAILURE"}
	builds := make([]*envelope.Record, 0, len(results)+2)
	for i, r := range results {
		builds = append(builds, jenkinsBuild(t, buildSpec{job: "legacy", result: r, sha: _jenkinsSHA, number: i + 1}))
	}
	builds = append(builds, jenkinsBuild(t, buildSpec{job: "legacy", result: "", sha: _jenkinsSHA, number: 6, running: true}))
	foreign := bytes.ReplaceAll(jenkinsRetry(t, envelope.ExecutionManifestName), []byte(`"job": "demo"`), []byte(`"job": "legacy"`))
	builds = append(builds, jenkinsBuild(t, buildSpec{job: "legacy", result: "SUCCESS", sha: "e" + _jenkinsSHA[1:], number: 7}))
	rc := e.page(t, conn, scope, jenkinsPage(t, jenkinsManifest("legacy", 1, 2, 3, 4, 5, 6), builds,
		map[int][]byte{7: foreign}, map[int]string{7: _jenkinsSHA}))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)

	want := map[int][3]string{
		1: {"SUCCESS", "success", "truncated"}, 2: {"UNSTABLE", "failed", "truncated"}, 3: {"ABORTED", "canceled", "truncated"},
		4: {"NOT_BUILT", "skipped", "truncated"}, 5: {"FAILURE", "failed", "truncated"}, 6: {"", "running", "truncated"},
		7: {"SUCCESS", "success", "truncated"},
	}
	for build, w := range want {
		native, outcome, history, _ := buildState(t, e, "legacy", build)
		assert.Equal(t, w, [3]string{native, outcome, history}, "build %d", build)
		attempts := jenkinsAttempts(t, e, "legacy", build)
		if build == 6 {
			assert.Empty(t, attempts, "a build in progress has no attempt yet")
			continue
		}
		require.Len(t, attempts, 1, "build %d is one check with one attempt", build)
		assert.Equal(t, attemptRow{Job: "build", Config: attempts[0].Config, Status: w[0], Outcome: w[1], No: 1, Seconds: ptr(int64(60))}, attempts[0])
	}
	recs := e.records(t, rc.ID)
	var rejected []entity.IngestRecord
	for _, r := range recs {
		if r.Outcome == entity.RecordRejected {
			rejected = append(rejected, r)
		}
	}
	require.Len(t, rejected, 1)
	assert.Equal(t, "execution_manifest", rejected[0].Kind)
	assert.Equal(t, entity.RecordManifestMismatch, rejected[0].ReasonCode, "the manifest of another SHA")
	assert.Equal(t, entity.ReceiptPartial, e.receipt(t, rc.ID).Status)

	e.renormalize(t, rc.ID)
	e.drain(t)
	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.ci_run_version"))
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"))
}

func TestNormalizeJenkins_LaterManifestReplacesTheBuildAttempt(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	conn, scope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "demo")
	build := []*envelope.Record{jenkinsBuild(t, buildSpec{job: "demo", result: "SUCCESS", sha: _jenkinsSHA, number: 7})}
	e.page(t, conn, scope, jenkinsPage(t, jenkinsManifest("demo", 7), build, nil, nil))
	e.drain(t)
	require.Len(t, jenkinsAttempts(t, e, "demo", 7), 1)

	e.imported(t, e.admin, jenkinsStepPackage(t))
	e.drain(t)
	assert.Equal(t, _demoAttempts, sortedAttempts(jenkinsAttempts(t, e, "demo", 7)), "the steps replace the attempt of the build")
}

// jenkinsCalc builds the CI series of an area and reads its revision.
func jenkinsCalc(t *testing.T, e *reviewEnv, scope entity.ReviewScope) entity.ID {
	t.Helper()
	series, err := e.calc.CISeries(t.Context(), e.orgID, scope, entity.StepWeek)
	require.NoError(t, err)
	w := entity.Window{From: _buildStart.Add(-24 * time.Hour), To: _buildStart.AddDate(0, 0, 14), Step: entity.StepWeek}
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, series.ID, w, w.To))
	e.build(t)
	rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	return rev.ID
}

func TestCalcCI_JenkinsJobSeries(t *testing.T) {
	t.Parallel()
	e := &reviewEnv{normalizeEnv: newNormalizeEnv(t)}
	conn, scope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "demo")
	e.page(t, conn, scope, jenkinsPage(t, jenkinsManifest("demo"), []*envelope.Record{
		jenkinsBuild(t, buildSpec{job: "demo", result: "SUCCESS", sha: _jenkinsSHA, number: 7}),
	}, map[int][]byte{7: jenkinsRetry(t, envelope.ExecutionManifestName)}, map[int]string{7: _jenkinsSHA}))
	results := []string{"SUCCESS", "UNSTABLE", "ABORTED", "NOT_BUILT", "FAILURE"}
	builds := make([]*envelope.Record, 0, len(results))
	for i, r := range results {
		builds = append(builds, jenkinsBuild(t, buildSpec{job: "legacy", result: r, sha: _jenkinsSHA, number: i + 1}))
	}
	legacyConn, legacyScope := e.sourceScope(t, entity.ProviderJenkins, _jenkinsInstance, entity.ScopeCIProject, "legacy")
	e.page(t, legacyConn, legacyScope, jenkinsPage(t, jenkinsManifest("legacy", 1, 2, 3, 4, 5), builds, nil, nil))
	e.drain(t)
	orgs, _, err := newOrgAccess(e.app, e.clock, e.connect, nil)
	require.NoError(t, err)
	collection, err := newSync(e.app, e.keyring, e.connect, e.uc, e.norm, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.norm, collection: collection}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)

	demo := jenkinsCalc(t, e, entity.ReviewScope{RepositoryID: entity.ID(jobArea(t, e.normalizeEnv, "demo"))})
	exact := map[[2]string]float64{
		{"I01", "runs"}: 1, {"I01", "jobs"}: 1, {"I02", ""}: .5, {"I03", ""}: .5,
		{"I04", "attempts"}: 1, {"I04", "seconds"}: 10, {"I04", "total_seconds"}: 30, {"I07", "unknown"}: 1,
		{"I06", "jobs:success"}: 1,
	}
	for k, want := range exact {
		pt := e.point(t, demo, k[0], k[1], _buildStart)
		require.NotNil(t, pt.Value, "%v", k)
		assert.InDelta(t, want, *pt.Value, 1e-9, "%v", k)
	}
	assert.Nil(t, e.point(t, demo, "I05", "", _buildStart).Value, "Jenkins has no allow_failure")
	loaded := e.point(t, demo, "I10", "loaded", _buildStart)
	assert.Equal(t, &entity.Bound{Kind: entity.BoundLower, Value: 1}, loaded.Bound)

	legacy := jenkinsCalc(t, e, entity.ReviewScope{RepositoryID: entity.ID(jobArea(t, e.normalizeEnv, "legacy"))})
	final := e.point(t, legacy, "I01", "runs", _buildStart)
	require.NotNil(t, final.Value)
	assert.InDelta(t, 1.0/3, *final.Value, 1e-9)
	assert.InDelta(t, 3, *final.Denominator, 0, "ABORTED and NOT_BUILT are not in the denominator")
	unstable := e.point(t, legacy, "I06", "runs:unstable", _buildStart)
	require.NotNil(t, unstable.Numerator)
	assert.InDelta(t, 1, *unstable.Numerator, 0, "UNSTABLE is its own class")
	assert.Equal(t, entity.StateBounded, e.point(t, legacy, "I04", "seconds", _buildStart).State)
	for _, metric := range []string{"I02", "I03"} {
		assert.Nil(t, e.point(t, legacy, metric, "", _buildStart).Value, metric)
	}
	assert.Equal(t, entity.StateUnknown, e.point(t, legacy, "I10", "", _buildStart).State)

	st, err := orgs.Structure(t.Context(), e.admin)
	require.NoError(t, err)
	in := entity.StructureInput{Teams: []entity.AreaInput{{Name: "Platform", Status: "active"}}}
	for _, p := range st.Projects {
		in.Projects = append(in.Projects, entity.AreaInput(p))
	}
	st, err = orgs.ReplaceStructure(t.Context(), e.admin, st.Version, in)
	require.NoError(t, err)
	team := st.Teams[0].ID
	in = entity.StructureInput{Teams: []entity.AreaInput{{ID: team, Name: "Platform", Status: "active"}}}
	for _, p := range st.Projects {
		in.Projects = append(in.Projects, entity.AreaInput(p))
		in.TeamProjects = append(in.TeamProjects, entity.LinkInput{From: team, To: p.ID})
	}
	_, err = orgs.ReplaceStructure(t.Context(), e.admin, st.Version, in)
	require.NoError(t, err)
	for _, job := range []string{"demo", "legacy"} {
		_, err := e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
			VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, e.orgID, st.Projects[0].ID, jobArea(t, e.normalizeEnv, job), e.clock.Now())
		require.NoError(t, err)
	}
	byTeam := jenkinsCalc(t, e, entity.ReviewScope{TeamID: team})
	teamFinal := e.point(t, byTeam, "I01", "runs", _buildStart)
	require.NotNil(t, teamFinal.Value)
	assert.InDelta(t, 2.0/4, *teamFinal.Value, 1e-9, "the team counts the builds of both jobs of its project")
	assert.InDelta(t, 4, *teamFinal.Denominator, 0)
}
