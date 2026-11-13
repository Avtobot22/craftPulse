package adaptersdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const (
	_serviceKey  = "service-key-current-0123456789"
	_previousKey = "service-key-previous-0123456789"
	_secret      = "jira-token-MARKER-6f1c2a"
)

type fakeSource struct {
	adaptersdk.Unimplemented
	probe func(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error)
	fetch func(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error)
}

func (f *fakeSource) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	if f.probe == nil {
		return adaptersdk.ProbeResult{Provider: adaptersdk.ProbeResultProviderJiraCloud, InstanceKey: "jira.example.test"}, nil
	}
	return f.probe(ctx, call, in)
}

func (f *fakeSource) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	return f.fetch(ctx, call, in, page)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type harness struct {
	t     *testing.T
	url   string
	logs  *syncBuffer
	spool string
	org   uuid.UUID
	conn  uuid.UUID
}

func newHarness(t *testing.T, store *adaptersdk.Store, src adaptersdk.Source) *harness {
	t.Helper()
	logs, spool := &syncBuffer{}, t.TempDir()
	h, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: src, Store: store, ServiceKeys: []string{_serviceKey, _previousKey},
		Log: zerolog.New(logs), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: spool,
	})
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{t: t, url: srv.URL, logs: logs, spool: spool, org: uuid.New(), conn: uuid.New()}
}

// reply is a finished response: the body is already read and closed.
type reply struct {
	StatusCode int
	Header     http.Header
}

func (h *harness) call(method, path string, body any, edit ...func(*http.Request)) (resp *reply, data []byte) {
	h.t.Helper()
	var r io.Reader = http.NoBody
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(h.t, err)
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.url+"/v1/connections/"+h.conn.String()+path, r)
	require.NoError(h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+_serviceKey)
	req.Header.Set("X-Craftpulse-Organization", h.org.String())
	req.Header.Set("X-Craftpulse-Contract-Version", "1")
	req.Header.Set("X-Correlation-Id", "corr-test-1")
	for _, e := range edit {
		e(req)
	}
	got, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer func() { _ = got.Body.Close() }()
	data, err = io.ReadAll(got.Body)
	require.NoError(h.t, err)
	return &reply{StatusCode: got.StatusCode, Header: got.Header}, data
}

func (h *harness) putSecret(purpose, secret string) *reply {
	h.t.Helper()
	resp, _ := h.call(http.MethodPut, "/credentials", map[string]any{"purpose": purpose, "secret": secret, "user_id": nil, "oauth_code": nil, "redirect_uri": nil})
	return resp
}

func adapterError(t *testing.T, body []byte) adaptersdk.AdapterError {
	t.Helper()
	var e adaptersdk.AdapterError
	require.NoError(t, json.Unmarshal(body, &e), string(body))
	return e
}

func TestHandler_ServiceKey(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t)
	h := newHarness(t, adaptersdk.NewStore(db.pool(t, db.app()), keyring(t, masterKey(t, 1))), &fakeSource{})

	for name, auth := range map[string]string{
		"missing": "", "wrong": "Bearer not-the-key", "basic scheme": "Basic " + _serviceKey, "prefix of key": "Bearer service-key",
	} {
		resp, body := h.call(http.MethodPost, "/probe", map[string]any{"base_url": "https://example.atlassian.net", "scopes": []any{}},
			func(r *http.Request) { r.Header.Set("Authorization", auth) })
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, name)
		e := adapterError(t, body)
		assert.Equal(t, adaptersdk.AdapterErrorCodeUnauthorized, e.Code, name)
		assert.Equal(t, "corr-test-1", e.CorrelationId, name)
	}
	for name, key := range map[string]string{"current": _serviceKey, "previous during rotation": _previousKey} {
		resp, body := h.call(http.MethodPost, "/probe", map[string]any{"base_url": "https://example.atlassian.net", "scopes": []any{}},
			func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) })
		assert.Equal(t, http.StatusOK, resp.StatusCode, name+": "+string(body))
	}
}

func TestHandler_RefusesUnsupportedContractVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t, adaptersdk.NewStore(nil, nil), &fakeSource{})
	resp, body := h.call(http.MethodPost, "/probe", map[string]any{"base_url": "https://example.atlassian.net", "scopes": []any{}},
		func(r *http.Request) { r.Header.Set("X-Craftpulse-Contract-Version", "2") })
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Equal(t, adaptersdk.AdapterErrorCodeUnsupportedContractVersion, adapterError(t, body).Code)
}

func TestHandler_UnimplementedOperation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, adaptersdk.NewStore(nil, nil), &fakeSource{})
	resp, body := h.call(http.MethodPost, "/find-published", map[string]any{
		"action": "mr_comment", "marker": "m", "target": map[string]any{"object_kind": "merge_request", "native_ns": "", "native_id": "1"},
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, adaptersdk.AdapterErrorCodeNotSupported, adapterError(t, body).Code)
}

func TestPutCredentials_NeverStoresOrLogsPlaintext(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t)
	h := newHarness(t, adaptersdk.NewStore(db.pool(t, db.app()), keyring(t, masterKey(t, 1))), &fakeSource{})

	resp, body := h.call(http.MethodPut, "/credentials", map[string]any{"purpose": "collect", "secret": _secret, "user_id": nil, "oauth_code": nil, "redirect_uri": nil})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	assert.Empty(t, body)
	user := uuid.New()
	resp, body = h.call(http.MethodPut, "/credentials", map[string]any{"purpose": "user_token", "secret": _secret + "-user", "user_id": user, "oauth_code": nil, "redirect_uri": nil})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))

	dump := db.dump(t)
	assert.Contains(t, dump, "adapter.credential", "the dump holds the table")
	assert.NotContains(t, dump, _secret)

	logs := h.logs.String()
	assert.NotContains(t, logs, _secret)
	var line map[string]any
	found := false
	for _, l := range strings.Split(strings.TrimSpace(logs), "\n") {
		line = map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(l), &line))
		if found = line["message"] == "credentials replaced" && line["purpose"] == "collect"; found {
			break
		}
	}
	require.True(t, found, logs)
	assert.Equal(t, true, line["set"])
	assert.NotEmpty(t, line["replaced_at"])
	assert.Equal(t, "corr-test-1", line["correlation_id"])

	resp, body = h.call(http.MethodPut, "/credentials", map[string]any{"purpose": "collect", "secret": nil, "user_id": nil, "oauth_code": nil, "redirect_uri": nil})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidRequest, adapterError(t, body).Code)
	resp, body = h.call(http.MethodPut, "/credentials", map[string]any{"purpose": 7, "secret": _secret})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotContains(t, string(body), _secret)
}

func previousCount(t *testing.T, owner *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, owner.QueryRow(t.Context(), "SELECT count(*) FROM adapter.credential WHERE previous_ciphertext IS NOT NULL").Scan(&n))
	return n
}

func TestPutCredentials_KeepsPreviousUntilProbeSucceeds(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t)
	owner := db.pool(t, db.owner())
	store := adaptersdk.NewStore(db.pool(t, db.app()), keyring(t, masterKey(t, 1)))
	var seen []string
	fail := true
	src := &fakeSource{probe: func(ctx context.Context, call adaptersdk.Call, _ adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
		v, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeCollect)
		if err != nil {
			return adaptersdk.ProbeResult{}, err
		}
		seen = append(seen, v)
		if fail {
			return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "source returned 401")
		}
		return adaptersdk.ProbeResult{Provider: adaptersdk.ProbeResultProviderJiraCloud, InstanceKey: "jira.example.test"}, nil
	}}
	h := newHarness(t, store, src)
	probe := func() int {
		resp, _ := h.call(http.MethodPost, "/probe", map[string]any{"base_url": "https://example.atlassian.net", "scopes": []any{}})
		return resp.StatusCode
	}

	require.Equal(t, http.StatusNoContent, h.putSecret("collect", "first").StatusCode)
	require.Equal(t, http.StatusNoContent, h.putSecret("collect", "second").StatusCode)
	assert.Equal(t, 1, previousCount(t, owner), "the replaced value is kept")

	assert.Equal(t, http.StatusForbidden, probe())
	assert.Equal(t, 1, previousCount(t, owner), "a failed probe keeps it")
	fail = false
	assert.Equal(t, http.StatusOK, probe())
	assert.Zero(t, previousCount(t, owner), "a successful probe drops it")
	assert.Equal(t, []string{"second", "second"}, seen, "the probe reads the new value")

	for range 2 {
		resp, _ := h.call(http.MethodDelete, "/credentials?purpose=collect", nil)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "a repeated delete answers the same")
	}
	resp, body := h.call(http.MethodPost, "/probe", map[string]any{"base_url": "https://example.atlassian.net", "scopes": []any{}})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, adaptersdk.AdapterErrorCodeAuth, adapterError(t, body).Code, "deleted credentials are not set")
}

func TestStore_MasterKeyRotation(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t)
	app, owner := db.pool(t, db.app()), db.pool(t, db.owner())
	v1, v2 := masterKey(t, 1), masterKey(t, 2)
	orgA, orgB, conn := uuid.New(), uuid.New(), uuid.New()
	ctx := t.Context()

	before := adaptersdk.NewStore(app, keyring(t, v1))
	_, err := before.PutCredential(ctx, orgA, conn, adaptersdk.CredentialsInputPurposeCollect, "secret-a")
	require.NoError(t, err)
	_, err = before.PutCredential(ctx, orgB, conn, adaptersdk.CredentialsInputPurposeWriteBot, "secret-b")
	require.NoError(t, err)
	var ciphertext []byte
	require.NoError(t, owner.QueryRow(ctx, "SELECT ciphertext FROM adapter.credential WHERE organization_id = $1", orgA).Scan(&ciphertext))

	during := adaptersdk.NewStore(app, keyring(t, v2, v1))
	got, err := during.Credential(ctx, orgA, conn, adaptersdk.CredentialsInputPurposeCollect)
	require.NoError(t, err)
	assert.Equal(t, "secret-a", got, "values sealed under the previous master key open")

	rewrapped, err := adaptersdk.NewStore(owner, keyring(t, v2, v1)).RewrapKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rewrapped, "organization A was rewrapped on read, B by migrate")
	var stale int
	require.NoError(t, owner.QueryRow(ctx, "SELECT count(*) FROM adapter.org_key WHERE master_key_version <> 2").Scan(&stale))
	assert.Zero(t, stale)
	var after []byte
	require.NoError(t, owner.QueryRow(ctx, "SELECT ciphertext FROM adapter.credential WHERE organization_id = $1", orgA).Scan(&after))
	assert.Equal(t, ciphertext, after, "rotation rewraps only data keys")

	only := adaptersdk.NewStore(app, keyring(t, v2))
	got, err = only.Credential(ctx, orgB, conn, adaptersdk.CredentialsInputPurposeWriteBot)
	require.NoError(t, err)
	assert.Equal(t, "secret-b", got, "the previous master key is no longer needed")

	_, err = owner.Exec(ctx, "UPDATE adapter.credential SET ciphertext = set_byte(ciphertext, 0, get_byte(ciphertext, 0) # 1) WHERE organization_id = $1", orgA)
	require.NoError(t, err)
	_, err = only.Credential(ctx, orgA, conn, adaptersdk.CredentialsInputPurposeCollect)
	assert.ErrorIs(t, err, crypto.ErrDecrypt, "a modified ciphertext is refused")

	_, err = owner.Exec(ctx, "UPDATE adapter.credential SET organization_id = $1, connection_id = $2 WHERE organization_id = $3", orgA, uuid.New(), orgB)
	require.NoError(t, err)
	var moved uuid.UUID
	require.NoError(t, owner.QueryRow(ctx, "SELECT connection_id FROM adapter.credential WHERE purpose = 'write_bot'").Scan(&moved))
	_, err = only.Credential(ctx, orgA, moved, adaptersdk.CredentialsInputPurposeWriteBot)
	assert.ErrorIs(t, err, crypto.ErrDecrypt, "a ciphertext moved to another row is refused")
}

func TestStore_IsolatesOrganizations(t *testing.T) {
	t.Parallel()
	db := newAdapterDB(t)
	app, owner := db.pool(t, db.app()), db.pool(t, db.owner())
	store := adaptersdk.NewStore(app, keyring(t, masterKey(t, 1)))
	orgA, orgB, conn, user := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ctx := t.Context()
	_, err := store.PutCredential(ctx, orgA, conn, adaptersdk.CredentialsInputPurposeCollect, "secret-a")
	require.NoError(t, err)
	_, err = store.PutUserToken(ctx, orgA, conn, user, adaptersdk.UserToken{Access: "access-a", Refresh: "refresh-a", AccountNativeID: "acc-1"})
	require.NoError(t, err)

	_, err = store.Credential(ctx, orgB, conn, adaptersdk.CredentialsInputPurposeCollect)
	assert.ErrorIs(t, err, adaptersdk.ErrNoCredential)
	_, err = store.UserToken(ctx, orgB, conn, user)
	assert.ErrorIs(t, err, adaptersdk.ErrNoCredential)
	tok, err := store.UserToken(ctx, orgA, conn, user)
	require.NoError(t, err)
	assert.Equal(t, adaptersdk.UserToken{Access: "access-a", Refresh: "refresh-a", AccountNativeID: "acc-1"}, tok)

	count := func(ctx context.Context) int {
		var n int
		require.NoError(t, pg.Conn(ctx, app).QueryRow(ctx,
			"SELECT (SELECT count(*) FROM adapter.credential) + (SELECT count(*) FROM adapter.user_token) + (SELECT count(*) FROM adapter.org_key)").Scan(&n))
		return n
	}
	require.NoError(t, pg.InOrgTx(ctx, app, orgB, func(ctx context.Context) error {
		assert.Zero(t, count(ctx), "rows of another organization are not read")
		tag, err := pg.Conn(ctx, app).Exec(ctx, "DELETE FROM adapter.credential")
		assert.Zero(t, tag.RowsAffected(), "nor changed")
		return err
	}))
	assert.Zero(t, count(ctx), "without an organization nothing is read")
	require.NoError(t, pg.InOrgTx(ctx, app, orgA, func(ctx context.Context) error {
		assert.Equal(t, 3, count(ctx))
		return nil
	}))

	var rows int
	require.NoError(t, owner.QueryRow(ctx, "SELECT count(*) FROM adapter.credential").Scan(&rows))
	assert.Equal(t, 1, rows)
	_, err = app.Exec(ctx, "CREATE TABLE adapter.extra (id int)")
	assert.Error(t, err, "the process role owns no schema")
}

func TestHandler_VersionNamesContract(t *testing.T) {
	t.Parallel()
	h := newHarness(t, adaptersdk.NewStore(nil, nil), &fakeSource{})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.url+"/version", http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "1", body["contract_version"])
	assert.Contains(t, body, "commit_sha")
	assert.Contains(t, body, "schema_version")
}
