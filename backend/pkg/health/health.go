package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const _checkTimeout = 2 * time.Second

// Set by -ldflags "-X .../pkg/health._commitSHA=<sha>".
var _commitSHA = "unknown"

func CommitSHA() string { return _commitSHA }

type Check func(ctx context.Context) error

type Readiness struct {
	mu       sync.RWMutex
	checks   []Check
	draining atomic.Bool
}

func (r *Readiness) Register(c Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks = append(r.checks, c)
}

func (r *Readiness) Drain() { r.draining.Store(true) }

func (r *Readiness) Ready(ctx context.Context) bool {
	r.mu.RLock()
	checks := r.checks
	r.mu.RUnlock()
	if r.draining.Load() || len(checks) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, _checkTimeout)
	defer cancel()
	for _, c := range checks {
		if c(ctx) != nil {
			return false
		}
	}
	return true
}

func Handler(ready *Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Ready(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, versionBody{CommitSHA: CommitSHA()})
	})
	return mux
}

type statusBody struct {
	Status string `json:"status"`
}

type versionBody struct {
	CommitSHA     string `json:"commit_sha"`
	SchemaVersion *int64 `json:"schema_version"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
