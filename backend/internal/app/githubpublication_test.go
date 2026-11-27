package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_githubWriteToken    = "fixture-github-write-secret"
	_githubCommentPath   = "/repos/demo/project-7/issues/1/comments"
	_githubCommentUpdate = "/repos/demo/project-7/issues/comments/555"
	_githubFixtureHead   = "2222222222222222222222222222222222222222"
)

//go:embed testdata/github/publication.json
var githubPublicationResponses []byte

type githubPublicationFixture struct {
	publicKey        *rsa.PublicKey
	responses        map[string]json.RawMessage
	mu               sync.Mutex
	body             string
	writes           []string
	cutReply         bool
	wrongMarkerReply bool
	staleBodyReply   bool
	dropReply        bool
	denyWrite        bool
	denyGrant        bool
	broadGrant       bool
	headUnavailable  bool
	archived         bool
}

func (f *githubPublicationFixture) serve(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "Bearer "+_githubToken {
		if r.Method == http.MethodGet {
			return false
		}
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		w.WriteHeader(http.StatusBadRequest)
		return true
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	switch path {
	case "/app", "/app/installations/42/access_tokens":
		f.serveIssuer(w, r)
		return true
	default:
		if auth != "Bearer "+_githubWriteToken {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
	}
	switch path {
	case "/installation/repositories":
		page := r.URL.Query().Get("page")
		if page == "1" {
			w.Header().Set("Link", `<https://api.github.com/installation/repositories?page=2>; rel="next"`)
		}
		path += "?page=" + page
		if page == "2" && f.archived {
			path += "&archived=true"
		}
	case "/repos/demo/project-7/pulls/1":
		if f.headUnavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
	case _githubCommentPath, _githubCommentUpdate:
		f.serveComment(w, r)
		return true
	}
	if r.Method != http.MethodGet || f.responses[path] == nil {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	_, _ = w.Write(f.responses[path])
	return true
}

func (f *githubPublicationFixture) serveIssuer(w http.ResponseWriter, r *http.Request) {
	token, err := jwt.ParseSigned(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), []jose.SignatureAlgorithm{jose.RS256})
	var claims jwt.Claims
	if err != nil || token.Claims(f.publicKey, &claims) != nil || claims.Issuer != "fixture-client-id" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/app" {
		if f.denyGrant {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		var body struct {
			Permissions map[string]string `json:"permissions"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil ||
			len(body.Permissions) != 1 || body.Permissions["pull_requests"] != "write" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		if f.broadGrant {
			_, _ = w.Write(f.responses[r.URL.Path+"?broad"])
			return
		}
	}
	_, _ = w.Write(f.responses[r.URL.Path])
}

func (f *githubPublicationFixture) serveComment(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if r.URL.Query().Get("page") != "2" {
			w.Header().Set("Link", `<https://api.github.com/repositories/7/issues/1/comments?page=2>; rel="next"`)
			// A human-authored lookalike must not be adopted as the bot's comment.
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 444, "body": f.body, "user": map[string]string{"login": "fixture-summary", "type": "User"}}})
			return
		}
		if f.body == "" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_ = json.NewEncoder(w).Encode([]any{f.comment()})
		return
	}
	if f.denyWrite {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (r.Method != http.MethodPost || r.URL.Path != _githubCommentPath) &&
		(r.Method != http.MethodPatch || r.URL.Path != _githubCommentUpdate || f.body == "") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	previousBody := f.body
	f.body = req.Body
	f.writes = append(f.writes, r.Method)
	if f.dropReply {
		panic(http.ErrAbortHandler)
	}
	if f.cutReply {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":`))
		return
	}
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
	}
	response := f.comment()
	if f.wrongMarkerReply {
		response["body"] = "<!-- unrelated -->"
	}
	if f.staleBodyReply {
		response["body"] = previousBody
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (f *githubPublicationFixture) comment() map[string]any {
	return map[string]any{"id": 555, "body": f.body, "user": map[string]string{"login": "fixture-summary[bot]", "type": "Bot"}}
}

func TestGitHubPublicationContract(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &githubPublicationFixture{publicKey: &key.PublicKey}
	require.NoError(t, json.Unmarshal(githubPublicationResponses, &f.responses))
	e, _ := newGitHubFixtureWithPublication(t, f)
	initialProbe := e.probe(t)
	assert.Nil(t, initialProbe.AccessMode)
	assertGitHubProbeCapabilities(t, initialProbe, adaptersdk.CapabilityStateVerified)

	secret, err := json.Marshal(map[string]any{
		"client_id": "fixture-client-id", "installation_id": 42,
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
	})
	require.NoError(t, err)
	target := adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "500"}
	head := _githubFixtureHead
	in := adaptersdk.PublishInput{
		Action: adaptersdk.PublishInputActionMrComment, Target: target, Marker: "summary-500",
		ExpectedHeadSha: &head, BodyMarkdown: "first",
	}

	out := callGitHubPublish(t, e, in)
	require.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)

	value := string(secret)
	status, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{Purpose: adaptersdk.CredentialsInputPurposeWriteBot, Secret: &value})
	require.Equal(t, http.StatusNoContent, status, string(body))

	status, body = e.call(t, http.MethodPost, "probe", e.target.Probe)
	require.Equal(t, http.StatusOK, status, string(body))
	var probe adaptersdk.ProbeResult
	require.NoError(t, json.Unmarshal(body, &probe))
	capabilities := make(map[adaptersdk.CapabilityCapability]adaptersdk.Capability)
	for _, c := range probe.Capabilities {
		capabilities[c.Capability] = c
	}
	assert.Equal(t, adaptersdk.CapabilityStateVerified, capabilities[adaptersdk.CapabilityCapabilityWriteMrComment].State)
	webhook := capabilities[adaptersdk.CapabilityCapabilityWebhook]
	assert.Equal(t, adaptersdk.CapabilityStateMissing, webhook.State)
	require.NotNil(t, webhook.ReasonCode)
	assert.Equal(t, "github_webhook_unsupported", *webhook.ReasonCode)
	assertGitHubAccessUnverified(t, e)

	find := adaptersdk.FindPublishedInput{Action: adaptersdk.FindPublishedInputActionMrComment, Target: target, Marker: in.Marker}
	assert.False(t, callGitHubFind(t, e, find).Found)
	outOfScope := find
	outOfScope.Target.NativeId = "601"
	status, body = e.call(t, http.MethodPost, "find-published", outOfScope)
	require.Equal(t, http.StatusForbidden, status, string(body))
	assert.Contains(t, string(body), "permission_missing")
	f.mu.Lock()
	assert.Empty(t, f.writes)
	f.mu.Unlock()

	f.mu.Lock()
	f.dropReply, f.headUnavailable = true, true
	f.mu.Unlock()
	withoutHead := in
	withoutHead.ExpectedHeadSha = nil
	firstAttempt := callGitHubPublish(t, e, withoutHead)
	assert.Equal(t, adaptersdk.PublishResultOutcomeUnknown, firstAttempt.Outcome)
	assert.Nil(t, firstAttempt.ExternalId)
	published := callGitHubFind(t, e, find)
	require.True(t, published.Found)
	require.NotNil(t, published.ExternalId)
	assert.Equal(t, "555", *published.ExternalId)

	f.mu.Lock()
	f.dropReply, f.headUnavailable = false, false
	f.mu.Unlock()
	out = callGitHubPublish(t, e, in)
	require.Equal(t, adaptersdk.PublishResultOutcomeSucceeded, out.Outcome)
	require.NotNil(t, out.ExternalId)
	assert.Equal(t, "555", *out.ExternalId)
	require.NotNil(t, out.ExternalUrl)
	assert.Equal(t, "https://github.com/demo/project-7/pull/1#issuecomment-555", *out.ExternalUrl)
	in.BodyMarkdown = "updated"
	assert.Equal(t, out, callGitHubPublish(t, e, in))
	f.mu.Lock()
	assert.Equal(t, []string{http.MethodPost, http.MethodPatch}, f.writes)
	assert.Contains(t, f.body, "craftpulse:mr-summary:"+e.target.Organization+":500 ")
	assert.True(t, strings.HasSuffix(f.body, "\n\nupdated"))
	f.cutReply = true
	f.mu.Unlock()

	in.BodyMarkdown = "committed before response was cut"
	unknown := callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeUnknown, unknown.Outcome)
	assert.Nil(t, unknown.ExternalId)
	found := callGitHubFind(t, e, find)
	require.True(t, found.Found)
	assert.Equal(t, out.ExternalId, found.ExternalId)
	find.Marker = "another summary"
	assert.False(t, callGitHubFind(t, e, find).Found)

	f.mu.Lock()
	f.cutReply, f.wrongMarkerReply = false, true
	f.mu.Unlock()
	in.BodyMarkdown = "committed before response reported the wrong marker"
	unknown = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeUnknown, unknown.Outcome)
	assert.Nil(t, unknown.ExternalId)
	find.Marker = in.Marker
	assert.Equal(t, found, callGitHubFind(t, e, find))

	f.mu.Lock()
	f.wrongMarkerReply, f.staleBodyReply = false, true
	f.mu.Unlock()
	in.BodyMarkdown = "committed before response reported the previous body"
	unknown = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeUnknown, unknown.Outcome)
	assert.Nil(t, unknown.ExternalId)
	assert.Equal(t, found, callGitHubFind(t, e, find))

	f.mu.Lock()
	f.staleBodyReply, f.denyWrite = false, true
	f.mu.Unlock()
	in.BodyMarkdown = "denied"
	out = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)

	head = strings.Repeat("3", 40)
	status, body = e.call(t, http.MethodPost, "publish", in)
	require.Equal(t, http.StatusConflict, status, string(body))
	assert.Contains(t, string(body), "head_conflict")

	f.mu.Lock()
	f.denyGrant = true
	assert.Equal(t, []string{http.MethodPost, http.MethodPatch, http.MethodPatch, http.MethodPatch, http.MethodPatch}, f.writes)
	f.mu.Unlock()
	out = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)

	f.mu.Lock()
	f.denyGrant, f.denyWrite, f.broadGrant = false, false, true
	f.mu.Unlock()
	head = _githubFixtureHead
	out = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)

	f.mu.Lock()
	f.broadGrant, f.archived = false, true
	f.mu.Unlock()
	status, body = e.call(t, http.MethodPost, "probe", e.target.Probe)
	require.Equal(t, http.StatusOK, status, string(body))
	require.NoError(t, json.Unmarshal(body, &probe))
	for _, c := range probe.Capabilities {
		capabilities[c.Capability] = c
	}
	writeCapability := capabilities[adaptersdk.CapabilityCapabilityWriteMrComment]
	assert.Equal(t, adaptersdk.CapabilityStateMissing, writeCapability.State)
	require.NotNil(t, writeCapability.ReasonCode)
	assert.Equal(t, "write_permission_missing", *writeCapability.ReasonCode)

	head = _githubFixtureHead
	out = callGitHubPublish(t, e, in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)
	find.Marker = in.Marker
	assert.Equal(t, found, callGitHubFind(t, e, find))
	f.mu.Lock()
	assert.Equal(t, []string{http.MethodPost, http.MethodPatch, http.MethodPatch, http.MethodPatch, http.MethodPatch}, f.writes)
	f.mu.Unlock()
	assert.NotContains(t, e.logs.String(), _githubToken)
	assert.NotContains(t, e.logs.String(), _githubWriteToken)
	assert.NotContains(t, e.logs.String(), "RSA PRIVATE KEY")
}

func assertGitHubAccessUnverified(t *testing.T, e gitLabTestEnv) {
	t.Helper()
	in := *e.target.CheckAccess
	mr := adaptersdk.AccessItemObjectKindMergeRequest
	in.Items = append(in.Items, adaptersdk.AccessItem{
		ItemKind: adaptersdk.AccessItemItemKindObject, ObjectKind: &mr, NativeId: "500",
	})
	const sourceRequest = "GitHub request"
	requests := strings.Count(e.logs.String(), sourceRequest)
	status, body := e.call(t, http.MethodPost, "check-access", in)
	require.Equal(t, http.StatusOK, status, string(body))

	var access adaptersdk.CheckAccessResult
	require.NoError(t, json.Unmarshal(body, &access))
	assert.Nil(t, access.MaxAgeSeconds)
	assert.False(t, access.CheckedAt.IsZero())
	require.Len(t, access.Decisions, len(in.Items))
	for _, decision := range access.Decisions {
		assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, decision.Decision)
		assert.True(t, access.CheckedAt.Equal(decision.CheckedAt))
		require.NotNil(t, decision.ReasonCode)
		assert.Equal(t, "github_access_mode_unverified", *decision.ReasonCode)
	}
	assert.Equal(t, requests, strings.Count(e.logs.String(), sourceRequest))
}

func callGitHubPublish(t *testing.T, e gitLabTestEnv, in adaptersdk.PublishInput) adaptersdk.PublishResult {
	t.Helper()
	status, body := e.call(t, http.MethodPost, "publish", in)
	require.Equal(t, http.StatusOK, status, string(body))
	assert.NotContains(t, string(body), _githubToken)
	assert.NotContains(t, string(body), _githubWriteToken)
	var out adaptersdk.PublishResult
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

func callGitHubFind(t *testing.T, e gitLabTestEnv, in adaptersdk.FindPublishedInput) adaptersdk.FindPublishedResult {
	t.Helper()
	status, body := e.call(t, http.MethodPost, "find-published", in)
	require.Equal(t, http.StatusOK, status, string(body))
	assert.NotContains(t, string(body), _githubToken)
	assert.NotContains(t, string(body), _githubWriteToken)
	var out adaptersdk.FindPublishedResult
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}
