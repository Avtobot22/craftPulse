package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _checksInstance = "gitlab.example"

// identityEnv is the env of the matrix of checks with normalize and the job of change that identifies findings.
type identityEnv struct {
	*checksEnv
	normalized *normalizeEnv
	identifier *change.UseCase
}

func newIdentityEnv(t *testing.T) *identityEnv {
	t.Helper()
	e := &identityEnv{checksEnv: newChecksEnv(t)}
	e.normalized = &normalizeEnv{ingestEnv: e.ingestEnv, norm: e.norm}
	e.identifier = newIdentifier(e.app, e.norm, e.orgs, e.clock)
	return e
}

// drain runs normalize and then the jobs of change it queued, until none is left.
func (e *identityEnv) drain(t *testing.T) {
	t.Helper()
	for {
		e.normalized.drain(t)
		rows, err := e.owner.Query(t.Context(), "SELECT id, args FROM river_job WHERE kind = $1 AND state = 'available' ORDER BY id", entity.KindChangeIdentify)
		require.NoError(t, err)
		type job struct {
			id   int64
			args entity.ChangeIdentifyJob
		}
		jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (job, error) {
			var (
				j   job
				raw []byte
			)
			if err := row.Scan(&j.id, &raw); err != nil {
				return j, err
			}
			return j, json.Unmarshal(raw, &j.args)
		})
		require.NoError(t, err)
		if len(jobs) == 0 {
			return
		}
		for _, j := range jobs {
			require.NoError(t, e.identifier.RunIdentify(t.Context(), j.args, completeJob(j.id)))
		}
	}
}

func (e *identityEnv) submit(t *testing.T, data []byte) entity.Receipt {
	t.Helper()
	rc := e.imported(t, e.admin, data)
	require.Equal(t, entity.ReceiptAccepted, rc.Status, rc.ReasonCode)
	e.drain(t)
	return rc
}

// hit is a result of a SARIF report: content is the matchBasedId of Opengrep, empty for a tool without one.
type hit struct {
	rule, path string
	line       int
	content    string
}

func sarifOf(hits ...hit) []byte {
	results := make([]map[string]any, len(hits))
	for i, h := range hits {
		r := map[string]any{"ruleId": h.rule, "level": "error", "message": map[string]any{"text": "finding"}}
		if h.path != "" {
			r["locations"] = []any{map[string]any{"physicalLocation": map[string]any{
				"artifactLocation": map[string]any{"uri": h.path}, "region": map[string]any{"startLine": h.line},
			}}}
		}
		if h.content != "" {
			r["fingerprints"] = map[string]string{"matchBasedId/v1": h.content}
		}
		results[i] = r
	}
	raw, _ := json.Marshal(map[string]any{"version": "2.1.0", "runs": []any{map[string]any{
		"tool": map[string]any{"driver": map[string]any{"name": "opengrep"}}, "results": results,
	}}})
	return raw
}

// codeReport is a SARIF report of the code of the Go component on a branch, complete unless said otherwise.
func (e *identityEnv) codeReport(t *testing.T, sha, branch, job string, content []byte, exitAndCompleteness ...string) []byte {
	ci := gitlabCI("5"+job, job)
	ci.RefName = branch
	spec := reportSpec{
		fixture: "opengrep.sarif", name: job + ".sarif", kind: "code", format: "sarif", producer: "opengrep", version: "1.33.1", ci: ci,
		component: &envelope.ScanComponent{Id: &e.goComponent, PathPrefix: "services/billing"}, content: content,
		exit: "findings", completeness: "complete",
	}
	if len(exitAndCompleteness) == 2 {
		spec.exit, spec.completeness = exitAndCompleteness[0], exitAndCompleteness[1]
	}
	t.Helper()
	return reportPackageOf(t, _checksInstance, sha, spec)
}

type identityRow struct {
	rule, path, match string
	identity          uuid.UUID
}

func (e *identityEnv) identities(t *testing.T, receipt entity.Receipt) []identityRow {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), `SELECT f.rule_id, COALESCE(f.path, ''), o.match_kind, o.finding_identity_id
		FROM normalize.finding_occurrence f JOIN normalize.report r ON r.organization_id = f.organization_id AND r.id = f.report_id
		JOIN change.occurrence_identity o ON o.organization_id = f.organization_id AND o.occurrence_id = f.id
		WHERE r.receipt_id = $1 ORDER BY f.rule_id, f.path`, uuid.UUID(receipt.ID))
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (identityRow, error) {
		var r identityRow
		return r, row.Scan(&r.rule, &r.path, &r.match, &r.identity)
	})
	require.NoError(t, err)
	return out
}

func TestChangeIdentity_ExactMovedAndUncertain(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	first := e.submit(t, e.codeReport(t, strings.Repeat("1", 40), "feature", "7001", sarifOf(
		hit{"go-sqli", "app/a.go", 10, "m1"}, hit{"errcheck", "app/b.go", 5, ""},
	)))
	got := e.identities(t, first)
	require.Len(t, got, 2)
	for _, r := range got {
		assert.Equal(t, entity.MatchExact, r.match, "a new finding starts its canonical finding")
	}
	errcheck, sqli := got[0].identity, got[1].identity
	assert.Equal(t, 1, e.normalized.n(t, "SELECT count(*) FROM change.finding_identity WHERE id = $1 AND first_report_id = (SELECT id FROM normalize.report WHERE receipt_id = $2)",
		sqli, uuid.UUID(first.ID)))

	second := e.submit(t, e.codeReport(t, strings.Repeat("2", 40), "feature", "7002", sarifOf(
		hit{"go-sqli", "app/a.go", 20, "m1"}, hit{"errcheck", "app/b.go", 5, ""}, hit{"go-weak-hash", "", 0, ""},
	)))
	assert.Equal(t, []identityRow{
		{rule: "errcheck", path: "app/b.go", match: entity.MatchExact, identity: errcheck},
		{rule: "go-sqli", path: "app/a.go", match: entity.MatchMoved, identity: sqli},
	}, e.identities(t, second)[:2], "the same finding of the next report is exact; a moved line is moved")
	assert.Equal(t, entity.MatchUncertain, e.identities(t, second)[2].match, "a finding without a place is uncertain")
	assert.Equal(t, 3, e.normalized.n(t, "SELECT count(*) FROM change.finding_identity"))

	e.submit(t, e.codeReport(t, strings.Repeat("3", 40), "feature", "7003", sarifOf(hit{"go-xss", "app/x.go", 1, "m2"}, hit{"go-xss", "app/y.go", 1, "m2"})))
	assert.Equal(t, 5, e.normalized.n(t, "SELECT count(*) FROM change.finding_identity"), "two copies in one report are two findings")
	fourth := e.submit(t, e.codeReport(t, strings.Repeat("4", 40), "feature", "7004", sarifOf(hit{"go-xss", "app/z.go", 1, "m2"})))
	got = e.identities(t, fourth)
	require.Len(t, got, 1)
	assert.Equal(t, entity.MatchUncertain, got[0].match, "two candidates make the match uncertain")
	assert.Equal(t, 5, e.normalized.n(t, "SELECT count(*) FROM change.finding_identity"), "an uncertain finding gets no second canonical finding")

	for _, rc := range []entity.Receipt{first, second, fourth} {
		e.normalized.renormalize(t, rc.ID)
	}
	e.drain(t)
	assert.Equal(t, 5, e.normalized.n(t, "SELECT count(*) FROM change.finding_identity"), "identifying the reports again adds nothing")
	assert.Equal(t, 8, e.normalized.n(t, "SELECT count(*) FROM change.occurrence_identity"))
	assert.Equal(t, 0, e.normalized.n(t, "SELECT count(*) FROM change.baseline"), "a branch other than the main one gives no baseline")
}

type baselineRow struct {
	ComponentID   uuid.UUID  `json:"component_id"`
	Language      string     `json:"language"`
	AnalysisKind  string     `json:"analysis_kind"`
	State         string     `json:"state"`
	ReportID      *uuid.UUID `json:"report_id"`
	Branch        string     `json:"branch"`
	SHA           *string    `json:"sha"`
	ReasonCode    *string    `json:"reason_code"`
	EstablishedAt *string    `json:"established_at"`
}

type baselineBody struct {
	Data []baselineRow `json:"data"`
}

func (b baselineBody) of(kind string) baselineRow {
	for _, r := range b.Data {
		if r.AnalysisKind == kind {
			return r
		}
	}
	return baselineRow{}
}

func (e *identityEnv) baselines(t *testing.T, b *browser, project uuid.UUID) (int, baselineBody) {
	t.Helper()
	rec := e.call(t, b, "", http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/baselines", project), nil, nil)
	if rec.Code != http.StatusOK {
		return rec.Code, baselineBody{}
	}
	return rec.Code, decode[baselineBody](t, rec)
}

func (e *identityEnv) reportOf(t *testing.T, rc entity.Receipt) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.report WHERE receipt_id = $1", uuid.UUID(rc.ID)).Scan(&id))
	return id
}

func TestChangeIdentity_V24BaselineOfTheMainBranch(t *testing.T) {
	t.Parallel()
	e := newIdentityEnv(t)
	project := uuid.UUID(e.project)
	rec := e.confirm(t, project, `"0"`, uuid.New(), row(e.goComponent, "go", "code", "builtin", "go-default"), row(e.goComponent, "", "sca", "import", "sca-import"),
		row(e.goComponent, "", "secrets", "builtin", "gitleaks-default"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	e.submit(t, pack(t, manifestOf(envelope.ManifestSchemaIdMergeRequests, envelope.SourceProviderGitlab, _checksInstance, "42"), mrRepository(t, "42", "team/billing", "r1")))

	code, list := e.baselines(t, e.lead, project)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Data, 3)
	for _, b := range list.Data {
		assert.Equal(t, []any{"pending", "main", "full_run_missing"}, []any{b.State, b.Branch, *b.ReasonCode}, b.AnalysisKind)
		assert.Nil(t, b.ReportID)
	}

	h0, h1, h2 := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	f := sarifOf(hit{"go-sqli", "app/a.go", 10, "m1"})
	e.submit(t, e.codeReport(t, h0, "main", "7001", f, "partial", "partial"))
	_, list = e.baselines(t, e.lead, project)
	assert.Equal(t, []any{"pending", "scan_incomplete"}, []any{list.of("code").State, *list.of("code").ReasonCode}, "a partial run of main is no baseline")
	e.submit(t, e.codeReport(t, h0, "main", "7002", f, "timeout", "unknown"))
	_, list = e.baselines(t, e.lead, project)
	assert.Equal(t, []any{"failed", "scan_failed"}, []any{list.of("code").State, *list.of("code").ReasonCode})
	assert.Equal(t, 0, e.normalized.n(t, "SELECT count(*) FROM change.baseline"))

	full := e.submit(t, e.codeReport(t, h1, "main", "7003", f))
	_, list = e.baselines(t, e.lead, project)
	b := list.of("code")
	assert.Equal(t, []any{"code", "go", "ready", h1, e.reportOf(t, full)}, []any{b.AnalysisKind, b.Language, b.State, *b.SHA, *b.ReportID}, "V-24: the first full run of main is the baseline")
	assert.Nil(t, b.ReasonCode)
	assert.NotNil(t, b.EstablishedAt)
	assert.Equal(t, "pending", list.of("sca").State, "SCA has no full run yet")
	assert.Equal(t, 1, e.normalized.n(t, `SELECT count(*) FROM change.baseline b JOIN change.occurrence_identity o ON true
		JOIN normalize.finding_occurrence f ON f.organization_id = o.organization_id AND f.id = o.occurrence_id AND f.report_id = b.report_id
		WHERE b.superseded_at IS NULL`), "f is in the baseline")
	assert.Equal(t, 1, e.normalized.n(t, `SELECT count(*) FROM change.baseline b JOIN change.profile_version v ON v.organization_id = b.organization_id
		AND v.id = b.profile_version_id WHERE v.version_no = 1`), "the baseline names the version of the matrix")

	e.submit(t, e.codeReport(t, h2, "main", "7004", f, "partial", "partial"))
	e.submit(t, e.codeReport(t, h2, "feature", "7005", f))
	_, list = e.baselines(t, e.lead, project)
	assert.Equal(t, []any{"ready", h1}, []any{list.of("code").State, *list.of("code").SHA}, "a partial run of main and a branch keep the baseline")

	newer := e.submit(t, e.codeReport(t, h2, "main", "7006", f))
	_, list = e.baselines(t, e.lead, project)
	assert.Equal(t, []any{h2, e.reportOf(t, newer)}, []any{*list.of("code").SHA, *list.of("code").ReportID})
	assert.Equal(t, 1, e.normalized.n(t, "SELECT count(*) FROM change.baseline WHERE superseded_at IS NOT NULL"), "the former baseline is superseded")
	e.normalized.renormalize(t, full.ID)
	e.drain(t)
	assert.Equal(t, 2, e.normalized.n(t, "SELECT count(*) FROM change.baseline"), "an older report does not take the baseline back")

	hidden := e.tokenProject(t, e.orgID)
	code, _ = e.baselines(t, e.lead, hidden)
	assert.Equal(t, http.StatusNotFound, code, "a project outside the scope is not found")
}
