// Package netguard refuses requests to destinations a user-entered address must not reach (TZ 13.7).
package netguard

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

type Reason string

const (
	ReasonInsecureScheme   Reason = "insecure_scheme"
	ReasonLoopback         Reason = "loopback"
	ReasonLinkLocal        Reason = "link_local"
	ReasonUnspecified      Reason = "unspecified"
	ReasonMulticast        Reason = "multicast"
	ReasonReserved         Reason = "reserved"
	ReasonInstallation     Reason = "installation_network"
	ReasonPrivate          Reason = "private_network"
	ReasonInvalidAddress   Reason = "invalid_address"
	ReasonTooManyRedirects Reason = "too_many_redirects"
)

var ErrRefused = errors.New("netguard: destination refused")

// RefusedError names the class of a refused destination and never the address, so it can reach a user.
type RefusedError struct {
	Reason Reason
}

func (e *RefusedError) Error() string { return "netguard: destination refused: " + string(e.Reason) }

func (*RefusedError) Is(target error) bool { return target == ErrRefused }

var (
	_nat64     = netip.MustParsePrefix("64:ff9b::/96")
	_sharedNAT = netip.MustParsePrefix("100.64.0.0/10")
	_thisNet   = netip.MustParsePrefix("0.0.0.0/8")
	_reserved  = []netip.Prefix{
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("240.0.0.0/4"),
	}
)

// Policy is the network scope of an installation. Installation networks are always refused; private
// networks are refused unless Private lists them, which only an in-contour installation does.
type Policy struct {
	Installation []netip.Prefix
	Private      []netip.Prefix
}

func (p Policy) Check(addr netip.Addr) error {
	addr = addr.WithZone("").Unmap()
	if _nat64.Contains(addr) {
		b := addr.As16()
		addr = netip.AddrFrom4([4]byte(b[12:]))
	}
	if reason, refused := p.classify(addr); refused {
		return &RefusedError{Reason: reason}
	}
	return nil
}

func (p Policy) classify(addr netip.Addr) (Reason, bool) {
	switch {
	case !addr.IsValid():
		return ReasonInvalidAddress, true
	case addr.IsLoopback():
		return ReasonLoopback, true
	case addr.IsMulticast():
		return ReasonMulticast, true
	case addr.IsLinkLocalUnicast():
		return ReasonLinkLocal, true
	case addr.IsUnspecified(), _thisNet.Contains(addr):
		return ReasonUnspecified, true
	case contains(_reserved, addr):
		return ReasonReserved, true
	case contains(p.Installation, addr):
		return ReasonInstallation, true
	case (addr.IsPrivate() || _sharedNAT.Contains(addr)) && !contains(p.Private, addr):
		return ReasonPrivate, true
	}
	return "", false
}

// Control checks the address a socket is about to connect to; set it on net.Dialer.
func (p Policy) Control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return &RefusedError{Reason: ReasonInvalidAddress}
	}
	return p.Check(ap.Addr())
}

func contains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func ParsePrefixes(cidrs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, s := range cidrs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("netguard: %q is not a CIDR network", s)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Dialer resolves a name, refuses it if any of its addresses is refused, and connects only to the
// addresses it checked. The default connection checks the address once more on the socket.
type Dialer struct {
	Policy   Policy
	Resolver Resolver
	// Connect opens a connection to a checked IP address and port; nil uses a net.Dialer with Policy.Control.
	Connect func(ctx context.Context, network, address string) (net.Conn, error)
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, &RefusedError{Reason: ReasonInvalidAddress}
	}
	addrs, err := d.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if err := d.Policy.Check(a); err != nil {
			return nil, err
		}
	}
	connect := d.Connect
	if connect == nil {
		nd := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: d.Policy.Control}
		connect = nd.DialContext
	}
	var errs []error
	for _, a := range addrs {
		conn, err := connect(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

func (d *Dialer) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a}, nil
	}
	r := d.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("netguard: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("netguard: resolve %s: no addresses", host)
	}
	return addrs, nil
}

type ClientOptions struct {
	Timeout      time.Duration
	MaxRedirects int
	RootCAs      *x509.CertPool
}

// NewClient returns an HTTPS-only client that dials through d, ignores proxy variables and follows at
// most opts.MaxRedirects redirects, each of them checked again.
func NewClient(d *Dialer, opts ClientOptions) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: opts.RootCAs},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: httpsOnly{transport},
		Timeout:   opts.Timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > opts.MaxRedirects {
				return &RefusedError{Reason: ReasonTooManyRedirects}
			}
			return nil
		},
	}
}

// httpsOnly refuses a request that is not HTTPS, the first one and every redirect alike.
type httpsOnly struct {
	next http.RoundTripper
}

func (t httpsOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, &RefusedError{Reason: ReasonInsecureScheme}
	}
	return t.next.RoundTrip(req)
}
