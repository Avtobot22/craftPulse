package atlassiancheck

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testServer(t *testing.T) *Server {
	t.Helper()
	c := Config{
		Listen: "127.0.0.1:8092", Callback: "https://stand.example/source-links/callback",
		CloudID: "00000000-0000-0000-0000-000000000001", ClientID: "app-client",
		ClientSecret: "test-secret", ControlSecret: strings.Repeat("control", 8),
		MasterKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32)),
		Store:     filepath.Join(t.TempDir(), "tokens.enc"),
		Accounts: map[string]string{
			"owner": "id-owner", "a_allowed": "id-allowed", "a_denied": "id-denied",
			"b": "id-b", "collector": "id-collector",
		},
	}
	s, err := New(c)
	require.NoError(t, err)
	return s
}

func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func call(s *Server, method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func flow(t *testing.T, s *Server, role string) (string, *http.Cookie) {
	t.Helper()
	s.tickets["ticket"] = pending{Role: role, Expires: time.Now().Add(time.Minute)}
	w := call(s, http.MethodPost, "https://stand.example/atlassian-check/start", `{"ticket":"ticket"}`)
	require.Equal(t, http.StatusOK, w.Code)
	var result map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	u, err := url.Parse(result["url"])
	require.NoError(t, err)
	require.Equal(t, "auth.atlassian.com", u.Host)
	require.Equal(t, s.config.Callback, u.Query().Get("redirect_uri"))
	require.Equal(t, _scopes, u.Query().Get("scope"))
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.True(t, cookies[0].Secure && cookies[0].HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	return u.Query().Get("state"), cookies[0]
}

func TestCallbackBindsIdentityAndConsumesState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		account string
		want    int
	}{
		{"expected", "id-allowed", http.StatusOK},
		{"wrong_account", "id-b", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := testServer(t)
			exchanges := 0
			s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/oauth/token" {
					exchanges++
					return reply(200, `{"access_token":"access-private","refresh_token":"refresh-private","expires_in":3600,"token_type":"Bearer"}`), nil
				}
				require.Equal(t, "Bearer access-private", r.Header.Get("Authorization"))
				return reply(200, `{"active":true,"accountId":"`+tc.account+`"}`), nil
			})
			stateID, cookie := flow(t, s, "a_allowed")
			target := "https://stand.example/source-links/callback?state=" + stateID + "&code=private-code"
			wrongCookie := *cookie
			wrongCookie.Value = "other-browser"
			require.Equal(t, 403, call(s, "GET", target, "", &wrongCookie).Code)
			require.Zero(t, exchanges)
			w := call(s, "GET", target, "", cookie)
			require.Equal(t, tc.want, w.Code)
			require.NotContains(t, w.Body.String(), "private-code")
			require.NotContains(t, w.Body.String(), "access-private")
			require.Equal(t, 403, call(s, "GET", target, "", cookie).Code)
			require.Equal(t, 1, exchanges)
			_, saved := s.state.Tokens["a_allowed"]
			require.Equal(t, tc.want == 200, saved)
		})
	}
}

func TestStartRejectsExpiredReusedAndCrossOriginTickets(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	s.tickets["expired"] = pending{Role: "b", Expires: time.Now().Add(-time.Minute)}
	require.Equal(t, 403, call(s, "POST", "https://stand.example/atlassian-check/start", `{"ticket":"expired"}`).Code)
	_, _ = flow(t, s, "b")
	require.Equal(t, 403, call(s, "POST", "https://stand.example/atlassian-check/start", `{"ticket":"ticket"}`).Code)
	r := httptest.NewRequestWithContext(t.Context(), "POST", "https://stand.example/atlassian-check/start", strings.NewReader(`{"ticket":"ticket"}`))
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	require.Equal(t, 403, w.Code)
	require.Equal(t, 403, call(s, "POST", "/control/status", "").Code)
}

func TestEncryptedStoreSurvivesRestartAndFailsClosed(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	s.state.Tokens["owner"] = token{AccountID: "id-owner", Access: "private-access", Refresh: "private-refresh"}
	require.NoError(t, s.store.save(s.state))
	b, err := os.ReadFile(s.config.Store)
	require.NoError(t, err)
	require.NotContains(t, string(b), "private-access")
	require.NotContains(t, string(b), "id-owner")
	restarted, err := New(s.config)
	require.NoError(t, err)
	require.Equal(t, "private-refresh", restarted.state.Tokens["owner"].Refresh)
	b[len(b)-1] ^= 1
	require.NoError(t, os.WriteFile(s.config.Store, b, 0o600))
	_, err = New(s.config)
	require.Error(t, err)
	require.NoError(t, s.store.save(s.state))
	wrong := s.config
	wrong.MasterKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{18}, 32))
	_, err = New(wrong)
	require.Error(t, err)
}

func TestRefreshRotationSerializedAndRevocationErased(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	s.state.Tokens["owner"] = token{AccountID: "id-owner", Access: "old", Refresh: "old-refresh", Retrieved: time.Now()}
	calls := 0
	s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		var input map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		require.Equal(t, "old-refresh", input["refresh_token"])
		return reply(200, `{"access_token":"new","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`), nil
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			access, err := s.access(context.Background(), "owner")
			require.NoError(t, err)
			require.Equal(t, "new", access)
		})
	}
	wg.Wait()
	require.Equal(t, 1, calls)
	require.Equal(t, "rotated", s.state.Tokens["owner"].Refresh)
	st, err := s.store.load()
	require.NoError(t, err)
	require.Equal(t, "rotated", st.Tokens["owner"].Refresh)
	current := s.state.Tokens["owner"]
	current.Expires = time.Time{}
	s.state.Tokens["owner"] = current
	s.provider.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return reply(400, `{"error":"invalid_grant","secret":"do-not-log"}`), nil
	})
	_, err = s.access(context.Background(), "owner")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "do-not-log")
	require.Empty(t, s.state.Tokens)
}

func TestAccessChecksUseActualUserAndFailClosed(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	s.state.Tokens["a_denied"] = token{AccountID: "id-denied", Access: "denied-access", Expires: time.Now().Add(time.Hour)}
	s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer denied-access", r.Header.Get("Authorization"))
		if strings.HasSuffix(r.URL.Path, "/myself") {
			return reply(200, `{"active":true,"accountId":"id-denied"}`), nil
		}
		if strings.Contains(r.URL.Path, "/issue/") || strings.HasSuffix(r.URL.Path, "/491521") ||
			strings.HasSuffix(r.URL.Path, "/360631") || strings.HasSuffix(r.URL.Path, "/CPZ") {
			return reply(404, `{}`), nil
		}
		return reply(200, `{}`), nil
	})
	results, err := s.check(context.Background(), "a_denied")
	require.NoError(t, err)
	require.Len(t, results, 7)
	for _, result := range results {
		require.True(t, result.Matches, result.Object)
	}
	s.provider.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return reply(429, `{}`), nil
	})
	_, err = s.check(context.Background(), "a_denied")
	require.Error(t, err)
}

func TestStorageFailureBlocksUseUntilErasureIsDurable(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	inventory(s, "owner", "id-owner")
	require.NoError(t, s.persist())
	original := s.store.path
	s.store.path = filepath.Join(s.config.Store, "cannot-create-file-inside-file")
	delete(s.state.Tokens, "owner")
	require.Error(t, s.persist())
	w := call(s, "POST", "/atlassian-check/start", `{"ticket":"unused"}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	s.store.path = original
	require.NoError(t, s.persist())
	restarted, err := New(s.config)
	require.NoError(t, err)
	require.Empty(t, restarted.state.Tokens)
}

func TestReauthorizationKeepsPrivacyCycleAfterErasure(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	next := time.Now().Add(7 * 24 * time.Hour).UTC()
	s.state.Schedules["a_allowed"] = next
	s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/oauth/token" {
			return reply(200, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"token_type":"Bearer"}`), nil
		}
		return reply(200, `{"active":true,"accountId":"id-allowed"}`), nil
	})
	stateID, cookie := flow(t, s, "a_allowed")
	w := call(s, "GET", "https://stand.example/source-links/callback?state="+stateID+"&code=code", "", cookie)
	require.Equal(t, 200, w.Code)
	require.Equal(t, next, s.state.Tokens["a_allowed"].Next)
}
