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

type Probes struct {
	Ready         *Readiness
	Live          Check
	SchemaVersion func(ctx context.Context) (int64, error)
}

func Handler(p Probes) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		if p.Live != nil && p.Live(r.Context()) != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !p.Ready.Ready(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		body := versionBody{CommitSHA: CommitSHA()}
		if p.SchemaVersion != nil {
			if v, err := p.SchemaVersion(r.Context()); err == nil {
				body.SchemaVersion = &v
			}
		}
		writeJSON(w, http.StatusOK, body)
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
