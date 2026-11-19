package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	v1 "github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	connectrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/connect"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	syncuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _webhookSecret = "hook-secret-150-token"

type webhookEnv struct {
	*syncEnv
	conn   entity.ID
	route  entity.WebhookRoute
	router http.Handler
	logs   *lockedBuffer
}

func newWebhookEnv(t *testing.T) *webhookEnv {
	t.Helper()
	e := newSyncEnv(t, testadapter.Config{})
	conn := e.connection(t, "10000")
	repo := connectrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		return repo.EnsureWebhookRoute(ctx, e.org, conn, e.clock.Now())
	}))
	st, err := e.connect.State(t.Context(), e.org, conn)
	require.NoError(t, err)
	require.NotNil(t, st.WebhookRouteID)
	route, err := e.connect.WebhookRoute(t.Context(), *st.WebhookRouteID)
	require.NoError(t, err)
	require.NoError(t, e.connect.PutCredentials(t.Context(), e.org, conn, entity.CredentialsInput{Purpose: "collect", Secret: entity.Secret(`{"webhook_secret":"` + _webhookSecret + `"}`)}))
	logs := &lockedBuffer{}
	h := handler.New(handler.Deps{Webhooks: e.uc, Registerer: e.reg})
	router := httpctrl.NewRouter(zerolog.New(logs), e.reg, health.Probes{Ready: &health.Readiness{}}, v1.Deps{Handler: h})
	return &webhookEnv{syncEnv: e, conn: conn, route: route, router: router, logs: logs}
}

func (e *webhookEnv) post(t *testing.T, route entity.ID, secret string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/webhooks/"+route.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Token", secret)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func hookBody(t *testing.T, kind, ns, id string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"event_kind": "objects_changed", "keys": []map[string]string{{"object_kind": kind, "native_ns": ns, "native_id": id}}})
	require.NoError(t, err)
	return raw
}

func (e *webhookEnv) active(t *testing.T, kind string) int {
	t.Helper()
	var n int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE kind=$1 AND state NOT IN ('completed','discarded','cancelled')", kind).Scan(&n))
	return n
}

func (e *webhookEnv) nextKind(t *testing.T, kind string) *queued {
	t.Helper()
	var q queued
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id,kind,args,scheduled_at FROM river_job WHERE kind=$1 AND state NOT IN ('completed','discarded','cancelled') ORDER BY id LIMIT 1", kind).Scan(&q.id, &q.kind, &q.args, &q.scheduledAt))
	return &q
}

func (e *webhookEnv) runHooks(t *testing.T) {
	t.Helper()
	for e.active(t, entity.KindSyncWebhook) > 0 {
		require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindSyncWebhook)))
	}
}

func TestWebhook_HTTPBoundsEncryptionAndRejection(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	raw := hookBody(t, "task", "10000", "20003")
	assert.Equal(t, http.StatusNotFound, e.post(t, entity.ID(uuid.New()), _webhookSecret, raw).Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, e.post(t, e.route.ID, _webhookSecret, bytes.Repeat([]byte("x"), (1<<20)+1)).Code)
	assert.Zero(t, e.active(t, entity.KindSyncWebhook))
	assert.Equal(t, uuid.Version(4), uuid.UUID(e.route.ID).Version())
	exact := append([]byte(`{"event_kind":"unknown"}`), bytes.Repeat([]byte(" "), (1<<20)-len(`{"event_kind":"unknown"}`))...)
	require.Equal(t, http.StatusOK, e.post(t, e.route.ID, _webhookSecret, exact).Code)
	e.runHooks(t)
	require.Equal(t, http.StatusOK, e.post(t, e.route.ID, "wrong-secret", raw).Code)
	q := e.nextKind(t, entity.KindSyncWebhook)
	assert.NotContains(t, string(q.args), "wrong-secret")
	require.ErrorContains(t, e.uc.RunWebhook(t.Context(), args[entity.SyncWebhookJob](t, q), func(context.Context) error { return errors.New("transaction rolled back") }), "transaction rolled back")
	before, err := e.uc.Status(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	assert.Zero(t, before.Webhooks.RejectedCount)
	require.NoError(t, e.runner().run(t, q))
	require.NoError(t, e.runner().run(t, q))
	assert.Zero(t, e.active(t, entity.KindSyncWebhook))
	assert.Zero(t, e.active(t, entity.KindSyncDirty))
	assert.Zero(t, e.adapter.journal.calls(testadapter.OpFetch))
	st, err := e.uc.Status(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	require.NotNil(t, st.Webhooks)
	assert.Equal(t, 1, st.Webhooks.RejectedCount)
	assert.Equal(t, "signature_invalid", st.Webhooks.LastRejectReason)
	assert.NotNil(t, st.Webhooks.LastReceivedAt)
	metrics, err := e.reg.Gather()
	require.NoError(t, err)
	var rejected float64
	for _, m := range metrics {
		if m.GetName() == "craftpulse_webhook_rejections_total" {
			rejected = m.Metric[0].GetCounter().GetValue()
		}
	}
	assert.Equal(t, 1.0, rejected)
	e.clock.Advance(25 * time.Hour)
	st, err = e.uc.Status(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	assert.Zero(t, st.Webhooks.RejectedCount)
	assert.NotNil(t, st.Webhooks.LastRejectedAt)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		return connectrepo.NewRepo(e.app).RevokeWebhookRoutes(ctx, e.org, e.conn, e.clock.Now())
	}))
	assert.Equal(t, http.StatusNotFound, e.post(t, e.route.ID, _webhookSecret, raw).Code)
	assert.NotContains(t, e.logs.String(), _webhookSecret)
	assert.NotContains(t, e.adapter.journal.String(), _webhookSecret)
	code, dump := exec(t, []string{"pg_dump", "--data-only", "-U", "postgres", "-d", e.owner.Config().ConnConfig.Database})
	require.Zero(t, code)
	assert.NotContains(t, dump, _webhookSecret)
	assert.NotContains(t, dump, "wrong-secret")
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.New(), func(ctx context.Context) error {
		assert.Zero(t, count(t, ctx, e.app, "SELECT count(*) FROM sync.webhook_delivery"))
		return nil
	}))
}

func TestWebhook_AdapterFailureIsRetriedAndUnknownEventIgnored(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Faults: []testadapter.Fault{{Operation: testadapter.OpParseWebhook, Kind: testadapter.FaultUnavailable, Attempts: []int{1}}}}))
	rec := e.post(t, e.route.ID, _webhookSecret, hookBody(t, "task", "10000", "20003"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, e.adapter.journal.calls(testadapter.OpParseWebhook), "HTTP does not wait for the adapter")
	q := e.nextKind(t, entity.KindSyncWebhook)
	assert.NotContains(t, string(q.args), _webhookSecret)
	require.NoError(t, e.runner().run(t, q))
	retry := e.nextKind(t, entity.KindSyncWebhook)
	assert.Equal(t, e.clock.Now().Add(time.Second), retry.scheduledAt.UTC())
	require.NoError(t, e.runner().run(t, retry))
	assert.Equal(t, 1, e.active(t, entity.KindSyncDirty))
	before := e.active(t, entity.KindSyncDirty)
	require.Equal(t, http.StatusOK, e.post(t, e.route.ID, _webhookSecret, []byte(`{"event_kind":"unknown","keys":[]}`)).Code)
	e.runHooks(t)
	assert.Equal(t, before, e.active(t, entity.KindSyncDirty))
	assert.NotContains(t, e.logs.String(), _webhookSecret)
	assert.NotContains(t, e.adapter.journal.String(), _webhookSecret)
}

func TestWebhook_HundredDeliveriesCoalesceAndPageYields(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	for i := 0; i < 100; i++ {
		require.Equal(t, http.StatusOK, e.post(t, e.route.ID, _webhookSecret, hookBody(t, "task", "10000", string(rune('A'+i)))).Code)
	}
	e.runHooks(t)
	assert.Equal(t, 1, e.active(t, entity.KindSyncDirty))
	assert.Equal(t, 100, count(t, t.Context(), e.owner, "SELECT count(*) FROM sync.dirty_key"))
	q := e.nextKind(t, entity.KindSyncPage)
	require.NoError(t, e.runner().run(t, q))
	next := e.nextKind(t, entity.KindSyncPage)
	assert.Equal(t, e.clock.Now().Add(5*time.Second), next.scheduledAt.UTC())
	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection_scope SET scope_kind='repository' WHERE connection_id=$1", uuid.UUID(e.conn))
	require.NoError(t, err)
	instance := routeInstance(t, e)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		_, err := connectrepo.NewRepo(e.app).AddScope(ctx, e.org, e.conn, instance, entity.NewScope{Kind: entity.ScopeRepository, NativeScopeID: "20000", DisplayName: "Second"}, e.clock.Now())
		return err
	}))
	for i := 0; i < 100; i++ {
		require.Equal(t, http.StatusOK, e.post(t, e.route.ID, _webhookSecret, hookBody(t, "merge_request", "", "9001")).Code)
	}
	e.runHooks(t)
	assert.Equal(t, 1, e.active(t, entity.KindSyncMR))
	mr := e.nextKind(t, entity.KindSyncMR)
	assert.Len(t, args[entity.SyncMRJob](t, mr).ScopeIDs, 2)
	var queueName string
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT queue FROM river_job WHERE id=$1", mr.id).Scan(&queueName))
	assert.Equal(t, entity.QueueInteractive, queueName)
	assert.Equal(t, e.clock.Now().Add(5*time.Second), mr.scheduledAt.UTC())
}

// webhookFetch keeps the real adapter's parsing and serves a canonical MR and pipeline page for the source read.
type webhookFetch struct {
	*connect.UseCase
	mr, ci  []byte
	staleMR []byte
	calls   []entity.FetchInput
	fail    error
}

func (c *webhookFetch) Fetch(ctx context.Context, org, conn entity.ID, in entity.FetchInput) (io.ReadCloser, error) {
	c.calls = append(c.calls, in)
	if c.fail != nil {
		return nil, c.fail
	}
	if in.SchemaID == "merge_requests" {
		if len(in.ObjectKeys) == 0 && c.staleMR != nil {
			return io.NopCloser(bytes.NewReader(c.staleMR)), nil
		}
		return io.NopCloser(bytes.NewReader(c.mr)), nil
	}
	if in.SchemaID == "ci_runs" {
		return io.NopCloser(bytes.NewReader(c.ci)), nil
	}
	return c.UseCase.Fetch(ctx, org, conn, in)
}

func TestWebhook_MRAndPipelineReachNormalize(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	// The test source uses a repository scope of the same instance and empty namespaces like GitLab.
	_, err := e.owner.Exec(t.Context(), "UPDATE connect.connection_scope SET scope_kind='repository' WHERE connection_id=$1", uuid.UUID(e.conn))
	require.NoError(t, err)
	m := mrManifest("10000")
	m.Source.Provider, m.Source.InstanceKey = envelope.SourceProviderJiraCloud, _jiraInstance
	yes := true
	source := &webhookFetch{UseCase: e.connect, mr: pack(t, m, mrRepository(t, "10000", "demo/repo", "r1"), mrState(t, "9001", "10000", "Changed", "v2", _t2, &yes))}
	ci := ciManifest("10000")
	ci.Source = m.Source
	source.ci = pack(t, ci, ciRun(t, runSpec{id: "5001", project: "10000", sha: strings.Repeat("a", 40), status: "success", revision: "p2", updated: _t2, mr: "9001"}))
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	e.uc = syncuc.New(syncuc.Deps{Keyring: adapterKeyring(t), Repo: syncrepo.NewRepo(e.app), Connect: source, Ingest: e.intake, Normalize: e.norm, Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics, Clock: e.clock, Rand: func() float64 { return 0 }})
	route := e.route
	require.NoError(t, e.uc.ReceiveWebhook(t.Context(), route, entity.WebhookInput{Headers: map[string]string{"X-Gitlab-Token": _webhookSecret}, Body: hookBody(t, "merge_request", "", "9001")}, entity.ID{}))
	e.runHooks(t)
	q := e.nextKind(t, entity.KindSyncMR)
	require.NoError(t, e.runner().run(t, q))
	// Normalize the MR before the pipeline, as both jobs become available independently.
	for e.active(t, entity.KindNormalizeBatch) > 0 {
		require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindNormalizeBatch)))
	}
	require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindSyncMR)))
	for e.active(t, entity.KindNormalizeBatch) > 0 {
		require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindNormalizeBatch)))
	}
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM normalize.mr_version WHERE title='Changed' AND superseded_seq IS NULL"))
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM normalize.ci_run_version WHERE mr_id IS NOT NULL AND superseded_seq IS NULL"))
	key := entity.ObjectKey{ObjectKind: "ci_run", NativeID: "5001"}
	targets, err := e.norm.WebhookTargets(t.Context(), e.org, routeInstance(t, e), []entity.ObjectKey{key})
	target := targets[key]
	require.NoError(t, err)
	assert.True(t, target.Open)
	assert.Equal(t, "9001", target.Key.NativeID)
	assert.Equal(t, "10000", target.ScopeNativeID)
	assert.Equal(t, 2, count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind='normalize.batch' AND args->>'observed_via'='webhook'"))
}

func routeInstance(t *testing.T, e *webhookEnv) entity.ID {
	t.Helper()
	st, err := e.connect.State(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	return st.Instance.ID
}

func TestWebhook_RouteLifecycleAndAPIURL(t *testing.T) {
	t.Parallel()
	e := newConnectionsEnv(t, testadapter.Config{Capabilities: []string{"read_current", "read_status_history", "read_field_history", "read_resolution_history", "read_permissions", "webhook"}})
	c, tag := e.create(t, jiraConnection("https://example.atlassian.net", "10000"))
	before := e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String(), nil)
	var draft map[string]any
	require.NoError(t, json.Unmarshal(before.Body.Bytes(), &draft))
	assert.Contains(t, draft, "webhook_url")
	assert.Nil(t, draft["webhook_url"])
	probe := e.probe(t, c.ID)
	rec := e.activate(t, c.ID, probe.ID, tag)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var active map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &active))
	url, ok := active["webhook_url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(url, _site+"/api/v1/webhooks/"))
	for _, path := range []string{"/api/v1/connections/" + c.ID.String(), "/api/v1/connections"} {
		get := e.do(t, e.admin, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, get.Code, get.Body.String())
		assert.Contains(t, get.Body.String(), url)
	}
	emptyDiagnostics := e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String()+"/sync-status", nil)
	require.Equal(t, http.StatusOK, emptyDiagnostics.Code)
	assert.Contains(t, emptyDiagnostics.Body.String(), `"webhooks":null`)
	repo := syncrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
		id, err := repo.ReceiveWebhook(ctx, e.orgID, entity.ID(c.ID), e.clock.Now())
		if err != nil {
			return err
		}
		_, err = repo.RejectWebhook(ctx, e.orgID, id, e.clock.Now(), "signature_invalid")
		return err
	}))
	diagnostics := e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+c.ID.String()+"/sync-status", nil)
	require.Equal(t, http.StatusOK, diagnostics.Code, diagnostics.Body.String())
	var status struct {
		Webhooks struct {
			RejectedCount    int        `json:"rejected_count"`
			LastReceivedAt   *time.Time `json:"last_received_at"`
			LastRejectedAt   *time.Time `json:"last_rejected_at"`
			LastRejectReason string     `json:"last_reject_reason"`
		} `json:"webhooks"`
	}
	require.NoError(t, json.Unmarshal(diagnostics.Body.Bytes(), &status))
	assert.Equal(t, 1, status.Webhooks.RejectedCount)
	assert.Equal(t, "signature_invalid", status.Webhooks.LastRejectReason)
	assert.NotNil(t, status.Webhooks.LastReceivedAt)
	assert.NotNil(t, status.Webhooks.LastRejectedAt)
	// Activation is idempotent and does not rotate the address.
	again := e.activate(t, c.ID, probe.ID, tag)
	require.Equal(t, http.StatusOK, again.Code)
	assert.Contains(t, again.Body.String(), url)
	del := e.json(t, e.admin, http.MethodDelete, "/api/v1/connections/"+c.ID.String(), nil, nil)
	require.Equal(t, http.StatusNoContent, del.Code, del.Body.String())
	hook := httptest.NewRequestWithContext(t.Context(), http.MethodPost, strings.TrimPrefix(url, _site), strings.NewReader("{}"))
	answer := httptest.NewRecorder()
	e.router.ServeHTTP(answer, hook)
	assert.Equal(t, http.StatusNotFound, answer.Code)
}

func (c *webhookFetch) Manifest(ctx context.Context, org, conn entity.ID, in entity.ManifestInput) (entity.ManifestPage, error) {
	if in.SchemaID == "merge_requests" {
		return entity.ManifestPage{Items: []entity.ManifestItem{{RecordKind: "merge_request", NativeID: "9001", Version: "v3"}}}, nil
	}
	return c.UseCase.Manifest(ctx, org, conn, in)
}

func TestWebhook_NightlyReconcileFindsLostMRDelivery(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	_, err := e.owner.Exec(t.Context(), "UPDATE river_job SET state='cancelled',finalized_at=now() WHERE kind='sync.page'")
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "UPDATE connect.connection_scope SET scope_kind='repository' WHERE connection_id=$1", uuid.UUID(e.conn))
	require.NoError(t, err)
	m := mrManifest("10000")
	m.Source.Provider, m.Source.InstanceKey = envelope.SourceProviderJiraCloud, _jiraInstance
	yes := true
	old := pack(t, m, mrRepository(t, "10000", "demo/repo", "r1"), mrState(t, "9001", "10000", "Before", "v2", _t1, &yes))
	latest := pack(t, m, mrRepository(t, "10000", "demo/repo", "r1"), mrState(t, "9001", "10000", "After lost webhook", "v3", _t2, &yes))
	source := &webhookFetch{UseCase: e.connect, mr: latest, staleMR: old}
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	e.uc = syncuc.New(syncuc.Deps{Keyring: adapterKeyring(t), Repo: syncrepo.NewRepo(e.app), Connect: source, Ingest: e.intake, Normalize: e.norm, Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics, Clock: e.clock, Rand: func() float64 { return 0 }})
	st, err := e.connect.State(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	page := entity.SyncPageJob{OrganizationID: e.org, ConnectionID: e.conn, DataKind: "merge_requests"}
	// A page misses the change; the source manifest names the new version.
	require.NoError(t, e.uc.RunPage(t.Context(), page, "history", nil))
	for e.active(t, entity.KindNormalizeBatch) > 0 {
		require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindNormalizeBatch)))
	}
	require.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM normalize.mr_version WHERE title='Before'"))
	e.runner().settle(t)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, "SELECT count(*) FROM normalize.mr_version WHERE title='After lost webhook' AND superseded_seq IS NULL"))
	assert.Zero(t, e.adapter.journal.calls(testadapter.OpParseWebhook))
	// The scheduler starts the next round a day later without a delivery.
	e.clock.Advance(24 * time.Hour)
	require.NoError(t, e.uc.Schedule(t.Context(), e.org))
	assert.Equal(t, 1, e.active(t, entity.KindSyncPage))
	require.NotEmpty(t, st.Scopes)
}

func TestWebhook_StoppedAdapterDoesNotBlockAcceptance(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	addr := strings.TrimPrefix(e.adapter.URL, "https://")
	oldHandler, tlsConfig := e.adapter.Config.Handler, e.adapter.TLS.Clone()
	e.adapter.Close()
	require.Equal(t, http.StatusOK, e.post(t, e.route.ID, _webhookSecret, hookBody(t, "task", "10000", "20003")).Code)
	require.NoError(t, e.runner().run(t, e.nextKind(t, entity.KindSyncWebhook)))
	assert.Equal(t, 1, e.active(t, entity.KindSyncWebhook))
	assert.Zero(t, e.active(t, entity.KindSyncDirty))
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", addr)
	require.NoError(t, err)
	recovered := httptest.NewUnstartedServer(oldHandler)
	require.NoError(t, recovered.Listener.Close())
	recovered.Listener, recovered.TLS = listener, tlsConfig
	recovered.StartTLS()
	t.Cleanup(recovered.Close)
	e.runHooks(t)
	assert.Zero(t, e.active(t, entity.KindSyncWebhook))
	assert.Equal(t, 1, e.active(t, entity.KindSyncDirty))
}

func TestWebhook_MRLeaseAndRateLimitPreserveInteractiveRetry(t *testing.T) {
	t.Parallel()
	e := newWebhookEnv(t)
	scopes, err := e.connect.Scopes(t.Context(), e.org, e.conn)
	require.NoError(t, err)
	at := e.clock.Now().Add(time.Minute)
	source := &webhookFetch{UseCase: e.connect, fail: &entity.AdapterError{Class: entity.CallSource, Code: entity.SourceRateLimited, RetryAt: &at}}
	jobs, err := riverInsertClient(e.app)
	require.NoError(t, err)
	e.uc = syncuc.New(syncuc.Deps{Keyring: adapterKeyring(t), Repo: syncrepo.NewRepo(e.app), Connect: source, Ingest: e.intake, Normalize: e.norm, Queue: queue.New(jobs), Tx: persistent.NewTransactor(e.app), Metrics: e.metrics, Clock: e.clock, Rand: func() float64 { return 0 }})
	repo := syncrepo.NewRepo(e.app)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error {
		got, err := repo.AcquireLease(ctx, e.org, e.conn, "history", e.clock.Now().Add(time.Minute), e.clock.Now())
		require.True(t, got)
		return err
	}))
	job := entity.SyncMRJob{OrganizationID: e.org, ConnectionID: e.conn, ScopeID: scopes[0].ID, NativeID: "9001"}
	require.ErrorAs(t, e.uc.RunMR(t.Context(), job, "webhook", nil), new(*entity.RetryLaterError))
	assert.Empty(t, source.calls)
	require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.org), func(ctx context.Context) error { return repo.ReleaseLease(ctx, e.org, e.conn, "history") }))
	require.NoError(t, e.uc.RunMR(t.Context(), job, "webhook", nil))
	retry := e.nextKind(t, entity.KindSyncMR)
	assert.True(t, at.Equal(retry.scheduledAt))
	assert.Equal(t, 1, args[entity.SyncMRJob](t, retry).Retry)
	assert.Equal(t, 1, e.active(t, entity.KindSyncMR))
	// Permanent auth failure completes the retry without another source call being queued.
	source.fail = &entity.AdapterError{Class: entity.CallSource, Code: entity.SourceAuth}
	require.NoError(t, e.runner().run(t, retry))
	assert.Zero(t, e.active(t, entity.KindSyncMR))
}
