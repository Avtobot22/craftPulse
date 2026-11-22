package app

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_jiraUserToken   = "fixture-only-oauth-token"
	_jiraAccessRoute = "/ex/jira/a71a171a-0000-4000-8000-000000000001/rest/api/3/"
	_jiraAccountID   = "fixture-account-123"
)

type jiraAccessResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func jiraAccessFixtures(t *testing.T) map[string]jiraAccessResponse {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "jiracloud", "access.json"))
	require.NoError(t, err)
	var fixtures map[string]jiraAccessResponse
	require.NoError(t, json.Unmarshal(data, &fixtures))
	return fixtures
}

func (r jiraAccessResponse) write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.Status)
	_, _ = w.Write(r.Body)
}

func jiraAccessRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	path, matches := strings.CutPrefix(r.URL.Path, _jiraAccessRoute)
	if r.Method != http.MethodGet || !matches {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}
	if r.Header.Get("Authorization") != "Bearer "+_jiraUserToken {
		w.WriteHeader(http.StatusUnauthorized)
		return "", false
	}
	return path, true
}

func (e jiraTestEnv) accessInput(t *testing.T) adaptersdk.CheckAccessInput {
	t.Helper()
	e.credentials(t)
	e.probe(t)
	in := *e.target.CheckAccess
	in.AccountNativeId = _jiraAccountID
	status, _, body := e.call(t, http.MethodPut, "credentials", adaptersdk.CredentialsInput{
		Purpose: adaptersdk.CredentialsInputPurposeUserToken, Secret: new(_jiraUserToken), UserId: &in.UserId,
	})
	require.Equal(t, http.StatusNoContent, status, string(body))
	return in
}

func (e jiraTestEnv) checkAccess(t *testing.T, in adaptersdk.CheckAccessInput) adaptersdk.CheckAccessResult {
	t.Helper()
	status, _, body := e.call(t, http.MethodPost, "check-access", in)
	require.Equal(t, http.StatusOK, status, string(body))
	var result adaptersdk.CheckAccessResult
	require.NoError(t, json.Unmarshal(body, &result))
	require.Len(t, result.Decisions, len(in.Items))
	assert.Equal(t, adaptersdk.CheckAccessResultModeUserToken, result.Mode)
	assert.False(t, result.CheckedAt.IsZero())
	for _, d := range result.Decisions {
		assert.False(t, d.CheckedAt.IsZero())
		assert.False(t, d.CheckedAt.After(result.CheckedAt))
	}
	return result
}

func jiraProjectAccess(id string) adaptersdk.AccessItem {
	return adaptersdk.AccessItem{
		ItemKind: adaptersdk.AccessItemItemKindArea, AreaKind: new(adaptersdk.AccessItemAreaKindTrackerProject), NativeId: id,
	}
}

func jiraTaskAccess(id string) adaptersdk.AccessItem {
	return adaptersdk.AccessItem{
		ItemKind: adaptersdk.AccessItemItemKindObject, ObjectKind: new(adaptersdk.AccessItemObjectKindTask), NativeId: id,
	}
}

func TestJiraCloudCheckAccess(t *testing.T) {
	t.Parallel()
	fixtures := jiraAccessFixtures(t)
	var reads atomic.Int32
	var revoked atomic.Bool
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := jiraAccessRequest(w, r)
		if !ok {
			return
		}
		if path == "myself" {
			fixtures["account"].write(w)
			return
		}
		reads.Add(1)
		switch path {
		case "project/10000":
			if revoked.Load() {
				fixtures["hidden"].write(w)
				return
			}
			fixtures["project"].write(w)
		case "project/10001":
			fixtures["forbidden"].write(w)
		case "project/10002":
			fixtures["source_failure"].write(w)
		case "project/10003":
			fixtures["expired"].write(w)
		case "project/10004":
			fixtures["project"].write(w)
		case "issue/10006", "issue/10007":
			if r.URL.RawQuery != "fields=id" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if path == "issue/10007" {
				fixtures["hidden"].write(w)
				return
			}
			fixtures["issue"].write(w)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	e := newJiraTestEnvWithAccess(t, upstream, adaptersdk.SourceTimeout)
	in := e.accessInput(t)
	in.Items = []adaptersdk.AccessItem{
		jiraProjectAccess("10000"), jiraTaskAccess("10007"), jiraProjectAccess("10001"),
		jiraTaskAccess("10006"), jiraProjectAccess("10002"), jiraProjectAccess("10003"), jiraProjectAccess("10004"),
		jiraTaskAccess("../10006"),
	}
	result := e.checkAccess(t, in)
	require.NotNil(t, result.MaxAgeSeconds)
	assert.Zero(t, *result.MaxAgeSeconds)
	want := []string{
		"jira_access_revocation_unverified", "jira_read_denied", "jira_read_denied",
		"jira_access_revocation_unverified", "jira_read_unverified", "token_expired", "jira_read_unverified",
		"jira_access_item_unsupported",
	}
	for i, reason := range want {
		state := adaptersdk.AccessDecisionDecisionUnknown
		if reason == "jira_read_denied" {
			state = adaptersdk.AccessDecisionDecisionDenied
		}
		assert.Equal(t, state, result.Decisions[i].Decision, "item %d", i)
		require.NotNil(t, result.Decisions[i].ReasonCode)
		assert.Equal(t, reason, *result.Decisions[i].ReasonCode, "item %d", i)
	}
	assert.EqualValues(t, 7, reads.Load())

	in.AccountNativeId = "fixture-account-12"
	result = e.checkAccess(t, in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, result.Decisions[0].Decision)
	assert.Equal(t, new("jira_account_unverified"), result.Decisions[0].ReasonCode)
	assert.EqualValues(t, 7, reads.Load())

	in.AccountNativeId = _jiraAccountID
	in.Items = in.Items[:1]
	revoked.Store(true)
	result = e.checkAccess(t, in)
	assert.Equal(t, adaptersdk.AccessDecisionDecisionDenied, result.Decisions[0].Decision)
	assert.Equal(t, new(60), result.MaxAgeSeconds)
	assert.EqualValues(t, 8, reads.Load())
	assert.NotContains(t, e.logs.String(), _jiraUserToken)
}

func TestJiraCloudCheckAccessBudgetAndConnectionLimit(t *testing.T) {
	t.Parallel()
	fixtures := jiraAccessFixtures(t)
	started := make(chan struct{}, 24)
	var active, peak atomic.Int32
	var slow atomic.Bool
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := jiraAccessRequest(w, r)
		if !ok {
			return
		}
		if path == "myself" {
			fixtures["account"].write(w)
			return
		}
		if path != "project/10000" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if slow.Load() {
			<-r.Context().Done()
			return
		}
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := peak.Load()
			if current <= seen || peak.CompareAndSwap(seen, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			_, _ = io.WriteString(w, `{"id":"10000"}`)
		case <-r.Context().Done():
		}
	})
	e := newJiraTestEnvWithAccess(t, upstream, adaptersdk.SourceTimeout)
	in := e.accessInput(t)
	in.Items = make([]adaptersdk.AccessItem, 12)
	for i := range in.Items {
		in.Items[i] = jiraProjectAccess("10000")
	}
	type outcome struct {
		status int
		body   []byte
		err    error
	}
	results := make(chan outcome, 2)
	for range cap(results) {
		req := e.request(t, http.MethodPost, "check-access", in)
		go func() {
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			results <- outcome{status: resp.StatusCode, body: body, err: err}
		}()
	}
	wait := time.NewTimer(1500 * time.Millisecond)
	defer wait.Stop()
	for range 8 {
		select {
		case <-started:
		case <-wait.C:
			t.Fatal("parallel checks did not reach the connection limit")
		}
	}
	select {
	case <-started:
		t.Error("checks exceeded the connection limit")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range cap(results) {
		actual := <-results
		require.NoError(t, actual.err)
		require.Equal(t, http.StatusOK, actual.status, string(actual.body))
		var result adaptersdk.CheckAccessResult
		require.NoError(t, json.Unmarshal(actual.body, &result))
		require.Len(t, result.Decisions, len(in.Items))
		for _, decision := range result.Decisions {
			assert.Equal(t, new("jira_access_revocation_unverified"), decision.ReasonCode)
		}
	}
	assert.EqualValues(t, 8, peak.Load())
	slow.Store(true)
	start := time.Now()
	result := e.checkAccess(t, in)
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Equal(t, new(0), result.MaxAgeSeconds)
	for _, decision := range result.Decisions {
		assert.Equal(t, adaptersdk.AccessDecisionDecisionUnknown, decision.Decision)
		assert.Equal(t, new("jira_access_timeout"), decision.ReasonCode)
	}
}
