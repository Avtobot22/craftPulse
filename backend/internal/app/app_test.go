package app

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_FinishesActiveRequestOnShutdown(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}), ReadHeaderTimeout: time.Second}
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() { served <- run(ctx, srv, ln) }()

	response := make(chan int, 1)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/", http.NoBody)
	require.NoError(t, err)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			response <- 0
			return
		}
		_ = resp.Body.Close()
		response <- resp.StatusCode
	}()
	<-started

	cancel()
	require.Eventually(t, func() bool {
		dialer := net.Dialer{Timeout: 100 * time.Millisecond}
		conn, err := dialer.DialContext(t.Context(), "tcp", addr)
		if err != nil {
			return true
		}
		_ = conn.Close()
		return false
	}, 5*time.Second, 10*time.Millisecond, "listener is closed for new connections")

	close(release)
	assert.Equal(t, http.StatusOK, <-response, "active request is finished")
	require.NoError(t, <-served)
}
