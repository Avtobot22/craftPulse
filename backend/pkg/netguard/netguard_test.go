package netguard_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	p, err := netguard.ParsePrefixes(cidrs)
	require.NoError(t, err)
	return p
}

func reason(err error) netguard.Reason {
	if r, ok := errors.AsType[*netguard.RefusedError](err); ok {
		return r.Reason
	}
	return ""
}

func TestPolicy_Check(t *testing.T) {
	t.Parallel()
	cloud := netguard.Policy{Installation: prefixes(t, "172.20.0.0/16")}
	contour := netguard.Policy{Installation: prefixes(t, "10.1.0.0/16"), Private: prefixes(t, "10.0.0.0/8")}

	tests := []struct {
		name   string
		policy netguard.Policy
		addr   string
		want   netguard.Reason
	}{
		{"ipv4 loopback", cloud, "127.0.0.1", netguard.ReasonLoopback},
		{"ipv6 loopback", cloud, "::1", netguard.ReasonLoopback},
		{"mapped loopback", cloud, "::ffff:127.0.0.1", netguard.ReasonLoopback},
		{"nat64 loopback", cloud, "64:ff9b::7f00:1", netguard.ReasonLoopback},
		{"cloud metadata", cloud, "169.254.169.254", netguard.ReasonLinkLocal},
		{"ipv6 link-local", cloud, "fe80::1", netguard.ReasonLinkLocal},
		{"unspecified ipv4", cloud, "0.0.0.0", netguard.ReasonUnspecified},
		{"this network", cloud, "0.1.2.3", netguard.ReasonUnspecified},
		{"unspecified ipv6", cloud, "::", netguard.ReasonUnspecified},
		{"multicast", cloud, "224.0.0.251", netguard.ReasonMulticast},
		{"broadcast", cloud, "255.255.255.255", netguard.ReasonReserved},
		{"installation network", cloud, "172.20.0.5", netguard.ReasonInstallation},
		{"private in cloud", cloud, "10.0.0.5", netguard.ReasonPrivate},
		{"shared nat in cloud", cloud, "100.64.0.1", netguard.ReasonPrivate},
		{"ula in cloud", cloud, "fd00::1", netguard.ReasonPrivate},
		{"private in contour scope", contour, "10.2.0.5", ""},
		{"installation inside contour scope", contour, "10.1.0.5", netguard.ReasonInstallation},
		{"private outside contour scope", contour, "192.168.1.5", netguard.ReasonPrivate},
		{"public ipv4", cloud, "203.0.113.10", ""},
		{"public ipv6", cloud, "2001:db8::10", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.policy.Check(netip.MustParseAddr(tt.addr))
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, netguard.ErrRefused)
			assert.Equal(t, tt.want, reason(err))
			assert.NotContains(t, err.Error(), tt.addr, "the message names the class, not the address")
		})
	}
}

func TestPolicy_ControlChecksTheSocketAddress(t *testing.T) {
	t.Parallel()
	var p netguard.Policy
	assert.Equal(t, netguard.ReasonLoopback, reason(p.Control("tcp4", "127.0.0.1:443", nil)))
	assert.Equal(t, netguard.ReasonLinkLocal, reason(p.Control("tcp6", "[fe80::1]:443", nil)))
	assert.NoError(t, p.Control("tcp4", "203.0.113.10:443", nil))
}

func TestParsePrefixes_RejectsNonCIDR(t *testing.T) {
	t.Parallel()
	_, err := netguard.ParsePrefixes([]string{"10.0.0.1"})
	assert.Error(t, err)
}

// rebinding answers a public address first and an internal one afterwards.
type rebinding struct {
	calls atomic.Int32
}

func (r *rebinding) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	if r.calls.Add(1) == 1 {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

type mixed struct{}

func (mixed) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("203.0.113.10"), netip.MustParseAddr("10.0.0.1")}, nil
}

func TestDialer_ChecksEveryResolution(t *testing.T) {
	t.Parallel()
	var connected []string
	connect := func(_ context.Context, _, address string) (net.Conn, error) {
		connected = append(connected, address)
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	d := &netguard.Dialer{Resolver: &rebinding{}, Connect: connect}

	conn, err := d.DialContext(t.Context(), "tcp", "source.example.test:443")
	require.NoError(t, err)
	_ = conn.Close()
	_, err = d.DialContext(t.Context(), "tcp", "source.example.test:443")
	assert.Equal(t, netguard.ReasonLoopback, reason(err), "a changed DNS answer is checked again")
	assert.Equal(t, []string{"203.0.113.10:443"}, connected, "only the checked address is connected")

	d = &netguard.Dialer{Resolver: mixed{}, Connect: connect}
	_, err = d.DialContext(t.Context(), "tcp", "source.example.test:443")
	assert.Equal(t, netguard.ReasonPrivate, reason(err), "one internal address refuses the name")
	assert.Len(t, connected, 1)
}

func TestNewClient_RefusesPlainHTTP(t *testing.T) {
	t.Parallel()
	var dials atomic.Int32
	d := &netguard.Dialer{Connect: func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("must not dial")
	}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://203.0.113.10/", http.NoBody)
	require.NoError(t, err)
	resp, err := netguard.NewClient(d, netguard.ClientOptions{}).Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	assert.Equal(t, netguard.ReasonInsecureScheme, reason(err))
	assert.Zero(t, dials.Load())
}
