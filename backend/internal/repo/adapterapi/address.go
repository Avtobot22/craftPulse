package adapterapi

import (
	"context"
	"net/netip"
	"net/url"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

const _resolveTimeout = 5 * time.Second

// Builtin reports whether the installation runs a built-in adapter of the provider.
func (c *Caller) Builtin(p entity.Provider) bool {
	_, ok := c.builtin[p]
	return ok
}

// CheckSourceURL refuses a source address in a network the adapters must not reach.
func (c *Caller) CheckSourceURL(ctx context.Context, raw string) error {
	return c.checkURL(ctx, raw, c.sources)
}

// CheckAdapterURL refuses the address of an adapter of a client in a network the core must not reach.
func (c *Caller) CheckAdapterURL(ctx context.Context, raw string) error {
	return c.checkURL(ctx, raw, c.policy)
}

// checkURL refuses the address if any address its name resolves to is refused; a name that does not
// resolve now is left to the check made when the connection is made.
func (c *Caller) checkURL(ctx context.Context, raw string, p netguard.Policy) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return entity.ErrOutsideScope
	}
	if a, err := netip.ParseAddr(u.Hostname()); err == nil {
		if p.Check(a) != nil {
			return entity.ErrOutsideScope
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, _resolveTimeout)
	defer cancel()
	addrs, err := c.resolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil {
		return nil //nolint:nilerr // an unresolved name is checked again on every connection
	}
	for _, a := range addrs {
		if p.Check(a) != nil {
			return entity.ErrOutsideScope
		}
	}
	return nil
}
