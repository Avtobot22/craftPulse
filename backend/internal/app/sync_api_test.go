package app

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
)

type syncStatusBody struct {
	ConnectionID     uuid.UUID      `json:"connection_id"`
	ConnectionStatus string         `json:"connection_status"`
	DataKinds        []syncKindBody `json:"data_kinds"`
	LastReconcile    *struct {
		StartedAt    time.Time  `json:"started_at"`
		FinishedAt   *time.Time `json:"finished_at"`
		Outcome      string     `json:"outcome"`
		MissingCount *int       `json:"missing_count"`
	} `json:"last_reconcile"`
}

type syncKindBody struct {
	DataKind          string     `json:"data_kind"`
	Phase             string     `json:"phase"`
	LastOutcome       *string    `json:"last_outcome"`
	LastErrorCode     *string    `json:"last_error_code"`
	FailedPagePresent bool       `json:"failed_page_present"`
	LastSuccessAt     *time.Time `json:"last_success_at"`
	Watermark         *time.Time `json:"watermark"`
	LagSeconds        *int       `json:"lag_seconds"`
	Completeness      string     `json:"completeness"`
	UnknownFields     []string   `json:"unknown_fields"`
	AffectedMetrics   []string   `json:"affected_metrics"`
}

type operationBody struct {
	ID                uuid.UUID `json:"id"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	ResultURI         *string   `json:"result_uri"`
	RetryAfterSeconds *int      `json:"retry_after_seconds"`
	Error             *struct {
		Code          string    `json:"code"`
		CorrelationID uuid.UUID `json:"correlation_id"`
	} `json:"error"`
}

type lastSuccessBody struct {
	ID            uuid.UUID  `json:"id"`
	LastSuccessAt *time.Time `json:"last_success_at"`
}

// activeConnection connects project 10000 and lets its first load and reconciliation finish.
func (e *connectionsEnv) activeConnection(t *testing.T) uuid.UUID {
	t.Helper()
	c, tag := e.create(t, jiraConnection("https://example.atlassian.net", "10000"))
	p := e.probe(t, c.ID)
	require.Equal(t, "succeeded", p.Status)
	require.Equal(t, http.StatusOK, e.activate(t, c.ID, p.ID, tag).Code)
	e.settle(t)
	return c.ID
}

// settle runs the queued jobs; the clock moves to their times, so the administrator signs in again.
func (e *connectionsEnv) settle(t *testing.T) {
	t.Helper()
	e.runner.settle(t)
	e.admin = e.loginEnv.admin(t, "demo")
}

func (e *connectionsEnv) syncRun(t *testing.T, b *browser, id uuid.UUID, kind string) *httptest.ResponseRecorder {
	t.Helper()
	return e.json(t, b, http.MethodPost, "/api/v1/connections/"+id.String()+"/sync-runs", nil, map[string]string{"kind": kind, "data_kind": "tasks"})
}

func (e *connectionsEnv) operation(t *testing.T, b *browser, location string) (int, operationBody) {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, location, nil)
	if rec.Code != http.StatusOK {
		return rec.Code, operationBody{}
	}
	return rec.Code, decode[operationBody](t, rec)
}

func TestConnections_SyncStatusAndLastSuccess(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})
	id := e.activeConnection(t)

	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+id.String()+"/sync-status", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	st := decode[syncStatusBody](t, rec)
	assert.Equal(t, "active", st.ConnectionStatus)
	i := slices.IndexFunc(st.DataKinds, func(k syncKindBody) bool { return k.DataKind == "tasks" })
	require.GreaterOrEqual(t, i, 0)
	kind := st.DataKinds[i]
	assert.Equal(t, "complete", kind.Completeness)
	assert.Equal(t, "success", *kind.LastOutcome)
	assert.False(t, kind.FailedPagePresent)
	assert.NotNil(t, kind.UnknownFields)
	assert.Contains(t, kind.AffectedMetrics, "T08")
	require.NotNil(t, kind.LastSuccessAt)
	require.NotNil(t, kind.Watermark)
	require.NotNil(t, kind.LagSeconds)
	require.NotNil(t, st.LastReconcile)
	assert.Equal(t, "complete", st.LastReconcile.Outcome)
	assert.Equal(t, 0, *st.LastReconcile.MissingCount)

	var stored time.Time
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT last_success_at FROM sync.state WHERE connection_id = $1 AND data_kind = 'tasks'", id).Scan(&stored))
	assert.True(t, stored.Equal(*kind.LastSuccessAt), "the status is the state of sync")

	list := decode[struct {
		Data []lastSuccessBody `json:"data"`
	}](t, e.do(t, e.admin, http.MethodGet, "/api/v1/connections?limit=10", nil))
	require.Len(t, list.Data, 1)
	require.NotNil(t, list.Data[0].LastSuccessAt)
	assert.True(t, list.Data[0].LastSuccessAt.Equal(*kind.LastSuccessAt), "listConnections has the same last success")
	one := decode[lastSuccessBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+id.String(), nil))
	require.NotNil(t, one.LastSuccessAt)
	assert.True(t, one.LastSuccessAt.Equal(*kind.LastSuccessAt), "getConnection has the same last success")
}

func TestConnections_SyncRunsAreBackgroundOperations(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})
	id := e.activeConnection(t)

	first := e.syncRun(t, e.admin, id, "resume")
	require.Equal(t, http.StatusAccepted, first.Code)
	location := first.Header().Get("Location")
	code, op := e.operation(t, e.admin, location)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "/api/v1/operations/"+op.ID.String(), location)
	assert.Equal(t, "sync_run", op.Kind)
	assert.Equal(t, "queued", op.Status)
	require.NotNil(t, op.RetryAfterSeconds)
	assert.Nil(t, op.ResultURI)

	again := e.syncRun(t, e.admin, id, "resume")
	require.Equal(t, http.StatusAccepted, again.Code)
	assert.Equal(t, location, again.Header().Get("Location"), "a repeat during the run returns the same operation")
	assert.Equal(t, http.StatusConflict, e.syncRun(t, e.admin, id, "reconcile").Code, "another request for the connection meanwhile")
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = 'sync.page' AND state <> 'completed'"),
		"one collection is queued")

	other := newBrowser()
	e.login(t, other, _otherSub)
	code, _ = e.operation(t, other, location)
	assert.Equal(t, http.StatusNotFound, code, "another user does not see the operation")

	e.settle(t)
	code, op = e.operation(t, e.admin, location)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "succeeded", op.Status)
	require.NotNil(t, op.ResultURI)
	assert.Equal(t, "/api/v1/connections/"+id.String()+"/sync-status", *op.ResultURI)
	assert.Nil(t, op.RetryAfterSeconds)
	assert.Nil(t, op.Error)

	check := e.syncRun(t, e.admin, id, "reconcile")
	require.Equal(t, http.StatusAccepted, check.Code, "after the run a new request creates a new operation")
	assert.NotEqual(t, location, check.Header().Get("Location"))
	e.settle(t)
	_, done := e.operation(t, e.admin, check.Header().Get("Location"))
	assert.Equal(t, "succeeded", done.Status)

	require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Kind: testadapter.FaultAuth},
	}}))
	failed := e.syncRun(t, e.admin, id, "resume")
	require.Equal(t, http.StatusAccepted, failed.Code)
	e.settle(t)
	_, op = e.operation(t, e.admin, failed.Header().Get("Location"))
	assert.Equal(t, "failed", op.Status)
	require.NotNil(t, op.Error)
	assert.Equal(t, "access_lost", op.Error.Code, "a safe class, the cause stays in the logs")
	assert.NotEqual(t, uuid.Nil, op.Error.CorrelationID)

	e.clock.Advance(25 * time.Hour)
	e.admin = e.loginEnv.admin(t, "demo")
	code, _ = e.operation(t, e.admin, location)
	assert.Equal(t, http.StatusNotFound, code, "an operation finished more than 24 h ago")

	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection SET status = 'access_lost', access_lost_at = now() WHERE id = $1", id)
	require.NoError(t, err)
	lost := e.json(t, e.admin, http.MethodPost, "/api/v1/connections/"+id.String()+"/sync-runs", nil, map[string]string{"kind": "resume", "data_kind": "tasks"})
	require.Equal(t, http.StatusConflict, lost.Code)
	assert.Equal(t, "state_conflict", decode[problemBody](t, lost).Code)
}
