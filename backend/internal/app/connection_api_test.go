package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	workerctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/worker"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/adapterapi"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/oidc"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const _jiraToken = "jira-token-API-3f9a1c77"

type capabilityBody struct {
	Capability string  `json:"capability"`
	State      string  `json:"state"`
	ReasonCode *string `json:"reason_code"`
}

type connectionBody struct {
	ID       uuid.UUID `json:"id"`
	Status   string    `json:"status"`
	Instance struct {
		ID      uuid.UUID `json:"id"`
		BaseURL *string   `json:"base_url"`
		Edition *string   `json:"edition"`
	} `json:"instance"`
	CredentialsSetAt *time.Time `json:"credentials_set_at"`
	SyncPaused       bool       `json:"sync_paused"`
	Scopes           []struct {
		ID            uuid.UUID `json:"id"`
		Kind          string    `json:"kind"`
		NativeScopeID string    `json:"native_scope_id"`
	} `json:"scopes"`
	Capabilities []capabilityBody `json:"capabilities"`
}

type probeBody struct {
	ID                   uuid.UUID        `json:"id"`
	Status               string           `json:"status"`
	ReasonCode           *string          `json:"reason_code"`
	ContractVersion      *string          `json:"contract_version"`
	Capabilities         []capabilityBody `json:"capabilities"`
	RetryAfterSeconds    *int             `json:"retry_after_seconds"`
	ConfigurationVersion int              `json:"configuration_version"`
}

// connectionsEnv is the api with the test adapter as the built-in Jira adapter and a worker taking jobs.
type connectionsEnv struct {
	*loginEnv
	adapter *adapterServer
	admin   *browser
}

func newConnectionsEnv(t *testing.T, scenario testadapter.Config) *connectionsEnv {
	t.Helper()
	e := newLoginEnv(t)
	ca := newTestCA(t, "internal CA")
	cfg, _ := ca.serverTLS(t, time.Now().Add(time.Hour))
	srv := testAdapter(t, cfg, scenario, _adapterKey)
	reg := prometheus.NewRegistry()
	caller, err := adapterapi.NewCaller(adapterapi.Config{
		Builtin:    map[entity.Provider]adapterapi.Builtin{entity.ProviderJiraCloud: {URL: srv.URL, Key: _adapterKey}},
		InternalCA: ca.pool, Registerer: reg, Resolver: mapResolver{"jira.loop.test": "127.0.0.1"},
	})
	require.NoError(t, err)
	uc, err := newConnect(e.app, e.keyring, caller, e.clock)
	require.NoError(t, err)
	api := newAPI(e.app, e.keyring, oidc.New(e.idp.srv.Client()), e.clock, _site, reg, uc)
	e.router = httpctrl.NewRouter(zerolog.New(e.logs), reg, health.Probes{Ready: &health.Readiness{}}, api)

	workers := river.NewWorkers()
	workerctrl.Register(workers, workerctrl.Deps{Probes: uc})
	jobs, err := river.NewClient(riverpgxv5.New(e.app), &river.Config{
		Queues: map[string]river.QueueConfig{entity.QueueInteractive: {MaxWorkers: 2}}, Workers: workers,
		FetchCooldown: 20 * time.Millisecond, FetchPollInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, jobs.Start(context.WithoutCancel(t.Context())))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = jobs.Stop(ctx)
	})
	e.secrets = append(e.secrets, _jiraToken, _adapterKey)
	return &connectionsEnv{loginEnv: e, adapter: srv, admin: e.admin(t, "demo")}
}

func jiraConnection(baseURL, project string) map[string]any {
	return map[string]any{
		"name": "Jira основной", "provider": "jira_cloud", "base_url": baseURL, "adapter_kind": "builtin",
		"adapter_url": nil, "adapter_key": nil,
		"credentials": map[string]string{"email": "svc-craftpulse@example.org", "token": _jiraToken},
		"scopes":      []map[string]any{{"kind": "tracker_project", "native_scope_id": project, "history_from": "2026-01-01T00:00:00Z"}},
	}
}

func (e *connectionsEnv) json(t *testing.T, b *browser, method, target string, header http.Header, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	h := http.Header{"Origin": {_site}, "Content-Type": {"application/json"}}
	for k, v := range header {
		h[k] = v
	}
	return e.send(t, b, method, target, h, bytes.NewReader(raw))
}

func (e *connectionsEnv) create(t *testing.T, body map[string]any) (c connectionBody, etag string) {
	t.Helper()
	rec := e.json(t, e.admin, http.MethodPost, "/api/v1/connections", nil, body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	c = decode[connectionBody](t, rec)
	assert.Equal(t, "/api/v1/connections/"+c.ID.String(), rec.Header().Get("Location"))
	return c, rec.Header().Get("ETag")
}

// probe starts a probe and waits for the worker to finish it.
func (e *connectionsEnv) probe(t *testing.T, id uuid.UUID) probeBody {
	t.Helper()
	rec := e.json(t, e.admin, http.MethodPost, "/api/v1/connections/"+id.String()+"/probes", nil, nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	started := decode[probeBody](t, rec)
	location := rec.Header().Get("Location")
	require.Equal(t, "/api/v1/connections/"+id.String()+"/probes/"+started.ID.String(), location)
	var p probeBody
	require.Eventually(t, func() bool {
		get := e.do(t, e.admin, http.MethodGet, location, nil)
		require.Equal(t, http.StatusOK, get.Code, get.Body.String())
		p = decode[probeBody](t, get)
		return p.Status == "succeeded" || p.Status == "failed"
	}, 15*time.Second, 50*time.Millisecond)
	assert.Nil(t, p.RetryAfterSeconds, "a finished probe has no poll hint")
	return p
}

func (e *connectionsEnv) activate(t *testing.T, id, probe uuid.UUID, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	h := http.Header{}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return e.json(t, e.admin, http.MethodPost, "/api/v1/connections/"+id.String()+"/activation", h, map[string]any{"probe_id": probe})
}

func (e *connectionsEnv) patch(t *testing.T, id uuid.UUID, ifMatch string, body any) *httptest.ResponseRecorder {
	t.Helper()
	h := http.Header{"Content-Type": {"application/merge-patch+json"}}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return e.json(t, e.admin, http.MethodPatch, "/api/v1/connections/"+id.String(), h, body)
}

func (e *connectionsEnv) jobs(t *testing.T, kind string) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = '"+kind+"'")
}

func (e *connectionsEnv) credentials(t *testing.T, id uuid.UUID) int {
	t.Helper()
	return count(t, t.Context(), e.adapter.db.pool(t, "owner"), "SELECT count(*) FROM adapter.credential WHERE connection_id = '"+id.String()+"'")
}

func capabilityState(caps []capabilityBody, name string) string {
	for _, c := range caps {
		if c.Capability == name {
			return c.State
		}
	}
	return ""
}

func TestConnections_ConnectProbeAndActivate(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})

	rec := e.do(t, e.admin, http.MethodGet, "/api/v1/adapters", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"provider":"jira_cloud"`)
	assert.Contains(t, rec.Body.String(), `"contract_versions":["1","0"]`)
	assert.NotContains(t, rec.Body.String(), "gitlab", "only adapters the installation runs are listed")

	c, tag := e.create(t, jiraConnection("https://example.atlassian.net", "10000"))
	assert.Equal(t, "draft", c.Status)
	assert.Equal(t, `"1"`, tag)
	require.NotNil(t, c.CredentialsSetAt)
	assert.True(t, e.clock.Now().Equal(*c.CredentialsSetAt))
	assert.Equal(t, 1, e.credentials(t, c.ID), "the adapter keeps the credentials")

	p := e.probe(t, c.ID)
	require.Equal(t, "succeeded", p.Status, "reason %v", p.ReasonCode)
	assert.Equal(t, 1, p.ConfigurationVersion)
	require.NotNil(t, p.ContractVersion)
	assert.Equal(t, "1", *p.ContractVersion)
	assert.Equal(t, "verified", capabilityState(p.Capabilities, "read_current"))
	assert.Equal(t, "missing", capabilityState(p.Capabilities, "write_tracker_issue"), "write is not proven by reading")

	again := e.json(t, e.admin, http.MethodPost, "/api/v1/connections/"+c.ID.String()+"/probes", nil, nil)
	require.Equal(t, http.StatusAccepted, again.Code)
	newer := decode[probeBody](t, again)
	require.Eventually(t, func() bool {
		return decode[probeBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String()+"/probes/"+newer.ID.String(), nil)).Status == "succeeded"
	}, 15*time.Second, 50*time.Millisecond)
	stale := e.activate(t, c.ID, p.ID, tag)
	require.Equal(t, http.StatusConflict, stale.Code, "a probe that is not the latest is refused")
	assert.Equal(t, "state_conflict", decode[problemBody](t, stale).Code)
	assert.Equal(t, http.StatusPreconditionRequired, e.activate(t, c.ID, newer.ID, "").Code)
	assert.Equal(t, http.StatusPreconditionFailed, e.activate(t, c.ID, newer.ID, `"7"`).Code)

	ok := e.activate(t, c.ID, newer.ID, tag)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	active := decode[connectionBody](t, ok)
	assert.Equal(t, "active", active.Status)
	assert.Equal(t, `"2"`, ok.Header().Get("ETag"))
	assert.Equal(t, "verified", capabilityState(active.Capabilities, "read_current"))
	assert.Equal(t, "missing", capabilityState(active.Capabilities, "write_tracker_issue"), "an unproven capability is not stored as verified")
	require.NotNil(t, active.Instance.Edition)
	assert.Equal(t, "cloud", *active.Instance.Edition)
	var key string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT instance_key FROM connect.source_instance WHERE id = $1", active.Instance.ID).Scan(&key))
	assert.Equal(t, _jiraInstance, key, "the connection moved to the instance the probe named")
	assert.Equal(t, 3, e.jobs(t, entity.KindSyncPage), "one first load per data kind of the adapter")

	repeat := e.activate(t, c.ID, newer.ID, tag)
	require.Equal(t, http.StatusOK, repeat.Code, "a repeat with the former ETag returns the current state")
	assert.Equal(t, `"2"`, repeat.Header().Get("ETag"))
	assert.Equal(t, 3, e.jobs(t, entity.KindSyncPage), "a repeat queues no second load")

	e.clock.Advance(10 * time.Minute)
	replaced := e.patch(t, c.ID, `"2"`, map[string]any{"credentials": map[string]string{"email": "svc-craftpulse@example.org", "token": _jiraToken + "-new"}})
	require.Equal(t, http.StatusOK, replaced.Code, replaced.Body.String())
	assert.Equal(t, `"3"`, replaced.Header().Get("ETag"), "new credentials need a new probe before another activation")
	updated := decode[connectionBody](t, replaced)
	assert.Equal(t, "active", updated.Status, "collection goes on with the credentials the adapter took")
	require.NotNil(t, updated.CredentialsSetAt)
	assert.True(t, e.clock.Now().Equal(*updated.CredentialsSetAt))
	e.secrets = append(e.secrets, _jiraToken+"-new")

	for _, op := range []string{"createConnection", "startConnectionProbe", "activateConnection", "updateConnection"} {
		assert.NotEmpty(t, e.audit(t, op), op)
	}
	e.assertNoCredentials(t)
}

// assertNoCredentials looks for the token in every answer path the test saw and in the core database.
func (e *connectionsEnv) assertNoCredentials(t *testing.T) {
	t.Helper()
	for _, table := range []string{"connect.connection", "connect.connection_scope", "connect.capability", "connect.probe", "connect.source_instance", "audit.event", "river_job"} {
		var dump string
		require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT coalesce(string_agg(t::text, E'\\n'), '') FROM "+table+" t").Scan(&dump))
		assert.NotContains(t, dump, _jiraToken, table)
	}
	assert.NotContains(t, e.logs.String(), _jiraToken)
	list := e.do(t, e.admin, http.MethodGet, "/api/v1/connections", nil)
	require.Equal(t, http.StatusOK, list.Code)
	assert.NotContains(t, list.Body.String(), _jiraToken)
	assert.NotContains(t, list.Body.String(), "svc-craftpulse@example.org", "credential fields do not come back")
}

func TestConnections_ProbeFailures(t *testing.T) {
	t.Parallel()
	t.Run("cloud edition on the DC adapter", func(t *testing.T) {
		t.Parallel()
		e := newConnectionsEnv(t, testadapter.Config{Edition: testadapter.EditionUnsupported})
		c, _ := e.create(t, jiraConnection("https://jira.example.org", "10000"))
		p := e.probe(t, c.ID)
		assert.Equal(t, "failed", p.Status)
		require.NotNil(t, p.ReasonCode)
		assert.Equal(t, "edition_mismatch", *p.ReasonCode)
		got := decode[connectionBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String(), nil))
		assert.Equal(t, "draft", got.Status, "a failed probe does not make the connection working")
		assert.Equal(t, http.StatusConflict, e.activate(t, c.ID, p.ID, `"1"`).Code)
	})
	t.Run("no right to read history", func(t *testing.T) {
		t.Parallel()
		e := newConnectionsEnv(t, testadapter.Config{HistoryDenied: true})
		c, tag := e.create(t, jiraConnection("https://jira.example.org", "10000"))
		p := e.probe(t, c.ID)
		require.Equal(t, "succeeded", p.Status)
		assert.Equal(t, "missing", capabilityState(p.Capabilities, "read_status_history"))
		ok := e.activate(t, c.ID, p.ID, tag)
		require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
		active := decode[connectionBody](t, ok)
		assert.Equal(t, "verified", capabilityState(active.Capabilities, "read_current"))
		assert.Equal(t, "missing", capabilityState(active.Capabilities, "read_status_history"), "partial history stays visible")
	})
}

func TestConnections_RefusedChanges(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})

	for _, base := range []string{"https://127.0.0.1:8443", "https://jira.loop.test"} {
		rec := e.json(t, e.admin, http.MethodPost, "/api/v1/connections", nil, jiraConnection(base, "10000"))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, base)
		p := decode[problemBody](t, rec)
		require.Len(t, p.Errors, 1)
		assert.Equal(t, "/base_url", p.Errors[0].Field)
		assert.Equal(t, "outside_scope", p.Errors[0].Code)
	}
	assert.Zero(t, e.adapter.hits.Load(), "a refused address never reaches the adapter")

	first, tag := e.create(t, jiraConnection("https://jira.example.org", "10000"))
	taken := e.json(t, e.admin, http.MethodPost, "/api/v1/connections", nil, jiraConnection("https://jira.example.org/", "10000"))
	require.Equal(t, http.StatusConflict, taken.Code, taken.Body.String())
	assert.Equal(t, "state_conflict", decode[problemBody](t, taken).Code)
	assert.Equal(t, 1, count(t, t.Context(), e.adapter.db.pool(t, "owner"), "SELECT count(*) FROM adapter.credential"),
		"credentials of the refused connection are deleted in the adapter again")

	missing := jiraConnection("https://jira.example.org", "10001")
	missing["credentials"] = map[string]string{"email": "svc-craftpulse@example.org"}
	rec := e.json(t, e.admin, http.MethodPost, "/api/v1/connections", nil, missing)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "/credentials/token", decode[problemBody](t, rec).Errors[0].Field)

	assert.Equal(t, http.StatusPreconditionRequired, e.patch(t, first.ID, "", map[string]any{"name": "x"}).Code)
	assert.Equal(t, http.StatusPreconditionFailed, e.patch(t, first.ID, `"5"`, map[string]any{"name": "x"}).Code)
	renamed := e.patch(t, first.ID, tag, map[string]any{"name": "Jira архив", "sync_paused": true})
	require.Equal(t, http.StatusOK, renamed.Code, renamed.Body.String())
	assert.Equal(t, tag, renamed.Header().Get("ETag"), "a name and the pause of sync are not settings a probe checked")
	assert.True(t, decode[connectionBody](t, renamed).SyncPaused)

	moved := e.patch(t, first.ID, tag, map[string]any{"scopes": []map[string]any{{"kind": "tracker_project", "native_scope_id": "10002", "history_from": nil}}})
	require.Equal(t, http.StatusOK, moved.Code, moved.Body.String())
	assert.Equal(t, `"2"`, moved.Header().Get("ETag"))
	scopes := decode[connectionBody](t, moved).Scopes
	require.Len(t, scopes, 1)
	assert.Equal(t, "10002", scopes[0].NativeScopeID)
	second, _ := e.create(t, jiraConnection("https://jira.example.org", "10000"))
	assert.NotEqual(t, first.ID, second.ID, "the scope the first connection left is free")

	member := newBrowser()
	require.Equal(t, http.StatusFound, e.login(t, member, _otherSub).Code)
	assert.Equal(t, http.StatusForbidden, e.do(t, member, http.MethodGet, "/api/v1/connections/"+first.ID.String(), nil).Code)
	other := newBrowser()
	require.Equal(t, http.StatusFound, e.loginTo(t, other, "other", _adminSub).Code)
	assert.Equal(t, http.StatusNotFound, e.do(t, other, http.MethodGet, "/api/v1/connections/"+first.ID.String(), nil).Code,
		"a connection of another organization does not exist for its administrator")
}

func TestConnections_DeleteRemovesCredentials(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})
	c, _ := e.create(t, jiraConnection("https://jira.example.org", "10000"))
	require.Equal(t, 1, e.credentials(t, c.ID))

	for range 2 {
		rec := e.json(t, e.admin, http.MethodDelete, "/api/v1/connections/"+c.ID.String(), nil, nil)
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	}
	assert.Zero(t, e.credentials(t, c.ID))
	got := decode[connectionBody](t, e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String(), nil))
	assert.Equal(t, "deleted", got.Status)
	assert.Empty(t, got.Scopes, "the scopes of a deleted connection are closed")
	assert.Equal(t, http.StatusNotFound, e.json(t, e.admin, http.MethodPost, "/api/v1/connections/"+c.ID.String()+"/probes", nil, nil).Code)
	assert.Len(t, e.audit(t, "deleteConnection"), 1, "a repeated delete is not a second deletion")

	list := e.do(t, e.admin, http.MethodGet, "/api/v1/connections?status=deleted&limit=1", nil)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	assert.True(t, strings.Contains(list.Body.String(), c.ID.String()))
}
