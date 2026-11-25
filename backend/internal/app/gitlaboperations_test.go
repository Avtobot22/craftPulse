package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_gitLabWriteToken    = "fixture-gitlab-writer-145"
	_gitLabNewWriteToken = "fixture-gitlab-writer-146"
	_gitLabWebhookSecret = "fixture-gitlab-hook-145"
	_glProject           = "/api/v4/projects/7"
	_glMRPath            = _glProject + "/merge_requests/1"
	_glNotes             = _glMRPath + "/notes"
)

type gitLabOperationFixture struct {
	gitLabFixture
	recorded       map[string]json.RawMessage
	revoked        atomic.Bool
	peak           atomic.Int32
	active         atomic.Int32
	countAccess    atomic.Bool
	noWriteScope   atomic.Bool
	botRoleMissing atomic.Bool
	botCannotSeeMR atomic.Bool
	writesDenied   atomic.Bool
	dropReply      atomic.Bool
	movedHead      atomic.Bool

	mu       sync.Mutex
	noteBody string
	creates  int
	updates  int
}

func newGitLabOperationFixture(t *testing.T) *gitLabOperationFixture {
	t.Helper()
	f := &gitLabOperationFixture{}
	data, err := os.ReadFile("testdata/gitlab-operations/responses.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &f.recorded))
	return f
}

func (f *gitLabOperationFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Header.Get("PRIVATE-TOKEN") {
	case _gitLabToken:
		f.serveAccess(w, r)
	case _gitLabWriteToken, _gitLabNewWriteToken:
		f.serveBot(w, r)
	default:
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (f *gitLabOperationFixture) serveAccess(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case _glProject:
		if f.countAccess.Load() {
			n := f.active.Add(1)
			defer f.active.Add(-1)
			for {
				old := f.peak.Load()
				if old >= n || f.peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	case "/api/v4/projects/98":
		<-r.Context().Done()
		return
	case "/api/v4/projects/97":
		w.WriteHeader(http.StatusBadGateway)
		return
	case _glProject + "/members/all/7":
		if f.revoked.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/notes"), strings.Contains(r.URL.Path, "/notes/"), r.URL.Path == "/api/v4/merge_requests":
		w.WriteHeader(http.StatusForbidden)
	case f.recorded[r.URL.Path] != nil:
		_, _ = w.Write(f.recorded[r.URL.Path])
	default:
		f.gitLabFixture.ServeHTTP(w, r)
	}
}

func (f *gitLabOperationFixture) serveBot(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v4/user":
		if r.Header.Get("PRIVATE-TOKEN") == _gitLabNewWriteToken {
			_, _ = io.WriteString(w, `{"id":10,"state":"active"}`)
			return
		}
		_, _ = w.Write(f.recorded["bot"])
	case "/api/v4/personal_access_tokens/self":
		if f.noWriteScope.Load() {
			_, _ = io.WriteString(w, `{"scopes":["read_api"]}`)
			return
		}
		_, _ = w.Write(f.recorded["write_scope"])
	case _glProject, "/api/v4/projects/8":
		_, _ = w.Write(f.recorded[r.URL.Path])
	case _glProject + "/members/all/10":
		_, _ = io.WriteString(w, `{"id":10,"state":"active","access_level":30}`)
	case _glProject + "/members/all/9":
		if f.botRoleMissing.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(f.recorded["bot_membership"])
	case "/api/v4/merge_requests":
		if r.URL.Query().Get("scope") != "all" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.botCannotSeeMR.Load() {
			_, _ = io.WriteString(w, "[]")
			return
		}
		_, _ = w.Write(f.recorded["merge_requests"])
	case _glMRPath:
		body := f.recorded["merge_request"]
		if f.movedHead.Load() {
			body = []byte(strings.ReplaceAll(string(body), _glHead, _glBase))
		}
		_, _ = w.Write(body)
	case _glNotes, _glNotes + "/1234":
		f.serveNotes(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *gitLabOperationFixture) serveNotes(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodGet {
		f.listNotes(w, r)
		return
	}
	if f.writesDenied.Load() {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"403 Forbidden: `+_gitLabWriteToken+`"}`)
		return
	}
	var note struct{ Body string }
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil || note.Body == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == _glNotes:
		f.creates++
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPut && r.URL.Path == _glNotes+"/1234" && f.noteBody != "":
		f.updates++
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f.noteBody = note.Body
	if f.dropReply.Swap(false) {
		_, _ = io.WriteString(w, `{"id":`)
		return
	}
	_, _ = w.Write(f.recorded["note"])
}

func (f *gitLabOperationFixture) listNotes(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") != "2" {
		w.Header().Set("Link", `<https://example.com`+_glNotes+`?page=2>; rel="next"`)
		if f.noteBody == "" {
			_, _ = w.Write(f.recorded["other_note"])
			return
		}
		// A human copying the exact marker must not take ownership of the bot's note.
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1235, "body": f.noteBody, "author": map[string]int{"id": 2}}})
		return
	}
	if f.noteBody == "" {
		_, _ = io.WriteString(w, "[]")
		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1234, "body": f.noteBody, "author": map[string]int{"id": 9}}})
}

func gitLabCredential(t *testing.T, e gitLabTestEnv, purpose adaptersdk.CredentialsInputPurpose, secret string) {
	t.Helper()
	status, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{Purpose: purpose, Secret: &secret})
	require.Equal(t, http.StatusNoContent, status, string(body))
}

func readGitLabOperation[T any](t *testing.T, e gitLabTestEnv, operation string, input any) T {
	t.Helper()
	status, body := e.call(t, http.MethodPost, operation, input)
	require.Equal(t, http.StatusOK, status, string(body))
	assertGitLabOperationSecrets(t, string(body))
	var out T
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

func assertGitLabOperationSecrets(t *testing.T, value string) {
	t.Helper()
	for _, secret := range []string{_gitLabToken, _gitLabWriteToken, _gitLabNewWriteToken, _gitLabWebhookSecret} {
		assert.NotContains(t, value, secret)
	}
}

func TestContract_GitLabCheckAccess(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	e.probe(t)

	for _, tc := range []struct {
		name, user string
		project    string
		want       adaptersdk.AccessDecisionDecision
	}{
		{"direct Reporter", "7", "7", adaptersdk.AccessDecisionDecisionAllowed},
		{"inherited Reporter", "8", "7", adaptersdk.AccessDecisionDecisionAllowed},
		{"non-member", "10", "7", adaptersdk.AccessDecisionDecisionDenied},
		{"blocked Maintainer", "4", "7", adaptersdk.AccessDecisionDecisionDenied},
		{"member below Reporter", "11", "7", adaptersdk.AccessDecisionDecisionDenied},
		{"public non-member", "10", "12", adaptersdk.AccessDecisionDecisionAllowed},
		{"public blocked account", "4", "12", adaptersdk.AccessDecisionDecisionDenied},
		{"anonymous public", "", "12", adaptersdk.AccessDecisionDecisionAllowed},
		{"internal member", "7", "13", adaptersdk.AccessDecisionDecisionAllowed},
		{"internal non-member", "10", "13", adaptersdk.AccessDecisionDecisionDenied},
		{"GitLab failure", "7", "97", adaptersdk.AccessDecisionDecisionUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := *e.target.CheckAccess
			in.AccountNativeId = tc.user
			in.Items[0].NativeId = tc.project
			out := readGitLabOperation[adaptersdk.CheckAccessResult](t, e, "check-access", in)
			require.Len(t, out.Decisions, 1)
			assert.Equal(t, tc.want, out.Decisions[0].Decision)
			assert.Equal(t, adaptersdk.CheckAccessResultModeServiceAccount, out.Mode)
			require.NotNil(t, out.MaxAgeSeconds)
			assert.Equal(t, 60, *out.MaxAgeSeconds)
			assert.Equal(t, out.CheckedAt, out.Decisions[0].CheckedAt)
		})
	}

	f.revoked.Store(true)
	in := *e.target.CheckAccess
	in.AccountNativeId = "7"
	in.Items[0].NativeId = "7"
	out := readGitLabOperation[adaptersdk.CheckAccessResult](t, e, "check-access", in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionDenied, out.Decisions[0].Decision)

	in.Items[0].NativeId = "98"
	start := time.Now()
	out = readGitLabOperation[adaptersdk.CheckAccessResult](t, e, "check-access", in)
	assert.Less(t, time.Since(start), 2500*time.Millisecond)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, out.Decisions[0].Decision)
	require.NotNil(t, out.Decisions[0].ReasonCode)
	assert.Equal(t, "access_timeout", *out.Decisions[0].ReasonCode)
	assertGitLabOperationSecrets(t, e.logs.String())
}

func TestContract_GitLabAccessLimitSharedByConcurrentCalls(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	e.probe(t)

	in := *e.target.CheckAccess
	in.Items = make([]adaptersdk.AccessItem, 24)
	for i := range in.Items {
		in.Items[i] = e.target.CheckAccess.Items[0]
	}
	f.countAccess.Store(true)
	t.Run("concurrent calls", func(t *testing.T) {
		for i := range 4 {
			t.Run(fmt.Sprintf("request %d", i), func(t *testing.T) {
				t.Parallel()
				out := readGitLabOperation[adaptersdk.CheckAccessResult](t, e, "check-access", in)
				require.Len(t, out.Decisions, len(in.Items))
				for _, decision := range out.Decisions {
					assert.Equal(t, adaptersdk.AccessDecisionDecisionAllowed, decision.Decision)
				}
			})
		}
	})
	assert.Equal(t, int32(8), f.peak.Load())
}

func TestContract_GitLabParseWebhook(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	in := adaptersdk.WebhookInput{
		Headers:    map[string]string{"x-gitlab-token": _gitLabWebhookSecret},
		BodyBase64: f.recorded["mr_webhook"],
	}
	status, body := e.call(t, http.MethodPost, "webhooks/parse", in)
	require.Equal(t, http.StatusUnprocessableEntity, status, string(body))
	assert.Contains(t, string(body), "invalid_signature")
	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWebhookSecret, _gitLabWebhookSecret)
	for _, headers := range []map[string]string{
		{},
		{"X-Gitlab-Token": "wrong"},
		{"X-Gitlab-Token": _gitLabWebhookSecret, "x-gitlab-token": _gitLabWebhookSecret},
	} {
		bad := in
		bad.Headers = headers
		status, body := e.call(t, http.MethodPost, "webhooks/parse", bad)
		assert.Equal(t, http.StatusUnprocessableEntity, status, string(body))
		assert.Contains(t, string(body), "invalid_signature")
		assertGitLabOperationSecrets(t, string(body))
	}
	for _, tc := range []struct {
		fixture, event string
		keys           []adaptersdk.ObjectKey
	}{
		{"mr_webhook", "merge_request", []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"}}},
		{"note_webhook", "merge_request", []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"}}},
		{"pipeline_webhook", "pipeline", []adaptersdk.ObjectKey{
			{ObjectKind: adaptersdk.ObjectKeyObjectKindCiRun, NativeId: "32"},
			{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"},
		}},
		{"job_webhook", "pipeline", []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindCiRun, NativeId: "32"}}},
		{"issue_note_webhook", "", []adaptersdk.ObjectKey{}},
		{"other_webhook", "", []adaptersdk.ObjectKey{}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			in.BodyBase64 = f.recorded[tc.fixture]
			out := readGitLabOperation[adaptersdk.WebhookResult](t, e, "webhooks/parse", in)
			assert.Equal(t, tc.keys, out.Keys)
			assert.Equal(t, tc.event == "", out.Ignored)
			if tc.event == "" {
				assert.Nil(t, out.EventKind)
			} else {
				require.NotNil(t, out.EventKind)
				assert.Equal(t, tc.event, *out.EventKind)
			}
		})
	}
	in.BodyBase64 = []byte(`{"object_kind":"build","build_id":36}`)
	status, body = e.call(t, http.MethodPost, "webhooks/parse", in)
	assert.Equal(t, http.StatusBadRequest, status, string(body))
	assertGitLabOperationSecrets(t, e.logs.String())
}

func TestContract_GitLabPublishAndFindPublished(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	e.probe(t)
	head := _glHead
	target := adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"}
	in := adaptersdk.PublishInput{
		Action: adaptersdk.PublishInputActionMrComment, Marker: "summary-145", Target: target,
		ExpectedHeadSha: &head, BodyMarkdown: "Initial summary\n  /merge",
	}
	find := adaptersdk.FindPublishedInput{Action: adaptersdk.FindPublishedInputActionMrComment, Target: target, Marker: in.Marker}

	missing := readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, missing.Outcome)
	require.NotNil(t, missing.ReasonCode)
	assert.Equal(t, "write_permission_missing", *missing.ReasonCode)

	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWriteBot, `{"token":"`+_gitLabWriteToken+`"}`)
	f.botCannotSeeMR.Store(true)
	status, body := e.call(t, http.MethodPost, "find-published", find)
	assert.Equal(t, http.StatusForbidden, status, string(body))
	assert.Contains(t, string(body), `"code":"auth"`)
	f.botCannotSeeMR.Store(false)
	assert.False(t, readGitLabOperation[adaptersdk.FindPublishedResult](t, e, "find-published", find).Found)

	// A request to update a missing note must never turn into a POST.
	in.ExternalId = new("1234")
	out := readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	in.ExternalId = nil

	// Lose the create response after GitLab accepts it. The next Publish must update it.
	f.dropReply.Store(true)
	out = readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeUnknown, out.Outcome)
	found := readGitLabOperation[adaptersdk.FindPublishedResult](t, e, "find-published", find)
	require.True(t, found.Found)
	require.NotNil(t, found.ExternalId)
	assert.Equal(t, "1234", *found.ExternalId)
	require.NotNil(t, found.ExternalUrl)
	assert.Equal(t, "https://example.com/demo/project-7/-/merge_requests/1#note_1234", *found.ExternalUrl)

	in.BodyMarkdown = "Updated summary"
	human := "1235"
	in.ExternalId = &human
	out = readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeSucceeded, out.Outcome)
	assert.Equal(t, found.ExternalId, out.ExternalId)
	assert.Equal(t, found.ExternalUrl, out.ExternalUrl)

	in.BodyMarkdown = "Latest summary"
	out = readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeSucceeded, out.Outcome)
	assert.Equal(t, found.ExternalId, out.ExternalId)

	f.writesDenied.Store(true)
	out = readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)

	f.writesDenied.Store(false)
	f.movedHead.Store(true)
	status, body = e.call(t, http.MethodPost, "publish", in)
	assert.Equal(t, http.StatusConflict, status, string(body))
	assert.Contains(t, string(body), "head_conflict")

	f.movedHead.Store(false)
	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWriteBot, _gitLabNewWriteToken)
	assert.False(t, readGitLabOperation[adaptersdk.FindPublishedResult](t, e, "find-published", find).Found)
	in.ExternalId = found.ExternalId
	out = readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
	assert.Equal(t, adaptersdk.PublishResultOutcomeFailed, out.Outcome)
	require.NotNil(t, out.ReasonCode)
	assert.Equal(t, "write_permission_missing", *out.ReasonCode)
	f.mu.Lock()
	assert.Equal(t, 1, f.creates)
	assert.Equal(t, 2, f.updates)
	assert.Contains(t, f.noteBody, "<!-- craftpulse:mr-summary:"+e.target.Organization+":10 ")
	assert.Contains(t, f.noteBody, in.BodyMarkdown)
	f.mu.Unlock()
	assertGitLabOperationSecrets(t, e.logs.String())
}

func TestContract_GitLabOptionalProbeCapabilities(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	check := func(out adaptersdk.ProbeResult, kind adaptersdk.CapabilityCapability, state adaptersdk.CapabilityState) adaptersdk.Capability {
		t.Helper()
		for _, cap := range out.Capabilities {
			if cap.Capability == kind {
				assert.Equal(t, state, cap.State)
				return cap
			}
		}
		t.Fatalf("missing capability %s", kind)
		return adaptersdk.Capability{}
	}

	out := e.probe(t)
	check(out, adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityStateMissing)
	hook := check(out, adaptersdk.CapabilityCapabilityWebhook, adaptersdk.CapabilityStateMissing)
	require.NotNil(t, hook.ReasonCode)
	assert.Contains(t, *hook.ReasonCode, "nightly_reconciliation")

	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWriteBot, _gitLabWriteToken)
	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWebhookSecret, _gitLabWebhookSecret)
	out = readGitLabOperation[adaptersdk.ProbeResult](t, e, "probe", e.target.Probe)
	check(out, adaptersdk.CapabilityCapabilityWebhook, adaptersdk.CapabilityStateVerified)
	check(out, adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityStateVerified)

	probe := e.target.Probe
	probe.Scopes = append(probe.Scopes, adaptersdk.ScopeRef{Kind: adaptersdk.ScopeRefKindRepository, NativeScopeId: "8"})
	out = readGitLabOperation[adaptersdk.ProbeResult](t, e, "probe", probe)
	check(out, adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityStatePartial)

	probe.Scopes = probe.Scopes[1:]
	out = readGitLabOperation[adaptersdk.ProbeResult](t, e, "probe", probe)
	check(out, adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityStateMissing)

	f.noWriteScope.Store(true)
	out = readGitLabOperation[adaptersdk.ProbeResult](t, e, "probe", e.target.Probe)
	check(out, adaptersdk.CapabilityCapabilityWriteMrComment, adaptersdk.CapabilityStateMissing)
	f.mu.Lock()
	assert.Zero(t, f.creates)
	assert.Zero(t, f.updates)
	f.mu.Unlock()
	assertGitLabOperationSecrets(t, e.logs.String())
}

func TestContract_GitLabConcurrentPublish(t *testing.T) {
	t.Parallel()
	f := newGitLabOperationFixture(t)
	e := newGitLabTestEnvWithSource(t, f)
	e.probe(t)
	gitLabCredential(t, e, adaptersdk.CredentialsInputPurposeWriteBot, _gitLabWriteToken)

	in := adaptersdk.PublishInput{
		Action: adaptersdk.PublishInputActionMrComment, Marker: "concurrent-145", BodyMarkdown: "Summary",
		Target: adaptersdk.ObjectKey{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"},
	}
	t.Run("overlapping calls", func(t *testing.T) {
		for i := range 4 {
			t.Run(fmt.Sprintf("call %d", i), func(t *testing.T) {
				t.Parallel()
				out := readGitLabOperation[adaptersdk.PublishResult](t, e, "publish", in)
				require.Equal(t, adaptersdk.PublishResultOutcomeSucceeded, out.Outcome)
				require.NotNil(t, out.ExternalId)
				assert.Equal(t, "1234", *out.ExternalId)
			})
		}
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.creates)
	assert.Equal(t, 3, f.updates)
}
