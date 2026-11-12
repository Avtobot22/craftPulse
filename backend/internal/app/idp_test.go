package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testIDP is an OpenID Connect provider with discovery, keys and a token endpoint that checks PKCE and
// the client secret. A code is issued by approve, the way the provider does after the user signs in.
type testIDP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	grants map[string]grant
	client map[string]string
}

type grant struct {
	challenge string
	clientID  string
	claims    map[string]any
	key       *rsa.PrivateKey
}

// approval is what the provider saw in the authorization request.
type approval struct {
	code  string
	state string
	nonce string
}

func newTestIDP(t *testing.T, clients map[string]string) *testIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	p := &testIDP{key: key, grants: map[string]grant{}, client: clients}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.keys)
	mux.HandleFunc("POST /token", p.token)
	p.srv = httptest.NewTLSServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *testIDP) issuer() string { return p.srv.URL }

// approve checks the authorization request of craftPulse and issues a code for the subject; edit changes
// the grant to forge the response.
func (p *testIDP) approve(t *testing.T, authURL, subject string, edit ...func(*grant)) approval {
	t.Helper()
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, p.issuer()+"/authorize", u.Scheme+"://"+u.Host+u.Path)
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "openid profile", q.Get("scope"))
	assert.Equal(t, "query", q.Get("response_mode"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("state"))
	require.NotEmpty(t, q.Get("nonce"))

	now := time.Now()
	g := grant{
		challenge: q.Get("code_challenge"),
		clientID:  q.Get("client_id"),
		key:       p.key,
		claims: map[string]any{
			"iss": p.issuer(), "aud": q.Get("client_id"), "sub": subject, "nonce": q.Get("nonce"),
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "name": "Same Name",
		},
	}
	for _, e := range edit {
		e(&g)
	}
	code := rand.Text()
	p.mu.Lock()
	p.grants[code] = g
	p.mu.Unlock()
	return approval{code: code, state: q.Get("state"), nonce: q.Get("nonce")}
}

func (p *testIDP) discovery(w http.ResponseWriter, _ *http.Request) {
	iss := p.issuer()
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
		"jwks_uri": iss + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *testIDP) keys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &p.key.PublicKey, KeyID: "k1", Algorithm: string(jose.RS256), Use: "sig"},
	}})
}

func (p *testIDP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	clientID, secret, ok := r.BasicAuth()
	if !ok {
		clientID, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	p.mu.Lock()
	g, found := p.grants[r.PostForm.Get("code")]
	delete(p.grants, r.PostForm.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case p.client[clientID] == "" || p.client[clientID] != secret:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	case !found || g.clientID != clientID || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: g.key, KeyID: "k1"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	payload, _ := json.Marshal(g.claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	idToken, _ := jws.CompactSerialize()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": "MARKER-access-token", "refresh_token": "MARKER-refresh-token",
		"token_type": "Bearer", "expires_in": 3600, "id_token": idToken,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
