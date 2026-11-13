package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const _contractSecret = "jira-token-CONTRACT-7c41e9"

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testAdapterServer runs the test adapter with its own database, as the adapter process does.
func testAdapterServer(t *testing.T, logs io.Writer) *httptest.Server {
	t.Helper()
	db := newAdapterDB(t, true)
	src, err := testadapter.New(testadapter.Config{})
	require.NoError(t, err)
	h, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: src, Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)), ServiceKeys: []string{_adapterKey},
		Log: zerolog.New(logs), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func contractTarget(baseURL string) contracttest.Target {
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindTrackerProject, NativeScopeId: "10000"}
	return contracttest.Target{
		BaseURL: baseURL, ServiceKey: _adapterKey, Organization: uuid.NewString(), Connection: uuid.NewString(), Secret: _contractSecret,
		Probe:    adaptersdk.ProbeInput{BaseUrl: "https://example.atlassian.net", Scopes: []adaptersdk.ScopeRef{scope}},
		Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdTasks, Scope: scope, Limit: 3, ObjectKeys: []adaptersdk.ObjectKey{}},
		Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdTasks, Scope: scope, Limit: 4},
		CheckAccess: &adaptersdk.CheckAccessInput{UserId: uuid.New(), AccountNativeId: "demo-person-01", Items: []adaptersdk.AccessItem{
			{ItemKind: adaptersdk.AccessItemItemKindObject, NativeNs: "", NativeId: "20003"},
		}},
	}
}

func TestContract_TestAdapterPasses(t *testing.T) {
	t.Parallel()
	logs := &lockedBuffer{}
	srv := testAdapterServer(t, logs)
	contracttest.Run(t, contractTarget(srv.URL))

	journal := logs.String()
	assert.Contains(t, journal, `"operation":"fetch","schema_id":"tasks","page":3`, "the suite follows the cursor to the last page")
	assert.Contains(t, journal, `"operation":"manifest","schema_id":"tasks","page":2`)
	assert.Contains(t, journal, `"operation":"check_access"`)
	assert.Contains(t, journal, `"message":"credentials replaced"`)
	assert.NotContains(t, journal, _contractSecret, "the journal has no credential values")
}

// corrupting answers like the adapter, except that one kind of answer is broken.
func corrupting(t *testing.T, target string, edit func(path string, status int, body []byte) []byte) *httptest.Server {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		body = edit(resp.Request.URL.Path, resp.StatusCode, body)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	srv := httptest.NewServer(proxy)
	t.Cleanup(srv.Close)
	return srv
}

func TestContract_FailsOnBrokenAnswers(t *testing.T) {
	t.Parallel()
	adapter := testAdapterServer(t, io.Discard)

	missing := corrupting(t, adapter.URL, func(path string, status int, body []byte) []byte {
		if strings.HasSuffix(path, "/probe") && status == http.StatusOK {
			return bytes.Replace(body, []byte(`"instance_key":"jira.test.craftpulse.internal",`), nil, 1)
		}
		return body
	})
	violations := contracttest.Check(t.Context(), contractTarget(missing.URL))
	require.NotEmpty(t, violations, "a probe without instance_key fails the suite")
	assert.Contains(t, violations[0].Error(), `property "instance_key" is missing`)

	unknown := corrupting(t, adapter.URL, func(_ string, status int, body []byte) []byte {
		if status == http.StatusUnauthorized {
			return bytes.Replace(body, []byte(`"code":"unauthorized"`), []byte(`"code":"key_rejected"`), 1)
		}
		return body
	})
	violations = contracttest.Check(t.Context(), contractTarget(unknown.URL))
	require.NotEmpty(t, violations, "an unknown error code fails the suite")
	assert.Equal(t, "service key", violations[0].Step)
	assert.Contains(t, violations[0].Error(), `"/code"`)

	assert.Empty(t, contracttest.Check(t.Context(), contractTarget(adapter.URL)), "the same adapter without the proxy passes")
}
