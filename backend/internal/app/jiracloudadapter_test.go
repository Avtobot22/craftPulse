package app

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/adapter/jiracloud"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_jiraFixtureEmail       = "collector@example.test"
	_jiraFixtureToken       = "fixture-only-credential"
	_jiraFixtureCredentials = `{"email":"` + _jiraFixtureEmail + `","token":"` + _jiraFixtureToken + `"}`
)

type jiraFixture struct {
	responses     map[string]json.RawMessage
	searchPages   [2]jiraSearchFixture
	changelog     map[string]json.RawMessage
	probeHistory  []json.RawMessage
	failure       atomic.Int32
	historyDenied atomic.Bool
	dc            atomic.Bool
	updated       atomic.Bool
	renamed       atomic.Bool
	requests      atomic.Int32
}

type jiraSearchFixture struct {
	Issues []json.RawMessage `json:"issues"`
}

func readJiraFixture(t *testing.T) *jiraFixture {
	t.Helper()
	f := &jiraFixture{changelog: make(map[string]json.RawMessage)}
	read := func(name string) []byte {
		body, err := os.ReadFile(filepath.Join("testdata", "jiracloud", name+".json"))
		require.NoError(t, err)
		return body
	}
	require.NoError(t, json.Unmarshal(read("responses"), &f.responses))
	require.NoError(t, json.Unmarshal(f.responses["search1"], &f.searchPages[0]))
	require.NoError(t, json.Unmarshal(f.responses["search2"], &f.searchPages[1]))
	for _, start := range []string{"0", "50", "100"} {
		f.changelog[start] = read("changelog-" + start)
	}
	var history struct {
		Values []json.RawMessage `json:"values"`
	}
	require.NoError(t, json.Unmarshal(f.changelog["0"], &history))
	f.probeHistory = history.Values
	return f
}

func (f *jiraFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/_edge/tenant_info" && r.Header.Get("Authorization") == "" {
		_, _ = w.Write(f.responses["tenant"])
		return
	}
	if email, token, valid := r.BasicAuth(); !valid || email != _jiraFixtureEmail || token != _jiraFixtureToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	const api = "/rest/api/3/"
	switch r.URL.Path {
	case api + "serverInfo":
		if f.dc.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(f.responses["server"])
	case "/rest/api/2/serverInfo":
		_, _ = io.WriteString(w, `{"deploymentType":"DataCenter","version":"10.7.0"}`)
	case api + "myself":
		_, _ = w.Write(f.responses["actor"])
	case api + "project/10000":
		_, _ = w.Write(f.responses["project"])
	case api + "mypermissions":
		_, _ = w.Write(f.responses["permissions"])
	case api + "project/10000/statuses":
		_, _ = w.Write(f.responses["status"])
	case api + "resolution/search":
		_, _ = w.Write(f.responses["resolution"])
	case api + "field":
		_, _ = w.Write(f.responses["fields"])
	case api + "user/viewissue/search":
		f.accounts(w, r)
	case api + "search/jql":
		f.search(w, r)
	case api + "issue/10002":
		_, _ = w.Write(f.issue(0))
	case api + "issue/10010":
		_, _ = w.Write(f.issue(1))
	case api + "issue/10002/changelog", api + "issue/10010/changelog":
		f.history(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *jiraFixture) accounts(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("projectKey") != "DEMO" || r.URL.Query().Get("query") != "*" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("startAt") != "0" {
		_, _ = io.WriteString(w, "[]")
		return
	}
	body := f.responses["account"]
	if f.renamed.Load() {
		body = bytes.ReplaceAll(body, []byte("Demo user"), []byte("Renamed user"))
	}
	_, _ = w.Write(body)
}

func (f *jiraFixture) issue(i int) json.RawMessage {
	body := f.searchPages[i].Issues[0]
	if f.updated.Load() && i == 0 {
		body = bytes.ReplaceAll(body, []byte("2026-11-21T10:00:00.123+0700"), []byte("2026-11-21T10:01:00.456+0700"))
	}
	return body
}

func (f *jiraFixture) search(w http.ResponseWriter, r *http.Request) {
	switch status := f.failure.Load(); {
	case status < 0:
		<-r.Context().Done()
		return
	case status > 0:
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(int(status))
		_, _ = w.Write(f.responses["error"])
		return
	}
	q := r.URL.Query()
	if !strings.HasPrefix(q.Get("jql"), "project = 10000") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	i := 0
	last := q.Get("nextPageToken") == "fixture-next"
	if last {
		i = 1
	}
	if f.updated.Load() {
		i = 1 - i
	}
	if strings.Contains(q.Get("jql"), "DESC") {
		i, last = 1, true
	}
	next := "fixture-next"
	if last {
		next = ""
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"issues": []json.RawMessage{f.issue(i)}, "isLast": last, "nextPageToken": next})
}

func (f *jiraFixture) history(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "/10002/") {
		_, _ = w.Write(f.responses["emptyHistory"])
		return
	}
	if f.historyDenied.Load() {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(f.responses["error"])
		return
	}
	q := r.URL.Query()
	if q.Get("maxResults") == "1" && q.Get("startAt") == "0" {
		_ = json.NewEncoder(w).Encode(map[string]any{"startAt": 0, "total": 122, "isLast": false, "values": f.probeHistory[:1]})
		return
	}
	if body := f.changelog[q.Get("startAt")]; body != nil && q.Get("maxResults") == "50" {
		_, _ = w.Write(body)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
}

type jiraTestEnv struct {
	target  contracttest.Target
	fixture *jiraFixture
	logs    *lockedBuffer
}

func newJiraTestEnv(t *testing.T) jiraTestEnv {
	t.Helper()
	fixture := readJiraFixture(t)
	upstream := httptest.NewTLSServer(fixture)
	t.Cleanup(upstream.Close)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		RootCAs: roots, Resolver: mapResolver{"example.com": _gitPublic}, Timeout: 500 * time.Millisecond,
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	logs := &lockedBuffer{}
	db := newAdapterDB(t, true)
	handler, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: jiracloud.New(client), Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)),
		ServiceKeys: []string{_adapterKey}, Log: zerolog.New(logs), SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindTrackerProject, NativeScopeId: "10000"}
	area := adaptersdk.AccessItemAreaKindTrackerProject
	return jiraTestEnv{
		fixture: fixture, logs: logs,
		target: contracttest.Target{
			BaseURL: server.URL, ServiceKey: _adapterKey, Organization: uuid.NewString(), Connection: uuid.NewString(),
			Secret: _jiraFixtureCredentials, Probe: adaptersdk.ProbeInput{BaseUrl: "https://EXAMPLE.com:443/", Scopes: []adaptersdk.ScopeRef{scope}},
			Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdTasks, Scope: scope, Limit: 1, ObjectKeys: []adaptersdk.ObjectKey{}},
			Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdTasks, Scope: scope, Limit: 1},
			CheckAccess: &adaptersdk.CheckAccessInput{UserId: uuid.New(), Items: []adaptersdk.AccessItem{
				{ItemKind: adaptersdk.AccessItemItemKindArea, AreaKind: &area, NativeId: "10000"},
			}},
		},
	}
}

func (e jiraTestEnv) call(t *testing.T, method, operation string, input any) (status int, headers http.Header, response []byte) {
	t.Helper()
	body, err := json.Marshal(input)
	require.NoError(t, err)
	target := e.target
	req, err := http.NewRequestWithContext(t.Context(), method,
		target.BaseURL+"/v1/connections/"+target.Connection+"/"+operation, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+target.ServiceKey)
	req.Header.Set("X-Craftpulse-Organization", target.Organization)
	req.Header.Set("X-Craftpulse-Contract-Version", adaptersdk.CurrentContractVersion)
	req.Header.Set("X-Correlation-Id", "jira-fixture-"+operation)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, out
}

func (e jiraTestEnv) credentials(t *testing.T) {
	t.Helper()
	status, _, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{
		Purpose: adaptersdk.CredentialsInputPurposeCollect, Secret: &e.target.Secret,
	})
	require.Equal(t, http.StatusNoContent, status, string(body))
}

func (e jiraTestEnv) probe(t *testing.T) adaptersdk.ProbeResult {
	t.Helper()
	status, _, body := e.call(t, http.MethodPost, "probe", e.target.Probe)
	require.Equal(t, http.StatusOK, status, string(body))
	var probe adaptersdk.ProbeResult
	require.NoError(t, json.Unmarshal(body, &probe))
	return probe
}

func (e jiraTestEnv) fetch(t *testing.T, fetch adaptersdk.FetchInput) (*envelope.Package, *envelope.Result) {
	t.Helper()
	status, _, body := e.call(t, http.MethodPost, "fetch", fetch)
	require.Equal(t, http.StatusOK, status, string(body))
	res := envelope.Check(bytes.NewReader(body), envelope.DefaultLimits)
	require.Nil(t, res.Envelope)
	for _, rec := range res.Records {
		require.Nil(t, rec.Rejection, "%s %s: %+v", rec.Kind, rec.NativeID, rec.Rejection)
	}
	pkg, err := envelope.ReadCleaned(bytes.NewReader(body), envelope.DefaultLimits)
	require.NoError(t, err)
	return pkg, res
}

func TestContract_JiraCloudAdapter(t *testing.T) {
	t.Parallel()
	e := newJiraTestEnv(t)
	contracttest.Run(t, e.target)
	logs := e.logs.String()
	assert.Contains(t, logs, "Jira request")
	assert.NotContains(t, logs, _jiraFixtureToken)
	assert.NotContains(t, logs, _jiraFixtureEmail)
	assert.NotContains(t, logs, "Authorization")
}

func TestJiraCloudAdapterProbeAndFailures(t *testing.T) {
	t.Parallel()
	e := newJiraTestEnv(t)
	e.credentials(t)
	probe := e.probe(t)
	assert.Equal(t, "a71a171a-0000-4000-8000-000000000001", probe.InstanceKey)
	require.NotNil(t, probe.Version)
	assert.Contains(t, *probe.Version, "100296")
	require.NotNil(t, probe.Edition)
	assert.Equal(t, "cloud", *probe.Edition)
	e.fixture.historyDenied.Store(true)
	probe = e.probe(t)
	for _, capability := range probe.Capabilities {
		switch capability.Capability {
		case adaptersdk.CapabilityCapabilityReadCurrent, adaptersdk.CapabilityCapabilityReadLinks:
			assert.Equal(t, adaptersdk.CapabilityStateVerified, capability.State)
		default:
			assert.Equal(t, adaptersdk.CapabilityStateMissing, capability.State)
			assert.NotNil(t, capability.ReasonCode)
		}
	}
	in := e.target.Fetch
	in.SchemaId = adaptersdk.FetchInputSchemaIdTaskEvents
	in.Limit = 2
	got, _ := e.fetch(t, in)
	assert.Equal(t, envelope.ManifestCompletenessPartial, got.Manifest.Completeness)
	require.Len(t, got.Manifest.Selection.Truncated, 1)
	assert.Equal(t, "10010", got.Manifest.Selection.Truncated[0].NativeId)
	count := e.fixture.requests.Load()
	status, _, body := e.call(t, http.MethodPost, "check-access", e.target.CheckAccess)
	require.Equal(t, http.StatusOK, status, string(body))
	var access adaptersdk.CheckAccessResult
	require.NoError(t, json.Unmarshal(body, &access))
	require.Len(t, access.Decisions, 1)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, access.Decisions[0].Decision)
	assert.Equal(t, count, e.fixture.requests.Load())
	for _, test := range []struct {
		upstream int32
		status   int
		code     adaptersdk.AdapterErrorCode
	}{
		{401, 403, adaptersdk.AdapterErrorCodeAuth},
		{403, 403, adaptersdk.AdapterErrorCodeAuth},
		{429, 429, adaptersdk.AdapterErrorCodeRateLimited},
		{-1, 504, adaptersdk.AdapterErrorCodeTimeout},
	} {
		e.fixture.failure.Store(test.upstream)
		status, header, body := e.call(t, http.MethodPost, "fetch", in)
		require.Equal(t, test.status, status, string(body))
		var actual adaptersdk.AdapterError
		require.NoError(t, json.Unmarshal(body, &actual))
		assert.Equal(t, test.code, actual.Code)
		if test.upstream == 429 {
			delay, err := strconv.Atoi(header.Get("Retry-After"))
			require.NoError(t, err)
			assert.InDelta(t, 30, delay, 2)
		}
	}
	e.fixture.dc.Store(true)
	status, _, body = e.call(t, http.MethodPost, "probe", e.target.Probe)
	require.Equal(t, http.StatusUnprocessableEntity, status, string(body))
	var actual adaptersdk.AdapterError
	require.NoError(t, json.Unmarshal(body, &actual))
	assert.Equal(t, adaptersdk.AdapterErrorCodeUnsupportedEdition, actual.Code)
}
