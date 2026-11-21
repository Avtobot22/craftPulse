package app

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type executionRow struct {
	key, outcome, fingerprint string
	ordinal                   int
	attempt                   *uuid.UUID
	ms                        *int64
}

func (e *normalizeEnv) executions(t *testing.T, receipt entity.ID) []executionRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT c.case_key, x.outcome, encode(x.config_fingerprint, 'hex'), x.ordinal, x.attempt_id, x.duration_ms
		FROM normalize.test_execution x
		JOIN normalize.test_case c ON c.organization_id = x.organization_id AND c.id = x.test_case_id
		JOIN normalize.report r ON r.organization_id = x.organization_id AND r.id = x.report_id
		WHERE r.receipt_id = $1 ORDER BY c.case_key, encode(x.config_fingerprint, 'hex'), x.ordinal`, uuid.UUID(receipt))
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (executionRow, error) {
		var x executionRow
		return x, row.Scan(&x.key, &x.outcome, &x.fingerprint, &x.ordinal, &x.attempt, &x.ms)
	})
	require.NoError(t, err)
	return out
}

type factRow struct {
	format, path, kind  string
	covered, executable int
	lineMap             string
}

func (e *normalizeEnv) coverage(t *testing.T, receipt entity.ID) []factRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT r.report_format, f.path, f.kind, f.covered, f.executable, COALESCE(f.line_map_key, '')
		FROM normalize.coverage_fact f JOIN normalize.report r ON r.organization_id = f.organization_id AND r.id = f.report_id
		WHERE r.receipt_id = $1 ORDER BY r.report_format, f.path, f.kind`, uuid.UUID(receipt))
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (factRow, error) {
		var f factRow
		return f, row.Scan(&f.format, &f.path, &f.kind, &f.covered, &f.executable, &f.lineMap)
	})
	require.NoError(t, err)
	return out
}

// v40 is the CI package of V-40: the same JUnit cases in the linux and windows jobs of a matrix and the coverage
// of the linux job.
func v40() []reportSpec {
	return []reportSpec{
		{fixture: "junit-gotestsum.xml", name: "linux.xml", kind: "junit", format: "junit_xml", producer: "gotestsum", version: "1.12.0", ci: gitlabCI("5001", "7001")},
		{fixture: "junit-gotestsum.xml", name: "windows.xml", kind: "junit", format: "junit_xml", producer: "gotestsum", version: "1.12.0", ci: gitlabCI("5001", "7003")},
		{
			fixture: "coverage-py.xml", kind: "coverage", format: "cobertura", producer: "coverage.py", version: "7.6.1", ci: gitlabCI("5001", "7001"),
			sourceRoot: "/builds/demo/app",
		},
		{fixture: "lcov-istanbul.info", kind: "coverage", format: "lcov", producer: "istanbul", version: "6.0.3", ci: gitlabCI("5001", "7001")},
	}
}

func TestNormalizeTests_V40ConfigurationsAndCoverage(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	ci := pack(t, ciManifest("42"),
		ciRun(t, runSpec{id: "5001", project: "42", sha: _reportSHA, status: "success", revision: "p1", updated: _t1}),
		ciAttempt(t, attemptSpec{id: "7001", run: "5001", job: "test", fingerprint: _linux, status: "success", no: 1, seconds: 60}),
		ciAttempt(t, attemptSpec{id: "7003", run: "5001", job: "test", fingerprint: _windows, status: "success", no: 1, seconds: 40}),
	)
	require.Equal(t, entity.ReceiptAccepted, e.imported(t, e.admin, ci).Status)
	e.drain(t)
	data := reportPackage(t, _reportSHA, v40()...)
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, rc.ID).Status)

	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.test_case"), "a case in two configurations is one test")
	got := e.executions(t, rc.ID)
	require.Len(t, got, 14)
	linux, windows := objectID(t, e, "ci_attempt", "7001"), objectID(t, e, "ci_attempt", "7003")
	var ms int64
	assert.Equal(t, []executionRow{
		{key: "example.com/demo/calc::TestAbsBroken", outcome: "failed", fingerprint: _linux, ordinal: 1, attempt: &linux, ms: &ms},
		{key: "example.com/demo/calc::TestAbsBroken", outcome: "failed", fingerprint: _windows, ordinal: 1, attempt: &windows, ms: &ms},
	}, got[:2], "V-40: two executions with their configurations, not two attempts of one test")
	assert.Equal(t, 1, e.n(t, "SELECT count(DISTINCT run_started_at) FROM normalize.test_execution WHERE run_started_at = $1", _t1),
		"an execution starts with its attempt")
	assert.Equal(t, 14, e.n(t, "SELECT count(*) FROM normalize.test_execution WHERE run_started_at = $1", _t1))

	facts := e.coverage(t, rc.ID)
	keys := e.keys(t, "line-maps/")
	require.Len(t, keys, 2, "one line map per coverage file")
	cobertura := ""
	for i := range facts {
		assert.Contains(t, keys, facts[i].lineMap)
		if facts[i].format == "cobertura" {
			cobertura = facts[i].lineMap
		}
		facts[i].lineMap = ""
	}
	assert.Equal(t, []factRow{
		{format: "cobertura", path: "src/calc/__init__.py", kind: "branch"},
		{format: "cobertura", path: "src/calc/__init__.py", kind: "line"},
		{"cobertura", "src/calc/ops.py", "branch", 1, 2, ""},
		{"cobertura", "src/calc/ops.py", "line", 2, 4, ""},
		{format: "cobertura", path: "src/calc/util.py", kind: "branch"},
		{"cobertura", "src/calc/util.py", "line", 3, 3, ""},
		{"lcov", "src/calc.js", "branch", 2, 4, ""},
		{"lcov", "src/calc.js", "line", 4, 5, ""},
		{format: "lcov", path: "src/index.js", kind: "branch"},
		{"lcov", "src/index.js", "line", 1, 1, ""},
	}, facts, "V-40: ops.py covers 2 of 4 lines; line and branch coverage stay apart")
	assert.Equal(t, 0, e.n(t, `SELECT count(*) FROM normalize.coverage_fact f JOIN normalize.report r ON r.organization_id = f.organization_id
		AND r.id = f.report_id WHERE r.report_kind = 'junit'`), "JUnit gives no coverage")
	r := e.report(t, rc.ID, "coverage")
	assert.Contains(t, string(r.SkippedTargets), `{"path": "/usr/lib/python3/dist-packages/six.py", "reason": "outside_repository"}`)
	assert.Equal(t, "04", e.reportOf(t, rc.ID, "cobertura").SchemaVersion)

	body, err := e.store.Get(t.Context(), cobertura)
	require.NoError(t, err)
	maps := lineMaps(t, body)
	assert.Contains(t, maps, `{"path":"src/calc/ops.py","kind":"line","points":[[1,1],[2,1],[3,0],[5,0]]}`)
	assert.Contains(t, maps, `{"path":"src/calc/ops.py","kind":"branch","points":[[2,1,2]]}`)

	for range 2 {
		e.renormalize(t, rc.ID)
		e.drain(t)
	}
	assert.Equal(t, entity.ReceiptDuplicate, e.imported(t, e.admin, data).Status)
	e.drain(t)
	assert.Equal(t, 14, e.n(t, "SELECT count(*) FROM normalize.test_execution"), "normalizing the package again adds no execution")
	assert.Equal(t, 10, e.n(t, "SELECT count(*) FROM normalize.coverage_fact"))
	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.test_case"))
}

func (e *normalizeEnv) reportOf(t *testing.T, receipt entity.ID, format string) reportRow {
	t.Helper()
	var r reportRow
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT schema_version, parser_version FROM normalize.report WHERE receipt_id = $1 AND report_format = $2",
		uuid.UUID(receipt), format).Scan(&r.SchemaVersion, &r.ParserVersion))
	return r
}

func lineMaps(t *testing.T, body io.ReadCloser) []string {
	t.Helper()
	defer body.Close()
	var out []string
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		require.True(t, json.Valid(sc.Bytes()))
		out = append(out, sc.Text())
	}
	require.NoError(t, sc.Err())
	return out
}

func TestNormalizeTests_BrokenReportsKeepTheFormerOnes(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	first := e.imported(t, e.admin, reportPackage(t, _reportSHA, v40()[0], v40()[2]))
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, first.ID).Status)
	junit := reportFixture(t, "junit-gotestsum.xml")
	cobertura := reportFixture(t, "coverage-py.xml")
	old := bytes.Replace(cobertura, []byte(`<?xml version="1.0" ?>`),
		[]byte(`<?xml version="1.0" ?><!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-03.dtd">`), 1)
	require.NotEqual(t, cobertura, old)
	cases := []struct {
		name, reason string
		spec         reportSpec
		content      []byte
	}{
		{"truncated JUnit", entity.RecordReportInvalid, v40()[0], junit[:len(junit)/2]},
		{"Cobertura coverage-03", entity.RecordReportVersionUnsupported, v40()[2], old},
	}
	for _, tt := range cases {
		spec := tt.spec
		spec.content, spec.ci = tt.content, gitlabCI("5002", "7009")
		rc := e.imported(t, e.admin, reportPackage(t, _reportSHA, spec))
		require.Equal(t, entity.ReceiptAccepted, rc.Status, "%s: %s", tt.name, rc.ReasonCode)
		e.drain(t)
		assert.Equal(t, entity.ReceiptPartial, e.receipt(t, rc.ID).Status, tt.name)
		recs := e.records(t, rc.ID)
		require.Len(t, recs, 1)
		assert.Equal(t, tt.reason, recs[0].ReasonCode, tt.name)
	}
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.report WHERE sha = $1", _reportSHA), "the former reports of the SHA stay in force")
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.report WHERE receipt_id = $1", uuid.UUID(first.ID)))
	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.test_execution"))
	assert.Equal(t, 6, e.n(t, "SELECT count(*) FROM normalize.coverage_fact"))
	unknown := hex.EncodeToString(attemptConfig("7001"))
	assert.Equal(t, 7, e.n(t, "SELECT count(*) FROM normalize.test_execution WHERE encode(config_fingerprint, 'hex') = $1", unknown),
		"an attempt not collected yet stands for its own configuration")
}

// attemptConfig is the configuration of an attempt whose version is not known yet: SHA-256 of its parts, each
// after its length as a uvarint.
func attemptConfig(native string) []byte {
	h := sha256.New()
	for _, p := range []string{"config", "attempt", native} {
		h.Write(binary.AppendUvarint(nil, uint64(len(p))))
		h.Write([]byte(p))
	}
	return h.Sum(nil)
}

func TestNormalizeTests_AdapterArtifactsOfAnAttempt(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	conn, scope := e.gitlabScope(t, "42")
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, ciManifest("42"))
	require.NoError(t, err)
	sha, run, attempt := _reportSHA, "5001", "7002"
	require.NoError(t, w.AddRecord(ciRun(t, runSpec{id: run, project: "42", sha: sha, status: "success", revision: "p1", updated: _t1})))
	require.NoError(t, w.AddRecord(ciAttempt(t, attemptSpec{id: attempt, run: run, job: "test", fingerprint: _linux, status: "success", no: 1, seconds: 30})))
	for _, f := range []struct{ kind, format, fixture string }{{"junit", "junit_xml", "junit-pytest.xml"}, {"coverage", "lcov", "lcov-istanbul.info"}} {
		ref, err := w.AddFile("files/artifacts/7002/"+f.fixture, "application/xml", reportFixture(t, f.fixture))
		require.NoError(t, err)
		rec := mrRecord(t, "report_file", f.kind+":"+ref.Path, attempt, ref.Sha256, envelope.ReportFileV1{
			ReportKind: envelope.ReportFileV1ReportKind(f.kind), ReportFormat: envelope.ReportFileV1ReportFormat(f.format), Sha: &sha,
			RunNativeId: &run, AttemptNativeId: &attempt, Producer: "gitlab", ProducerVersion: "unknown",
		})
		rec.RecordClass, rec.File = envelope.RecordRecordClassFile, &ref
		require.NoError(t, w.AddRecord(rec))
	}
	require.NoError(t, w.Close())
	rc := e.page(t, conn, scope, buf.Bytes())
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)

	id := objectID(t, e, "ci_attempt", attempt)
	got := e.executions(t, rc.ID)
	require.Len(t, got, 5)
	for _, x := range got {
		assert.Equal(t, _linux, x.fingerprint, "the configuration of the attempt the adapter read the artifact of")
		assert.Equal(t, id, *x.attempt)
	}
	assert.Equal(t, []string{"error", "failed", "passed", "passed", "skipped"}, outcomes(got))
	assert.Len(t, e.coverage(t, rc.ID), 4, "relative LCOV paths need no checkout")
}

func outcomes(rows []executionRow) []string {
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].outcome
	}
	slices.Sort(out)
	return out
}

func TestNormalizeTests_JaCoCoWithItsDocumentType(t *testing.T) {
	t.Parallel()
	e := newNormalizeEnv(t)
	rc := e.imported(t, e.admin, reportPackage(t, _reportSHA, reportSpec{
		fixture: "jacoco.xml", kind: "coverage", format: "jacoco", producer: "jacoco", version: "0.8.13", ci: gitlabCI("5001", "7001"),
		sourceDir: "src/main/java",
	}))
	require.Equal(t, entity.ReceiptAccepted, rc.Status, "a DOCTYPE that names an external DTD passes intake: %s", rc.ReasonCode)
	e.drain(t)
	require.Equal(t, entity.ReceiptAccepted, e.receipt(t, rc.ID).Status)
	facts := e.coverage(t, rc.ID)
	require.Len(t, facts, 2)
	assert.Equal(t, []any{"src/main/java/com/example/calc/Ops.java", "branch", 1, 2}, []any{facts[0].path, facts[0].kind, facts[0].covered, facts[0].executable})
	assert.Equal(t, []any{"src/main/java/com/example/calc/Ops.java", "line", 3, 4}, []any{facts[1].path, facts[1].kind, facts[1].covered, facts[1].executable})
	assert.Equal(t, "jacoco-1.1/1", e.reportOf(t, rc.ID, "jacoco").ParserVersion)
}
