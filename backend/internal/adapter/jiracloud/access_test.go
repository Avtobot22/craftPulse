package jiracloud

import (
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

type accessTestResolver struct{}

func (accessTestResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
}

func TestAccessReadRechecksJira(t *testing.T) {
	t.Parallel()
	var status atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/10006" || r.URL.RawQuery != "fields=id" ||
			r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, `{"id":"10006"}`)
	}))
	defer upstream.Close()
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	client := adaptersdk.NewSourceClient(adaptersdk.ClientConfig{
		RootCAs: roots, Resolver: accessTestResolver{},
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	user := accessUser{
		client: client, gate: &accessGate{slots: make(chan struct{}, 1)}, base: "https://example.com", token: "fixture-token",
	}
	item := adaptersdk.AccessItem{
		ItemKind: adaptersdk.AccessItemItemKindObject, ObjectKind: new(adaptersdk.AccessItemObjectKindTask), NativeId: "10006",
	}
	for _, tc := range []struct {
		status int32
		state  adaptersdk.AccessDecisionDecision
		reason string
	}{
		{http.StatusOK, adaptersdk.AccessDecisionDecisionUnknown, _revocationUnverified},
		{http.StatusNotFound, adaptersdk.AccessDecisionDecisionDenied, "jira_read_denied"},
		{http.StatusOK, adaptersdk.AccessDecisionDecisionUnknown, _revocationUnverified},
		{http.StatusForbidden, adaptersdk.AccessDecisionDecisionDenied, "jira_read_denied"},
		{http.StatusUnauthorized, adaptersdk.AccessDecisionDecisionUnknown, "token_expired"},
		{http.StatusTooManyRequests, adaptersdk.AccessDecisionDecisionUnknown, "jira_read_unverified"},
		{http.StatusServiceUnavailable, adaptersdk.AccessDecisionDecisionUnknown, "jira_read_unverified"},
	} {
		status.Store(tc.status)
		got := user.check(t.Context(), item)
		if got.Decision != tc.state || got.ReasonCode == nil || *got.ReasonCode != tc.reason {
			t.Errorf("status %d: got %s (%v), want %s (%s)", tc.status, got.Decision, got.ReasonCode, tc.state, tc.reason)
		}
	}

	user.gate.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 0)
	defer cancel()
	got := user.check(ctx, item)
	if got.Decision != adaptersdk.AccessDecisionDecisionUnknown || got.ReasonCode == nil || *got.ReasonCode != _accessTimeoutReason {
		t.Errorf("expired check: got %s (%v), want unknown (%s)", got.Decision, got.ReasonCode, _accessTimeoutReason)
	}
}
