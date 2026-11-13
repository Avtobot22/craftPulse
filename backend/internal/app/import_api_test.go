package app

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/ingest"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const _secretTitle = "title-not-for-the-receipt-5b1e"

type receiptBody struct {
	ID              uuid.UUID  `json:"id"`
	Status          string     `json:"status"`
	RecordCount     int        `json:"record_count"`
	AcceptedCount   int        `json:"accepted_count"`
	RejectedCount   int        `json:"rejected_count"`
	ReplayCount     int        `json:"replay_count"`
	ConflictCount   int        `json:"conflict_count"`
	ReasonCode      *string    `json:"reason_code"`
	DuplicateOf     *uuid.UUID `json:"duplicate_of"`
	AffectedMetrics []string   `json:"affected_metrics"`
	RetryAfter      *int       `json:"retry_after_seconds"`
}

type acceptedBody struct {
	ReceiptID   uuid.UUID  `json:"receipt_id"`
	Status      string     `json:"status"`
	DuplicateOf *uuid.UUID `json:"duplicate_of"`
	RetryAfter  *int       `json:"retry_after_seconds"`
}

type importsEnv struct {
	*loginEnv
	uc      *ingest.UseCase
	admin   *browser
	project uuid.UUID
}

func newImportsEnv(t *testing.T) *importsEnv {
	t.Helper()
	e := newLoginEnv(t)
	caller, err := adapterapi.NewCaller(adapterapi.Config{Resolver: mapResolver{}})
	require.NoError(t, err)
	conn, err := newConnect(e.app, e.keyring, caller, e.clock)
	require.NoError(t, err)
	_, rights := newOrgAccess(e.app, e.clock)
	reg := prometheus.NewRegistry()
	uc, err := newIngest(e.app, sharedStore(t), conn, rights, e.clock, reg)
	require.NoError(t, err)
	e.router = httpctrl.NewRouter(zerolog.New(e.logs), reg, health.Probes{Ready: &health.Readiness{}},
		newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, conn, uc))
	return &importsEnv{loginEnv: e, uc: uc, admin: e.admin(t, "demo"), project: e.tokenProject(t, e.orgID)}
}

func (e *importsEnv) upload(t *testing.T, b *browser, fields map[string]string, contentType string, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="tasks"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(file)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return e.send(t, b, http.MethodPost, "/api/v1/imports", http.Header{"Origin": {_site}, "Content-Type": {w.FormDataContentType()}}, &body)
}

func importForm(project uuid.UUID, version string) map[string]string {
	return map[string]string{
		"schema_id": "tasks", "schema_version": version, "source_provider": "jira_cloud",
		"source_instance_key": _jiraInstance, "project_id": project.String(),
	}
}

// imported sends the file, runs the job of the package as the worker does and returns the receipt as the
// administrator reads it.
func (e *importsEnv) imported(t *testing.T, b *browser, fields map[string]string, contentType string, file []byte) (acceptedBody, receiptBody) {
	t.Helper()
	rec := e.upload(t, b, fields, contentType, file)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	a := decode[acceptedBody](t, rec)
	assert.Equal(t, "/api/v1/receipts/"+a.ReceiptID.String(), rec.Header().Get("Location"))
	if a.Status == string(entity.ReceiptQuarantined) {
		require.NotNil(t, a.RetryAfter)
		quarantined := e.receipt(t, e.admin, a.ReceiptID)
		assert.Equal(t, "quarantined", quarantined.Status)
		require.NotNil(t, quarantined.RetryAfter, "a receipt in quarantine tells when to ask again")
		require.NoError(t, e.uc.ProcessPackage(t.Context(), e.orgID, entity.ID(a.ReceiptID)))
	}
	return a, e.receipt(t, e.admin, a.ReceiptID)
}

func (e *importsEnv) receipt(t *testing.T, b *browser, id uuid.UUID) receiptBody {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/receipts/"+id.String(), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[receiptBody](t, rec)
}

func taskJSON(t *testing.T, recs ...map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(recs)
	require.NoError(t, err)
	return raw
}

func taskRecord(t *testing.T, id, title, revision string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(task(t, id, title, revision))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func (e *importsEnv) accepted(t *testing.T) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE outcome = 'accepted'")
}

func TestImports_UniversalImportOfTasks(t *testing.T) {
	e := newImportsEnv(t)
	file := taskJSON(t, taskRecord(t, "IMP-1", _secretTitle, ""), taskRecord(t, "", "no id", ""))

	first, rc := e.imported(t, e.admin, importForm(e.project, "1"), "application/json", file)
	assert.Equal(t, "quarantined", first.Status)
	assert.Equal(t, "partial", rc.Status)
	assert.Equal(t, 2, rc.RecordCount)
	assert.Equal(t, 1, rc.AcceptedCount)
	assert.Equal(t, 1, rc.RejectedCount)
	assert.Contains(t, rc.AffectedMetrics, "T04")
	assert.Nil(t, rc.RetryAfter)

	rejected := e.do(t, e.admin, http.MethodGet, "/api/v1/receipts/"+first.ReceiptID.String()+"/records?outcome=rejected", nil)
	require.Equal(t, http.StatusOK, rejected.Code, rejected.Body.String())
	assert.JSONEq(t, `{"data":[{"record_no":2,"record_kind":"task","native_key":null,"outcome":"rejected","reason_code":"missing_source_id"}],"next_cursor":null}`, rejected.Body.String())
	all := e.do(t, e.admin, http.MethodGet, "/api/v1/receipts/"+first.ReceiptID.String()+"/records?limit=1", nil)
	require.Equal(t, http.StatusOK, all.Code)
	assert.NotContains(t, all.Body.String(), _secretTitle, "records come without their content")
	assert.Contains(t, all.Body.String(), `"next_cursor":"`)

	again := e.upload(t, e.admin, importForm(e.project, "1"), "application/json", file)
	require.Equal(t, http.StatusAccepted, again.Code, again.Body.String())
	dup := decode[acceptedBody](t, again)
	assert.Equal(t, "duplicate", dup.Status, "the same file sent again")
	require.NotNil(t, dup.DuplicateOf)
	assert.Equal(t, first.ReceiptID, *dup.DuplicateOf)
	assert.Nil(t, dup.RetryAfter)
	assert.Equal(t, 1, e.accepted(t), "the repeat adds no accepted record")

	csv := []byte("native_id,project_native_id,task_key,type_native_id,status_native_id,created_at\n" +
		"IMP-7,IMP,IMP-7,Story,Done,2026-02-03T09:00:00+03:00\n")
	_, snap := e.imported(t, e.admin, importForm(e.project, "1"), "text/csv", csv)
	assert.Equal(t, "accepted", snap.Status)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE receipt_id = '"+snap.ID.String()+"' AND record_class = 'snapshot'"))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.record WHERE record_kind = 'task_event'"), "a CSV of tasks has no history")

	before := e.accepted(t)
	unknown, bad := e.imported(t, e.admin, importForm(e.project, "9"), "application/json", taskJSON(t, taskRecord(t, "IMP-1", "changed", "")))
	assert.Equal(t, "rejected", unknown.Status, "an unsupported schema is refused at once")
	assert.Equal(t, "rejected", bad.Status)
	require.NotNil(t, bad.ReasonCode)
	assert.Equal(t, entity.PackageSchemaUnsupported, *bad.ReasonCode)
	assert.Equal(t, before, e.accepted(t), "the former result stays")

	large := make([]byte, 26<<20)
	assert.Equal(t, http.StatusRequestEntityTooLarge, e.upload(t, e.admin, importForm(e.project, "1"), "application/json", large).Code)
	assert.Equal(t, http.StatusUnsupportedMediaType, e.upload(t, e.admin, importForm(e.project, "1"), "application/xml", []byte("<r/>")).Code)

	assert.NotEmpty(t, e.audit(t, "createImport"))
}

func TestImports_RightsAndOrganizations(t *testing.T) {
	e := newImportsEnv(t)
	other := e.tokenProject(t, e.orgID)
	lead := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, lead, _otherSub).Code)
	assert.Equal(t, http.StatusForbidden, e.upload(t, lead, importForm(e.project, "1"), "application/json", taskJSON(t, taskRecord(t, "IMP-1", "A", ""))).Code,
		"a user without imports.create")

	_, err := e.owner.Exec(t.Context(), `INSERT INTO org.role_assignment (organization_id, user_id, role, project_id, assigned_by, assigned_at)
		VALUES ($1, $2, 'tech_lead', $3, $2, now())`, uuid.UUID(e.orgID), e.userID(t, e.orgID, _otherSub), other)
	require.NoError(t, err)
	_, rc := e.imported(t, lead, importForm(e.project, "1"), "application/json", taskJSON(t, taskRecord(t, "IMP-1", "A", "")))
	assert.Equal(t, http.StatusNotFound, e.do(t, lead, http.MethodGet, "/api/v1/receipts/"+rc.ID.String(), nil).Code,
		"a tech lead does not see the receipt of a project they may not import into")
	page := e.do(t, e.admin, http.MethodGet, "/api/v1/receipts/"+rc.ID.String()+"/records", nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `"reason_code":"outside_scope"`, "records of a project without the right to import")

	_, own := e.imported(t, lead, importForm(other, "1"), "application/json", taskJSON(t, taskRecord(t, "IMP-2", "B", "")))
	assert.Equal(t, "accepted", own.Status, "the tech lead imports into their project")

	foreign := newBrowser()
	require.Equal(t, http.StatusFound, e.loginTo(t, foreign, "other", _adminSub).Code)
	assert.Equal(t, http.StatusNotFound, e.do(t, foreign, http.MethodGet, "/api/v1/receipts/"+own.ID.String(), nil).Code)
	assert.Equal(t, http.StatusForbidden, e.do(t, lead, http.MethodGet, "/api/v1/receipts", nil).Code, "the journal of intake needs sync.read")
	list := e.do(t, e.admin, http.MethodGet, "/api/v1/receipts?channel=import&status=accepted", nil)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	assert.Contains(t, list.Body.String(), own.ID.String())
	assert.NotContains(t, list.Body.String(), rc.ID.String(), "the filter of status holds")
}

type conflictBody struct {
	ID         uuid.UUID `json:"id"`
	ReceiptID  uuid.UUID `json:"receipt_id"`
	RecordNo   int       `json:"record_no"`
	NativeKey  string    `json:"native_key"`
	Status     string    `json:"status"`
	Resolution *string   `json:"resolution"`
}

func TestImports_ConflictsAreResolvedOnce(t *testing.T) {
	e := newImportsEnv(t)
	for _, id := range []string{"IMP-10", "IMP-11", "IMP-12"} {
		_, base := e.imported(t, e.admin, importForm(e.project, "1"), "application/json", taskJSON(t, taskRecord(t, id, "first", "5")))
		require.Equal(t, "accepted", base.Status)
		_, changed := e.imported(t, e.admin, importForm(e.project, "1"), "application/json", taskJSON(t, taskRecord(t, id, "second", "5")))
		require.Equal(t, 1, changed.ConflictCount)
	}
	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/ingest-conflicts", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	page := decode[struct {
		Data []conflictBody `json:"data"`
	}](t, rec)
	require.Len(t, page.Data, 3)
	assert.Equal(t, "IMP-10", page.Data[0].NativeKey)

	resolve := func(b *browser, id uuid.UUID, resolution string) *httptest.ResponseRecorder {
		raw, err := json.Marshal(map[string]string{"resolution": resolution})
		require.NoError(t, err)
		return e.send(t, b, http.MethodPost, "/api/v1/ingest-conflicts/"+id.String()+"/resolution", _changing, bytes.NewReader(raw))
	}
	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	assert.Equal(t, http.StatusForbidden, resolve(member, page.Data[0].ID, "kept_existing").Code)

	accepted, jobs := e.accepted(t), count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'normalize.batch'")
	for i, resolution := range []string{"kept_existing", "source_corrected", "accepted_new"} {
		res := resolve(e.admin, page.Data[i].ID, resolution)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		got := decode[conflictBody](t, res)
		assert.Equal(t, "resolved", got.Status)
		require.NotNil(t, got.Resolution)
		assert.Equal(t, resolution, *got.Resolution)
	}
	assert.Equal(t, accepted, e.accepted(t), "one accepted record per key: the new one took the place of the former")
	newest := page.Data[2]
	var outcome string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT outcome FROM ingest.record WHERE receipt_id = $1 AND record_no = $2", newest.ReceiptID, newest.RecordNo).Scan(&outcome))
	assert.Equal(t, "accepted", outcome, "accepted_new makes the new record the accepted one")
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT outcome FROM ingest.record WHERE receipt_id = $1 AND record_no = $2", page.Data[0].ReceiptID, page.Data[0].RecordNo).Scan(&outcome))
	assert.Equal(t, "conflict", outcome, "kept_existing leaves the accepted fact as it is")
	assert.Equal(t, jobs+1, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'normalize.batch'"), "the new record goes to normalization")

	again := resolve(e.admin, page.Data[0].ID, "accepted_new")
	assert.Equal(t, http.StatusConflict, again.Code, "a resolved conflict is not resolved again")
	assert.Equal(t, "state_conflict", decode[problemBody](t, again).Code)
	assert.Equal(t, 3, count(t, t.Context(), e.owner, "SELECT count(*) FROM audit.event WHERE operation = 'resolveIngestConflict' AND outcome = 'succeeded'"))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM ingest.conflict WHERE status = 'open'"))
}
