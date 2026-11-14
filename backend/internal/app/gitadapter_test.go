package app

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/netip"
	"os"
	oexec "os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	gitadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/git"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk/contracttest"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

const (
	_gitToken    = "glpat-READ-" + _secretMarker + "-5c0ffee"
	_gitHost     = "git.test"
	_gitPublic   = "203.0.113.7"
	_gitInternal = "10.9.0.5"
)

// gitFixture is a set of bare repositories served by https through git http-backend and by ssh through
// git-upload-pack, both reached by the name git.test.
type gitFixture struct {
	root, work string
	caFile     string
	httpsPort  string
	sshPort    string
	privateKey string
	knownHosts string
	mu         sync.Mutex
	clock      time.Time
}

func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	fx := &gitFixture{root: t.TempDir(), work: t.TempDir(), clock: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
	fx.serveHTTPS(t)
	fx.serveSSH(t)
	return fx
}

// git runs git as a developer of the fixture: a fixed identity and a commit time that grows by an hour.
func (fx *gitFixture) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fx.mu.Lock()
	fx.clock = fx.clock.Add(time.Hour)
	at := fx.clock.Format(time.RFC3339)
	fx.mu.Unlock()
	cmd := oexec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + fx.work, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.org", "GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.org",
		"GIT_AUTHOR_DATE=" + at, "GIT_COMMITTER_DATE=" + at,
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// repo creates a working copy with the files, commits it and publishes it as the bare repository name.
func (fx *gitFixture) repo(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(fx.work, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	fx.git(t, dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, files)
	fx.git(t, dir, "add", "-A")
	fx.git(t, dir, "commit", "-q", "-m", "initial")
	bare := filepath.Join(fx.root, name)
	fx.git(t, fx.work, "init", "-q", "--bare", "-b", "main", bare)
	for _, kv := range [][2]string{{"uploadpack.allowFilter", "true"}, {"uploadpack.allowAnySHA1InWant", "true"}, {"http.receivepack", "false"}} {
		fx.git(t, bare, "config", kv[0], kv[1])
	}
	fx.publish(t, name)
	return dir
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(dir, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		mode := os.FileMode(0o644)
		if strings.HasPrefix(body, "#!") {
			mode = 0o755
		}
		require.NoError(t, os.WriteFile(full, []byte(body), mode))
	}
}

func (fx *gitFixture) commit(t *testing.T, name, msg string, files map[string]string, remove ...string) string {
	t.Helper()
	dir := filepath.Join(fx.work, name)
	writeFiles(t, dir, files)
	for _, p := range remove {
		fx.git(t, dir, "rm", "-q", p)
	}
	fx.git(t, dir, "add", "-A")
	fx.git(t, dir, "commit", "-q", "-m", msg)
	fx.publish(t, name)
	return fx.git(t, dir, "rev-parse", "HEAD")
}

func (fx *gitFixture) publish(t *testing.T, name string) {
	t.Helper()
	fx.git(t, filepath.Join(fx.work, name), "push", "-q", "--mirror", filepath.Join(fx.root, name))
}

// serveHTTPS answers git smart http for the token of the fixture only.
func (fx *gitFixture) serveHTTPS(t *testing.T) {
	t.Helper()
	ca := newTestCA(t, "source CA")
	fx.caFile = filepath.Join(fx.work, "ca.pem")
	require.NoError(t, os.WriteFile(fx.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}), 0o600))
	gitBin, err := oexec.LookPath("git")
	require.NoError(t, err)
	backend := &cgi.Handler{
		Path: gitBin, Args: []string{"http-backend"},
		Env: []string{
			"GIT_PROJECT_ROOT=" + fx.root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"PATH=" + os.Getenv("PATH"),
		},
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "oauth2" || pass != _gitToken {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	srv.TLS, _ = ca.serverTLS(t, time.Now().Add(time.Hour), _gitHost)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	_, fx.httpsPort, err = net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
}

// serveSSH runs git-upload-pack for the key of the fixture and the user git.
func (fx *gitFixture) serveSSH(t *testing.T) {
	t.Helper()
	hostPub, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	require.NoError(t, err)
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(clientPriv, "")
	require.NoError(t, err)
	fx.privateKey = string(pem.EncodeToMemory(block))
	allowed, err := ssh.NewPublicKey(clientPub)
	require.NoError(t, err)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() == "git" && bytes.Equal(k.Marshal(), allowed.Marshal()) {
			return &ssh.Permissions{}, nil
		}
		return nil, errors.New("key is not authorized")
	}}
	cfg.AddHostKey(hostSigner)
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, fx.sshPort, err = net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	hostKey, err := ssh.NewPublicKey(hostPub)
	require.NoError(t, err)
	fx.knownHosts = knownhosts.Line([]string{knownhosts.Normalize(_gitHost + ":" + fx.sshPort)}, hostKey)
	var wg sync.WaitGroup
	// The listener closes before the wait: the accept loop ends only then.
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() { fx.handleSSH(conn, cfg) })
		}
	})
}

func (fx *gitFixture) handleSSH(conn net.Conn, cfg *ssh.ServerConfig) {
	defer func() { _ = conn.Close() }()
	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		fx.session(ch, requests)
	}
}

func (fx *gitFixture) session(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer func() { _ = ch.Close() }()
	var env []string
	for req := range requests {
		switch req.Type {
		case "env":
			var kv struct{ Name, Value string }
			if ssh.Unmarshal(req.Payload, &kv) == nil && kv.Name == "GIT_PROTOCOL" {
				env = append(env, "GIT_PROTOCOL="+kv.Value)
			}
			_ = req.Reply(true, nil)
		case "exec":
			var command struct{ Value string }
			fields := []string{}
			if ssh.Unmarshal(req.Payload, &command) == nil {
				fields = strings.Fields(command.Value)
			}
			if len(fields) != 2 || fields[0] != "git-upload-pack" {
				_ = req.Reply(false, nil)
				return
			}
			_ = req.Reply(true, nil)
			repo := filepath.Join(fx.root, filepath.Clean("/"+strings.Trim(fields[1], "'")))
			cmd := oexec.Command("git-upload-pack", repo) //nolint:noctx // ends with the channel of the session
			cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}, env...)
			// stdin goes through a pipe: Wait must not wait for the client to close the channel, which it does
			// only after the exit status.
			cmd.Stdout, cmd.Stderr, cmd.WaitDelay = ch, ch.Stderr(), 5*time.Second
			status := uint32(1)
			if stdin, err := cmd.StdinPipe(); err == nil && cmd.Start() == nil {
				go func() {
					_, _ = io.Copy(stdin, ch)
					_ = stdin.Close()
				}()
				if cmd.Wait() == nil {
					status = 0
				}
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

// gitAdapterEnv is the adapter git behind the contract, reaching git.test through a checked address.
type gitAdapterEnv struct {
	fx      *gitFixture
	adapter *gitadapter.Adapter
	url     string
	journal *lockedBuffer
	org     string
	conn    string
	mu      sync.Mutex
	execs   [][]string
	envs    [][]string
}

func newGitAdapterEnv(t *testing.T, fx *gitFixture, edit ...func(*gitadapter.Config)) *gitAdapterEnv {
	t.Helper()
	e := &gitAdapterEnv{fx: fx, journal: &lockedBuffer{}, org: uuid.NewString(), conn: uuid.NewString()}
	public, local := netip.MustParseAddr(_gitPublic), netip.MustParseAddr("127.0.0.1")
	cfg := gitadapter.Config{
		CacheDir: t.TempDir(), CAFile: fx.caFile, PrepareWait: 20 * time.Second, RetryAfter: time.Second,
		Dial: func(a netip.Addr) netip.Addr {
			if a == public {
				return local
			}
			return a
		},
		Observe: func(args, env []string) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.execs, e.envs = append(e.execs, slices.Clone(args)), append(e.envs, slices.Clone(env))
		},
	}
	for _, fn := range edit {
		fn(&cfg)
	}
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		Resolver: mapResolver{_gitHost: _gitPublic, "inside.test": _gitInternal},
		Policy:   netguard.Policy{Installation: []netip.Prefix{netip.MustParsePrefix("10.9.0.0/16")}},
	})
	a, err := gitadapter.New(cfg, client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	e.adapter = a
	db := newAdapterDB(t, true)
	h, err := adaptersdk.NewHandler(adaptersdk.Options{
		Source: a, Store: adaptersdk.NewStore(db.pool(t, "app"), adapterKeyring(t)), ServiceKeys: []string{_adapterKey},
		Log: zerolog.New(e.journal), Probes: health.Probes{Ready: &health.Readiness{}}, SpoolDir: t.TempDir(),
	})
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	e.url = srv.URL
	return e
}

func (e *gitAdapterEnv) httpsScope(name string) string {
	return _gitHost + ":" + e.fx.httpsPort + "/" + name
}

func (e *gitAdapterEnv) sshScope(name string) string {
	return _gitHost + ":" + e.fx.sshPort + "/" + name
}

func (e *gitAdapterEnv) httpsBase() string { return "https://" + _gitHost + ":" + e.fx.httpsPort }

func (e *gitAdapterEnv) sshBase() string { return "ssh://" + _gitHost + ":" + e.fx.sshPort }

func httpsSecret() string { return `{"token":"` + _gitToken + `"}` }

func (e *gitAdapterEnv) sshSecret(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"private_key": e.fx.privateKey, "known_hosts": e.fx.knownHosts})
	require.NoError(t, err)
	return string(raw)
}

func (e *gitAdapterEnv) call(t *testing.T, op string, body any) (status int, out []byte) {
	t.Helper()
	method := http.MethodPost
	if op == "credentials" {
		method = http.MethodPut
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), method, e.url+"/v1/connections/"+e.conn+"/"+op, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+_adapterKey)
	req.Header.Set("X-Craftpulse-Organization", e.org)
	req.Header.Set("X-Craftpulse-Contract-Version", adaptersdk.CurrentContractVersion)
	req.Header.Set("X-Correlation-Id", "git-test")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, out
}

func (e *gitAdapterEnv) credentials(t *testing.T, secret string) {
	t.Helper()
	status, body := e.call(t, "credentials", map[string]any{"purpose": "collect", "secret": secret, "user_id": nil, "oauth_code": nil, "redirect_uri": nil})
	require.Equal(t, http.StatusNoContent, status, string(body))
}

func (e *gitAdapterEnv) probe(t *testing.T, base string, scopes ...string) (status int, res adaptersdk.ProbeResult, body []byte) {
	t.Helper()
	refs := make([]adaptersdk.ScopeRef, len(scopes))
	for i, s := range scopes {
		refs[i] = adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: s}
	}
	status, body = e.call(t, "probe", adaptersdk.ProbeInput{BaseUrl: base, Scopes: refs})
	if status == http.StatusOK {
		require.NoError(t, json.Unmarshal(body, &res))
	}
	return status, res, body
}

// pkgRecord is a record of a fetched package with its body.
type pkgRecord struct {
	envelope.Record
	document envelope.DocumentV1
	ref      envelope.GitRefV1
}

type fetched struct {
	status   int
	err      adaptersdk.AdapterError
	manifest envelope.Manifest
	records  []pkgRecord
}

func (e *gitAdapterEnv) fetch(t *testing.T, scope string, cursor *adaptersdk.Cursor, keys ...adaptersdk.ObjectKey) fetched {
	t.Helper()
	status, body := e.call(t, "fetch", adaptersdk.FetchInput{
		SchemaId: adaptersdk.FetchInputSchemaIdDocuments, Scope: adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: scope},
		Cursor: cursor, Limit: 100, ObjectKeys: append([]adaptersdk.ObjectKey{}, keys...),
	})
	out := fetched{status: status}
	if status != http.StatusOK {
		require.NoError(t, json.Unmarshal(body, &out.err), string(body))
		return out
	}
	check := envelope.Check(bytes.NewReader(body), envelope.DefaultLimits)
	require.Nil(t, check.Envelope, "the package passes the envelope check")
	for _, r := range check.Records {
		require.Nil(t, r.Rejection, "record %d is accepted", r.No)
	}
	out.manifest, out.records = readPackage(t, body)
	return out
}

func readPackage(t *testing.T, body []byte) (m envelope.Manifest, records []pkgRecord) {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return m, records
		}
		require.NoError(t, err)
		switch h.Name {
		case "manifest.json":
			require.NoError(t, json.NewDecoder(tr).Decode(&m))
		case "records.jsonl":
			sc := bufio.NewScanner(tr)
			sc.Buffer(nil, 1<<20)
			for sc.Scan() {
				var r pkgRecord
				require.NoError(t, json.Unmarshal(sc.Bytes(), &r.Record))
				switch r.RecordKind {
				case envelope.RecordRecordKindDocument:
					require.NoError(t, json.Unmarshal(r.Payload, &r.document))
				case envelope.RecordRecordKindGitRef:
					require.NoError(t, json.Unmarshal(r.Payload, &r.ref))
				default:
				}
				records = append(records, r)
			}
			require.NoError(t, sc.Err())
		}
	}
}

// selection follows next_cursor from cursor to the end of the selection and returns every record.
func (e *gitAdapterEnv) selection(t *testing.T, scope string, cursor *adaptersdk.Cursor) ([]pkgRecord, envelope.Manifest) {
	t.Helper()
	var all []pkgRecord
	for range 50 {
		page := e.fetch(t, scope, cursor)
		require.Equal(t, http.StatusOK, page.status, "%+v", page.err)
		all = append(all, page.records...)
		sel := page.manifest.Selection
		if sel.End {
			return all, page.manifest
		}
		cursor = &adaptersdk.Cursor{ModifiedAt: sel.NextCursor.ModifiedAt, NativeId: sel.NextCursor.NativeId}
	}
	t.Fatal("the selection does not end")
	return nil, envelope.Manifest{}
}

func documents(records []pkgRecord) map[string]pkgRecord {
	out := map[string]pkgRecord{}
	for i := range records {
		if records[i].RecordKind == envelope.RecordRecordKindDocument {
			out[records[i].NativeId] = records[i]
		}
	}
	return out
}

func refsOf(records []pkgRecord) map[string]pkgRecord {
	out := map[string]pkgRecord{}
	for i := range records {
		if records[i].RecordKind == envelope.RecordRecordKindGitRef {
			out[records[i].NativeId] = records[i]
		}
	}
	return out
}

func (e *gitAdapterEnv) manifestItems(t *testing.T, scope string) map[string]string {
	t.Helper()
	out := map[string]string{}
	var cursor *string
	for range 50 {
		status, body := e.call(t, "manifest", adaptersdk.ManifestInput{
			SchemaId: adaptersdk.ManifestInputSchemaIdDocuments, Scope: adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: scope},
			Cursor: cursor, Limit: 2,
		})
		require.Equal(t, http.StatusOK, status, string(body))
		var page adaptersdk.ManifestPage
		require.NoError(t, json.Unmarshal(body, &page))
		for _, it := range page.Items {
			out[it.NativeId] = *it.Version
		}
		if page.NextCursor == nil {
			return out
		}
		cursor = page.NextCursor
	}
	t.Fatal("the manifest does not end")
	return nil
}

// lsTree is the documents of a commit by git ls-tree of the working copy, as the adapter must see them.
func (fx *gitFixture) lsTree(t *testing.T, name, rev string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for line := range strings.SplitSeq(fx.git(t, filepath.Join(fx.work, name), "ls-tree", "-r", rev), "\n") {
		meta, p, _ := strings.Cut(line, "\t")
		fields := strings.Fields(meta)
		if len(fields) == 3 && fields[1] == "blob" && (strings.HasSuffix(p, ".md") || strings.HasPrefix(filepath.Base(p), "openapi")) {
			out[p] = fields[2]
		}
	}
	return out
}

func (e *gitAdapterEnv) secretsAbsent(t *testing.T, secrets ...string) {
	t.Helper()
	journal := e.journal.String()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range secrets {
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			if len(line) < 12 {
				continue
			}
			assert.NotContains(t, journal, line, "the journal has no credential values")
			for i := range e.execs {
				assert.NotContains(t, strings.Join(e.execs[i], " "), line, "arguments of git have no credential values")
				assert.NotContains(t, strings.Join(e.envs[i], " "), line, "the environment of git has no credential values")
			}
		}
	}
}

func (e *gitAdapterEnv) ran(sub string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, args := range e.execs {
		if slices.Contains(args, sub) {
			n++
		}
	}
	return n
}

func docsRepo() map[string]string {
	return map[string]string{
		"README.md": "# Billing\n", "docs/guide.md": "Guide v1\n", "api/openapi.yaml": "openapi: 3.1.0\n",
		"src/main.go": "package main\n", "docs/notes.txt": "not a document\n",
	}
}

func TestGitAdapter_PassesTheContract(t *testing.T) {
	fx := newGitFixture(t)
	fx.repo(t, "group/docs", docsRepo())
	e := newGitAdapterEnv(t, fx)
	scope := adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: e.httpsScope("group/docs")}
	contracttest.Run(t, contracttest.Target{
		BaseURL: e.url, ServiceKey: _adapterKey, Organization: e.org, Connection: e.conn, Secret: httpsSecret(),
		Probe:    adaptersdk.ProbeInput{BaseUrl: e.httpsBase(), Scopes: []adaptersdk.ScopeRef{scope}},
		Fetch:    adaptersdk.FetchInput{SchemaId: adaptersdk.FetchInputSchemaIdDocuments, Scope: scope, Limit: 2, ObjectKeys: []adaptersdk.ObjectKey{}},
		Manifest: adaptersdk.ManifestInput{SchemaId: adaptersdk.ManifestInputSchemaIdDocuments, Scope: scope, Limit: 2},
	})
	journal := e.journal.String()
	assert.Contains(t, journal, `"route":"/v1/connections/:connection_id/fetch","status":200`)
	assert.Contains(t, journal, `"message":"credentials replaced"`)
	e.secretsAbsent(t, _gitToken)
}

func TestGitAdapter_InventoryByHTTPSAndSSH(t *testing.T) {
	fx := newGitFixture(t)
	work := fx.repo(t, "group/docs", docsRepo())
	first := fx.git(t, work, "rev-parse", "HEAD")
	fx.git(t, work, "tag", "-a", "v1.0", "-m", "release")
	fx.git(t, work, "branch", "feature")
	head := fx.commit(t, "group/docs", "guide v2", map[string]string{"docs/guide.md": "Guide v2\n"})
	fx.publish(t, "group/docs")

	for _, transport := range []string{"https", "ssh"} {
		t.Run(transport, func(t *testing.T) {
			e := newGitAdapterEnv(t, fx)
			base, scope, secret := e.httpsBase(), e.httpsScope("group/docs"), httpsSecret()
			if transport == "ssh" {
				base, scope, secret = e.sshBase(), e.sshScope("group/docs"), e.sshSecret(t)
			}
			e.credentials(t, secret)
			status, probe, body := e.probe(t, base, scope)
			require.Equal(t, http.StatusOK, status, string(body))
			require.Len(t, probe.Scopes, 1)
			assert.Equal(t, adaptersdk.ScopeCheckStateVerified, probe.Scopes[0].State, "%v", probe.Scopes[0].ReasonCode)
			assert.Equal(t, adaptersdk.ProbeResultProviderGit, probe.Provider)
			assert.Nil(t, probe.AccessMode, "the adapter checks no rights")
			assert.Contains(t, e.journal.String(), `"default_branch":"refs/heads/main","head":"`+head+`"`)

			records, m := e.selection(t, scope, nil)
			assert.Equal(t, scope, m.Area.NativeId)
			assert.Equal(t, envelope.AreaRefKindRepository, m.Area.Kind)
			docs := documents(records)
			want := fx.lsTree(t, "group/docs", head)
			require.Len(t, docs, len(want), "the inventory is the documents of git ls-tree of head")
			for p, blob := range want {
				d := docs[p]
				assert.Equal(t, blob, d.document.VersionRef, p)
				assert.Equal(t, blob, *d.SourceRevision, p)
				assert.Equal(t, scope, d.NativeNs)
			}
			assert.Equal(t, envelope.DocumentV1DocFormatOpenapi, docs["api/openapi.yaml"].document.DocFormat)
			assert.Equal(t, head, *docs["docs/guide.md"].document.CommitSha, "the last commit that changed the file")
			assert.Equal(t, first, *docs["README.md"].document.CommitSha)
			assert.Equal(t, e.manifestItems(t, scope), want, "Manifest gives the same paths and blobs")

			refs := refsOf(records)
			assert.Equal(t, head, refs["refs/heads/main"].ref.CommitSha)
			assert.True(t, refs["refs/heads/main"].ref.DefaultBranch)
			assert.Equal(t, first, refs["refs/heads/feature"].ref.CommitSha)
			assert.False(t, refs["refs/heads/feature"].ref.DefaultBranch)
			assert.Equal(t, first, refs["refs/tags/v1.0"].ref.CommitSha, "an annotated tag points to its commit")
			assert.Equal(t, envelope.GitRefV1RefKindTag, refs["refs/tags/v1.0"].ref.RefKind)

			again, _ := e.selection(t, scope, nil)
			for p, d := range documents(again) {
				assert.Equal(t, *docs[p].SourceRevision, *d.SourceRevision, "an unchanged document keeps its source_revision")
			}
			e.secretsAbsent(t, _gitToken, fx.privateKey)
		})
	}
}

func TestGitAdapter_IncrementReadsOnlyChangedPaths(t *testing.T) {
	fx := newGitFixture(t)
	fx.repo(t, "group/docs", docsRepo())
	e := newGitAdapterEnv(t, fx)
	e.credentials(t, httpsSecret())
	scope := e.httpsScope("group/docs")
	_, m := e.selection(t, scope, nil)
	oldHead := *m.ReadId
	require.True(t, m.Selection.End)

	head := fx.commit(t, "group/docs", "edit docs", map[string]string{"docs/guide.md": "Guide v2\n", "docs/new.md": "New\n", "src/main.go": "package main // v2\n"},
		"api/openapi.yaml")

	changed, m2 := e.selection(t, scope, &adaptersdk.Cursor{ModifiedAt: time.Now(), NativeId: oldHead})
	assert.Equal(t, head, *m2.ReadId)
	docs := documents(changed)
	require.ElementsMatch(t, []string{"docs/guide.md", "docs/new.md", "api/openapi.yaml"}, keys(docs), "only changed and deleted documents are read")
	assert.Nil(t, docs["docs/guide.md"].document.Deleted)
	deleted := docs["api/openapi.yaml"].document
	require.NotNil(t, deleted.Deleted)
	assert.True(t, *deleted.Deleted)
	assert.Equal(t, head, deleted.VersionRef, "a deleted document has the commit of its deletion as the version")
	assert.NotEmpty(t, refsOf(changed), "refs come with every selection")

	// sync keeps the cursor of the last page, the time of the old head, and starts the next round from it.
	byTime, _ := e.selection(t, scope, &adaptersdk.Cursor{ModifiedAt: m.Selection.Cursor.ModifiedAt})
	assert.ElementsMatch(t, keys(docs), keys(documents(byTime)), "a cursor of sync without a SHA diffs from the last commit before its time")

	reread := e.fetch(t, scope, nil,
		adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindDocument, NativeNs: scope, NativeId: "docs/new.md"},
		adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindDocument, NativeNs: scope, NativeId: "-oops.md"})
	require.Equal(t, http.StatusOK, reread.status)
	assert.Equal(t, []string{"docs/new.md"}, keys(documents(reread.records)), "a key with a path like an option is skipped")
	assert.True(t, reread.manifest.Selection.End)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestGitAdapter_RepositoryContentDoesNotRun arms git of the host the way a careless installation would:
// a global hooksPath into the repository and an LFS filter that leaves a marker. A plain clone runs both and
// loads a submodule by file://; the adapter, on the same repositories and environment, runs none of it.
func TestGitAdapter_RepositoryContentDoesNotRun(t *testing.T) {
	fx := newGitFixture(t)
	markers := t.TempDir()
	hook := "#!/bin/sh\ntouch \"$MARKER_DIR/hook.marker\"\n"
	fx.repo(t, "group/hooks", map[string]string{"README.md": "hooks\n", ".githooks/post-checkout": hook, ".githooks/post-merge": hook})
	fx.repo(t, "group/lfs", map[string]string{"README.md": "version https://git-lfs.github.com/spec/v1\n", ".gitattributes": "*.md filter=lfs diff=lfs merge=lfs -text\n"})
	lib := fx.repo(t, "group/lib", map[string]string{"README.md": "lib\n"})
	libHead := fx.git(t, lib, "rev-parse", "HEAD")
	sub := fx.repo(t, "group/sub", map[string]string{
		"README.md":   "sub\n",
		".gitmodules": "[submodule \"lib\"]\n\tpath = lib\n\turl = file://" + filepath.Join(fx.root, "group", "lib") + "\n",
	})
	fx.git(t, sub, "update-index", "--add", "--cacheinfo", "160000,"+libHead+",lib")
	fx.git(t, sub, "commit", "-q", "-m", "submodule")
	fx.publish(t, "group/sub")

	armed := filepath.Join(fx.work, "armed.gitconfig")
	require.NoError(t, os.WriteFile(armed, []byte("[core]\n\thooksPath = .githooks\n[filter \"lfs\"]\n\tsmudge = \"sh -c 'touch $MARKER_DIR/lfs.marker; cat'\"\n\trequired = true\n[protocol \"file\"]\n\tallow = always\n"), 0o600))
	control, controlMarkers := t.TempDir(), t.TempDir()
	plain := func(args ...string) {
		cmd := oexec.CommandContext(t.Context(), "git", args...)
		cmd.Dir, cmd.Env = control, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + control, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + armed, "MARKER_DIR=" + controlMarkers}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	plain("clone", "-q", "file://"+filepath.Join(fx.root, "group", "hooks"), "hooks")
	plain("clone", "-q", "file://"+filepath.Join(fx.root, "group", "lfs"), "lfs")
	plain("clone", "-q", "--recurse-submodules", "file://"+filepath.Join(fx.root, "group", "sub"), "sub")
	require.FileExists(t, filepath.Join(controlMarkers, "hook.marker"), "the fixture is armed: a plain clone runs the hook")
	require.FileExists(t, filepath.Join(controlMarkers, "lfs.marker"), "a plain clone runs the filter")
	require.FileExists(t, filepath.Join(control, "sub", "lib", "README.md"), "a plain clone loads the submodule")

	t.Setenv("GIT_CONFIG_GLOBAL", armed)
	t.Setenv("MARKER_DIR", markers)
	cache := t.TempDir()
	e := newGitAdapterEnv(t, fx, func(c *gitadapter.Config) { c.CacheDir = cache })
	e.credentials(t, httpsSecret())
	for _, name := range []string{"group/hooks", "group/lfs", "group/sub"} {
		records, _ := e.selection(t, e.httpsScope(name), nil)
		assert.Contains(t, keys(documents(records)), "README.md", name)
	}
	entries, err := os.ReadDir(markers)
	require.NoError(t, err)
	assert.Empty(t, entries, "no hook and no filter ran")
	var modules, lfsObjects []string
	require.NoError(t, filepath.WalkDir(cache, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == "modules" {
			modules = append(modules, p)
		}
		if err == nil && d.IsDir() && d.Name() == "lfs" {
			lfsObjects = append(lfsObjects, p)
		}
		return nil
	}))
	assert.Empty(t, modules, "no submodule is loaded")
	assert.Empty(t, lfsObjects, "no LFS object is loaded")
	for _, args := range e.execs {
		assert.False(t, slices.Contains(args, "checkout") || slices.Contains(args, "--recurse-submodules"), "%v", args)
	}

	status, _, body := e.probe(t, "file://"+filepath.Join(fx.root, "group", "lib"), "x/group/lib")
	assert.Equal(t, http.StatusBadRequest, status, string(body))
	assert.Contains(t, string(body), `"code":"invalid_request"`, "a file:// remote is refused")
}

func TestGitAdapter_LargeRepositoryAnswersPreparingAndRespectsLimits(t *testing.T) {
	fx := newGitFixture(t)
	files := map[string]string{"README.md": "# big\n"}
	for i := range 1500 {
		files["pkg/"+strconv.Itoa(i%50)+"/file"+strconv.Itoa(i)+".go"] = "package p // " + strconv.Itoa(i) + "\n"
	}
	work := fx.repo(t, "group/big", files)
	for i := range 20 {
		fx.commit(t, "group/big", "change "+strconv.Itoa(i), map[string]string{"docs/change" + strconv.Itoa(i) + ".md": "x\n"})
	}
	_ = work

	e := newGitAdapterEnv(t, fx, func(c *gitadapter.Config) { c.PrepareWait = time.Nanosecond })
	e.credentials(t, httpsSecret())
	scope := e.httpsScope("group/big")
	first := e.fetch(t, scope, nil)
	require.Equal(t, http.StatusServiceUnavailable, first.status)
	assert.Equal(t, adaptersdk.AdapterErrorCodePreparing, first.err.Code, "the first call does not wait for the partial clone")
	require.NotNil(t, first.err.RetryAt)
	var ready fetched
	require.Eventually(t, func() bool {
		ready = e.fetch(t, scope, nil)
		return ready.status == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond, "the next call answers from the cache")
	assert.Len(t, documents(ready.records), 21)
	clones := e.ran("clone")
	e.fetch(t, scope, nil)
	assert.Equal(t, clones, e.ran("clone"), "the clone is reused")
	assert.Positive(t, e.adapter.CacheUsage())
	assert.LessOrEqual(t, e.adapter.CacheUsage(), int64(2<<30))
	rss := maxRSS(t, e.journal.String())
	t.Logf("peak memory of a git command %d KiB, cache %d KiB", rss/1024, e.adapter.CacheUsage()/1024)
	assert.Less(t, rss, int64(256<<20), "a git command fits the memory of the container")

	small := newGitAdapterEnv(t, fx, func(c *gitadapter.Config) { c.CloneBytes, c.PrepareWait = 16<<10, 20*time.Second })
	small.credentials(t, httpsSecret())
	tooLarge := small.fetch(t, scope, nil)
	require.Equal(t, http.StatusBadGateway, tooLarge.status)
	assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidResponse, tooLarge.err.Code, "a clone over the limit is a classified error")
	assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidResponse, small.fetch(t, scope, nil).err.Code, "the failure is kept, not cloned again at once")

	slow := newGitAdapterEnv(t, fx, func(c *gitadapter.Config) { c.CloneTimeout, c.PrepareWait = time.Millisecond, 20*time.Second })
	slow.credentials(t, httpsSecret())
	timeout := slow.fetch(t, scope, nil)
	require.Equal(t, http.StatusGatewayTimeout, timeout.status)
	assert.Equal(t, adaptersdk.AdapterErrorCodeTimeout, timeout.err.Code)
}

func maxRSS(t *testing.T, journal string) int64 {
	t.Helper()
	var peak int64
	for line := range strings.SplitSeq(journal, "\n") {
		var ev struct {
			RSS int64 `json:"max_rss_bytes"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil {
			peak = max(peak, ev.RSS)
		}
	}
	require.Positive(t, peak, "the journal has the memory of git commands")
	return peak
}

func TestGitAdapter_RefusesUnsafeAddressesBeforeGit(t *testing.T) {
	fx := newGitFixture(t)
	fx.repo(t, "group/docs", docsRepo())
	e := newGitAdapterEnv(t, fx)
	e.credentials(t, httpsSecret())
	e.mu.Lock()
	before := len(e.execs)
	e.mu.Unlock()
	for _, tt := range []struct{ name, base, scope string }{
		{"loopback", "https://127.0.0.1:" + fx.httpsPort, "127.0.0.1:" + fx.httpsPort + "/group/docs"},
		{"installation network", "https://inside.test", "inside.test/group/docs"},
	} {
		status, _, body := e.probe(t, tt.base, tt.scope)
		assert.Equal(t, http.StatusOK, status, string(body))
		var res adaptersdk.ProbeResult
		require.NoError(t, json.Unmarshal(body, &res))
		require.Len(t, res.Scopes, 1)
		assert.Equal(t, adaptersdk.ScopeCheckStateMissing, res.Scopes[0].State, tt.name)
		assert.Equal(t, "invalid_request", *res.Scopes[0].ReasonCode, tt.name)
		fetched := e.fetch(t, tt.scope, nil)
		assert.Equal(t, adaptersdk.AdapterErrorCodeInvalidRequest, fetched.err.Code, tt.name)
	}
	_, res, _ := e.probe(t, e.httpsBase(), e.httpsScope("-upload-pack=touch"), e.httpsScope("group/docs.git"))
	for _, s := range res.Scopes {
		assert.Equal(t, "not_normalized", *s.ReasonCode, s.NativeScopeId)
	}
	bad := e.fetch(t, e.httpsScope("group/docs"), &adaptersdk.Cursor{ModifiedAt: time.Now(), NativeId: "--upload-pack=touch"})
	require.Equal(t, http.StatusOK, bad.status, "a cursor that is not a commit falls back to time")
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, args := range e.execs[before:] {
		assert.False(t, slices.ContainsFunc(args, func(a string) bool {
			return strings.Contains(a, "https://127.0.0.1") || strings.Contains(a, "inside.test") || strings.Contains(a, "upload-pack=touch")
		}), "git never starts for a refused address or value: %v", args)
	}
}
