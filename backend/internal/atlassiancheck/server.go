package atlassiancheck

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

type pending struct {
	Role    string
	Binding string
	Expires time.Time
}

type Server struct {
	config   Config
	provider provider
	store    store
	mu       sync.Mutex
	state    state
	tickets  map[string]pending
	flows    map[string]pending
	storeErr error
}

func New(c Config) (*Server, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	key, _ := base64.StdEncoding.DecodeString(c.MasterKey)
	keys, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: key})
	if err != nil {
		return nil, fmt.Errorf("initialize stand encryption: %w", err)
	}
	s := &Server{
		config: c, store: store{path: c.Store, keys: keys},
		provider: provider{config: c, http: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}},
		tickets: make(map[string]pending), flows: make(map[string]pending),
	}
	s.state, err = s.store.load()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /atlassian-check/{$}", landing)
	mux.Handle("POST /atlassian-check/start", http.NewCrossOriginProtection().Handler(http.HandlerFunc(s.start)))
	mux.HandleFunc("GET /source-links/callback", s.callback)
	mux.HandleFunc("POST /control/{command}", s.control)
	var rateMu sync.Mutex
	requests := make([]time.Time, 0, 10)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		rateMu.Lock()
		now := time.Now()
		requests = slices.DeleteFunc(requests, func(at time.Time) bool { return at.Before(now.Add(-time.Second)) })
		limited := len(requests) >= 10
		if !limited {
			requests = append(requests, now)
		}
		rateMu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/control/") && limited {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		mux.ServeHTTP(w, r)
	})
}

func landing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	nonce := scriptPolicy(w)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="ru"><meta charset="utf-8"><title>craftPulse OAuth</title>
<p id="status">Начинаем авторизацию тестового аккаунта…</p>
<script nonce="%s">
const ticket=location.hash.slice(1);history.replaceState(null,'',location.pathname);
if(!ticket){document.getElementById('status').textContent='Откройте персональную ссылку, выданную оператором.'}
else{fetch('/atlassian-check/start',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ticket})})
.then(async r=>{if(!r.ok)throw Error();const data=await r.json();location.replace(data.url)})
.catch(()=>document.getElementById('status').textContent='Ссылка недействительна или истекла. Запросите новую у оператора.')}
</script></html>`, nonce)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Ticket string `json:"ticket"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		http.Error(w, "stand storage unavailable", http.StatusServiceUnavailable)
		return
	}
	s.prune()
	p, ok := s.tickets[input.Ticket]
	delete(s.tickets, input.Ticket)
	if !ok || !p.Expires.After(time.Now()) {
		http.Error(w, "invalid or expired ticket", http.StatusForbidden)
		return
	}
	flow, binding := randomSecret(), randomSecret()
	p.Binding, p.Expires = binding, time.Now().Add(10*time.Minute)
	s.flows[flow] = p
	http.SetCookie(w, &http.Cookie{
		Name: "__Host-cp88", Value: binding, Path: "/",
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	query := url.Values{
		"audience": {"api.atlassian.com"}, "client_id": {s.config.ClientID},
		"scope": {_scopes}, "redirect_uri": {s.config.Callback},
		"response_type": {"code"}, "prompt": {"consent"}, "state": {flow},
	}
	writeJSON(w, map[string]string{"url": "https://auth.atlassian.com/authorize?" + query.Encode()})
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		http.Error(w, "stand storage unavailable", http.StatusServiceUnavailable)
		return
	}
	s.prune()
	p, ok := s.flows[r.URL.Query().Get("state")]
	cookie, err := r.Cookie("__Host-cp88")
	if !ok || err != nil || !p.Expires.After(time.Now()) ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(p.Binding)) != 1 {
		http.Error(w, "invalid or expired OAuth state", http.StatusForbidden)
		return
	}
	delete(s.flows, r.URL.Query().Get("state"))
	http.SetCookie(w, &http.Cookie{
		Name: "__Host-cp88", Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	code := r.URL.Query().Get("code")
	if code == "" || r.URL.Query().Get("error") != "" {
		http.Error(w, "authorization was declined; request a new link", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	t, err := s.provider.exchange(ctx, "authorization_code", code)
	if err != nil {
		http.Error(w, "token exchange failed; check app configuration and request a new link", http.StatusBadGateway)
		return
	}
	id, err := s.provider.account(ctx, t.Access)
	if err != nil || id != s.config.Accounts[p.Role] {
		http.Error(w, "account does not match this role or Jira scope is missing; request a new link", http.StatusForbidden)
		return
	}
	t.AccountID, t.Retrieved = id, time.Now().UTC()
	t.Next = s.state.Schedules[p.Role]
	if previous, exists := s.state.Tokens[p.Role]; exists {
		t.Next = previous.Next
		t.Retrieved = previous.Retrieved
	}
	s.state.Tokens[p.Role] = t
	if err = s.persist(); err != nil {
		http.Error(w, "could not persist authorization; request a new link", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	nonce := scriptPolicy(w)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="ru"><meta charset="utf-8"><title>craftPulse OAuth</title><p>Авторизация завершена. Можно закрыть вкладку и сообщить оператору.</p><script nonce="%s">history.replaceState(null,'','/atlassian-check/')</script></html>`, nonce)
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.ControlSecret)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		http.Error(w, "stand storage unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	role := r.URL.Query().Get("role")
	var result any
	var err error
	switch r.PathValue("command") {
	case "ticket":
		if s.config.Accounts[role] == "" {
			http.Error(w, "unknown role", http.StatusBadRequest)
			return
		}
		s.prune()
		ticket := randomSecret()
		s.tickets[ticket] = pending{Role: role, Expires: time.Now().Add(30 * time.Minute)}
		u, _ := url.Parse(s.config.Callback)
		result = map[string]string{"role": role, "url": u.Scheme + "://" + u.Host + "/atlassian-check/#" + ticket}
	case "status":
		roles := make(map[string]bool)
		for key := range s.config.Accounts {
			_, roles[key] = s.state.Tokens[key]
		}
		result = struct {
			Roles        map[string]bool `json:"roles"`
			LastReport   time.Time       `json:"last_report"`
			ReportStatus int             `json:"report_status"`
		}{roles, s.state.LastReport, s.state.ReportStatus}
	case "check":
		result, err = s.check(ctx, role)
	case "report":
		result, err = s.report(ctx)
	case "erase":
		delete(s.state.Tokens, role)
		err = s.persist()
		result = map[string]bool{"erased": err == nil}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, result)
}

func (s *Server) access(ctx context.Context, role string) (string, error) {
	t, ok := s.state.Tokens[role]
	if !ok {
		return "", errors.New("role needs OAuth authorization")
	}
	if t.Expires.After(time.Now().Add(time.Minute)) {
		return t.Access, nil
	}
	fresh, err := s.provider.exchange(ctx, "refresh_token", t.Refresh)
	if err != nil {
		var pe providerError
		if errors.As(err, &pe) && (pe.Status == 400 || pe.Status == 401 || pe.Status == 403) {
			delete(s.state.Tokens, role)
			if saveErr := s.persist(); saveErr != nil {
				return "", saveErr
			}
		}
		return "", err
	}
	fresh.AccountID, fresh.Retrieved, fresh.Next = t.AccountID, t.Retrieved, t.Next
	s.state.Tokens[role] = fresh
	if err := s.persist(); err != nil {
		return "", err
	}
	return fresh.Access, nil
}

func (s *Server) prune() {
	for key, p := range s.tickets {
		if !p.Expires.After(time.Now()) {
			delete(s.tickets, key)
		}
	}
	for key, p := range s.flows {
		if !p.Expires.After(time.Now()) {
			delete(s.flows, key)
		}
	}
}

func randomSecret() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) persist() error {
	s.storeErr = s.store.save(s.state)
	return s.storeErr
}

func scriptPolicy(w http.ResponseWriter) string {
	nonce := randomSecret()
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+
		"'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	return nonce
}
