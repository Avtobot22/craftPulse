package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var (
	_reportSHA  = strings.Repeat("d", 40)
	_rulesHash  = strings.Repeat("3", 64)
	_configHash = strings.Repeat("4", 64)
	_scopeHash  = strings.Repeat("5", 64)
	_imageHash  = "sha256:" + strings.Repeat("6", 64)
)

// reportSpec is one tool run of a scan manifest with its report file; content replaces the fixture and name the
// file name of the package when set.
type reportSpec struct {
	fixture, kind, format, producer, version string
	name, sourceRoot, sourceDir              string
	content                                  []byte
	ci                                       *envelope.ScanCiCoordinates
	component                                *envelope.ScanComponent
}

func reportFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "usecase", "normalize", "testdata", "reports", name))
	require.NoError(t, err)
	return data
}

func gitlabCI(run, attempt string) *envelope.ScanCiCoordinates {
	return &envelope.ScanCiCoordinates{
		Provider: envelope.ScanCiCoordinatesProviderGitlab, ProjectNativeId: "42", RunNativeId: run, JobNativeId: &attempt,
		AttemptNativeId: &attempt, AttemptNo: ptr(1), RefName: "demo/PA-1-change",
	}
}

// reportPackage is a package of the CI step: the files, their scan manifest and a report_file record of each.
func reportPackage(t *testing.T, sha string, specs ...reportSpec) []byte {
	t.Helper()
	m := manifestOf(envelope.ManifestSchemaIdFindings, envelope.SourceProviderGitlab, _gitlabInstance, "42")
	m.Area.Kind = envelope.AreaRefKindRepository
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, m)
	require.NoError(t, err)
	scan := &envelope.ScanManifestV1{SchemaVersion: envelope.ScanManifestV1SchemaVersionN1}
	started := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	for i := range specs {
		s := &specs[i]
		content := s.content
		if content == nil {
			content = reportFixture(t, s.fixture)
		}
		name := s.name
		if name == "" {
			name = s.fixture
		}
		path := "files/reports/" + s.kind + "-" + name
		ref, err := w.AddFile(path, "application/json", content)
		require.NoError(t, err)
		tool := envelope.ScanToolRun{
			Id: s.kind + "-" + name, Producer: s.producer, ProducerVersion: s.version, Mode: envelope.ScanToolRunModeBuiltin,
			ProfileId: ptr("go-default"), ProfileImageDigest: ptr(_imageHash), ReportKind: envelope.ScanToolRunReportKind(s.kind),
			Component: s.component, Language: ptr("go"), RulesHash: ptr(_rulesHash), ConfigHash: ptr(_configHash), DbVersion: ptr("2026-11-16"),
			DbPublishedAt: ptr(started.Add(-24 * time.Hour)), ScopeHash: _scopeHash, Sha: &sha, Ci: s.ci,
			StartedAt: started.Add(time.Duration(i) * time.Minute), FinishedAt: started.Add(time.Duration(i)*time.Minute + 10*time.Second),
			ExitClass: envelope.ScanToolRunExitClassFindings, Completeness: envelope.ScanToolRunCompletenessPartial,
			ScannedCount: ptr(12), SkippedCount: ptr(1),
			SkippedTargets: []envelope.ScanSkippedTarget{{Path: "app/generated.go", Reason: envelope.ScanSkippedTargetReasonUnsupported}},
			History:        &envelope.ScanHistory{FromSha: strings.Repeat("b", 40), ToSha: sha, Truncated: true},
			SourceRoot:     nonEmpty(s.sourceRoot), SourceDir: nonEmpty(s.sourceDir),
			Reports: []envelope.ScanReport{{
				File: ref, ReportKind: envelope.ScanReportReportKind(s.kind), ReportFormat: envelope.ScanReportReportFormat(s.format), FormatVersion: "1",
			}},
		}
		scan.Tools = append(scan.Tools, tool)
		payload := envelope.ReportFileV1{
			ReportKind: envelope.ReportFileV1ReportKind(s.kind), ReportFormat: envelope.ReportFileV1ReportFormat(s.format), Sha: &sha,
			Producer: s.producer, ProducerVersion: s.version,
		}
		ns := sha
		if s.ci != nil {
			payload.RunNativeId, payload.AttemptNativeId, ns = &s.ci.RunNativeId, s.ci.AttemptNativeId, *s.ci.AttemptNativeId
		}
		rec := mrRecord(t, "report_file", s.kind+":"+path, ns, ref.Sha256, payload)
		rec.RecordClass, rec.File = envelope.RecordRecordClassFile, &ref
		require.NoError(t, w.AddRecord(rec))
	}
	require.NoError(t, w.AddScanManifest(scan))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

type reportRow struct {
	Kind, Mode, Format, SchemaVersion, ParserVersion, ExitClass, Completeness string
	Producer, ProducerVersion, ProfileID, DBVersion, SHA                      *string
	RulesHash, ConfigHash, ScopeHash                                          []byte
	DBPublishedAt, StartedAt, FinishedAt                                      *time.Time
	ScannedCount, SkippedCount                                                *int
	SkippedTargets                                                            []byte
	HistoryFrom, HistoryTo                                                    *string
	HistoryTruncated                                                          *bool
	RunID, AttemptID, ComponentID                                             *uuid.UUID
	AreaID                                                                    uuid.UUID
}

func (e *normalizeEnv) report(t *testing.T, receipt entity.ID, kind string) reportRow {
	t.Helper()
	var r reportRow
	err := e.owner.QueryRow(t.Context(), `SELECT report_kind, mode, report_format, schema_version, parser_version, exit_class, completeness,
		producer, producer_version, profile_id, db_version, sha, rules_hash, config_hash, scope_hash, db_published_at, started_at, finished_at,
		scanned_count, skipped_count, skipped_targets, history_from_sha, history_to_sha, history_truncated, run_id, attempt_id, component_id, area_id
		FROM normalize.report WHERE receipt_id = $1 AND report_kind = $2`, uuid.UUID(receipt), kind).Scan(
		&r.Kind, &r.Mode, &r.Format, &r.SchemaVersion, &r.ParserVersion, &r.ExitClass, &r.Completeness, &r.Producer, &r.ProducerVersion,
		&r.ProfileID, &r.DBVersion, &r.SHA, &r.RulesHash, &r.ConfigHash, &r.ScopeHash, &r.DBPublishedAt, &r.StartedAt, &r.FinishedAt,
		&r.ScannedCount, &r.SkippedCount, &r.SkippedTargets, &r.HistoryFrom, &r.HistoryTo, &r.HistoryTruncated, &r.RunID, &r.AttemptID,
		&r.ComponentID, &r.AreaID)
	require.NoError(t, err)
	return r
}

type findingRow struct {
	rule, kind, path string
	start            int
	severity         string
	confidence       *string
	evidence         string
}

func (e *normalizeEnv) findings(t *testing.T, receipt entity.ID, kind string) []findingRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT f.rule_id, f.finding_kind, COALESCE(f.path, ''), COALESCE(f.line_start, 0),
		COALESCE(f.native_severity, ''), f.native_confidence, COALESCE(f.evidence::text, '')
		FROM normalize.finding_occurrence f JOIN normalize.report r ON r.organization_id = f.organization_id AND r.id = f.report_id
		WHERE r.receipt_id = $1 AND r.report_kind = $2 ORDER BY f.path, f.line_start, f.rule_id`, uuid.UUID(receipt), kind)
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (findingRow, error) {
		var f findingRow
		return f, row.Scan(&f.rule, &f.kind, &f.path, &f.start, &f.severity, &f.confidence, &f.evidence)
	})
	require.NoError(t, err)
	return out
}

func objectID(t *testing.T, e *normalizeEnv, kind, native string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind = $1 AND native_id = $2", kind, native).Scan(&id))
	return id
}

func sarifSpecs() []reportSpec {
	ci := gitlabCI("5001", "7002")
	return []reportSpec{
		{fixture: "golangci.sarif", kind: "code", format: "sarif", producer: "golangci-lint", version: "2.15.0", ci: ci},
		{fixture: "opengrep.sarif", kind: "sast", format: "sarif", producer: "opengrep", version: "1.33.1", ci: ci},
		{fixture: "gitleaks.sarif", kind: "secrets", format: "sarif", producer: "gitleaks", version: "8.30.1", ci: ci},
	}
}

func TestNormalizeReport_SARIFWithProvenanceAndRepeats(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	data := reportPackage(t, _reportSHA, sarifSpecs()...)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)

	area := mrArea(t, e, rc, "42")
	run, attempt := objectID(t, e, "ci_run", "5001"), objectID(t, e, "ci_attempt", "7002")
	for _, s := range sarifSpecs() {
		r := e.report(t, rc.ID, s.kind)
		assert.Equal(t, "builtin", r.Mode)
		assert.Equal(t, "sarif", r.Format)
		assert.Equal(t, "2.1.0", r.SchemaVersion, "the version the file declares")
		assert.Equal(t, "sarif-2.1.0/1", r.ParserVersion)
		assert.Equal(t, s.producer, *r.Producer)
		assert.Equal(t, s.version, *r.ProducerVersion)
		assert.Equal(t, "go-default", *r.ProfileID)
		assert.Equal(t, "2026-11-16", *r.DBVersion)
		assert.Equal(t, _reportSHA, *r.SHA)
		assert.Equal(t, bytes.Repeat([]byte{0x33}, 32), r.RulesHash)
		assert.Equal(t, bytes.Repeat([]byte{0x44}, 32), r.ConfigHash)
		assert.Equal(t, bytes.Repeat([]byte{0x55}, 32), r.ScopeHash)
		require.NotNil(t, r.DBPublishedAt)
		require.NotNil(t, r.StartedAt)
		assert.Equal(t, 10*time.Second, r.FinishedAt.Sub(*r.StartedAt))
		assert.Equal(t, "findings", r.ExitClass)
		assert.Equal(t, "partial", r.Completeness)
		assert.Equal(t, 12, *r.ScannedCount)
		assert.Equal(t, 1, *r.SkippedCount)
		assert.JSONEq(t, `[{"path":"app/generated.go","reason":"unsupported"}]`, string(r.SkippedTargets))
		assert.Equal(t, strings.Repeat("b", 40), *r.HistoryFrom)
		assert.Equal(t, _reportSHA, *r.HistoryTo)
		assert.True(t, *r.HistoryTruncated)
		assert.Equal(t, run, *r.RunID)
		assert.Equal(t, attempt, *r.AttemptID)
		assert.Equal(t, uuid.UUID(area), r.AreaID)
		assert.Nil(t, r.ComponentID)
	}
	code := e.findings(t, rc.ID, "code")
	require.Len(t, code, 4)
	assert.Equal(t, findingRow{rule: "errcheck", kind: "code", path: "app/main.go", start: 14, severity: "error", evidence: `{"tool": {"name": "golangci-lint"}}`}, code[1])
	assert.Equal(t, "gosec", code[0].rule)
	sast := e.findings(t, rc.ID, "sast")
	require.Len(t, sast, 1)
	assert.Equal(t, []any{"rules.go-sql-format-string", "security", "app/main.go", 13, "error"}, []any{sast[0].rule, sast[0].kind, sast[0].path, sast[0].start, sast[0].severity})
	assert.Contains(t, sast[0].evidence, `"cwe": ["CWE-89"]`)
	assert.Contains(t, sast[0].evidence, `"security_severity": "8.8"`)
	secrets := e.findings(t, rc.ID, "secrets")
	require.Len(t, secrets, 1)
	assert.Equal(t, []any{"aws-access-token", "secret", "config.env", 3}, []any{secrets[0].rule, secrets[0].kind, secrets[0].path, secrets[0].start})
	for _, f := range append(append(code, sast...), secrets...) {
		assert.Nil(t, f.confidence, "the tools name no confidence, so it stays unknown")
		for _, banned := range []string{"demo-secret-snippet", "SELECT * FROM users", "has detected secret"} {
			assert.NotContains(t, f.evidence, banned, "evidence keeps no code and no secret")
		}
	}
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.area_change WHERE area_id = $1 AND 'report' = ANY (fact_kinds)", uuid.UUID(area)))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM river_job WHERE kind = $1 AND args->>'area_id' = $2", entity.KindCalcRecount, area.String()))

	for range 2 {
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
	e.drain(t)
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.report"), "normalizing the package again adds no report")
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.finding_occurrence"))

	retry := sarifSpecs()[1]
	retry.ci = gitlabCI("5001", "7005")
	again := e.imported(t, e.admin, reportPackage(t, _reportSHA, retry))
	require.Equal(t, entity.ReceiptAccepted, again.Status)
	e.drain(t)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.report WHERE report_kind = 'sast'"), "the report of a retried job is a new row")
	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.finding_occurrence"))
}

func TestNormalizeReport_BrokenOrUnsupportedReportKeepsTheFormerOne(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	good := sarifSpecs()[1]
	first := e.imported(t, e.admin, reportPackage(t, _reportSHA, good))
	e.drain(t)
	opengrep := reportFixture(t, "opengrep.sarif")
	v20 := bytes.Replace(opengrep, []byte(`"version":"2.1.0"`), []byte(`"version":"2.0.0"`), 1)
	require.NotEqual(t, opengrep, v20)
	cases := []struct {
		name, reason string
		content      []byte
	}{
		{"truncated", entity.RecordReportInvalid, opengrep[:len(opengrep)/2]},
		{"SARIF 2.0", entity.RecordReportVersionUnsupported, v20},
	}
	for _, tt := range cases {
		spec := good
		spec.content = tt.content
		rc := e.imported(t, e.admin, reportPackage(t, _reportSHA, spec))
		require.Equal(t, entity.ReceiptAccepted, rc.Status, tt.name)
		e.drain(t)
		got := e.receipt(t, rc.ID)
		assert.Equal(t, entity.ReceiptPartial, got.Status, tt.name)
		assert.Equal(t, 0, got.AcceptedCount, tt.name)
		assert.Equal(t, 1, got.RejectedCount, tt.name)
		recs := e.records(t, rc.ID)
		require.Len(t, recs, 1)
		assert.Equal(t, entity.RecordRejected, recs[0].Outcome, tt.name)
		assert.Equal(t, tt.reason, recs[0].ReasonCode, tt.name)
		e.renormalize(t, rc.ID)
		e.drain(t)
		assert.Equal(t, 1, e.receipt(t, rc.ID).RejectedCount, "%s: normalizing again rejects nothing twice", tt.name)
	}
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.report WHERE sha = $1 AND report_kind = 'sast'", _reportSHA))
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.report WHERE receipt_id = $1", uuid.UUID(first.ID)), "the former report stays in force")
	assert.Equal(t, 1, e.n(t, "SELECT count(*) FROM normalize.finding_occurrence"))
}

func TestNormalizeReport_GitLabSecurityKinds(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	ci := gitlabCI("5001", "7003")
	specs := []reportSpec{
		{fixture: "gl-sast-report.json", kind: "sast", format: "gitlab_security", producer: "semgrep", version: "6.12.0", ci: ci},
		{fixture: "gl-secret-detection-report.json", kind: "secrets", format: "gitlab_security", producer: "secrets", version: "6.4.0", ci: ci},
		{fixture: "gl-dependency-scanning-report.json", kind: "sca", format: "gitlab_security", producer: "gemnasium", version: "6.3.1", ci: ci},
	}
	rc := e.imported(t, e.admin, reportPackage(t, _reportSHA, specs...))
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, rc.ID).Status)
	for _, s := range specs {
		r := e.report(t, rc.ID, s.kind)
		assert.Equal(t, "15.2.1", r.SchemaVersion)
		assert.Equal(t, "gitlab-security-15/1", r.ParserVersion)
	}
	sast := e.findings(t, rc.ID, "sast")
	require.Len(t, sast, 2)
	assert.Equal(t, []any{"go_sql_rule-concat-sqli", "security", "app/main.go", 13, "High"}, []any{sast[0].rule, sast[0].kind, sast[0].path, sast[0].start, sast[0].severity})
	secrets := e.findings(t, rc.ID, "secrets")
	require.Len(t, secrets, 1)
	assert.Equal(t, []any{"AWS", "secret", "config.env", 3, "Critical"}, []any{secrets[0].rule, secrets[0].kind, secrets[0].path, secrets[0].start, secrets[0].severity})
	for _, banned := range []string{"demo-secret-extract", "demo-commit-message", "demo-author"} {
		assert.NotContains(t, secrets[0].evidence, banned)
	}
	deps := e.findings(t, rc.ID, "sca")
	require.Len(t, deps, 1)
	assert.Equal(t, []any{"dependency", "go.sum", "Medium"}, []any{deps[0].kind, deps[0].path, deps[0].severity})
	assert.Contains(t, deps[0].evidence, `"package": {"name": "golang.org/x/net", "version": "v0.30.0"}`)
	for _, f := range append(append(sast, secrets...), deps...) {
		assert.Nil(t, f.confidence)
	}

	old := e.imported(t, e.admin, reportPackage(t, _reportSHA, reportSpec{
		fixture: "gl-sast-report-v14.json", kind: "sast", format: "gitlab_security", producer: "semgrep", version: "4.0.0", ci: gitlabCI("5002", "7004"),
	}))
	e.drain(t)
	recs := e.records(t, old.ID)
	require.Len(t, recs, 1)
	assert.Equal(t, entity.RecordReportVersionUnsupported, recs[0].ReasonCode)
	assert.Equal(t, entity.ReceiptPartial, e.receipt(t, old.ID).Status)
}

func TestNormalizeReport_ComponentByPathPrefix(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	component := uuid.Must(uuid.NewV7())
	spec := sarifSpecs()[0]
	spec.component = &envelope.ScanComponent{Id: &component, PathPrefix: "services/api"}
	before := e.imported(t, e.admin, reportPackage(t, _reportSHA, spec))
	e.drain(t)
	assert.Nil(t, e.report(t, before.ID, "code").ComponentID, "a path without a confirmed component")

	area := mrArea(t, e, before, "42")
	var service uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "INSERT INTO org.service (organization_id, name, status, created_at) VALUES ($1, 'api', 'active', now()) RETURNING id",
		uuid.UUID(e.orgID)).Scan(&service))
	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.component (organization_id, id, known_seq, service_id, repository_area_id, path_prefix, status, origin,
		confirmed_by, confirmed_at) VALUES ($1, $2, 1, $3, $4, 'services/api', 'confirmed', 'manual', $5, now())`,
		uuid.UUID(e.orgID), component, service, uuid.UUID(area), uuid.UUID(e.admin.UserID))
	require.NoError(t, err)
	spec.ci = gitlabCI("5002", "7006")
	after := e.imported(t, e.admin, reportPackage(t, strings.Repeat("c", 40), spec))
	e.drain(t)
	got := e.report(t, after.ID, "code").ComponentID
	require.NotNil(t, got)
	assert.Equal(t, component, *got)
}

// gitlabScope is a connection to GitLab that collects the repository native.
func (e *ingestEnv) gitlabScope(t *testing.T, native string) (conn, scope entity.ID) {
	t.Helper()
	inst, err := e.connect.EnsureInstance(t.Context(), e.orgID, entity.ProviderGitLab, _gitlabInstance)
	require.NoError(t, err)
	conn = entity.ID(uuid.Must(uuid.NewV7()))
	r := connectrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		err := r.CreateConnection(ctx, entity.Connection{
			ID: conn, OrganizationID: e.orgID, InstanceID: inst.ID, Name: "GitLab", AdapterKind: entity.AdapterBuiltin,
			ContractVersion: "1", AccessMode: entity.AccessServiceAccount, Status: entity.ConnectionActive, RowVersion: 1,
			CreatedBy: e.admin.UserID, CreatedAt: e.clock.Now(),
		})
		if err != nil {
			return err
		}
		scope, err = r.AddScope(ctx, e.orgID, conn, inst.ID, entity.NewScope{Kind: entity.ScopeRepository, NativeScopeID: native, DisplayName: native}, e.clock.Now())
		return err
	}))
	return conn, scope
}

func TestNormalizeReport_AdapterArtifactWithoutScanManifest(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	conn, scope := e.gitlabScope(t, "42")
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, ciManifest("42"))
	require.NoError(t, err)
	ref, err := w.AddFile("files/artifacts/7002/gl-sast-report.json", "application/json", reportFixture(t, "gl-sast-report.json"))
	require.NoError(t, err)
	sha, run, attempt := _reportSHA, "5001", "7002"
	exit, completeness := envelope.ReportFileV1ExitClassFindings, envelope.ReportFileV1CompletenessComplete
	rec := mrRecord(t, "report_file", "sast:"+ref.Path, attempt, ref.Sha256, envelope.ReportFileV1{
		ReportKind: "sast", ReportFormat: "gitlab_security", Sha: &sha, RunNativeId: &run, AttemptNativeId: &attempt,
		Producer: "gitlab", ProducerVersion: "unknown", ExitClass: &exit, Completeness: &completeness,
	})
	rec.RecordClass, rec.File = envelope.RecordRecordClassFile, &ref
	require.NoError(t, w.AddRecord(ciRun(t, runSpec{id: run, project: "42", sha: sha, status: "success", revision: "p1", updated: _t1})))
	require.NoError(t, w.AddRecord(rec))
	require.NoError(t, w.Close())

	rc := e.page(t, conn, scope, buf.Bytes())
	require.Equal(t, entity.ReceiptAccepted, rc.Status, "a page of the adapter carries its artifacts without a scan manifest")
	e.drain(t)
	r := e.report(t, rc.ID, "sast")
	assert.Nil(t, r.Producer, "the producer of an artifact is unknown")
	assert.Nil(t, r.ProducerVersion)
	assert.Nil(t, r.ScopeHash)
	assert.Nil(t, r.ProfileID)
	assert.Equal(t, "import", r.Mode)
	assert.Equal(t, sha, *r.SHA)
	assert.Equal(t, objectID(t, e, "ci_run", run), *r.RunID)
	assert.Equal(t, objectID(t, e, "ci_attempt", attempt), *r.AttemptID)
	assert.Equal(t, "findings", r.ExitClass)
	assert.Equal(t, "complete", r.Completeness)
	assert.Equal(t, "gitlab-security-15/1", r.ParserVersion)
	assert.Len(t, e.findings(t, rc.ID, "sast"), 2)

	imported := e.imported(t, e.admin, buf.Bytes())
	assert.Equal(t, entity.ReceiptRejected, imported.Status, "outside sync a report file needs its scan manifest")
}
